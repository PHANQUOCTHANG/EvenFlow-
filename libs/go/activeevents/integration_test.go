//go:build integration

package activeevents

import (
	"context"
	"slices"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/testx"
)

func TestTichHop_ReplaceThayTrongTapVaReadSapXep(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	if ids, err := Read(ctx, rdb); err != nil || len(ids) != 0 {
		t.Fatalf("key chua co: %v, %v", ids, err)
	}
	if err := Replace(ctx, rdb, []string{"b", "a"}); err != nil {
		t.Fatal(err)
	}
	if err := Replace(ctx, rdb, []string{"c", "a"}); err != nil {
		t.Fatal(err)
	}
	ids, err := Read(ctx, rdb)
	if err != nil || !slices.Equal(ids, []string{"a", "c"}) {
		t.Fatalf("Read = %v, %v; muon [a c] (b phai bi go)", ids, err)
	}
	if err := Replace(ctx, rdb, nil); err != nil {
		t.Fatal(err)
	}
	if n := rdb.Exists(ctx, Key).Val(); n != 0 {
		t.Fatalf("tap rong phai xoa key, con %d", n)
	}
}
