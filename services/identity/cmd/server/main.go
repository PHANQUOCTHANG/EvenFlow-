package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/libs/go/otelx"
	"github.com/eventflow/eventflow/libs/go/pii"

	identityHttp "github.com/eventflow/eventflow/services/identity/internal/adapter/http"
	"github.com/eventflow/eventflow/services/identity/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/identity/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/identity/internal/adapter/smtp"
	"github.com/eventflow/eventflow/services/identity/internal/adapter/telegram"
	"github.com/eventflow/eventflow/services/identity/internal/app"
	"github.com/eventflow/eventflow/services/identity/internal/config"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/joho/godotenv"
	goRedis "github.com/redis/go-redis/v9"

	_ "github.com/lib/pq"
)

func main() {
	// EV-115: mọi dòng log đi qua bộ lọc PII (mật khẩu, token, OTP, DSN, email, SĐT...).
	log := slog.New(pii.NewRedactingHandler(slog.NewJSONHandler(os.Stdout, nil)))
	slog.SetDefault(log)

	// Tự động nạp file .env ở thư mục root để lấy SMTP_USER/PASS
	_ = godotenv.Load("../../.env")

	// 1. Nạp cấu hình
	cfg := config.Load()
	if cfg.DevPIIKeys {
		log.Warn("PII_ENC_KEY/PII_HASH_KEY chua dat, dang dung khoa PII mac dinh cho dev")
	}
	protector, err := pii.NewProtector(cfg.PIIEncKey, cfg.PIIHashKey)
	if err != nil {
		log.Error("khoa PII khong hop le", "err", err)
		os.Exit(1)
	}

	// 2. Kết nối Database
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Error("khong the mo ket noi db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Error("khong the ping db", "err", err)
		os.Exit(1)
	}

	// 3. Khởi tạo các lớp (Dependency Injection)
	// - Redis
	rawRedis := goRedis.NewClient(&goRedis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0,
	})

	if err := rawRedis.Ping(context.Background()).Err(); err != nil {
		log.Error("khong the ping redis", "err", err)
		os.Exit(1)
	}

	redisAdapterClient := redis.NewRedisClient(rawRedis)

	// - Domain / Port
	identityRepo := postgres.NewIdentityRepo(db, protector)
	// EV-115: dòng cũ lưu SĐT thô trong phone_hash -> chuyển sang HMAC + AES-GCM (idempotent).
	if done, skipped, err := identityRepo.ProtectLegacyPhones(context.Background()); err != nil {
		log.Error("khong bao ve duoc so dien thoai cu", "err", err)
		os.Exit(1)
	} else if done+skipped > 0 {
		log.Warn("da ma hoa so dien thoai cu", "converted", done, "skipped_duplicates", skipped)
	}
	auditRepo := postgres.NewAuditRepo(db)
	hasher := domain.NewArgon2idHasher()
	// EV-112: access token 15 phút + refresh token xoay vòng lưu ở bảng refresh_tokens
	refreshRepo := postgres.NewRefreshTokenRepo(db)
	deviceSvc := app.NewDeviceService(postgres.NewDeviceRepo(db, protector))
	sessions := app.NewSessionService([]byte(cfg.JWTSecret), identityRepo, refreshRepo, auditRepo).WithDevices(deviceSvc)
	tokenGen := sessions

	// - Notifier
	var notifier domain.OTPNotifier
	// Chi can SMTP_HOST: relay noi bo (Mailpit) khong dung dang nhap; co user/pass thi gomail tu xac thuc.
	if cfg.SMTPHost != "" {
		log.Info("SMTP duoc cau hinh, se gui email that.")
		notifier = smtp.NewSMTPNotifier(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, "EventFlow Secure")
	} else {
		log.Warn("SMTP chua duoc cau hinh, chay che do MOCK cho email.")
	}

	// - App (Use cases)
	// OTP không có kênh gửi thật (SĐT, email chưa cấu hình SMTP): chỉ dev mới in ra stderr,
	// che định danh, và không đi qua log (EV-115). Production: SĐT nhận OTP qua Telegram bot.
	var registerOpts []app.RegisterOption
	var resetOpts []app.ForgotPasswordOption
	if !strings.EqualFold(cfg.AppEnv, "production") {
		sink := func(identifier, otp string) {
			fmt.Fprintf(os.Stderr, "[DEV ONLY] OTP %s cho %s\n", otp, pii.Mask(identifier))
		}
		registerOpts = append(registerOpts, app.WithDevOTPSink(sink))
		resetOpts = append(resetOpts, app.WithResetDevOTPSink(sink))
	}
	registerUC := app.NewRegisterUseCase(identityRepo, hasher, redisAdapterClient, notifier, registerOpts...)
	loginUC := app.NewLoginUseCase(identityRepo, hasher, tokenGen, app.WithDeviceRegistrar(deviceSvc))
	vneidUC := app.NewVNeIDUseCase(identityRepo, rawRedis, tokenGen)
	forgotPwdUC := app.NewForgotPasswordUseCase(identityRepo, hasher, redisAdapterClient, notifier,
		append(resetOpts, app.WithSessionRevoker(refreshRepo))...)

	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	googleLoginUC := app.NewGoogleLoginUseCase(identityRepo, hasher, tokenGen, googleClientID)

	// - Telegram Bot
	telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if telegramToken != "" {
		bot, err := telegram.NewBot(telegramToken, redisAdapterClient)
		if err != nil {
			log.Error("khong the khoi tao telegram bot", "err", err)
		} else {
			go bot.Start()
			defer bot.Stop()
		}
	} else {
		log.Warn("TELEGRAM_BOT_TOKEN chua duoc cau hinh, bot Telegram se khong chay.")
	}

	// - Adapter (HTTP)
	authHandler := identityHttp.NewAuthHandler(registerUC, loginUC, vneidUC, forgotPwdUC, googleLoginUC, sessions, rawRedis).WithDevices(deviceSvc)

	// 4. Khởi tạo Router và đăng ký API
	mux := http.NewServeMux()

	// API Health check (cho K8s/Docker)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Đăng ký API Đăng nhập / Đăng ký & RBAC
	identityHttp.RegisterRoutes(mux, authHandler, cfg.JWTSecret, auditRepo)
	// Prometheus scrape trong mang noi bo. Gateway chi dinh tuyen /api/v1/auth/ toi identity nen /metrics khong ra ngoai duoc.
	mux.Handle("GET /metrics", otelx.MetricsHandler())

	// Middleware CORS
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	// EV-103: trace noi tiep traceparent do gateway chuyen toi. otelx.HTTP phai nam sau CORS (ban sao request).
	shutdownTrace, err := otelx.Init(context.Background(), "identity")
	if err != nil {
		log.Error("khoi tao trace that bai", "err", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTrace(context.Background()) }()
	handler := httpx.Chain(corsMiddleware(mux), otelx.HTTP("identity"))

	// 5. Khởi chạy Server
	log.Info("identity dang lang nghe", "addr", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, handler); err != nil {
		log.Error("server dung", "err", err)
		os.Exit(1)
	}
}
