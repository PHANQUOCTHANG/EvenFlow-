package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string
	JWTSecret     string
	SMTPHost      string
	SMTPPort      int
	SMTPUser      string
	SMTPPass      string
}

func Load() *Config {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8083" // Default for identity service
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Fail fast if database URL is missing
		panic("DATABASE_URL environment variable is strictly required")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379" // Default for local dev
	}
	redisPass := os.Getenv("REDIS_PASSWORD")

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "development-secret"
	}

	smtpHost := os.Getenv("SMTP_HOST")
	smtpPortStr := os.Getenv("SMTP_PORT")
	smtpPort, _ := strconv.Atoi(smtpPortStr)
	if smtpPort == 0 {
		smtpPort = 587
	}
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")

	return &Config{
		HTTPAddr:      addr,
		DatabaseURL:   dbURL,
		RedisAddr:     redisAddr,
		RedisPassword: redisPass,
		JWTSecret:     jwtSecret,
		SMTPHost:      smtpHost,
		SMTPPort:      smtpPort,
		SMTPUser:      smtpUser,
		SMTPPass:      smtpPass,
	}
}
