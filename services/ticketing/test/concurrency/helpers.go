//go:build integration

package concurrency

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// TestEnv la moi truong tich hop dung Postgres + Redis THAT qua testcontainers.
//
// Khong dung mock o day: bat bien chong oversell nam trong constraint cua
// Postgres va tinh atomic cua Lua script. Mock hai thu do di thi test chi con
// kiem tra chinh mock, va bug that se lot thang ra production.
type TestEnv struct {
	Repo   port.Repository
	Gate   port.InventoryGate
	Logger *slog.Logger
}

// setupEnv dung ha tang test va don dep sau khi xong.
//
// TODO(EVF-39): hien thuc bang testcontainers-go
//   - postgres:16 + chay migrations/0001_core.sql
//   - redis:7 + nap cac Lua script tu internal/adapter/redis/script
//   - t.Cleanup() de go bo container
func setupEnv(t *testing.T) *TestEnv {
	t.Helper()
	t.Skip("TODO(EVF-39): can hien thuc testcontainers truoc khi bat test nay")
	return nil
}

// SeedEvent tao mot su kien dang ON_SALE voi quota cho truoc, da chia bucket
// va da nap ton kho tuong ung vao Redis.
func (e *TestEnv) SeedEvent(t *testing.T, quota int) (eventID, ticketTypeID string) {
	t.Helper()
	panic("TODO(EVF-39)")
}

func (e *TestEnv) SeedIdentity(t *testing.T, n int) string {
	t.Helper()
	panic("TODO(EVF-39)")
}

// AdmitToken cap mot queue token da o trang thai ADMITTED, de test tap trung
// vao ton kho thay vi vao phong cho.
func (e *TestEnv) AdmitToken(t *testing.T, eventID, identityID string) string {
	t.Helper()
	panic("TODO(EVF-39)")
}

// TotalAvailable cong ton kho con lai tren toan bo 32 bucket trong Postgres.
func (e *TestEnv) TotalAvailable(t *testing.T, ticketTypeID string) int {
	t.Helper()
	panic("TODO(EVF-39)")
}

// TotalHeldOrSold dem so ve dang bi giu hoac da ban.
func (e *TestEnv) TotalHeldOrSold(t *testing.T, ticketTypeID string) int {
	t.Helper()
	panic("TODO(EVF-39)")
}

// InventoryDrift do do lech giua Redis va Postgres. Phai luon bang 0 khi he
// thong o trang thai nghi.
func (e *TestEnv) InventoryDrift(t *testing.T, eventID, ticketTypeID string) int {
	t.Helper()
	panic("TODO(EVF-39)")
}

func (e *TestEnv) SetHoldTTL(t *testing.T, eventID string, ttl time.Duration) {
	t.Helper()
	panic("TODO(EVF-39)")
}

func (e *TestEnv) RunSweeper(ctx context.Context, eventID string) {
	panic("TODO(EVF-39)")
}
