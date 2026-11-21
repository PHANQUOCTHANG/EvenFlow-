// Package redisstate is the Redis StateStore of the anti-bot engine: one EVALSHA of observe.lua per join
// (contracts 4, AC-6), on a dedicated client with a hard latency budget (D7).
package redisstate

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

//go:embed observe.lua
var observeSrc string

var observeScript = redis.NewScript(observeSrc)

// joinTimeWindowMs is the R1-server window and the jt list TTL (contracts 4: 300 s).
const joinTimeWindowMs = 300_000

// NewClient returns the dedicated antibot client of contracts 4: no retries, a large warm pool, a short dial
// and every timeout equal to the Redis budget, honoured through the request context.
func NewClient(addr string, cfg antibot.Config) *redis.Client {
	timeout := cfg.RedisTimeout
	if timeout <= 0 {
		timeout = 4 * time.Millisecond
	}
	return redis.NewClient(&redis.Options{
		Addr:                  addr,
		ReadTimeout:           timeout,
		WriteTimeout:          timeout,
		PoolTimeout:           timeout,
		ContextTimeoutEnabled: true,
		MaxRetries:            -1,
		PoolSize:              128,
		MinIdleConns:          32,
		DialTimeout:           50 * time.Millisecond,
	})
}

// Store implements antibot.StateStore.
type Store struct {
	rdb       *redis.Client
	r4WindowS string
	r7WindowS string
	share     string
	shareMin  string
	jtWindow  string
}

// New loads observe.lua on the server (so the first on-sale request does not hit NOSCRIPT) and returns the store.
func New(ctx context.Context, rdb *redis.Client, cfg antibot.Config) (*Store, error) {
	if rdb == nil {
		return nil, errors.New("redisstate: nil client")
	}
	if err := observeScript.Load(ctx, rdb).Err(); err != nil {
		return nil, fmt.Errorf("redisstate: load observe.lua: %w", err)
	}
	return NewLazy(rdb, cfg), nil
}

// NewLazy returns a store without loading the script first (used when Redis is down at startup: the first
// successful call after a NOSCRIPT falls back to EVAL, which caches the script).
func NewLazy(rdb *redis.Client, cfg antibot.Config) *Store {
	r := cfg.Rules
	if r.R4WindowS <= 0 {
		r = antibot.DefaultRules()
	}
	return &Store{
		rdb:       rdb,
		r4WindowS: strconv.FormatInt(int64(r.R4WindowS)*1000, 10),
		r7WindowS: strconv.FormatInt(int64(r.R7WindowS)*1000, 10),
		share:     strconv.FormatFloat(r.R4NShare, 'g', -1, 64),
		shareMin:  strconv.Itoa(r.R4NShareMinJoins),
		jtWindow:  strconv.Itoa(joinTimeWindowMs),
	}
}

func key(ev, suffix string) string { return "ab:{" + ev + "}:" + suffix }

// Keys returns every key observe.lua touches for one join, built from in.Now (plan review R3-1).
func Keys(in antibot.Input, fpHash string, net netip.Prefix, sq string) []string {
	ev, id := in.EventID, in.IdentityID
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	minute := now.UnixMilli() / 60_000
	p := ""
	if net.IsValid() {
		p = net.String()
	}
	keys := make([]string, 0, 16)
	keys = append(keys,
		"wr:{"+ev+"}:tok",
		key(ev, "blk:"+id),
		key(ev, "jt:"+id),
		key(ev, "fp:"+fpHash),
		key(ev, "net:"+p),
		key(ev, "sq:"+sq),
	)
	for m := minute - 4; m <= minute; m++ {
		keys = append(keys, key(ev, "jc:"+strconv.FormatInt(m, 10)))
	}
	for m := minute - 4; m <= minute; m++ {
		keys = append(keys, key(ev, "nj:"+p+":"+strconv.FormatInt(m, 10)))
	}
	return keys
}

func flag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// Observe runs observe.lua once (EVALSHA; EVAL only after a NOSCRIPT).
func (s *Store) Observe(ctx context.Context, in antibot.Input, fpHash string, net netip.Prefix, sq string) (antibot.State, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	keys := Keys(in, fpHash, net, sq)
	args := []any{in.IdentityID, strconv.FormatInt(now.UnixMilli(), 10), flag(fpHash != ""), flag(net.IsValid()),
		flag(sq != ""), s.r4WindowS, s.r7WindowS, s.share, s.shareMin, s.jtWindow}
	res, err := s.rdb.EvalSha(ctx, observeScript.Hash(), keys, args...).Slice()
	if err != nil && isNoScript(err) {
		res, err = s.rdb.Eval(ctx, observeSrc, keys, args...).Slice()
	}
	if err != nil {
		if isPoolTimeout(err) {
			return antibot.State{}, fmt.Errorf("%w: %w", antibot.ErrPoolExhausted, err)
		}
		return antibot.State{}, err
	}
	if len(res) < 6 {
		return antibot.State{}, fmt.Errorf("redisstate: observe.lua returned %d fields", len(res))
	}
	st := antibot.State{
		HasSlot:        toInt(res[0]) == 1,
		Sticky:         toInt(res[1]) == 1,
		FPIdentities:   int(toInt(res[2])),
		NetIdentities:  int(toInt(res[3])),
		NetSuppressed:  toInt(res[4]) == 1,
		SeqIdentities:  int(toInt(res[5])),
		CardIdentities: -1,
		Known:          true,
	}
	if len(res) > 6 {
		st.JoinGapsMs = make([]int64, 0, len(res)-6)
		for _, v := range res[6:] {
			st.JoinGapsMs = append(st.JoinGapsMs, toInt(v))
		}
	}
	return st, nil
}

// isPoolTimeout matches go-redis' internal pool.ErrPoolTimeout (not exported by the redis package in v9.7).
func isPoolTimeout(err error) bool { return err.Error() == "redis: connection pool timeout" }

func isNoScript(err error) bool { return strings.HasPrefix(err.Error(), "NOSCRIPT") }

func toInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case string:
		x, _ := strconv.ParseInt(n, 10, 64)
		return x
	}
	return 0
}
