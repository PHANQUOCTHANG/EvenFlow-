# Contracts (frozen once spec.md is approved) — antibot-phase1

Source: develop @ 918667c. Any change after approval goes through a change request in this folder.

## 1. Signal payload v1 (client → `POST /v1/events/{id}/queue/join`, body `{"signals": {...}}`)

Body cap 4 KiB (`http.MaxBytesReader` in the join handler). Decoded into a typed struct, never `map[string]any`.
Unknown fields ignored. A field outside its range is treated as absent. Whole payload invalid (over cap, bad JSON,
`signals` not an object, `v` missing or != 1, or only `{"v":1}`) = **missing** → R3 and R5 (strong) fire.
Reasons for `antibot_signals_invalid_total{reason}`: `too_large`, `bad_json`, `bad_version`, `missing`.

| Field | Type | Range / cap | Meaning |
|---|---|---|---|
| `v` | int | `1` | schema version |
| `rt` | int | 0..600000 | ms from event-page first render (component mount, `performance.now()`) to the join click |
| `pe` | int | 0..10000 | pointer events (pointerdown/up, touchstart, pointermove sampled at most 1 per 100 ms), including the join click |
| `se` | int | 0..10000 | scroll events (sampled at most 1 per 100 ms) |
| `ke` | int | 0..10000 | keydown events (count only, no key values) |
| `iv` | int[] | <= 32 items, each 0..60000 | last inter-event intervals in ms (all event kinds above, after sampling) |
| `sq` | string | `^[0-9a-f]{16}$` | FNV-1a 64 digest of the last <= 32 events as `kind:bucket` (bucket = interval / 50 ms, capped 40); sent only when >= 6 events |
| `sn` | int | 0..32 | number of events folded into `sq` |
| `fp` | string | `^[A-Za-z0-9-]{8,64}$` | device id from `getDeviceFingerprint()` (`apps/web/src/lib/device.ts`), same value sent at login |
| `wd` | bool | | `navigator.webdriver === true` |
| `gl` | bool | | a WebGL context could be created |
| `fc` | int | 0..20 | how many of a fixed list of 20 common fonts are detected (canvas width probe) |
| `tz` | string | `^[A-Za-z_+\-/0-9]{1,64}$` | `Intl.DateTimeFormat().resolvedOptions().timeZone` |
| `tzo` | int | -840..840 | `-new Date().getTimezoneOffset()` (minutes east of UTC) |

Web: sessionStorage key `ef:ab:{eventId}`, JSON of the object above, written at the join click on the event page,
read by the waiting room for join and re-join, removed when the queue token is cleared. Serialised size <= 2 KiB.
The `{"signals":{}}` sent by `deploy/scripts/demo.sh` and `tests/load/opening-spike.js` = missing.

Server-side inputs (not from the body): `identity_id` and `device_id` from `authx.PrincipalFromHeaders`,
`User-Agent` header (first 512 bytes), client IP = first element of `X-Forwarded-For` parsed with `net/netip`
(absent/unparsable → R4-network not evaluated), `now`.

## 2. Go API (`libs/go/antibot`)

```go
type RuleID string // "R1".."R7", "R5W"; future rules add IDs (EV-178), weight 0 until approved
type Action string // "PASS" | "CHALLENGE" | "SLOW_LANE" | "BLOCK"  (DB value for PASS is 'ALLOW')
type Mode string   // "off" | "shadow" | "enforce"

type Signals struct { /* typed fields of section 1, each with a present flag */ }
func DecodeSignals(body []byte) (Signals, InvalidReason) // never panics; "" = valid

type Input struct {
    EventID, IdentityID, DeviceID string
    UserAgent string
    ClientIP  netip.Addr // zero = unknown
    Signals   Signals
    Now       time.Time
}
type State struct { // returned by the state store; zero value = unknown
    JoinGapsMs      []int64 // R1-server
    FPIdentities    int     // R4 (distinct identities, includes caller)
    NetIdentities   int     // R4
    SeqIdentities   int     // R7
    CardIdentities  int     // R6, -1 = unknown (always in phase 1)
    Sticky          bool    // block flag set
    Known           bool    // false when degraded
}
type Decision struct {
    Action   Action    // tier from score (before mode/fallback)
    Applied  Action    // what the handler does: PASS in shadow; fallback for CHALLENGE
    Score    float64   // 0..1, 3 decimals
    Fired    []RuleID  // sorted
    Tags     []string  // "MODE_SHADOW", "DEGRADED", "STICKY"
    FPHash   string    // 32 hex, "" if no fp
    NetPrefix netip.Prefix // /24 or /64, zero if unknown
}

func Evaluate(cfg Config, in Input, st State) Decision     // pure, no I/O, allocation-light
type StateStore interface {
    Observe(ctx context.Context, in Input, fpHash string, net netip.Prefix) (State, error) // one script call
    MarkBlocked(ctx context.Context, eventID, identityID string) error
}
type Guard struct{ /* cfg, store, sink, metrics */ }
func (g *Guard) Decide(ctx context.Context, in Input) Decision // never returns error; D7 inside
```

Waitingroom `BotGuard` becomes `Decide(ctx, antibot.Input) antibot.Decision` (old `Check` removed; it had no caller
besides the handler and a nil wiring). The handler maps `Applied`:

| Applied | HTTP | Queue effect |
|---|---|---|
| PASS | 200, develop shape | `join.lua` lane `n` |
| SLOW_LANE | 200, identical shape | `join.lua` lane `s`, penalty P |
| BLOCK | 403 `{"type":"/problems/request-denied","title":...,"detail":"yeu cau bi tu choi"}` | `revoke.lua` + `MarkBlocked` |
| CHALLENGE (phase 1) | never applied: replaced by fallback | — |

`ProblemRequestDenied = "/problems/request-denied"`; future `ProblemChallengeRequired = "/problems/challenge-required"`
is reserved, not emitted. The web keeps its generic 403 message; no rule detail reaches the client.

## 3. Config (env of waitingroom; read once at startup)

| Env | Default | Validation |
|---|---|---|
| `ANTIBOT_MODE` | `shadow` | `off`/`shadow`/`enforce` |
| `ANTIBOT_HASH_KEY` | dev key + warning; required in `APP_ENV=production` | base64 of >= 32 bytes |
| `ANTIBOT_WEIGHTS` | `R1=0.50,R2=0.40,R3=0.40,R4=0.60,R5=0.80,R5W=0.45,R6=0.60,R7=0.70` | known IDs, each in [0,1) |
| `ANTIBOT_TIERS` | `0.30,0.60,0.85` | strictly increasing, in (0,1) |
| `ANTIBOT_CHALLENGE_FALLBACK` | `pass` | `pass`/`slow_lane` |
| `ANTIBOT_SLOW_LANE_PENALTY` | `100000` | int 1..1e9 |
| `ANTIBOT_RULES` | `R1_CLIENT_CV=0.10,R1_CLIENT_MIN=8,R1_SERVER_CV=0.05,R1_SERVER_MIN=4,R2_MS=120,R4_FP_MAX=3,R4_NET_MAX=30,R4_WINDOW_S=300,R5_FONTS_MIN=3,R6_MAX=3,R7_MIN_IDS=3,R7_MIN_EVENTS=6,R7_WINDOW_S=900` | known keys, numeric, positive |
| `ANTIBOT_REDIS_TIMEOUT_MS` | `4` | 1..50 |
| `ANTIBOT_PASS_SAMPLE` | `0.01` | [0,1] |
| `ANTIBOT_UA_DENY` | built-in list: `HeadlessChrome`, `PhantomJS`, `python-requests`, `python-httpx`, `aiohttp`, `curl/`, `Wget`, `Go-http-client`, `okhttp`, `node-fetch`, `axios/`, `Scrapy`, `Puppeteer`, `Playwright`, `Selenium` | comma list replaces built-in; case-insensitive substring |

Expected default outcomes (checked by the corpus test): no signals = R3+R5 = 0.88 BLOCK; headless default
(R2+R3+R5) = 0.93 BLOCK; fixed-rhythm stealth (R1+R2) = 0.70 SLOW_LANE; account farm (R4) = 0.60 SLOW_LANE;
replay (R7) = 0.70 SLOW_LANE; one weak human signal (R2, R1 or R5W alone) <= 0.50 CHALLENGE → pass.

## 4. Redis keys

Antibot (dedicated go-redis client, same `REDIS_ADDR`, `ReadTimeout`/`WriteTimeout` = `ANTIBOT_REDIS_TIMEOUT_MS`,
`ContextTimeoutEnabled: true`). All keys share the hash tag `{ev}` so `observe.lua` touches one slot.

| Key | Type | Write | Cap / TTL |
|---|---|---|---|
| `ab:{ev}:jt:{identity}` | LIST of join ms | `LPUSH` + `LTRIM 0 8` | 9 items, TTL 300 s |
| `ab:{ev}:fp:{fphash}` | ZSET identity → last ms | `ZADD`, `ZREMRANGEBYSCORE < now-window`, keep newest 64 | TTL 2 × R4 window |
| `ab:{ev}:net:{prefix}` | ZSET identity → last ms | same | same |
| `ab:{ev}:sq:{sq}` | ZSET identity → last ms | same, R7 window | TTL 2 × R7 window |
| `ab:{ev}:blk:{identity}` | STRING `1` | `SET EX 86400` on BLOCK (enforce only) | 24 h |

`observe.lua` returns `{gaps..., fpCount, netCount, seqCount, sticky}`; keys absent from the input are skipped.
`prefix` is the canonical prefix string with `/` (e.g. `203.0.113.0/24`).

Waitingroom (existing `wr:{ev}:` space):

| Key / script | Change |
|---|---|
| `wr:{ev}:lobby_slow` SET (new) | slow-lane members before T0; TTL as lobby |
| `join.lua` | `ARGV[5]` lane `n`/`s`, `ARGV[6]` penalty P. Idempotency check also looks in `lobby_slow`. Post-T0 slow score `1e12 + seq + P` |
| `shuffle.lua` | after the main lottery, Fisher-Yates over `lobby_slow`, score `1e12 + P + i`, then `DEL`; `lottery_size` unchanged (main lane only) |
| `status.lua` | `SISMEMBER lobby_slow` → `LOBBY` |
| `revoke.lua` (new) | `HGET tok identity` → token; `SREM lobby`, `SREM lobby_slow`, `ZREM queue`, `ZREM admitted`, `HDEL tok identity`, `HDEL sess token`. Returns 1 if a slot was removed |

## 5. `bot_decisions` row (existing table, migrations/0001_core.sql:211; no migration)

| Column | Value |
|---|---|
| `event_id` | event UUID; NULL if not a UUID |
| `identity_id` | identity UUID |
| `device_fp` | `Decision.FPHash` (32 hex HMAC) or NULL |
| `ip` | network prefix as INET with mask (`203.0.113.0/24`, `2001:db8:1:2::/64`) or NULL |
| `score` | `Decision.Score` |
| `action` | tier `Action` (PASS stored as `ALLOW`), not `Applied` |
| `fired_rules` | sorted rule IDs + tags, e.g. `{R2,R3,R5}`, `{R4,MODE_SHADOW}`, `{STICKY}`, `{R3,R5,DEGRADED}` |
| `created_at` | default now() |

Writer: buffered channel 10 000 rows, flush every 200 ms or 500 rows with `pgx.CopyFrom`; on shutdown flush with
2 s budget. Disabled (count as dropped) when `DATABASE_URL` is absent. Appeal (EV-177) reads by
`(event_id, identity_id, created_at)`.

## 6. Metrics (waitingroom `/metrics`) and alerts

| Metric | Type | Labels |
|---|---|---|
| `antibot_decisions_total` | counter | `action` (tier), `applied`, `mode` |
| `antibot_rule_fired_total` | counter | `rule` |
| `antibot_decide_seconds` | histogram, buckets 0.25 ms .. 10 ms | — |
| `antibot_degraded_total` | counter | `reason` = `redis_timeout`/`redis_error`/`panic` |
| `antibot_signals_invalid_total` | counter | `reason` |
| `antibot_decisions_dropped_total` | counter | — |
| `antibot_slot_revoked_total` | counter | — |

No `event`/`identity` labels (cardinality). Alerts in `deploy/observability/alerts.yml` + `alerts_test.yml`:
`AntibotDegraded` (degraded / decisions > 1% for 5 m), `AntibotSlow` (p99 > 5 ms for 5 m), `AntibotBlockSpike`
(applied BLOCK / decisions > 20% for 5 m).
