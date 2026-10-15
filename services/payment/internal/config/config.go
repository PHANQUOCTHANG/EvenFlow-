package config

import (
	"os"
	"strings"
)

// Config chua toan bo cau hinh van hanh cua payment-svc.
type Config struct {
	HTTPAddr         string
	DatabaseURL      string
	DefaultProvider  string
	EnabledProviders []string

	// VNPay configuration
	VNPayTmnCode    string
	VNPayHashSecret string
	VNPayBaseURL    string
	VNPayReturnURL  string

	// MoMo configuration
	MoMoPartnerCode string
	MoMoAccessKey   string
	MoMoSecretKey   string
	MoMoEndpoint    string
	MoMoReturnURL   string
	MoMoNotifyURL   string

	// Stripe configuration
	StripePublishableKey string
	StripeSecretKey      string
	StripeWebhookSecret  string
	StripeSuccessURL     string
	StripeCancelURL      string

	// Sandbox configuration
	SandboxMode string // success, fail, delay
}

// LoadFromEnv doc cau hinh tu bien moi truong va ap dung gia tri mac dinh an toan.
func LoadFromEnv() *Config {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8085"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://eventflow:eventflow@localhost:5432/eventflow?sslmode=disable"
	}

	defaultProvider := os.Getenv("PAYMENT_DEFAULT_PROVIDER")
	if defaultProvider == "" {
		defaultProvider = "sandbox"
	}

	enabledStr := os.Getenv("PAYMENT_ENABLED_PROVIDERS")
	var enabled []string
	if enabledStr != "" {
		parts := strings.Split(enabledStr, ",")
		for _, p := range parts {
			trimmed := strings.ToLower(strings.TrimSpace(p))
			if trimmed != "" {
				enabled = append(enabled, trimmed)
			}
		}
	}
	if len(enabled) == 0 {
		enabled = []string{"sandbox", "vnpay", "momo", "stripe"}
	}

	sandboxMode := os.Getenv("PAYMENT_SANDBOX_MODE")
	if sandboxMode == "" {
		sandboxMode = "success"
	}

	return &Config{
		HTTPAddr:             addr,
		DatabaseURL:          dbURL,
		DefaultProvider:      strings.ToLower(strings.TrimSpace(defaultProvider)),
		EnabledProviders:     enabled,
		VNPayTmnCode:         getEnvOrDefault("VNPAY_TMN_CODE", "EVENTFLOW_SANDBOX"),
		VNPayHashSecret:      getEnvOrDefault("VNPAY_HASH_SECRET", "EVENTFLOW_VNPAY_SECRET_KEY_123456"),
		VNPayBaseURL:         getEnvOrDefault("VNPAY_BASE_URL", "https://sandbox.vnpayment.vn/paymentv2/vpcpay.html"),
		VNPayReturnURL:       os.Getenv("VNPAY_RETURN_URL"),
		MoMoPartnerCode:      getEnvOrDefault("MOMO_PARTNER_CODE", "MOMO_EVENTFLOW"),
		MoMoAccessKey:        getEnvOrDefault("MOMO_ACCESS_KEY", "MOMO_ACCESS_KEY_123"),
		MoMoSecretKey:        getEnvOrDefault("MOMO_SECRET_KEY", "MOMO_SECRET_KEY_123"),
		MoMoEndpoint:         getEnvOrDefault("MOMO_ENDPOINT", "https://test-payment.momo.vn/v2/gateway/api/create"),
		MoMoReturnURL:        os.Getenv("MOMO_RETURN_URL"),
		MoMoNotifyURL:        os.Getenv("MOMO_NOTIFY_URL"),
		StripePublishableKey: getEnvOrDefault("STRIPE_PUBLISHABLE_KEY", "pk_test_eventflow"),
		StripeSecretKey:      getEnvOrDefault("STRIPE_SECRET_KEY", "sk_test_eventflow"),
		StripeWebhookSecret:  os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripeSuccessURL:     os.Getenv("STRIPE_SUCCESS_URL"),
		StripeCancelURL:      os.Getenv("STRIPE_CANCEL_URL"),
		SandboxMode:          strings.ToLower(strings.TrimSpace(sandboxMode)),
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
