package config

import (
	"os"
	"strconv"
)

// Config chua cau hinh khoi dong cua notification service.
type Config struct {
	HTTPAddr      string
	EmailProvider string
	SMTPHost      string
	SMTPPort      int
	SMTPUser      string
	SMTPPassword  string
	SMTPFrom      string
	SMSProvider   string
	SMSWebhookURL string
	SMSAPIKey     string
	AMQPURL       string
	DatabaseURL   string
}

// LoadFromEnv nap cau hinh tu cac bien moi truong he thong.
func LoadFromEnv() Config {
	cfg := Config{
		HTTPAddr:      getEnv("HTTP_ADDR", ":8087"),
		EmailProvider: getEnv("EMAIL_PROVIDER", "mock"),
		SMTPHost:      getEnv("SMTP_HOST", "localhost"),
		SMTPPort:      getEnvInt("SMTP_PORT", 587),
		SMTPUser:      getEnv("SMTP_USER", ""),
		SMTPPassword:  getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:      getEnv("SMTP_FROM", "no-reply@evenflow.vn"),
		SMSProvider:   getEnv("SMS_PROVIDER", "mock"),
		SMSWebhookURL: getEnv("SMS_WEBHOOK_URL", ""),
		SMSAPIKey:     getEnv("SMS_API_KEY", ""),
		AMQPURL:       getEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		DatabaseURL:   getEnv("DATABASE_URL", ""),
	}
	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
