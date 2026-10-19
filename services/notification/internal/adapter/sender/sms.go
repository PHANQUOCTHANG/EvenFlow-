package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
	"github.com/eventflow/eventflow/services/notification/internal/port"
)

// WebhookSMSSender gui tin nhan SMS qua HTTP Webhook / nha mang.
type WebhookSMSSender struct {
	EndpointURL string
	APIKey      string
	Client      *http.Client
}

var _ port.SMSSender = (*WebhookSMSSender)(nil)

func NewWebhookSMSSender(endpointURL, apiKey string) *WebhookSMSSender {
	return &WebhookSMSSender{
		EndpointURL: endpointURL,
		APIKey:      apiKey,
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type smsPayload struct {
	To      string `json:"to"`
	Message string `json:"message"`
}

func (s *WebhookSMSSender) SendSMS(ctx context.Context, msg domain.SMSMessage) error {
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrSenderFailed, err)
	}

	payload := smsPayload{
		To:      msg.To,
		Message: msg.MessageBody,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal sms payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.EndpointURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create sms http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.APIKey)
	}

	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("dispatch sms request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sms provider returned non-2xx status: %d", resp.StatusCode)
	}

	return nil
}
