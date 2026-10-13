package outbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace"

	"github.com/eventflow/eventflow/libs/go/otelx"
)

// SchemaVersion la phien ban phong bi Envelope.
const SchemaVersion = 1

// maxDataBytes la gioi han kich thuoc JSON cua Event.Data.
const maxDataBytes = 64 * 1024

var (
	routingKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){2,}$`)
	uuidRe       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// Execer la phan cua pgx.Tx ma Write can (de test bang fake).
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Event la mot su kien can ghi vao outbox.
type Event struct {
	AggregateType string
	AggregateID   string // UUID dang chuoi
	RoutingKey    string // <domain>.<entity>.<action>
	Data          any    // JSON <= 64 KiB, luu nguyen vao cot payload; khong chua PII
}

// Envelope la body JSON duoc publish; dung tu cac cot cua row luc publish.
type Envelope struct {
	EventID       string          `json:"event_id"`
	OccurredAt    time.Time       `json:"occurred_at"`
	TraceID       string          `json:"trace_id,omitempty"`
	SchemaVersion int             `json:"schema_version"`
	Type          string          `json:"type"`
	Data          json.RawMessage `json:"data"`
}

const insertSQL = `INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, routing_key, payload, headers, trace_id)
VALUES ($1::uuid, $2, $3::uuid, $4, $5::jsonb, $6::jsonb, $7)`

// Write them mot row outbox trong transaction tx va tra ve event_id (UUID v4).
// Goi trong transaction nghiep vu; trace context cua ctx duoc luu vao headers + trace_id.
func Write(ctx context.Context, tx Execer, e Event) (string, error) {
	if e.AggregateType == "" {
		return "", errors.New("outbox: AggregateType rong")
	}
	if !uuidRe.MatchString(e.AggregateID) {
		return "", errors.New("outbox: AggregateID khong phai UUID")
	}
	if !routingKeyRe.MatchString(e.RoutingKey) {
		return "", fmt.Errorf("outbox: routing key %q khong hop le", e.RoutingKey)
	}
	payload, err := json.Marshal(e.Data)
	if err != nil {
		return "", fmt.Errorf("outbox: ma hoa data: %w", err)
	}
	if len(payload) > maxDataBytes {
		return "", fmt.Errorf("outbox: data %d byte vuot gioi han %d", len(payload), maxDataBytes)
	}

	h := otelx.AMQPHeaders{}
	otelx.InjectAMQP(ctx, h)
	headers := map[string]string{}
	for _, k := range []string{"traceparent", "tracestate", "baggage"} {
		if v := h.Get(k); v != "" {
			headers[k] = v
		}
	}
	hb, err := json.Marshal(headers)
	if err != nil {
		return "", fmt.Errorf("outbox: ma hoa headers: %w", err)
	}
	var traceID *string
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		s := sc.TraceID().String()
		traceID = &s
	}

	id, err := newUUIDv4()
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, insertSQL, id, e.AggregateType, e.AggregateID, e.RoutingKey, payload, hb, traceID); err != nil {
		return "", fmt.Errorf("outbox: ghi %s: %w", e.RoutingKey, err)
	}
	return id, nil
}

func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("outbox: sinh event_id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	s := hex.EncodeToString(b[:])
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}
