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
