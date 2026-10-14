package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// InitInventoryInput chứa các tham số cần thiết để khởi tạo tồn kho phân mảnh.
type InitInventoryInput struct {
	EventID      string
	TicketTypeID string
	Quota        int
	BucketCount  int
}

// InventoryService điều phối việc khởi tạo và đồng bộ tồn kho giữa Postgres và Redis Gate.
type InventoryService struct {
	repo   port.Repository
	seeder port.InventorySeeder
	log    *slog.Logger
}

func NewInventoryService(repo port.Repository, seeder port.InventorySeeder, log *slog.Logger) *InventoryService {
	return &InventoryService{
		repo:   repo,
		seeder: seeder,
		log:    log,
	}
}

// InitializeInventory phân mảnh quota thành 32 bucket trong Postgres và nạp vào Redis.
func (s *InventoryService) InitializeInventory(ctx context.Context, input InitInventoryInput) error {
	if input.Quota < 0 {
		return domain.ErrNegativeQuota
	}
	if input.BucketCount <= 0 {
		input.BucketCount = domain.DefaultBucketCount
	}

	// 1. Phân bổ và khởi tạo trong Postgres (nguồn sự thật)
	if err := s.repo.InitInventory(ctx, input.TicketTypeID, input.Quota, input.BucketCount); err != nil {
		return fmt.Errorf("khởi tạo tồn kho postgres: %w", err)
	}

	// 2. Tính toán các bucket để nạp nhanh lên Redis Gate
	buckets, err := domain.DistributeQuota(input.Quota, input.BucketCount)
	if err != nil {
		return err
	}

	if s.seeder != nil {
		perBucket := make(map[int]int, len(buckets))
		for b, qty := range buckets {
			perBucket[b] = qty
		}

		if err := s.seeder.SeedInventory(ctx, input.EventID, input.TicketTypeID, perBucket); err != nil {
			s.log.Warn("nạp tồn kho lên redis thất bại (worker seeder sẽ đồng bộ lại sau)",
				"event_id", input.EventID, "ticket_type_id", input.TicketTypeID, "err", err)
		}
	}

	return nil
}
