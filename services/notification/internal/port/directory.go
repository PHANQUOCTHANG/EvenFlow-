package port

import "context"

// Recipient la thong tin lien lac cua nguoi mua va ten su kien de dien vao thong bao.
type Recipient struct {
	Email      string
	EventTitle string
}

// RecipientDirectory tra cuu nguoi nhan theo identity. Payment va ticketing chi phat
// identity_id/event_id (khong dua PII len message bus), notification tu tra email.
//
// Khong tim thay identity: tra Recipient rong, err nil (use case se tu choi vi thieu nguoi
// nhan -> DLQ). err != nil chi danh cho loi tam thoi (DB mat) de message duoc retry.
type RecipientDirectory interface {
	Lookup(ctx context.Context, identityID, eventID string) (Recipient, error)
}
