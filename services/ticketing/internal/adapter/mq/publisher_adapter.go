// Package mqadapter cung cap adapter chuyen doi giua thu vien mq va cac port cua ticketing.
package mqadapter

import (
	"context"
	"encoding/json"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// PublisherAdapter bọc *mq.Publisher để thoả mãn port.Publisher.
// Dùng cho hold sweeper và các tác vụ phát sự kiện trực tiếp trong ticketing.
type PublisherAdapter struct {
	pub      *mq.Publisher
	exchange string
}

// NewPublisherAdapter khởi tạo một PublisherAdapter mới.
func NewPublisherAdapter(pub *mq.Publisher, exchange string) *PublisherAdapter {
	return &PublisherAdapter{
		pub:      pub,
		exchange: exchange,
	}
}

// Publish tuần tự hoá payload sang JSON và phát vào RabbitMQ exchange.
func (a *PublisherAdapter) Publish(ctx context.Context, routingKey string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return a.pub.Publish(ctx, a.exchange, routingKey, mq.Message{
		Body:        body,
		ContentType: "application/json",
	})
}

var _ port.Publisher = (*PublisherAdapter)(nil)
