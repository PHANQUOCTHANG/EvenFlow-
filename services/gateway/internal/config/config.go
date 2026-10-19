// Package config doc cau hinh gateway tu bien moi truong.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// DevJWTSecret trung gia tri mac dinh cua identity de local chay duoc khong can .env.
const DevJWTSecret = "development-secret"

// Rate la toc do (request/giay) va so request dot bien cho phep cua mot lop route.
type Rate struct {
	PerSecond float64
	Burst     int
}

// Config cua gateway.
type Config struct {
	HTTPAddr       string
	JWTSecret      string
	DevSecret      bool // dang dung DevJWTSecret: chi chap nhan khi APP_ENV khong phai production
	Upstreams      map[string]*url.URL
	AllowedOrigins []string
	Rates          map[string]Rate
	MaxBodyBytes   int64
}

// Ten upstream dung trong bang route.
const (
	Identity    = "identity"
	Event       = "event"
	Waitingroom = "waitingroom"
	Ticketing   = "ticketing"
	Payment     = "payment"
	Assistant   = "assistant"
)

var upstreamDefaults = map[string][2]string{
	Identity:    {"IDENTITY_URL", "http://localhost:8083"},
	Event:       {"EVENT_URL", "http://localhost:8084"},
	Waitingroom: {"WAITINGROOM_URL", "http://localhost:8081"},
	Ticketing:   {"TICKETING_URL", "http://localhost:8082"},
	Payment:     {"PAYMENT_URL", "http://localhost:8085"},
	Assistant:   {"ASSISTANT_URL", "http://localhost:8090"},
}

// Lop gioi han toc do (xem proxy.Routes).
const (
	RateAuth      = "auth"       // dang nhap, OTP, refresh: theo IP, chong do mat khau
	RateQueuePoll = "queue_poll" // GET queue/status: theo nguoi, chan poll day hon poll_after_ms (BR-Q4)
	RateDefault   = "default"    // con lai: theo nguoi neu da dang nhap, nguoc lai theo IP
)

var rateDefaults = map[string]Rate{
	RateAuth:      {PerSecond: 1, Burst: 10},
	RateQueuePoll: {PerSecond: 0.5, Burst: 3},
	RateDefault:   {PerSecond: 10, Burst: 30},
}

// Load doc cau hinh. Loi khi URL upstream hong, rate sai cu phap, hoac production ma thieu
// JWT_SECRET.
func Load() (*Config, error) {
	c := &Config{
		HTTPAddr:     getenv("HTTP_ADDR", ":8080"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
		Upstreams:    map[string]*url.URL{},
		Rates:        map[string]Rate{},
		MaxBodyBytes: 1 << 20,
	}
	if c.JWTSecret == "" {
		if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
			return nil, fmt.Errorf("JWT_SECRET bat buoc khi APP_ENV=production")
		}
		c.JWTSecret, c.DevSecret = DevJWTSecret, true
	}

	for name, d := range upstreamDefaults {
		raw := getenv(d[0], d[1])
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("%s=%q khong phai URL hop le", d[0], raw)
		}
		c.Upstreams[name] = u
	}

	for _, o := range strings.Split(getenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}

	for class, d := range rateDefaults {
		key := "RATE_" + strings.ToUpper(class)
		r, err := parseRate(os.Getenv(key), d)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		c.Rates[class] = r
	}
	return c, nil
}

// parseRate doc "rps:burst", vd "0.5:3". Rong thi lay mac dinh.
func parseRate(s string, def Rate) (Rate, error) {
	if s == "" {
		return def, nil
	}
	rps, burst, ok := strings.Cut(s, ":")
	if !ok {
		return Rate{}, fmt.Errorf("can dang rps:burst, nhan %q", s)
	}
	r, err := strconv.ParseFloat(rps, 64)
	if err != nil || r <= 0 {
		return Rate{}, fmt.Errorf("rps phai > 0, nhan %q", rps)
	}
	b, err := strconv.Atoi(burst)
	if err != nil || b < 1 {
		return Rate{}, fmt.Errorf("burst phai >= 1, nhan %q", burst)
	}
	return Rate{PerSecond: r, Burst: b}, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
