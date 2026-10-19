package pii

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

// Redacted thay the gia tri bi mat (mat khau, token, OTP...).
const Redacted = "[REDACTED]"

// secretKeys: attribute co ten CHUA mot trong cac chuoi nay bi thay toan bo bang Redacted.
var secretKeys = []string{
	"password", "passwd", "secret", "token", "otp", "authorization", "cookie",
	"api_key", "apikey", "private_key", "dsn", "database_url", "credential",
}

// piiKeys: attribute co ten nay bi che mot phan (giu du de debug, khong du de dinh danh).
var piiKeys = []string{"email", "phone", "identifier", "cccd", "full_name", "dob"}

var (
	emailRe    = regexp.MustCompile(`([A-Za-z0-9._%+-])[A-Za-z0-9._%+-]*@([A-Za-z0-9.-]+\.[A-Za-z]{2,})`)
	phoneRe    = regexp.MustCompile(`(?:\+?84|\b0)[35789]\d{6}(\d{2})\b`)
	jwtRe      = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}`)
	userinfoRe = regexp.MustCompile(`(://[^:/@\s]+):[^@/\s]+@`)
	cccdRe     = regexp.MustCompile(`\b\d{12}\b`)
)

// Scrub che PII va bi mat lot vao mot chuoi tu do (thong diep loi, URL...).
func Scrub(s string) string {
	s = userinfoRe.ReplaceAllString(s, "$1:***@")
	s = jwtRe.ReplaceAllString(s, "[REDACTED_JWT]")
	s = emailRe.ReplaceAllString(s, "$1***@$2")
	s = phoneRe.ReplaceAllString(s, "*******$1")
	s = cccdRe.ReplaceAllString(s, "[REDACTED_ID]")
	return s
}

// NewRedactingHandler boc mot slog.Handler: moi attribute (ke ca trong group, ke ca loi) di qua
// redactAttr truoc khi ghi. Thong diep log cung duoc Scrub.
//
// Day la luoi an toan, khong thay cho viec khong log PII ngay tu dau.
func NewRedactingHandler(inner slog.Handler) slog.Handler {
	return redactingHandler{inner: inner}
}

type redactingHandler struct{ inner slog.Handler }

func (h redactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, Scrub(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	red := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		red[i] = redactAttr(a)
	}
	return redactingHandler{inner: h.inner.WithAttrs(red)}
}

func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		attrs := v.Group()
		red := make([]any, len(attrs))
		for i, g := range attrs {
			red[i] = redactAttr(g)
		}
		return slog.Group(a.Key, red...)
	}

	key := strings.ToLower(a.Key)
	for _, k := range secretKeys {
		if strings.Contains(key, k) {
			return slog.String(a.Key, Redacted)
		}
	}
	for _, k := range piiKeys {
		if strings.Contains(key, k) {
			return slog.String(a.Key, mask(v.String()))
		}
	}

	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, Scrub(v.String()))
	case slog.KindAny:
		// Loi va struct: dua ve chuoi roi scrub (loi pq hay chua DSN, loi SMTP chua email).
		if err, ok := v.Any().(error); ok {
			return slog.String(a.Key, Scrub(err.Error()))
		}
		if s, ok := v.Any().(interface{ String() string }); ok {
			return slog.String(a.Key, Scrub(s.String()))
		}
		return a
	default:
		return slog.Attr{Key: a.Key, Value: v}
	}
}

// mask giu ky tu dau va 2 ky tu cuoi (vd so dien thoai) hoac ten mien (email).
func mask(s string) string {
	if s == "" {
		return s
	}
	if at := strings.LastIndex(s, "@"); at > 0 {
		return s[:1] + "***" + s[at:]
	}
	r := []rune(s)
	if len(r) <= 4 {
		return "***"
	}
	return string(r[0]) + strings.Repeat("*", len(r)-3) + string(r[len(r)-2:])
}

// Mask la ban xuat khau cua mask: che mot dinh danh (email / so dien thoai) truoc khi in.
func Mask(s string) string { return mask(s) }
