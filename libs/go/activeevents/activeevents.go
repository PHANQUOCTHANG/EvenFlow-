// Package activeevents la hop dong "su kien nao dang/sap mo ban" giua ticketing va waitingroom.
//
// ticketing-worker (chu cua bang events/ton kho) quet Postgres va ghi tap su kien vao Redis; admit
// controller cua phong cho doc tap nay de tu dieu khien su kien moi ma khong can sua bien moi truong
// hay khoi dong lai. ACTIVE_EVENT_IDS van duoc cong them (smoke, k6, moi truong khong co Postgres).
package activeevents

import (
	"context"
	"fmt"
	"slices"

	"github.com/redis/go-redis/v9"
)

// Key la SET cac event_id dang/sap mo ban. Khong co hash tag: key don, khong di cung key cua su kien.
const Key = "wr:active_events"

// Read tra tap su kien trong Redis, da sap xep. Key chua co thi tra rong.
func Read(ctx context.Context, rdb redis.Cmdable) ([]string, error) {
	ids, err := rdb.SMembers(ctx, Key).Result()
	if err != nil {
		return nil, fmt.Errorf("doc %s: %w", Key, err)
	}
	slices.Sort(ids)
	return ids, nil
}

// Replace thay toan bo tap bang ids trong MOT giao dich (MULTI): nguoi doc khong bao gio thay tap
// rong giua chung. ids rong thi xoa key.
func Replace(ctx context.Context, rdb redis.Cmdable, ids []string) error {
	pipe := rdb.TxPipeline()
	pipe.Del(ctx, Key)
	if len(ids) > 0 {
		members := make([]any, len(ids))
		for i, id := range ids {
			members[i] = id
		}
		pipe.SAdd(ctx, Key, members...)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("ghi %s: %w", Key, err)
	}
	return nil
}

// Union gop cac danh sach, bo rong va trung, sap xep.
func Union(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		for _, id := range l {
			if id != "" {
				out = append(out, id)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
