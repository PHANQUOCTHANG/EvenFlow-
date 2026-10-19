package config

import (
	"os"
	"strconv"
	"strings"
)

// Khoa PII chi dung cho dev/test (base64 cua 32 byte). KHONG BAO GIO dung o production:
// Load tu choi khoi dong khi APP_ENV=production ma thieu PII_ENC_KEY / PII_HASH_KEY.
const (
	DevPIIEncKey  = "ZXZlbnRmbG93LWRldi1waWktZW5jcnlwdGlvbi1rZXk=" // "eventflow-dev-pii-encryption-key"
	DevPIIHashKey = "ZXZlbnRmbG93LWRldi1waWktaGFzaGluZy1rZXktMzI=" // "eventflow-dev-pii-hashing-key-32"
)

type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string
	JWTSecret     string
	AppEnv        string
	PIIEncKey     string // base64 32 byte, AES-256-GCM cho phone_enc (EV-115)
	PIIHashKey    string // base64 32 byte, HMAC cho phone_hash (EV-115)
	DevPIIKeys    bool   // dang dung khoa PII mac dinh cua dev
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

	appEnv := os.Getenv("APP_ENV")
	isProd := strings.EqualFold(appEnv, "production")

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		if isProd {
			panic("JWT_SECRET is required when APP_ENV=production")
		}
		jwtSecret = "development-secret"
	}

	// EV-115: khoa PII bat buoc o production; dev dung khoa co dinh (KHONG bao mat) de chay ngay.
	piiEncKey, piiHashKey := os.Getenv("PII_ENC_KEY"), os.Getenv("PII_HASH_KEY")
	devPIIKeys := false
	if piiEncKey == "" || piiHashKey == "" {
		if isProd {
			panic("PII_ENC_KEY and PII_HASH_KEY are required when APP_ENV=production")
		}
		piiEncKey, piiHashKey, devPIIKeys = DevPIIEncKey, DevPIIHashKey, true
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
		AppEnv:        appEnv,
		PIIEncKey:     piiEncKey,
		PIIHashKey:    piiHashKey,
		DevPIIKeys:    devPIIKeys,
		SMTPHost:      smtpHost,
		SMTPPort:      smtpPort,
		SMTPUser:      smtpUser,
		SMTPPass:      smtpPass,
	}
}
