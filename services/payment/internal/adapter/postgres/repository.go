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

// SavePaymentInbox luu webhook vao bang payment_inbox khi don chua ton tai (BR-O6).
// Su dung ON CONFLICT de dam bao tinh idempotent khi luu ban ghi.
func (r *Repository) SavePaymentInbox(ctx context.Context, item domain.PaymentInboxItem) (int64, error) {
	rawBytes, err := json.Marshal(item.Payload)
	if err != nil {
		rawBytes = []byte("{}")
	}

	receivedAt := item.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO payment_inbox (provider, provider_txn_id, payload, received_at, attempts)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (provider, provider_txn_id) DO UPDATE
		SET attempts = payment_inbox.attempts
		RETURNING id;
	`
	var id int64
	err = r.pool.QueryRow(ctx, query,
		string(item.Provider),
		item.ProviderTxnID,
		rawBytes,
		receivedAt,
		item.Attempts,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("luu payment_inbox that bai: %w", err)
	}

	return id, nil
}

// GetPendingPaymentInbox lay danh sach ban ghi chua xu ly (processed_at IS NULL) voi so lan thu < maxAttempts.
func (r *Repository) GetPendingPaymentInbox(ctx context.Context, limit int, maxAttempts int) ([]domain.PaymentInboxItem, error) {
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, provider, provider_txn_id, payload, received_at, processed_at, attempts
		FROM payment_inbox
		WHERE processed_at IS NULL AND attempts < $1
		ORDER BY received_at ASC
		LIMIT $2;
	`

	rows, err := r.pool.Query(ctx, query, maxAttempts, limit)
	if err != nil {
		return nil, fmt.Errorf("truy van pending payment_inbox that bai: %w", err)
	}
	defer rows.Close()

	var items []domain.PaymentInboxItem
	for rows.Next() {
		var (
			it          domain.PaymentInboxItem
			providerStr string
			rawBytes    []byte
		)
		err := rows.Scan(
			&it.ID,
			&providerStr,
			&it.ProviderTxnID,
			&rawBytes,
			&it.ReceivedAt,
			&it.ProcessedAt,
			&it.Attempts,
		)
		if err != nil {
			return nil, fmt.Errorf("scan dong payment_inbox that bai: %w", err)
		}
		it.Provider = domain.NormalizeProvider(providerStr)
		if len(rawBytes) > 0 {
			_ = json.Unmarshal(rawBytes, &it.Payload)
		}
		items = append(items, it)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("doc danh sach payment_inbox that bai: %w", err)
	}

	return items, nil
}

// MarkPaymentInboxProcessed cap nhat processed_at = now() cho ban ghi inbox da xu ly xong.
func (r *Repository) MarkPaymentInboxProcessed(ctx context.Context, id int64, processedAt time.Time) error {
	if processedAt.IsZero() {
		processedAt = time.Now().UTC()
	}

	query := `
		UPDATE payment_inbox
		SET processed_at = $1
		WHERE id = $2;
	`
	_, err := r.pool.Exec(ctx, query, processedAt, id)
	if err != nil {
		return fmt.Errorf("cap nhat processed_at cho payment_inbox id %d that bai: %w", id, err)
	}
	return nil
}

// IncrementPaymentInboxAttempts tang so lan thu sau moi luot replay chua thanh cong.
func (r *Repository) IncrementPaymentInboxAttempts(ctx context.Context, id int64) error {
	query := `
		UPDATE payment_inbox
		SET attempts = attempts + 1
		WHERE id = $1;
	`
	_, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("tang attempts cho payment_inbox id %d that bai: %w", id, err)
	}
	return nil
}

// GetStalePaymentInbox tim cac ban ghi chua xu ly co thoi gian ton vuot qua nguong cho phep.
func (r *Repository) GetStalePaymentInbox(ctx context.Context, olderThan time.Duration, limit int) ([]domain.StaleInboxRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	thresholdTime := time.Now().UTC().Add(-olderThan)

	query := `
		SELECT id, provider, provider_txn_id, received_at, attempts
		FROM payment_inbox
		WHERE processed_at IS NULL AND received_at <= $1
		ORDER BY received_at ASC
		LIMIT $2;
	`

	rows, err := r.pool.Query(ctx, query, thresholdTime, limit)
	if err != nil {
		return nil, fmt.Errorf("truy van stale payment_inbox that bai: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	var records []domain.StaleInboxRecord
	for rows.Next() {
		var (
			rec         domain.StaleInboxRecord
			providerStr string
		)
		err := rows.Scan(
			&rec.ID,
			&providerStr,
			&rec.ProviderTxnID,
			&rec.ReceivedAt,
			&rec.Attempts,
		)
		if err != nil {
			return nil, fmt.Errorf("scan stale payment_inbox that bai: %w", err)
		}
		rec.Provider = domain.NormalizeProvider(providerStr)
		rec.Age = now.Sub(rec.ReceivedAt)
		records = append(records, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("doc stale payment_inbox that bai: %w", err)
	}

	return records, nil
}

// GetStalePendingOrders quet cac don hang o trang thai PAYMENT_PENDING ton tai qua nguong thoi gian olderThan.
func (r *Repository) GetStalePendingOrders(ctx context.Context, olderThan time.Duration, limit int) ([]domain.StaleOrderInfo, error) {
	if limit <= 0 {
		limit = 50
	}
	thresholdTime := time.Now().UTC().Add(-olderThan)

	query := `
		SELECT o.id, o.event_id, o.identity_id, o.status, o.total_cents, o.currency,
		       o.expires_at, o.updated_at, o.created_at,
		       COALESCE(p.id::text, ''), COALESCE(p.provider, ''), COALESCE(p.provider_txn_id, '')
		FROM orders o
		LEFT JOIN LATERAL (
			SELECT id, provider, provider_txn_id
			FROM payments
			WHERE order_id = o.id
			ORDER BY created_at DESC
			LIMIT 1
		) p ON true
		WHERE o.status = 'PAYMENT_PENDING'
		  AND (o.updated_at <= $1 OR o.created_at <= $1)
		ORDER BY o.updated_at ASC
		LIMIT $2;
	`

	rows, err := r.pool.Query(ctx, query, thresholdTime, limit)
	if err != nil {
		return nil, fmt.Errorf("truy van stale pending orders that bai: %w", err)
	}
	defer rows.Close()

	var results []domain.StaleOrderInfo
	for rows.Next() {
		var (
			info          domain.StaleOrderInfo
			providerStr   string
			paymentIDStr  string
			providerTxnID string
		)
		err := rows.Scan(
			&info.OrderID,
			&info.EventID,
			&info.IdentityID,
			&info.Status,
			&info.TotalCents,
			&info.Currency,
			&info.ExpiresAt,
			&info.UpdatedAt,
			&info.CreatedAt,
			&paymentIDStr,
			&providerStr,
			&providerTxnID,
		)
		if err != nil {
			return nil, fmt.Errorf("scan stale pending order that bai: %w", err)
		}

		info.PaymentID = paymentIDStr
		info.Provider = domain.NormalizeProvider(providerStr)
		info.ProviderTxnID = providerTxnID
		results = append(results, info)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("doc stale pending orders that bai: %w", err)
	}

	return results, nil
}

// ProcessOrderCancellationTx huy don hang treo, giai phong kho ve idempotent va cap nhat payment trong CUNG MOT TRANSACTION (Quy tac bat bien 1, BR-O7).
func (r *Repository) ProcessOrderCancellationTx(ctx context.Context, params domain.CancelOrderParams) (domain.CancelOrderResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.CancelOrderResult{}, fmt.Errorf("khoi tao transaction huy don that bai: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// 1. Doc va khoa dong don hang (FOR UPDATE de tranh race condition voi webhook den muon)
	orderQuery := `
		SELECT id, event_id, status
		FROM orders
		WHERE id = $1
		FOR UPDATE;
	`
	var (
		orderID string
		eventID string
		status  string
	)
	err = tx.QueryRow(ctx, orderQuery, params.OrderID).Scan(&orderID, &eventID, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CancelOrderResult{}, domain.ErrOrderNotFound
		}
		return domain.CancelOrderResult{}, fmt.Errorf("khoa don hang de huy that bai: %w", err)
	}

	// Neu don da o trang thai CANCELLED hoac EXPIRED -> da duoc huy truoc do (idempotent)
	if status == "CANCELLED" || status == "EXPIRED" {
		return domain.CancelOrderResult{
			OrderID:     orderID,
			AlreadyDone: true,
			Status:      status,
			Message:     fmt.Sprintf("don hang da o trang thai %s truoc do", status),
		}, nil
	}

	// Neu don da o trang thai PAID -> Webhook vua kip commit truoc, khong duoc phep huy!
	if status == "PAID" || status == "ISSUED" {
		return domain.CancelOrderResult{
			OrderID:     orderID,
			AlreadyDone: true,
			Status:      status,
			Message:     "don hang da thanh toan thanh cong (PAID), khong the huy",
		}, nil
	}

	// 2. Cap nhat trang thai orders -> CANCELLED
	updateOrderQuery := `
		UPDATE orders
		SET status = 'CANCELLED', updated_at = now()
		WHERE id = $1 AND status = 'PAYMENT_PENDING';
	`
	cmdTag, err := tx.Exec(ctx, updateOrderQuery, orderID)
	if err != nil {
		return domain.CancelOrderResult{}, fmt.Errorf("cap nhat don sang CANCELLED that bai: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return domain.CancelOrderResult{
			OrderID:     orderID,
			AlreadyDone: true,
			Status:      status,
			Message:     "don hang khong con o trang thai PAYMENT_PENDING",
		}, nil
	}

	// 3. Giai phong seat_holds idempotent (BR-O7: WHERE released_at IS NULL AND consumed_at IS NULL)
	claimHoldsQuery := `
		UPDATE seat_holds
		SET released_at = now()
		WHERE order_id = $1::uuid AND released_at IS NULL AND consumed_at IS NULL
		RETURNING ticket_type_id::text, bucket, quantity;
	`
	rows, err := tx.Query(ctx, claimHoldsQuery, orderID)
	if err != nil {
		return domain.CancelOrderResult{}, fmt.Errorf("giai phong seat_holds that bai: %w", err)
	}

	type releasedHold struct {
		ticketTypeID string
		bucket       int
		quantity     int
	}
	var releasedList []releasedHold
	for rows.Next() {
		var rh releasedHold
		if err := rows.Scan(&rh.ticketTypeID, &rh.bucket, &rh.quantity); err != nil {
			rows.Close()
			return domain.CancelOrderResult{}, fmt.Errorf("scan released seat_hold that bai: %w", err)
		}
		releasedList = append(releasedList, rh)
	}
	rows.Close()

	// 4. Hoan tra kho ve ticket_inventory tuong ung cho tung bucket
	ticketsReturned := 0
	for _, rh := range releasedList {
		restoreQuery := `
			UPDATE ticket_inventory
			SET available = available + $3
			WHERE ticket_type_id = $1::uuid AND bucket = $2;
		`
		if _, err := tx.Exec(ctx, restoreQuery, rh.ticketTypeID, rh.bucket, rh.quantity); err != nil {
			return domain.CancelOrderResult{}, fmt.Errorf("hoan tra ve vao ticket_inventory that bai: %w", err)
		}
		ticketsReturned += rh.quantity
	}

	// 5. Cap nhat ban ghi payments sang FAILED neu co
	updatePaymentQuery := `
		UPDATE payments
		SET status = 'FAILED', raw_payload = jsonb_build_object('cancel_reason', $2::text, 'cancelled_at', now()::text)
		WHERE order_id = $1::uuid AND status = 'PENDING';
	`
	_, _ = tx.Exec(ctx, updatePaymentQuery, orderID, params.Reason)

	// 6. Ghi su kien outbox_events hold.released
	outboxPayload := domain.OutboxHoldReleasedPayload{
		OrderID:    orderID,
		EventID:    eventID,
		Reason:     params.Reason,
		ReleasedAt: time.Now().UTC(),
	}
	outboxBytes, err := json.Marshal(outboxPayload)
	if err != nil {
		return domain.CancelOrderResult{}, fmt.Errorf("marshal outbox payload that bai: %w", err)
	}

	insertOutboxQuery := `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload, trace_id, created_at)
		VALUES ('order', $1, 'hold.released', $2, $3, now());
	`
	_, err = tx.Exec(ctx, insertOutboxQuery, orderID, outboxBytes, params.TraceID)
	if err != nil {
		return domain.CancelOrderResult{}, fmt.Errorf("chen outbox event that bai: %w", err)
	}

	// Commit Transaction
	if err := tx.Commit(ctx); err != nil {
		return domain.CancelOrderResult{}, fmt.Errorf("commit transaction huy don that bai: %w", err)
	}

	return domain.CancelOrderResult{
		OrderID:         orderID,
		AlreadyDone:     false,
		HoldsReleased:   len(releasedList),
		TicketsReturned: ticketsReturned,
		Status:          "CANCELLED",
		Message:         "huy don thanh cong, da tra kho ve va phat outbox",
	}, nil
}

// GetPaidOrdersByEvent lay danh sach cac don hang o trang thai PAID, ISSUED hoac REFUNDING cua mot su kien de hoan tien (EV-154 / EVF-74).
func (r *Repository) GetPaidOrdersByEvent(ctx context.Context, eventID string, limit int) ([]domain.RefundOrderInfo, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT o.id::text, o.event_id::text, o.identity_id::text, o.status, o.total_cents, o.currency,
		       o.created_at, o.updated_at,
		       COALESCE(p.id::text, ''), COALESCE(p.provider, ''), COALESCE(p.provider_txn_id, '')
		FROM orders o
		LEFT JOIN payments p ON p.order_id = o.id AND p.status IN ('SUCCEEDED', 'REFUNDED')
		WHERE o.event_id = $1::uuid AND o.status IN ('PAID', 'ISSUED', 'REFUNDING')
		ORDER BY o.created_at ASC
		LIMIT $2;
	`

	rows, err := r.pool.Query(ctx, query, eventID, limit)
	if err != nil {
		return nil, fmt.Errorf("truy van don hang can hoan tien that bai: %w", err)
	}
	defer rows.Close()

	var results []domain.RefundOrderInfo
	for rows.Next() {
		var (
			orderID       string
			evtID         string
			identityID    string
			status        string
			totalCents    int64
			currency      string
			createdAt     time.Time
			updatedAt     time.Time
			paymentID     string
			providerStr   string
			providerTxnID string
		)

		if err := rows.Scan(
			&orderID, &evtID, &identityID, &status, &totalCents, &currency,
			&createdAt, &updatedAt,
			&paymentID, &providerStr, &providerTxnID,
		); err != nil {
			return nil, fmt.Errorf("scan refund order info that bai: %w", err)
		}

		results = append(results, domain.RefundOrderInfo{
			OrderID:       orderID,
			EventID:       evtID,
			IdentityID:    identityID,
			Status:        status,
			TotalCents:    totalCents,
			Currency:      currency,
			PaymentID:     paymentID,
			Provider:      domain.NormalizeProvider(providerStr),
			ProviderTxnID: providerTxnID,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
		})
	}

	return results, nil
}

// MarkOrderRefundingTx cap nhat trang thai don hang sang REFUNDING trong transaction rieng biet (Quy tac bat bien 1).
func (r *Repository) MarkOrderRefundingTx(ctx context.Context, orderID string) error {
	query := `
		UPDATE orders
		SET status = 'REFUNDING', updated_at = now()
		WHERE id = $1::uuid AND status IN ('PAID', 'ISSUED');
	`
	cmdTag, err := r.pool.Exec(ctx, query, orderID)
	if err != nil {
		return fmt.Errorf("cap nhat don hang sang REFUNDING that bai: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		// Kiem tra xem don co ton tai hoac da o REFUNDED chua
		var status string
		err := r.pool.QueryRow(ctx, "SELECT status FROM orders WHERE id = $1::uuid", orderID).Scan(&status)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrOrderNotFound
			}
			return fmt.Errorf("kiem tra trang thai don hang that bai: %w", err)
		}
		if status == "REFUNDED" {
			return nil // Da duoc hoan tien truoc do
		}
		if status == "REFUNDING" {
			return nil // Dang o trang thai REFUNDING
		}
		return domain.ErrOrderNotRefundable
	}
	return nil
}

// ProcessRefundSuccessTx cap nhat trang thai orders -> REFUNDED, payments -> REFUNDED, tickets -> REFUNDED
// va ghi outbox event trong CUNG MOT TRANSACTION (Quy tac bat bien 1, BR-O4).
func (r *Repository) ProcessRefundSuccessTx(ctx context.Context, params domain.ProcessRefundParams) (domain.ProcessRefundResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.ProcessRefundResult{}, fmt.Errorf("khoi tao transaction hoan tien that bai: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// 1. Khoa ban ghi order bang FOR UPDATE
	var (
		orderID    string
		eventID    string
		identityID string
		status     string
		currency   string
	)
	lockQuery := `
		SELECT id::text, event_id::text, identity_id::text, status, currency
		FROM orders
		WHERE id = $1::uuid
		FOR UPDATE;
	`
	err = tx.QueryRow(ctx, lockQuery, params.OrderID).Scan(&orderID, &eventID, &identityID, &status, &currency)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ProcessRefundResult{}, domain.ErrOrderNotFound
		}
		return domain.ProcessRefundResult{}, fmt.Errorf("khoa don hang de hoan tien that bai: %w", err)
	}

	// Idempotent: Neu don da o trang thai REFUNDED truoc do
	if status == "REFUNDED" {
		return domain.ProcessRefundResult{
			OrderID:         orderID,
			PaymentID:       params.PaymentID,
			AlreadyDone:     true,
			TicketsRefunded: 0,
			Status:          status,
			Message:         "don hang da duoc hoan tien truoc do (idempotent)",
		}, nil
	}

	if status != "PAID" && status != "ISSUED" && status != "REFUNDING" {
		return domain.ProcessRefundResult{}, domain.ErrOrderNotRefundable
	}

	// 2. Cap nhat orders sang REFUNDED
	updateOrderQuery := `
		UPDATE orders
		SET status = 'REFUNDED', updated_at = now()
		WHERE id = $1::uuid;
	`
	if _, err := tx.Exec(ctx, updateOrderQuery, orderID); err != nil {
		return domain.ProcessRefundResult{}, fmt.Errorf("cap nhat don hang sang REFUNDED that bai: %w", err)
	}

	// 3. Cap nhat payments sang REFUNDED neu co
	updatePaymentQuery := `
		UPDATE payments
		SET status = 'REFUNDED',
		    raw_payload = COALESCE(raw_payload, '{}'::jsonb) || jsonb_build_object(
		        'refund_txn_id', $2::text,
		        'refund_reason', $3::text,
		        'refunded_at', now()::text
		    )
		WHERE order_id = $1::uuid;
	`
	if _, err := tx.Exec(ctx, updatePaymentQuery, orderID, params.RefundTxnID, params.Reason); err != nil {
		return domain.ProcessRefundResult{}, fmt.Errorf("cap nhat payments sang REFUNDED that bai: %w", err)
	}

	// 4. Cap nhat tickets sang REFUNDED
	updateTicketsQuery := `
		UPDATE tickets
		SET status = 'REFUNDED'
		WHERE order_id = $1::uuid AND status != 'VOID';
	`
	cmdTickets, err := tx.Exec(ctx, updateTicketsQuery, orderID)
	ticketsRefunded := 0
	if err == nil {
		ticketsRefunded = int(cmdTickets.RowsAffected())
	}

	// 5. Ghi outbox event payment.refunded
	now := time.Now().UTC()
	outboxPayload := domain.OutboxPaymentRefundedPayload{
		OrderID:     orderID,
		EventID:     eventID,
		IdentityID:  identityID,
		PaymentID:   params.PaymentID,
		Provider:    string(params.Provider),
		RefundTxnID: params.RefundTxnID,
		AmountCents: params.AmountCents,
		Currency:    currency,
		Reason:      params.Reason,
		RefundedAt:  now,
	}
	outboxBytes, err := json.Marshal(outboxPayload)
	if err != nil {
		return domain.ProcessRefundResult{}, fmt.Errorf("marshal outbox payload that bai: %w", err)
	}

	insertOutboxQuery := `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload, trace_id, created_at)
		VALUES ('payment', $1::uuid, 'payment.refunded', $2, $3, now());
	`
	if _, err := tx.Exec(ctx, insertOutboxQuery, orderID, outboxBytes, params.TraceID); err != nil {
		return domain.ProcessRefundResult{}, fmt.Errorf("chen outbox event payment.refunded that bai: %w", err)
	}

	// Commit Transaction
	if err := tx.Commit(ctx); err != nil {
		return domain.ProcessRefundResult{}, fmt.Errorf("commit transaction hoan tien that bai: %w", err)
	}

	return domain.ProcessRefundResult{
		OrderID:         orderID,
		PaymentID:       params.PaymentID,
		AlreadyDone:     false,
		TicketsRefunded: ticketsRefunded,
		Status:          "REFUNDED",
		Message:         "hoan tien thanh cong, da cap nhat trang thai va phat outbox",
	}, nil
}

// ProcessRefundFailedTx ghi nhan trang thai hoan tien that bai hoac can retry vao payments table.
func (r *Repository) ProcessRefundFailedTx(ctx context.Context, orderID string, paymentID string, reason string) error {
	query := `
		UPDATE payments
		SET raw_payload = COALESCE(raw_payload, '{}'::jsonb) || jsonb_build_object(
		    'refund_last_error', $2::text,
		    'refund_attempt_at', now()::text
		)
		WHERE order_id = $1::uuid;
	`
	_, err := r.pool.Exec(ctx, query, orderID, reason)
	return err
}
