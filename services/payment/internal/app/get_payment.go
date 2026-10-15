package app

import (
	"context"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

type GetPaymentUseCase struct {
	repo port.PaymentRepository
}

func NewGetPaymentUseCase(repo port.PaymentRepository) *GetPaymentUseCase {
	return &GetPaymentUseCase{repo: repo}
}

func (uc *GetPaymentUseCase) Execute(ctx context.Context, paymentID string) (domain.Payment, error) {
	if paymentID == "" {
		return domain.Payment{}, domain.ErrInvalidInput
	}
	return uc.repo.GetPayment(ctx, paymentID)
}

type ListMethodsUseCase struct {
	registry port.GatewayRegistry
}

func NewListMethodsUseCase(registry port.GatewayRegistry) *ListMethodsUseCase {
	return &ListMethodsUseCase{registry: registry}
}

func (uc *ListMethodsUseCase) Execute(_ context.Context) []domain.PaymentMethodInfo {
	return uc.registry.ListEnabled()
}
