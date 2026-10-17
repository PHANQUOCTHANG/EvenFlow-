package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetOrder lay thong tin don hang tu bang orders.
func (r *Repository) GetOrder(ctx context.Context, orderID string) (domain.OrderView, error) {
	query := `
		SELECT id, event_id, identity_id, status, total_cents, currency, expires_at, created_at
		FROM orders
		WHERE id = $1;
	`
	var (
		id         string
		eventID    string
		identityID string
		status     string
		totalCents int64
		currency   string
		expiresAt  time.Time
		createdAt  time.Time
	)

	err := r.pool.QueryRow(ctx, query, orderID).Scan(
		&id, &eventID, &identityID, &status, &totalCents, &currency, &expiresAt, &createdAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.OrderView{}, domain.ErrOrderNotFound
		}
		return domain.OrderView{}, fmt.Errorf("truy van don hang that bai: %w", err)
	}

	return domain.OrderView{
		ID:         id,
		EventID:    eventID,
		IdentityID: identityID,
		Status:     status,
		TotalCents: totalCents,
		Currency:   currency,
		ExpiresAt:  expiresAt,
		CreatedAt:  createdAt,
	}, nil
}

// CreatePaymentAndMarkPending cap nhat trang thai don hang HELD -> PAYMENT_PENDING
// va chen ban ghi payments trong CUNG MOT TRANSACTION (Quy tac bat bien 1).
func (r *Repository) CreatePaymentAndMarkPending(ctx context.Context, p domain.Payment) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("khoi tao transaction that bai: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// 1. Cap nhat trang thai don hang chi khi con o trang thai HELD
	updateQuery := `
		UPDATE orders
		SET status = 'PAYMENT_PENDING', updated_at = now()
		WHERE id = $1 AND status = 'HELD';
	`
	cmdTag, err := tx.Exec(ctx, updateQuery, p.OrderID)
	if err != nil {
		return fmt.Errorf("cap nhat don hang sang PAYMENT_PENDING that bai: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("%w: don khong con o trang thai HELD", domain.ErrOrderNotHeld)
	}

	// 2. Chen ban ghi vao bang payments
	insertQuery := `
		INSERT INTO payments (id, order_id, provider, provider_txn_id, amount_cents, status, raw_payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
	`
	rawBytes, err := json.Marshal(p.RawPayload)
	if err != nil {
		rawBytes = []byte("{}")
	}

	_, err = tx.Exec(ctx, insertQuery,
		p.ID,
		p.OrderID,
		string(p.Provider),
		p.ProviderTxnID,
		p.AmountCents,
		string(p.Status),
		rawBytes,
		p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("chen ban ghi payments that bai: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction that bai: %w", err)
	}

	return nil
}

// GetPayment lay thong tin thanh toan theo payment_id.
func (r *Repository) GetPayment(ctx context.Context, paymentID string) (domain.Payment, error) {
	query := `
		SELECT id, order_id, provider, provider_txn_id, amount_cents, status, raw_payload, created_at
		FROM payments
		WHERE id = $1;
	`
	var (
		id            string
		orderID       string
		providerStr   string
		providerTxnID string
		amountCents   int64
		statusStr     string
		rawBytes      []byte
		createdAt     time.Time
	)

	err := r.pool.QueryRow(ctx, query, paymentID).Scan(
		&id, &orderID, &providerStr, &providerTxnID, &amountCents, &statusStr, &rawBytes, &createdAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Payment{}, errors.New("khong tim thay ban ghi payment")
		}
		return domain.Payment{}, fmt.Errorf("truy van payment that bai: %w", err)
	}

	var raw map[string]any
	if len(rawBytes) > 0 {
		_ = json.Unmarshal(rawBytes, &raw)
	}

	return domain.Payment{
		ID:            id,
		OrderID:       orderID,
		Provider:      domain.NormalizeProvider(providerStr),
		ProviderTxnID: providerTxnID,
		AmountCents:   amountCents,
		Status:        domain.PaymentStatus(statusStr),
		RawPayload:    raw,
		CreatedAt:     createdAt,
	}, nil
}

// GetPaymentByOrder lay thong tin thanh toan gan nhat cua order.
func (r *Repository) GetPaymentByOrder(ctx context.Context, orderID string) (domain.Payment, error) {
	query := `
		SELECT id, order_id, provider, provider_txn_id, amount_cents, status, raw_payload, created_at
		FROM payments
		WHERE order_id = $1
		ORDER BY created_at DESC
		LIMIT 1;
	`
	var (
		id            string
		ordID         string
		providerStr   string
		providerTxnID string
		amountCents   int64
		statusStr     string
		rawBytes      []byte
		createdAt     time.Time
	)

	err := r.pool.QueryRow(ctx, query, orderID).Scan(
		&id, &ordID, &providerStr, &providerTxnID, &amountCents, &statusStr, &rawBytes, &createdAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Payment{}, errors.New("khong tim thay payment cho order")
		}
		return domain.Payment{}, fmt.Errorf("truy van payment theo order that bai: %w", err)
	}

	var raw map[string]any
	if len(rawBytes) > 0 {
		_ = json.Unmarshal(rawBytes, &raw)
	}

	return domain.Payment{
		ID:            id,
		OrderID:       ordID,
		Provider:      domain.NormalizeProvider(providerStr),
		ProviderTxnID: providerTxnID,
		AmountCents:   amountCents,
		Status:        domain.PaymentStatus(statusStr),
		RawPayload:    raw,
		CreatedAt:     createdAt,
	}, nil
}

// ProcessPaymentSuccessTx xu ly giao dich thanh toan thanh cong mot cach atomic (BR-O6 exactly-once).
// Bao gom: kiem tra trung lap -> khoa row order -> cap nhat orders PAID -> upsert payments SUCCEEDED
// -> ghi outbox order.paid trong CUNG MOT TRANSACTION (Quy tac bat bien 1).
func (r *Repository) ProcessPaymentSuccessTx(ctx context.Context, params domain.ProcessWebhookParams) (domain.WebhookProcessResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.WebhookProcessResult{}, fmt.Errorf("khoi tao transaction that bai: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// 1. Kiem tra xem (provider, provider_txn_id) da tung duoc ghi nhan SUCCEEDED chua (BR-O6)
	checkPaymentQuery := `
		SELECT id, order_id, status
		FROM payments
		WHERE provider = $1 AND provider_txn_id = $2;
	`
	var (
		existingPaymentID string
		existingOrderID   string
		existingStatus    string
	)
	err = tx.QueryRow(ctx, checkPaymentQuery, string(params.Provider), params.ProviderTxnID).Scan(
		&existingPaymentID, &existingOrderID, &existingStatus,
	)
	if err == nil && existingStatus == string(domain.StatusSucceeded) {
		// Exactly-once: webhook lap lai la no-op tra 200 (BR-O6)
		return domain.WebhookProcessResult{
			Success:       true,
			AlreadyDone:   true,
			OrderID:       existingOrderID,
			PaymentID:     existingPaymentID,
			ProviderTxnID: params.ProviderTxnID,
			Status:        domain.StatusSucceeded,
			Message:       "giao dich da duoc ghi nhan thanh cong truoc do (BR-O6 idempotent)",
		}, nil
	}

	// 2. Doc va khoa dong don hang (FOR UPDATE de tranh race condition)
	orderQuery := `
		SELECT id, event_id, identity_id, status, total_cents, currency, expires_at
		FROM orders
		WHERE id = $1
		FOR UPDATE;
	`
	var (
		orderID    string
		eventID    string
		identityID string
		status     string
		totalCents int64
		currency   string
		expiresAt  time.Time
	)
	err = tx.QueryRow(ctx, orderQuery, params.OrderID).Scan(
		&orderID, &eventID, &identityID, &status, &totalCents, &currency, &expiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WebhookProcessResult{}, domain.ErrOrderNotFound
		}
		return domain.WebhookProcessResult{}, fmt.Errorf("truy van khoa don hang that bai: %w", err)
	}

	if status == "PAID" {
		// Don da sang PAID (vi du do luong khac vua commit)
		return domain.WebhookProcessResult{
			Success:       true,
			AlreadyDone:   true,
			OrderID:       orderID,
			PaymentID:     params.PaymentID,
			ProviderTxnID: params.ProviderTxnID,
			Status:        domain.StatusSucceeded,
			Message:       "don hang da o trang thai PAID truoc do",
		}, nil
	}

	if status != "HELD" && status != "PAYMENT_PENDING" {
		return domain.WebhookProcessResult{}, fmt.Errorf("%w: don hang o trang thai %s", domain.ErrOrderNotHeld, status)
	}

	// 3. Cap nhat orders -> PAID
	updateOrderQuery := `
		UPDATE orders
		SET status = 'PAID', updated_at = now()
		WHERE id = $1 AND status IN ('HELD', 'PAYMENT_PENDING');
	`
	cmdTag, err := tx.Exec(ctx, updateOrderQuery, orderID)
	if err != nil {
		return domain.WebhookProcessResult{}, fmt.Errorf("cap nhat don sang PAID that bai: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return domain.WebhookProcessResult{
			Success:       true,
			AlreadyDone:   true,
			OrderID:       orderID,
			ProviderTxnID: params.ProviderTxnID,
			Status:        domain.StatusSucceeded,
			Message:       "don hang da duoc cap nhat boi tien trinh khac",
		}, nil
	}

	// 4. Cap nhat hoac chen ban ghi vao payments
	rawBytes, err := json.Marshal(params.RawPayload)
	if err != nil {
		rawBytes = []byte("{}")
	}

	paymentID := params.PaymentID
	if paymentID == "" {
		if existingPaymentID != "" {
			paymentID = existingPaymentID
		}
	}

	upsertPaymentQuery := `
		INSERT INTO payments (id, order_id, provider, provider_txn_id, amount_cents, status, raw_payload, created_at)
		VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, 'SUCCEEDED', $6, now())
		ON CONFLICT (provider, provider_txn_id)
		DO UPDATE SET status = 'SUCCEEDED', raw_payload = EXCLUDED.raw_payload
		RETURNING id;
	`
	var finalPaymentID string
	err = tx.QueryRow(ctx, upsertPaymentQuery,
		paymentID,
		orderID,
		string(params.Provider),
		params.ProviderTxnID,
		totalCents,
		rawBytes,
	).Scan(&finalPaymentID)
	if err != nil {
		// Neu co ban ghi payments theo order_id ma chua gan provider_txn_id
		updateExistingPaymentQuery := `
			UPDATE payments
			SET status = 'SUCCEEDED', provider_txn_id = $1, raw_payload = $2
			WHERE order_id = $3 AND provider = $4
			RETURNING id;
		`
		uErr := tx.QueryRow(ctx, updateExistingPaymentQuery,
			params.ProviderTxnID,
			rawBytes,
			orderID,
			string(params.Provider),
		).Scan(&finalPaymentID)
		if uErr != nil {
			return domain.WebhookProcessResult{}, fmt.Errorf("upsert ban ghi payments that bai: %w", err)
		}
	}

	// 5. Ghi su kien outbox_events order.paid
	outboxPayload := domain.OutboxOrderPaidPayload{
		OrderID:       orderID,
		EventID:       eventID,
		IdentityID:    identityID,
		PaymentID:     finalPaymentID,
		Provider:      string(params.Provider),
		ProviderTxnID: params.ProviderTxnID,
		AmountCents:   totalCents,
		Currency:      currency,
		PaidAt:        time.Now().UTC(),
	}
	outboxBytes, err := json.Marshal(outboxPayload)
	if err != nil {
		return domain.WebhookProcessResult{}, fmt.Errorf("marshal outbox payload that bai: %w", err)
	}

	insertOutboxQuery := `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload, trace_id, created_at)
		VALUES ('order', $1, 'order.paid', $2, $3, now());
	`
	_, err = tx.Exec(ctx, insertOutboxQuery, orderID, outboxBytes, params.TraceID)
	if err != nil {
		return domain.WebhookProcessResult{}, fmt.Errorf("chen outbox event that bai: %w", err)
	}

	// Commit Transaction
	if err := tx.Commit(ctx); err != nil {
		return domain.WebhookProcessResult{}, fmt.Errorf("commit transaction that bai: %w", err)
	}

	return domain.WebhookProcessResult{
		Success:       true,
		AlreadyDone:   false,
		OrderID:       orderID,
		PaymentID:     finalPaymentID,
		ProviderTxnID: params.ProviderTxnID,
		Status:        domain.StatusSucceeded,
		Message:       "thanh toan thanh cong, don da sang PAID va phat outbox",
	}, nil
}

// ProcessPaymentFailedTx ghi nhan trang thai thanh toan that bai vao bang payments trong transaction.
func (r *Repository) ProcessPaymentFailedTx(ctx context.Context, params domain.ProcessWebhookParams) (domain.WebhookProcessResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.WebhookProcessResult{}, fmt.Errorf("khoi tao transaction that bai: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rawBytes, err := json.Marshal(params.RawPayload)
	if err != nil {
		rawBytes = []byte("{}")
	}

	query := `
		INSERT INTO payments (id, order_id, provider, provider_txn_id, amount_cents, status, raw_payload, created_at)
		VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, 'FAILED', $6, now())
		ON CONFLICT (provider, provider_txn_id)
		DO UPDATE SET status = 'FAILED', raw_payload = EXCLUDED.raw_payload
		RETURNING id;
	`
	var paymentID string
	err = tx.QueryRow(ctx, query,
		params.PaymentID,
		params.OrderID,
		string(params.Provider),
		params.ProviderTxnID,
		params.AmountCents,
		rawBytes,
	).Scan(&paymentID)
	if err != nil {
		// Thu cap nhat neu da ton tai theo order_id
		updateQuery := `
			UPDATE payments
			SET status = 'FAILED', provider_txn_id = $1, raw_payload = $2
			WHERE order_id = $3 AND provider = $4
			RETURNING id;
		`
		_ = tx.QueryRow(ctx, updateQuery, params.ProviderTxnID, rawBytes, params.OrderID, string(params.Provider)).Scan(&paymentID)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.WebhookProcessResult{}, fmt.Errorf("commit transaction failed that bai: %w", err)
	}

	return domain.WebhookProcessResult{
		Success:       false,
		AlreadyDone:   false,
		OrderID:       params.OrderID,
		PaymentID:     paymentID,
		ProviderTxnID: params.ProviderTxnID,
		Status:        domain.StatusFailed,
		Message:       "da ghi nhan thanh toan that bai",
	}, nil
}
