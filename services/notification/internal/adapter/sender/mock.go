package sender

import (
	"context"
	"fmt"
	"sync"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
	"github.com/eventflow/eventflow/services/notification/internal/port"
)

// MockEmailSender luu tru cac email da gui phuc vu kiem thu.
type MockEmailSender struct {
	mu            sync.Mutex
	sentEmails    []domain.EmailMessage
	simulateError error
}

var _ port.EmailSender = (*MockEmailSender)(nil)

func NewMockEmailSender() *MockEmailSender {
	return &MockEmailSender{
		sentEmails: make([]domain.EmailMessage, 0),
	}
}

func (s *MockEmailSender) SetSimulateError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.simulateError = err
}

func (s *MockEmailSender) SendEmail(ctx context.Context, msg domain.EmailMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.simulateError != nil {
		return s.simulateError
	}
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrSenderFailed, err)
	}

	s.sentEmails = append(s.sentEmails, msg)
	return nil
}

func (s *MockEmailSender) GetSentEmails() []domain.EmailMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]domain.EmailMessage, len(s.sentEmails))
	copy(result, s.sentEmails)
	return result
}

func (s *MockEmailSender) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sentEmails = make([]domain.EmailMessage, 0)
	s.simulateError = nil
}

// MockSMSSender luu tru cac tin nhan SMS da gui phuc vu kiem thu.
type MockSMSSender struct {
	mu            sync.Mutex
	sentSMS       []domain.SMSMessage
	simulateError error
}

var _ port.SMSSender = (*MockSMSSender)(nil)

func NewMockSMSSender() *MockSMSSender {
	return &MockSMSSender{
		sentSMS: make([]domain.SMSMessage, 0),
	}
}

func (s *MockSMSSender) SetSimulateError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.simulateError = err
}

func (s *MockSMSSender) SendSMS(ctx context.Context, msg domain.SMSMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.simulateError != nil {
		return s.simulateError
	}
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrSenderFailed, err)
	}

	s.sentSMS = append(s.sentSMS, msg)
	return nil
}

func (s *MockSMSSender) GetSentSMS() []domain.SMSMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]domain.SMSMessage, len(s.sentSMS))
	copy(result, s.sentSMS)
	return result
}

func (s *MockSMSSender) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sentSMS = make([]domain.SMSMessage, 0)
	s.simulateError = nil
}
