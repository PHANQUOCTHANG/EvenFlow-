package main

import (
	"database/sql"
	"log/slog"
	"net/http"
	"os"

	"github.com/eventflow/eventflow/services/identity/internal/adapter/postgres"
	identityHttp "github.com/eventflow/eventflow/services/identity/internal/adapter/http"
	"github.com/eventflow/eventflow/services/identity/internal/app"
	"github.com/eventflow/eventflow/services/identity/internal/config"
	"github.com/eventflow/eventflow/services/identity/internal/domain"

	_ "github.com/lib/pq"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

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
	// - Domain / Port
	identityRepo := postgres.NewIdentityRepo(db)
	hasher := domain.NewArgon2idHasher()

	// - App (Use cases)
	registerUC := app.NewRegisterUseCase(identityRepo, hasher)
	loginUC := app.NewLoginUseCase(identityRepo, hasher)

	// - Adapter (HTTP)
	authHandler := identityHttp.NewAuthHandler(registerUC, loginUC)

	// 4. Khởi tạo Router và đăng ký API
	mux := http.NewServeMux()
	
	// API Health check (cho K8s/Docker)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Đăng ký API Đăng nhập / Đăng ký
	identityHttp.RegisterRoutes(mux, authHandler)

	// 5. Khởi chạy Server
	log.Info("identity dang lang nghe", "addr", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, mux); err != nil {
		log.Error("server dung", "err", err)
		os.Exit(1)
	}
}
