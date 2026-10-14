//go:build integration

package redisadapter_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/testx"
	redisadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

func TestMain(m *testing.M) {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

func setupTestGate(t *testing.T) (*redisadapter.Gate, *redis.Client) {
	t.Helper()
	ctx := context.Background()
	addr := testx.Redis(t)

	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	t.Cleanup(func() { _ = rdb.Close() })

	gate, err := redisadapter.NewGate(ctx, rdb)
	if err != nil {
		t.Fatalf("failed to create gate: %v", err)
	}

	return gate, rdb
}

func TestGate_Hold_Success(t *testing.T) {
	gate, rdb := setupTestGate(t)
	ctx := context.Background()

	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()
	identityID := uuid.New().String()
	holdID := uuid.New().String()

	// Seed 10 tickets in bucket 0
	err := gate.SeedInventory(ctx, eventID, ticketTypeID, map[int]int{0: 10})
	if err != nil {
		t.Fatalf("SeedInventory failed: %v", err)
	}

	req := port.HoldRequest{
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		IdentityID:   identityID,
		HoldID:       holdID,
		Quantity:     2,
		TTL:          10 * time.Minute,
		BucketCount:  32,
		StartBucket:  0,
	}

	res, err := gate.Hold(ctx, req)
	if err != nil {
		t.Fatalf("Hold failed: %v", err)
	}

	if res.Status != port.HoldOK {
		t.Fatalf("expected HoldOK, got %s", res.Status)
	}
	if res.Bucket != 0 {
		t.Fatalf("expected bucket 0, got %d", res.Bucket)
	}
	if res.ExpiresAt.Before(time.Now()) {
		t.Fatalf("expected ExpiresAt in the future, got %v", res.ExpiresAt)
	}

	// Verify inventory reduced to 8
	invKey := fmt.Sprintf("inv:{%s}:tt:%s", eventID, ticketTypeID)
	availStr, err := rdb.HGet(ctx, invKey, "0").Result()
	if err != nil {
		t.Fatalf("HGet failed: %v", err)
	}
	if availStr != "8" {
		t.Fatalf("expected 8 tickets left, got %s", availStr)
	}
}

func TestGate_Hold_SoldOut(t *testing.T) {
	gate, _ := setupTestGate(t)
	ctx := context.Background()

	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()
	identityID := uuid.New().String()

	// Seed 2 tickets in bucket 0
	_ = gate.SeedInventory(ctx, eventID, ticketTypeID, map[int]int{0: 2})

	req := port.HoldRequest{
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		IdentityID:   identityID,
		HoldID:       uuid.New().String(),
		Quantity:     5, // Greater than available
		TTL:          10 * time.Minute,
		BucketCount:  32,
		StartBucket:  0,
	}

	res, err := gate.Hold(ctx, req)
	if err != nil {
		t.Fatalf("Hold failed: %v", err)
	}

	if res.Status != port.HoldSoldOut {
		t.Fatalf("expected HoldSoldOut, got %s", res.Status)
	}
}

func TestGate_Hold_BadQty(t *testing.T) {
	gate, _ := setupTestGate(t)
	ctx := context.Background()

	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()

	res, err := gate.Hold(ctx, port.HoldRequest{
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		IdentityID:   uuid.New().String(),
		HoldID:       uuid.New().String(),
		Quantity:     0,
		TTL:          10 * time.Minute,
		BucketCount:  32,
		StartBucket:  0,
	})
	if err != nil {
		t.Fatalf("Hold failed: %v", err)
	}
	if res.Status != port.HoldBadQty {
		t.Fatalf("expected HoldBadQty, got %s", res.Status)
	}
}

func TestGate_Hold_AlreadyHolding_BR_O3(t *testing.T) {
	gate, _ := setupTestGate(t)
	ctx := context.Background()

	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()
	identityID := uuid.New().String()
	firstHoldID := uuid.New().String()

	_ = gate.SeedInventory(ctx, eventID, ticketTypeID, map[int]int{0: 20})

	// First hold succeeds
	res1, err := gate.Hold(ctx, port.HoldRequest{
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		IdentityID:   identityID,
		HoldID:       firstHoldID,
		Quantity:     2,
		TTL:          10 * time.Minute,
		BucketCount:  32,
		StartBucket:  0,
	})
	if err != nil || res1.Status != port.HoldOK {
		t.Fatalf("first hold failed: %v, res: %+v", err, res1)
	}

	// Second hold by same identity returns HoldAlreadyHolding (BR-O3)
	res2, err := gate.Hold(ctx, port.HoldRequest{
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		IdentityID:   identityID,
		HoldID:       uuid.New().String(),
		Quantity:     1,
		TTL:          10 * time.Minute,
		BucketCount:  32,
		StartBucket:  0,
	})
	if err != nil {
		t.Fatalf("second hold error: %v", err)
	}
	if res2.Status != port.HoldAlreadyHolding {
		t.Fatalf("expected HoldAlreadyHolding, got %s", res2.Status)
	}
	if res2.ExistingHold != firstHoldID {
		t.Fatalf("expected existing hold %s, got %s", firstHoldID, res2.ExistingHold)
	}
}

func TestGate_Hold_RingScan3Buckets(t *testing.T) {
	gate, _ := setupTestGate(t)
	ctx := context.Background()

	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()
	identityID := uuid.New().String()

	// Bucket 5: 0, Bucket 6: 0, Bucket 7: 10
	_ = gate.SeedInventory(ctx, eventID, ticketTypeID, map[int]int{
		5: 0,
		6: 0,
		7: 10,
	})

	// Start at bucket 5, scan up to 3 buckets (5, 6, 7) -> should choose bucket 7
	res, err := gate.Hold(ctx, port.HoldRequest{
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		IdentityID:   identityID,
		HoldID:       uuid.New().String(),
		Quantity:     3,
		TTL:          10 * time.Minute,
		BucketCount:  32,
		StartBucket:  5,
	})
	if err != nil {
		t.Fatalf("Hold failed: %v", err)
	}
	if res.Status != port.HoldOK {
		t.Fatalf("expected HoldOK, got %s", res.Status)
	}
	if res.Bucket != 7 {
		t.Fatalf("expected bucket 7, got %d", res.Bucket)
	}
}

func TestGate_Release_Idempotent_BR_O7(t *testing.T) {
	gate, rdb := setupTestGate(t)
	ctx := context.Background()

	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()
	identityID := uuid.New().String()
	holdID := uuid.New().String()

	_ = gate.SeedInventory(ctx, eventID, ticketTypeID, map[int]int{0: 10})

	// Hold 4 tickets -> available becomes 6
	res, err := gate.Hold(ctx, port.HoldRequest{
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		IdentityID:   identityID,
		HoldID:       holdID,
		Quantity:     4,
		TTL:          10 * time.Minute,
		BucketCount:  32,
		StartBucket:  0,
	})
	if err != nil || res.Status != port.HoldOK {
		t.Fatalf("Hold failed: %v", err)
	}

	invKey := fmt.Sprintf("inv:{%s}:tt:%s", eventID, ticketTypeID)

	// First release -> restores 4 tickets -> available becomes 10
	err = gate.Release(ctx, eventID, holdID)
	if err != nil {
		t.Fatalf("Release 1 failed: %v", err)
	}

	avail, _ := rdb.HGet(ctx, invKey, "0").Result()
	if avail != "10" {
		t.Fatalf("expected 10 after release, got %s", avail)
	}

	// Second release (idempotent, BR-O7) -> NOOP, inventory MUST NOT increase to 14
	err = gate.Release(ctx, eventID, holdID)
	if err != nil {
		t.Fatalf("Release 2 failed: %v", err)
	}

	avail2, _ := rdb.HGet(ctx, invKey, "0").Result()
	if avail2 != "10" {
		t.Fatalf("INVENTORY INFLATION DETECTED (BR-O7): expected 10 after 2nd release, got %s", avail2)
	}
}

func TestGate_Consume(t *testing.T) {
	gate, rdb := setupTestGate(t)
	ctx := context.Background()

	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()
	identityID := uuid.New().String()
	holdID := uuid.New().String()

	_ = gate.SeedInventory(ctx, eventID, ticketTypeID, map[int]int{0: 10})

	res, err := gate.Hold(ctx, port.HoldRequest{
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		IdentityID:   identityID,
		HoldID:       holdID,
		Quantity:     3,
		TTL:          10 * time.Minute,
		BucketCount:  32,
		StartBucket:  0,
	})
	if err != nil || res.Status != port.HoldOK {
		t.Fatalf("Hold failed: %v", err)
	}

	// Consume hold (e.g. after payment success)
	err = gate.Consume(ctx, eventID, holdID)
	if err != nil {
		t.Fatalf("Consume failed: %v", err)
	}

	// Verify hold is deleted from hold:h and hold:z
	holdHKey := fmt.Sprintf("hold:{%s}:h", eventID)
	exists, _ := rdb.HExists(ctx, holdHKey, holdID).Result()
	if exists {
		t.Fatalf("expected hold to be deleted from hold:h")
	}

	// Verify inventory is NOT restored (still 7)
	invKey := fmt.Sprintf("inv:{%s}:tt:%s", eventID, ticketTypeID)
	avail, _ := rdb.HGet(ctx, invKey, "0").Result()
	if avail != "7" {
		t.Fatalf("expected 7 tickets left after consume, got %s", avail)
	}
}

func TestGate_IsAdmitted(t *testing.T) {
	gate, rdb := setupTestGate(t)
	ctx := context.Background()

	eventID := uuid.New().String()
	tokenValid := "token-valid"
	tokenExpired := "token-expired"

	admittedKey := fmt.Sprintf("wr:{%s}:admitted", eventID)
	_ = rdb.ZAdd(ctx, admittedKey, redis.Z{
		Score:  float64(time.Now().Add(10 * time.Minute).UnixMilli()),
		Member: tokenValid,
	}).Err()
	_ = rdb.ZAdd(ctx, admittedKey, redis.Z{
		Score:  float64(time.Now().Add(-10 * time.Minute).UnixMilli()),
		Member: tokenExpired,
	}).Err()

	// Valid token
	admitted, err := gate.IsAdmitted(ctx, eventID, tokenValid)
	if err != nil || !admitted {
		t.Fatalf("expected tokenValid to be admitted, got %v, err: %v", admitted, err)
	}

	// Expired token
	admitted, err = gate.IsAdmitted(ctx, eventID, tokenExpired)
	if err != nil || admitted {
		t.Fatalf("expected tokenExpired to NOT be admitted, got %v, err: %v", admitted, err)
	}

	// Unknown token
	admitted, err = gate.IsAdmitted(ctx, eventID, "unknown-token")
	if err != nil || admitted {
		t.Fatalf("expected unknown-token to NOT be admitted, got %v, err: %v", admitted, err)
	}
}

func TestGate_ExpiredHolds(t *testing.T) {
	gate, rdb := setupTestGate(t)
	ctx := context.Background()

	eventID := uuid.New().String()
	holdZKey := fmt.Sprintf("hold:{%s}:z", eventID)

	expiredHoldID := "hold-expired-1"
	futureHoldID := "hold-future-1"

	_ = rdb.ZAdd(ctx, holdZKey, redis.Z{
		Score:  float64(time.Now().Add(-5 * time.Minute).UnixMilli()),
		Member: expiredHoldID,
	})
	_ = rdb.ZAdd(ctx, holdZKey, redis.Z{
		Score:  float64(time.Now().Add(5 * time.Minute).UnixMilli()),
		Member: futureHoldID,
	})

	expired, err := gate.ExpiredHolds(ctx, eventID, 10)
	if err != nil {
		t.Fatalf("ExpiredHolds failed: %v", err)
	}

	if len(expired) != 1 || expired[0] != expiredHoldID {
		t.Fatalf("expected [hold-expired-1], got %v", expired)
	}
}

func TestGate_SeedInventory_PreservesExisting(t *testing.T) {
	gate, rdb := setupTestGate(t)
	ctx := context.Background()

	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()

	// Initial seed: bucket 0 has 25
	_ = gate.SeedInventory(ctx, eventID, ticketTypeID, map[int]int{0: 25})

	// Second seed attempts to set bucket 0 to 50
	_ = gate.SeedInventory(ctx, eventID, ticketTypeID, map[int]int{0: 50, 1: 30})

	invKey := fmt.Sprintf("inv:{%s}:tt:%s", eventID, ticketTypeID)
	val0, _ := rdb.HGet(ctx, invKey, "0").Result()
	if val0 != "25" {
		t.Fatalf("HSetNX failed to preserve bucket 0: expected 25, got %s", val0)
	}

	val1, _ := rdb.HGet(ctx, invKey, "1").Result()
	if val1 != "30" {
		t.Fatalf("HSetNX failed to set new bucket 1: expected 30, got %s", val1)
	}
}
