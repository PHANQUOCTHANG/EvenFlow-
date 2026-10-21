//go:build integration

package redisadapter_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventflow/eventflow/libs/go/activeevents"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// T0 duoc ghi khi chua co va khi organizer doi lich truoc gio mo; T0 da qua thi khong bao gio sua
// (lottery da chay theo no). Su kien khong co T0 (co dinh) khong bi ghi sale_start_ms.
func TestPublishActiveEvents_GhiTapVaT0(t *testing.T) {
	gate, rdb := setupTestGate(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	meta := func(id string) string { return fmt.Sprintf("wr:{%s}:meta", id) }
	t0 := func(id string) (int64, error) { return rdb.HGet(ctx, meta(id), "sale_start_ms").Int64() }

	fresh, future, started, fixed := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	past := now.Add(-time.Hour)
	rdb.HSet(ctx, meta(future), "sale_start_ms", now.Add(time.Hour).UnixMilli())
	rdb.HSet(ctx, meta(started), "sale_start_ms", past.UnixMilli())

	newT0 := now.Add(3 * time.Hour)
	err := gate.PublishActiveEvents(ctx, []port.SellableEvent{
		{ID: fresh, SaleStartAt: newT0},
		{ID: future, SaleStartAt: newT0},
		{ID: started, SaleStartAt: newT0},
		{ID: fixed},
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	for id, want := range map[string]int64{fresh: newT0.UnixMilli(), future: newT0.UnixMilli(), started: past.UnixMilli()} {
		if got, err := t0(id); err != nil || got != want {
			t.Errorf("sale_start_ms cua %s = %d (%v), muon %d", id, got, err, want)
		}
	}
	if n := rdb.Exists(ctx, meta(fixed)).Val(); n != 0 {
		t.Errorf("su kien khong co T0 bi ghi meta")
	}
	ids, err := activeevents.Read(ctx, rdb)
	want := []string{fresh, future, started, fixed}
	slices.Sort(want)
	if err != nil || !slices.Equal(ids, want) {
		t.Errorf("tap = %v (%v), muon %v", ids, err, want)
	}
}
