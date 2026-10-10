package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"

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
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Tự động nạp file .env ở thư mục root để lấy SMTP_USER/PASS
	_ = godotenv.Load("../../.env")

	// 1. Nạp cấu hình
	cfg := config.Load()
	log.Info("ket noi database", "url", cfg.DatabaseURL)

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
	identityRepo := postgres.NewIdentityRepo(db)
	hasher := domain.NewArgon2idHasher()
	tokenGen := domain.NewJWTTokenGenerator(cfg.JWTSecret)

	// - Notifier
	var notifier domain.OTPNotifier
	if cfg.SMTPUser != "" && cfg.SMTPPass != "" {
		log.Info("SMTP duoc cau hinh, se gui email that.")
		notifier = smtp.NewSMTPNotifier(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, "EventFlow Secure")
	} else {
		log.Warn("SMTP chua duoc cau hinh, chay che do MOCK cho email.")
	}

	// - App (Use cases)
	registerUC := app.NewRegisterUseCase(identityRepo, hasher, redisAdapterClient, notifier)
	loginUC := app.NewLoginUseCase(identityRepo, hasher, tokenGen)
	vneidUC := app.NewVNeIDUseCase(identityRepo, rawRedis, tokenGen)
	forgotPwdUC := app.NewForgotPasswordUseCase(identityRepo, hasher, redisAdapterClient, notifier)

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
	authHandler := identityHttp.NewAuthHandler(registerUC, loginUC, vneidUC, forgotPwdUC, rawRedis)

	// 4. Khởi tạo Router và đăng ký API
	mux := http.NewServeMux()
	
	// API Health check (cho K8s/Docker)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Đăng ký API Đăng nhập / Đăng ký
	identityHttp.RegisterRoutes(mux, authHandler)

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

	handler := corsMiddleware(mux)

	// 5. Khởi chạy Server
	log.Info("identity dang lang nghe", "addr", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, handler); err != nil {
		log.Error("server dung", "err", err)
		os.Exit(1)
	}
}
