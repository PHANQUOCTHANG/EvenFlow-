# Contracts (frozen once spec.md is approved) — antibot-phase1

Source: develop @ 918667c. Revision 2 follows plan-review round 1 and spec Amendment 1 (marked **(R2)**); revision 3
follows plan-review round 2 and the human's revision of Amendment 1 (marked **(R3)**). Any
change after approval goes through a change request in this folder.

## 1. Signal payload v1 (client → `POST /v1/events/{id}/queue/join`, body `{"signals": {...}}`)

Body cap 4 KiB (`http.MaxBytesReader` in the join handler). The handler extracts the raw `signals` value
(`json.RawMessage`) and passes it in `Input`; decoding happens inside `Guard.Decide` (R2, P-10).

**Decoding (R2, P-8, D15).** `signals` is read as `map[string]json.RawMessage` restricted to the known keys below;
each field is decoded on its own. A wrong type or out-of-range value makes **only that field** absent. Numbers:
any finite JSON number is rounded to the nearest integer before the range check (`123.4` → 123). Unknown keys are
ignored. The payload is **missing** only when: body over cap (`too_large`), invalid JSON (`bad_json`), `signals`
absent or not an object (`missing`), `v` absent or != 1 (`bad_version`). Missing fires R3 and R5 (missing kind).
`{"v":1}` with nothing else is valid but empty: R3 fires, R5 does not (no strong sign). **(R3, N-6)** R3 is the one
rule where absent inputs count: it fires when `pe`, `se` and `ke` are each absent or 0 (this is the "missing input"
exception of spec 3). Every other rule with an absent input is not evaluated.

| Field | Type | Range / cap | Meaning |
|---|---|---|---|
| `v` | int | `1` | schema version |
| `rt` | int | 0..600000 | ms from first render of the event page to the join click |
| `pe` | int | 0..10000 | pointer events: pointerdown, touchstart, pointermove sampled <= 1 per 100 ms; includes the join click |
| `se` | int | 0..10000 | scroll events (sampled <= 1 per 100 ms) |
| `ke` | int | 0..10000 | keydown count (no key values) |
| `iv` | int[] | <= 32 items, each 0..60000; a bad item drops the whole array | **(R2)** intervals in ms between consecutive **discrete** events only: pointerdown, keydown, touchstart, first scroll of a burst (no scroll for >= 500 ms before). Sampled pointermove/scroll never enter `iv` |
| `sq` | string | `^[0-9a-f]{16}$` | **(R2)** FNV-1a 64 over `rtb` + the last <= 32 discrete events as `kind:bucket` (bucket = interval / 50 ms, capped 40; `rtb` = `rt` / 250 ms, capped 40). Sent only when >= 12 discrete events and >= 4 distinct buckets |
| `sn` | int | 12..32 | discrete events folded into `sq` |
| `sb` | int | 4..41 | **(R2)** distinct buckets in `sq` |
| `fp` | string | `^[A-Za-z0-9-]{8,64}$` | device id from `getDeviceFingerprint()` (same value as at login) |
| `wd` | bool | | `navigator.webdriver === true` |
| `gl` | bool | | a WebGL context could be created |
| `fc` | int | 0..20 | fonts detected from a fixed list of 20 (canvas width probe) |
| `tz` | string | `^[A-Za-z_+\-/0-9]{1,64}$` | IANA zone from `Intl` |
| `tzo` | int | -840..840 | `-new Date().getTimezoneOffset()` |

Web storage **(R3: D16 withdrawn)**: sessionStorage `ef:ab:{eventId}`, written at the join click on the event page,
read for join **and every re-join**, removed only when the buyer leaves the event (checkout success, sold-out
screen, explicit leave). The waiting room keeps auto-joining on mount (develop behaviour); when `readSignals`
returns `null` it sends `{"signals":{"v":1}}` (valid but empty: R3 only, 0.40, CHALLENGE → pass; or `REJOIN` when a
slot exists). **Trade-off (N-3):** any client that sends `{"v":1}` with a browser UA passes, and adding `"pe":1`
scores 0; forged signals are out of scope (Q-1), server-side R1, R4D, R4N, R5-UA, R7 and sticky remain. Shared fixtures: `libs/go/antibot/testdata/signals/*.json`
are golden files produced by the web collector test and decoded by the Go test.

Server-side inputs: `identity_id`, `device_id` (principal), `User-Agent` (first 512 bytes), client IP = first
`X-Forwarded-For` element parsed with `net/netip` (absent/unparsable → R4N not evaluated), `now`.

## 2. Go API (`libs/go/antibot`)

```go
type RuleID string // R1 R2 R3 R4D R4N R5 R5W R6 R7 (R2: R4 split); future IDs (EV-178) start at weight 0
type Action string // PASS | CHALLENGE | SLOW_LANE | BLOCK  (DB: PASS -> 'ALLOW')
type Mode string   // off | shadow | enforce

type Input struct {
    EventID, IdentityID, DeviceID string
    UserAgent  string
    ClientIP   netip.Addr      // zero = unknown
    RawSignals json.RawMessage // nil = missing
    BodyErr    InvalidReason   // set by the handler for too_large / bad_json
    Now        time.Time
}
func DecodeSignals(raw []byte) (Signals, InvalidReason) // exported; called inside Decide; never panics
func (s Signals) Missing() bool                          // true for the four whole-payload reasons

type State struct {
    JoinGapsMs     []int64 // R1-server (only joins made without a slot)
    FPIdentities   int     // R4D, includes caller
    NetIdentities  int     // R4N
    NetSuppressed  bool    // R3: prefix joins / event joins over the last 5 minute buckets > R4N_SHARE
    SeqIdentities  int     // R7
    CardIdentities int     // R6, -1 = unknown (always in phase 1)
    HasSlot        bool    // R2: HEXISTS wr:{ev}:tok identity
    Sticky         bool    // ab:{ev}:blk:{identity} exists
    Known          bool    // false when degraded
}
type Decision struct {
    Action    Action   // tier from the rounded score, after the D11 cap
    Applied   Action   // what the handler does (mode, fallback, D12)
    Score     float64  // 3 decimals; 1.000 for STICKY
    Fired     []RuleID // sorted
    Tags      []string // MODE_SHADOW DEGRADED STICKY REJOIN CAPPED
    FPHash    string   // 32 hex, "" if no fp
    NetPrefix netip.Prefix
}
func Evaluate(cfg Config, in Input, sig Signals, st State) Decision // pure
type StateStore interface {
    Observe(ctx context.Context, in Input, fpHash string, net netip.Prefix, sq string) (State, error) // one EVALSHA
}
func (g *Guard) Decide(ctx context.Context, in Input) Decision // decode + observe + evaluate under one recover()
```

**Block eligibility (R2, D11):** `BLOCK` only if `Fired` contains R1, R2, R4D, R6, R7, or R5 from a strong
non-missing sign; else `Action = SLOW_LANE`, tag `CAPPED`. **Slot awareness (R2, D12):** `HasSlot` and
`Action != BLOCK` → `Applied = PASS` (existing slot returned unchanged), tag `REJOIN`. `Sticky` → BLOCK with only
`STICKY`, rules not evaluated.

Waitingroom **(R2, P-4)**: `QueueStore.Join(ctx, ev, id, token)` and `(*Queue).Join` keep their 4-argument
signatures. New optional interface, found by type assertion on the store:

```go
type LaneStore interface {
    JoinLane(ctx context.Context, eventID, identityID, newToken string, lane Lane, penalty int64) (JoinResult, error)
    Revoke(ctx context.Context, eventID, identityID string) (bool, error) // also sets the block flag
}
```

`BotGuard` becomes `Decide(ctx, antibot.Input) antibot.Decision` (no test implements the old `Check`).
`cmd/server.buildGuard` returns an untyped nil for `off` (P-15). Handler mapping of `Applied`:

| Applied | HTTP | Queue effect |
|---|---|---|
| PASS | 200, develop shape | `Join` (off/shadow) or `JoinLane(n)` (enforce) |
| SLOW_LANE | 200, identical shape | `JoinLane(s, P)`; store without `LaneStore` → `Join` + warning log |
| BLOCK | 403 `/problems/request-denied`, generic detail | `Revoke` (sets the flag); without `LaneStore` → 403 only |
| `JoinLane` returns state `BLOCKED` (flag set by a concurrent BLOCK) | 403 `/problems/request-denied` | none |

`/problems/challenge-required` stays reserved for EV-176.

## 3. Config (env of waitingroom; read once at startup)

| Env | Default | Validation |
|---|---|---|
| `ANTIBOT_MODE` | `shadow` | `off`/`shadow`/`enforce` |
| `ANTIBOT_HASH_KEY` | dev key + warning; required when `APP_ENV=production` and mode != off | base64 of >= 32 bytes |
| `ANTIBOT_WEIGHTS` **(R2)** | `R1=0.50,R2=0.40,R3=0.40,R4D=0.60,R4N=0.25,R5=0.80,R5W=0.45,R6=0.60,R7=0.50` | known IDs, each in [0,1); a partial list **merges** with the defaults |
| `ANTIBOT_TIERS` | `0.30,0.60,0.85` | strictly increasing in (0,1); compared to the rounded score, lower bound inclusive |
| `ANTIBOT_CHALLENGE_FALLBACK` | `pass` | `pass`/`slow_lane` |
| `ANTIBOT_SLOW_LANE_PENALTY` | `100000` | integer 1..1e9 (also read by the controller for `shuffle.lua`) |
| `ANTIBOT_RULES` **(R2)** | `R1_CLIENT_CV=0.10,R1_CLIENT_MIN=8,R1_SERVER_CV=0.05,R1_SERVER_MIN=4,R2_MS=120,R4D_MAX=3,R4N_MAX=30,R4N_SHARE=0.05,R4N_SHARE_MIN_JOINS=1000,R4_WINDOW_S=300,R5_FONTS_MIN=3,R6_MAX=3,R7_MIN_IDS=3,R7_MIN_EVENTS=12,R7_MIN_BUCKETS=4,R7_WINDOW_S=900` | known keys; counts/windows/ms positive integers, CV/share in (0,1); partial list merges |
| `ANTIBOT_REDIS_TIMEOUT_MS` | `4` | 1..50 |
| `ANTIBOT_BREAKER` **(R2)** | `20/1s,open=3s` | N 1..10000, durations 100 ms..60 s |
| `ANTIBOT_PASS_SAMPLE` | `0.01` | [0,1] |
| `ANTIBOT_UA_DENY` | built-in: `HeadlessChrome PhantomJS python-requests python-httpx aiohttp curl/ Wget Go-http-client okhttp node-fetch axios/ Scrapy Puppeteer Playwright Selenium` | comma list replaces built-in; case-insensitive substring |

R5 details **(R2)**: strong = UA deny, empty UA, `wd` true, or missing payload (the last one is **not** block
eligible). Weak = `gl` false, `fc` < `R5_FONTS_MIN` (not evaluated when the UA contains `Android`, `iPhone`,
`iPad` or `Mobile`), `tz`/`tzo` disagree (`time.LoadLocation`; only valid zones are cached, <= 600 entries).

Default outcomes **(R2, re-checked; each row asserted in `score_test.go`)**:

| Case | Fired | Score | Tier |
|---|---|---|---|
| Missing payload, browser UA (old bundle, new join) | R3, R5(missing) | 0.880 | SLOW_LANE `CAPPED` |
| Missing payload, identity already holds a slot | R3, R5(missing) | 0.880 | SLOW_LANE `CAPPED`; applied PASS `REJOIN` |
| Raw HTTP, `python-requests` UA | R3, R5 | 0.880 | BLOCK |
| Headless default (automation click, `wd` true, `rt` < 120) | R2, R5 | 0.880 | BLOCK |
| Stealth, fixed rhythm | R1, R2 | 0.700 | SLOW_LANE |
| `{"v":1}`, browser UA, new join (no snapshot) **(R3)** | R3 | 0.400 | CHALLENGE → pass |
| `{"v":1}`, identity already holds a slot **(R3)** | R3 | 0.400 | CHALLENGE; applied PASS `REJOIN` |
| Account farm (shared fp + shared /24) | R4D, R4N | 0.700 | SLOW_LANE |
| Shared fp only | R4D | 0.600 | SLOW_LANE |
| Replay from a small proxy pool | R7, R4N | 0.625 | SLOW_LANE |
| Replay from distinct networks | R7 | 0.500 | CHALLENGE → pass (needs EV-176) |
| Human on CGNAT /24 | R4N | 0.250 | PASS |
| 40 humans on one carrier/campus /24 within 5 min (R4N fires: 40 > 30), each human | R4N | 0.250 | PASS |
| same cluster, one human with a fast click | R4N, R2 | 0.550 | CHALLENGE → pass |
| same cluster, one human on a privacy browser | R4N, R5W | 0.588 | CHALLENGE → pass |
| same cluster, fast click + privacy browser (rare) | R4N, R2, R5W | ~0.75 (0.7525 before rounding) | SLOW_LANE |
| Human, one weak sign (R2, R5W, R1 or R7) | one | 0.400-0.500 | CHALLENGE → pass |
| Human on CGNAT + privacy browser | R4N, R5W | 0.588 | CHALLENGE → pass |
| Human, fast click + privacy browser | R2, R5W | 0.670 | SLOW_LANE |
| Human, R1 + R7 (now rare, D14) | R1, R7 | 0.750 | SLOW_LANE (was BLOCK in R1) |

## 4. Redis

Antibot client **(R2, P-9)**: own `redis.Client`, same `REDIS_ADDR`; `ReadTimeout`/`WriteTimeout` and
`PoolTimeout` = `ANTIBOT_REDIS_TIMEOUT_MS`, `ContextTimeoutEnabled: true`, `MaxRetries: -1`, `PoolSize: 128`,
`MinIdleConns: 32`, `DialTimeout: 50ms`. Keys use hash tag `{ev}`, the same slot as `wr:{ev}:`.

| Key | Type | Write | Cap / TTL |
|---|---|---|---|
| `ab:{ev}:jt:{identity}` | LIST join ms | `LPUSH` + `LTRIM 0 8`, only when no slot | 9 items, TTL 300 s |
| `ab:{ev}:fp:{fphash}` | ZSET identity → ms | `ZADD`, `ZREMRANGEBYSCORE`; `ZREMRANGEBYRANK` only when `ZCARD` > 64 | TTL 2 × R4 window |
| `ab:{ev}:net:{prefix}` | ZSET identity → ms | same | same |
| `ab:{ev}:jc:{minute}` **(R2)** | STRING event join count | `INCR` per join | TTL 360 s |
| `ab:{ev}:nj:{prefix}:{minute}` **(R3, N-2)** | STRING prefix join count | `INCR` per join with a known IP | TTL 360 s |
| `ab:{ev}:sq:{sq}` | ZSET identity → ms | same as fp | TTL 2 × R7 window |
| `ab:{ev}:blk:{identity}` | STRING `1` | **(R2)** set only by `revoke.lua` | `EX 86400` |

Share **(R3, N-2)**: `share = Σ nj(prefix, last 5 minutes) / Σ jc(last 5 minutes)`, both in joins (same unit);
`NetSuppressed` when `Σ jc >= R4N_SHARE_MIN_JOINS` and `share > R4N_SHARE`. The `net` ZSET is used only for the
distinct-identity count (R4N_MAX), so its 64-member cap no longer bounds the share. A farm holding > 5% of an
event's joins from one /24 also suppresses R4N for itself; accepted (R4D, R1 remain; R4N is 0.25).

`observe.lua` (R2): reads `HEXISTS wr:{ev}:tok identity` and `EXISTS ab:{ev}:blk:{identity}` (returns early when
sticky); skips absent inputs; returns `{hasSlot, sticky, fpCount, netCount, netShareMilli, seqCount, gaps...}`.
About 15-19 commands per call (R3 adds one `INCR` on the prefix bucket and two `MGET`s over the 5 prefix and 5 event
buckets).

Waitingroom scripts:

| Script | Change |
|---|---|
| `join.lua` | **(R2)** `ARGV[5]` lane and `ARGV[6]` penalty are optional: absent = lane `n`, no flag check (4-arg `Join` unchanged). With `ARGV[5]`: existing slot returned first (BR-Q3, also checks `lobby_slow`); else if `ab:{ev}:blk:{identity}` exists → `{'BLOCKED','',-1,0}`; slow pre-T0 → `SADD lobby_slow`, state `LOBBY`; slow post-T0 score `1e12 + seq + P + m`, m = `HGET meta slow_lottery_size` (0 before the shuffle) |
| `shuffle.lua` | **(R2)** main lottery as today when n > 0; then, in **both** branches (also n == 0), Fisher-Yates over `lobby_slow`, scores `1e12 + P + i` (i = 1..m), `HSET meta slow_lottery_size m`, `DEL lobby_slow`; `lottery_size` = main lane only. P = optional `ARGV[3]` (default 100000) |
| `status.lua` | `SISMEMBER lobby_slow` → `LOBBY` |
| `revoke.lua` (new) | **(R2)** `HGET tok identity` → token; `SREM lobby`, `SREM lobby_slow`, `ZREM queue`, `ZREM admitted`, `HDEL tok`, `HDEL sess token`; `SET ab:{ev}:blk:{identity} 1 EX 86400`. Returns 1 if a slot was removed |

## 5. `bot_decisions` row (existing table, migrations/0001_core.sql:211; no migration)

| Column | Value |
|---|---|
| `event_id`, `identity_id` | UUID; **(R2)** non-UUID → NULL (counted in `antibot_decisions_nonuuid_total`) |
| `device_fp` | `Decision.FPHash` or NULL |
| `ip` | network prefix as INET with mask (`203.0.113.0/24`, `2001:db8:1:2::/64`), or NULL |
| `score` | `Decision.Score` (STICKY: 1.000) |
| `action` | tier `Action` (PASS → `ALLOW`) |
| `fired_rules` | sorted rule IDs + tags, e.g. `{R3,R5,CAPPED}`, `{R3,R5,CAPPED,REJOIN}`, `{STICKY}`, `{R4N,MODE_SHADOW}` |

Written: score >= 0.30, plus a `ANTIBOT_PASS_SAMPLE` sample of PASS. Writer **(R2, P-14)**: own pool from a new
`postgresadapter.NewPoolFromEnv` (`DATABASE_URL`, `MaxConns 2`, `QueryExecModeSimpleProtocol` for PgBouncer);
`NewAuditorFromEnv` is unchanged. Buffered channel 10 000; flush every 200 ms or 500 rows as one multi-row
`INSERT`; a failed batch is dropped and counted; the goroutine recovers panics; shutdown flush 2 s.

## 6. Metrics and alerts

| Metric | Type | Labels |
|---|---|---|
| `antibot_decisions_total` | counter | `action`, `applied`, `mode` |
| `antibot_rule_fired_total` | counter | `rule` |
| `antibot_decide_seconds` | histogram 0.25 ms .. 10 ms | — |
| `antibot_degraded_total` | counter | `reason`: `redis_timeout` `redis_error` `breaker_open` `panic` |
| `antibot_signals_invalid_total` | counter | `reason` |
| `antibot_decisions_dropped_total` | counter | — |
| `antibot_decisions_nonuuid_total` **(R2)** | counter | — |
| `antibot_slot_revoked_total` | counter | — |
| `antibot_net_suppressed_total` **(R2)** | counter | — |
| `antibot_capped_total` **(R2)** | counter | — |

No `event`/`identity` labels. Alerts (new cases only appended to the existing `alerts_test.yml`): `AntibotDegraded`
(> 1% for 5 m), `AntibotSlow` (p99 > 5 ms for 5 m), `AntibotBlockSpike` (applied BLOCK > 20% for 5 m), **(R2)**
`AntibotNetSuppressed` (any suppression for 5 m: likely a proxy missing from `TRUSTED_PROXY_CIDRS`).

## 7. Deploy (R2, P-3)

| File | Change |
|---|---|
| `deploy/k8s/base/waitingroom/deployment.yaml` | env `ANTIBOT_HASH_KEY` from `secretKeyRef {name: eventflow-auth, key: ANTIBOT_HASH_KEY}`; `ANTIBOT_MODE`, `ANTIBOT_SLOW_LANE_PENALTY` from `configMapKeyRef eventflow-app-config` |
| `deploy/k8s/base/waitingroom-controller/deployment.yaml` | `ANTIBOT_SLOW_LANE_PENALTY` from app config (no hash key) |
| `deploy/k8s/overlays/{local,staging}/app-config.env` | `ANTIBOT_MODE=shadow`, `ANTIBOT_SLOW_LANE_PENALTY=100000` |
| `deploy/k8s/overlays/local/secrets.dev.env` + `kustomization.yaml` | `ANTIBOT_HASH_KEY` dev value (base64 of 32 bytes containing "dev") + a replacement into `eventflow-auth` |
| `deploy/k8s/overlays/staging/kustomization.yaml` | header comment: `eventflow-auth JWT_SECRET, QUEUE_TOKEN_SECRET, ANTIBOT_HASH_KEY` |
| `docs/workflow/k8s-all/policy-contract.md` §8 **(R3, N-5, TCR approved)** | waitingroom row gains `ANTIBOT_HASH_KEY` (`eventflow-auth`); `eventflow-auth` key set gains it. Additions only |
| `deploy/k8s/policy/check_policy.py` **(R3)** | line 86 `waitingroom` map gains `"ANTIBOT_HASH_KEY": ("eventflow-auth", "ANTIBOT_HASH_KEY")`; line 116 `eventflow-auth` set gains `ANTIBOT_HASH_KEY`. Existing checks unchanged |
| `deploy/k8s/policy/rules.md` **(R3)** | same additions in the secret table |
| `deploy/k8s/policy/testdata/gen_fixtures.py` + regenerated `good-local.yaml`, `good-staging.yaml` **(R3)** | regenerate; `gen_fixtures.py --check` clean; every `bad/*.yaml` still trips exactly its own rule. No `ANTIBOT_*` key is on the AC-32 forbidden list |
| `deploy/compose/docker-compose.yml` | waitingroom `ANTIBOT_MODE: shadow`; waitingroom + controller `ANTIBOT_SLOW_LANE_PENALTY` |

## 8. Test-facing names and semantics (R2, answers to the test designer)

1. **Clock.** `observe.lua` uses `Input.Now` passed as `ARGV` (ms), never Redis `TIME`, same as `join.lua`
   (`now_ms` from Go). Tests control time through `Input.Now`.
2. **Corpus session outcome.** A session is one or more joins of one identity. Outcome = `BLOCK` if any join's
   `Applied` is BLOCK (enforce semantics); otherwise the `Applied` lane of the **first** join (later joins return the
   existing slot, D12). "Demoted" = SLOW_LANE or BLOCK. Bots "caught" = demoted (approved Q-1).
3. **Carrier-NAT / campus clusters.** ~40 humans on one /24 within 5 min fire only R4N (0.25) → PASS; with one weak
   sign they stay CHALLENGE → pass (rows in section 3). Only R4N + two other signs reach SLOW_LANE.
4. **Invalid-signal metric.** `Guard.Decide` increments `antibot_signals_invalid_total{reason}` after it decodes
   (`BodyErr` from the handler counts with its own reason; the handler does not touch the metric).
   `DecodeSignals([]byte) (Signals, InvalidReason)` and `Signals.Missing()` stay exported.
5. **Names (adopted as proposed).**

| Name | Signature |
|---|---|
| `antibot.LoadConfig` | `func(getenv func(string) string) (Config, error)`; `Config.Mode antibot.Mode`, `Config.DevKey bool` |
| `antibot.NewMetrics` | `func(prometheus.Registerer) *Metrics` |
| `antibot.NewGuard` | `func(cfg Config, store StateStore, sink Sink, m *Metrics) *Guard` |
| `antibot.Sink` | `interface{ Offer(Input, Decision) }` (non-blocking) |
| `antibot.FPHash` | `func(key []byte, fp string) string` (32 hex) |
| `antibot.NetPrefix` | `func(netip.Addr) netip.Prefix` (/24 or /64 after `Unmap`; zero for invalid) |
| `domain.Lane` (waitingroom) | `LaneNormal`, `LaneSlow` (Lua values `n` / `s`) |
| `(*Queue).JoinLane` | `(ctx, eventID, identityID, newToken string, lane domain.Lane, penalty int64) (JoinResult, error)` |
| `(*Queue).Revoke` | `(ctx, eventID, identityID string) (bool, error)` |
| `httpadapter.LaneStore` | optional interface with `JoinLane` (returning `httpadapter.JoinResult`) and `Revoke` |
| web `src/lib/antibot-signals.ts` | `startSignalCollector(eventId): () => void` (cleanup), `snapshotSignals(eventId)`, `readSignals(eventId)` (`null` when absent or corrupt; the waiting room then sends `{v:1}`), `clearSignals(eventId)`; key `ef:ab:{eventId}` |
