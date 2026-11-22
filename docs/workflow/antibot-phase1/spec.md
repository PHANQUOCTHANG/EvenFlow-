# Spec: anti-bot phase 1 (antibot-phase1)

Tier: **High-risk** (`tier.json`). Branch `feature/antibot-phase1` from develop @ 918667c. Jira EV-173 (EVF-100),
EV-174 (EVF-101), EV-175 (EVF-102). Business source: docs/01-nghiep-vu.md 2.5 (R1-R7, BR-B1, BR-B2),
docs/05 FR-B1..FR-B3, docs/04 E8 success criteria. Wire shapes, keys and columns: `contracts.md`. Status: **draft,
awaiting human approval** (Q-1..Q-5 in section 7).

## 1. Problem

`POST /v1/events/{id}/queue/join` already has a `BotGuard` seam (`services/waitingroom/internal/adapter/http/handler.go:35`)
but it is wired to `nil` (`cmd/server/main.go`), and the web sends `signals: {}`
(`apps/web/src/components/queue/waiting-room.tsx:67`). Nothing scores, demotes or blocks a scripted buyer today.
The risk the epic names is the opposite failure too: a false positive blocks a real buyer in the on-sale minute.

## 2. Decisions (to approve)

- **D1 In-process library, not a network hop.** The engine is a new package `libs/go/antibot` (pure rules +
  scoring + config) with a Redis state adapter `libs/go/antibot/redisstate`, called in-process by waitingroom
  through the existing `BotGuard` seam. A separate `antibot-svc` call would add serialisation, a second pool and a
  new failure domain to a 5 ms budget, on the hottest endpoint. The architecture row "antibot | Redis" still holds
  (state lives in Redis under `ab:` keys). `services/antibot` stays a stub; EV-177 (appeal API) and EV-178 (rule
  proposals) can live there later, reading `bot_decisions` and the same library. docs/02 gets a one-line note.
- **D2 Signals travel in the join body** (`{"signals": {...}}`, schema v1 in contracts 1), collected by a small web
  module on the event page (where the buyer clicks "join") and carried to the waiting room in `sessionStorage`.
  Server-side inputs: identity, device id, `User-Agent`, client IP (first `X-Forwarded-For` element, which the
  gateway sets to the resolved client since gateway-trusted-proxy). Body capped at 4 KiB; the payload is untrusted.
- **D3 Score = noisy-OR of fired-rule weights**: `score = 1 - prod(1 - w_i)` over fired rules, in `[0,1]`,
  monotonic, explainable (each fired rule is listed). Weights, rule thresholds and tier cut-offs are config
  (defaults in contracts 3). Tiers follow docs/01: `<0.30` PASS, `0.30-<0.60` CHALLENGE, `0.60-<0.85` SLOW_LANE,
  `>=0.85` BLOCK (Q-5: docs say "> 0.85"; we use `>=` so the default weights for "no browser at all" land in BLOCK).
- **D4 CHALLENGE without Turnstile**: phase 1 has no challenge mechanism, so CHALLENGE applies a configured
  fallback `ANTIBOT_CHALLENGE_FALLBACK=pass|slow_lane`, default **pass** (recorded + counted). A 0.30-0.60 score is
  one weak signal; demoting it would put the false-positive risk on real buyers. EV-176 plugs in by returning
  403 `/problems/challenge-required` for this tier; the decision struct already carries the tier.
- **D5 Slow lane = fixed penalty, same response.** A slow-laned join gets the same response shape and state as
  a normal one (the bot is not told). After T0 its queue score is `1e12 + seq + P` (P = `ANTIBOT_SLOW_LANE_PENALTY`,
  default 100000 positions). Before T0 it goes to a separate lobby set; the T0 lottery shuffles that set too and
  places it after the first P post-T0 joiners. An existing slot is never moved by a later decision except BLOCK.
- **D6 BLOCK = 403 + revoke + sticky.** BLOCK returns 403 `/problems/request-denied` with a generic message (no rule
  list to the client), removes the identity's existing slot for that event (lobby, slow lobby, queue, admitted,
  token map) and sets a 24 h per-event block flag so retrying with better signals does not help. Unblock (EV-177)
  = delete that flag.
- **D7 Fail-soft, then fail-open.** Antibot never blocks a join because antibot itself failed. Redis state has a
  4 ms budget (dedicated client with context timeouts). Timeout/error: stateful rules (R1-server, R4, R7) are skipped
  and the decision uses stateless rules only, marked `DEGRADED`. A panic or config fault: PASS, counted. Both are
  metrics with an alert (contracts 6).
- **D8 Mode flag** `ANTIBOT_MODE=off|shadow|enforce`, default **shadow**. `off` = today's behaviour exactly (guard
  not wired). `shadow` = evaluate, persist, count, but every join proceeds as PASS. `enforce` = apply tiers.
  Rollout: ship web signals first, run shadow through at least one rehearsal on-sale, switch to enforce only when
  the shadow demotion rate on real traffic is under 1%.
- **D9 Persistence reuses `bot_decisions`** (migrations/0001_core.sql:211, already has `score`, `action`,
  `fired_rules`): **no migration**. Async batched writer from waitingroom's existing Postgres pool, never on the
  request path; full buffer = drop + metric. Written: every decision with score >= 0.30, plus a 1% sample of PASS.
  Mode and degradation go into `fired_rules` as tags `MODE_SHADOW`, `DEGRADED`, `STICKY` (Q-2).
- **D10 Privacy.** Device fingerprint is stored and used only as `HMAC-SHA256(ANTIBOT_HASH_KEY, fp)` truncated to
  32 hex chars; IP only as its network prefix (/24 IPv4, /64 IPv6). No raw signal, UA or full IP is stored or logged.

## 3. Rules (phase 1 definitions; thresholds are config, defaults in contracts 3)

| Rule | Fires when | Inputs |
|---|---|---|
| R1 machine rhythm | client inter-event intervals: >= 8 samples and coefficient of variation < 0.10; **or** server: >= 4 join inter-arrivals for this identity in 5 min with CV < 0.05 | `iv`; Redis join times |
| R2 too fast | render-to-click `rt` < 120 ms | `rt` |
| R3 no interaction | pointer + scroll + key counts all 0, or signals missing/invalid | `pe`,`se`,`ke` |
| R4 shared device / network | distinct identities on the same fp hash in 5 min > 3, **or** on the same network prefix in 5 min > 30 | fp, IP |
| R5 headless / abnormal client | any strong sign (UA denylist match, empty UA, `wd` true, signals missing/invalid) → `R5`; else >= 2 weak signs (`gl` false, `fc` < 3, `tz`/`tzo` disagree) → `R5W` (own, lower weight: privacy browsers trip these) | UA, `wd`,`gl`,`fc`,`tz`,`tzo` |
| R6 shared payment card | > 3 identities on one card hash | none at join in phase 1: **dormant** (Q-3) |
| R7 replayed sequence | the quantised interaction digest `sq` (>= 6 events) seen for >= 3 distinct identities in 15 min | `sq` |

The join click itself is recorded before the snapshot, so a real click or Enter always gives `pe` or `ke` >= 1 and R3
cannot fire for a browser buyer. R4-network threshold is deliberately higher than R4-device: campuses and mobile carriers put many real buyers on
one /24. A rule whose input is absent or out of range is "not evaluated" (never fires), except the missing-signals
case named in R3/R5. Rules are registered by ID in a table, so a future rule (EV-178) is config + code with weight 0
until a human approves it (BR-B2); AI never adds or enables a rule at runtime.

## 4. Acceptance criteria

**Signals (EV-173)**
- **AC-1** Given the event page is open, when the buyer clicks join, then the web stores a v1 signal snapshot for
  that event (`rt`, counts, last <= 32 intervals, `sq`, `fp`, `wd`, `gl`, `fc`, `tz`, `tzo`) in sessionStorage, and
  the waiting room sends it as `signals` on join (also on re-join). No snapshot → `signals: {"v":1}` (not `{}`).
- **AC-2** Given the collector, then listeners are passive, counts and buffers are capped (contracts 1), font/WebGL
  probes run once per page, and the serialised snapshot is <= 2 KiB; no raw keystrokes, coordinates or text are kept.
- **AC-3** Given a join body over 4 KiB, malformed JSON, a wrong type, or `v` != 1, then the request is not
  rejected for that reason; the signals count as missing (R3 + R5 strong) and `antibot_signals_invalid_total{reason}`
  increments. Unknown fields are ignored; each out-of-range field is treated as absent.

**Rule engine (EV-174)**
- **AC-4** Given the inputs in section 3, each rule R1-R7 fires exactly at its configured threshold (table tests at
  threshold, just below, just above, absent) and never panics on any input (fuzz on the signal decoder + engine).
- **AC-5** Given a warm Redis, then `Decide` (decode + state script + rules + score) has p99 < 5 ms over 10 000
  calls in the integration benchmark, and the pure engine (no I/O) p99 < 0.2 ms; `antibot_decide_seconds` records it.
- **AC-6** Given one join, then all Redis state for that decision is read and written in **one** script call under
  `ab:{ev}:` keys (one slot per event), with TTLs and caps from contracts 4; memory per identity is bounded.
- **AC-7** Given Redis is slower than 4 ms or errors, then the decision is computed from stateless rules only,
  tagged `DEGRADED`, `antibot_degraded_total{reason}` increments, and the join is never refused because of the
  failure. A panic inside antibot → PASS + `antibot_degraded_total{reason="panic"}`.

**Score and tiers (EV-175)**
- **AC-8** Given fired rules, then `score = 1 - prod(1 - w_i)` rounded to 3 decimals, and the tier follows the
  configured cut-offs (default 0.30 / 0.60 / 0.85, lower bound inclusive). Given no fired rule, score 0 and PASS.
- **AC-9** Given `enforce` and PASS, then the join behaves exactly as develop (response, state, rank).
- **AC-10** Given `enforce` and CHALLENGE, then the configured fallback applies (`pass` default, or `slow_lane`), and
  the decision is persisted as `CHALLENGE`.
- **AC-11** Given `enforce` and SLOW_LANE, then the join returns 200 with the same fields and state values as a
  PASS join; after T0 the slot is placed P positions later; before T0 it shows `LOBBY` and after the lottery is
  ranked behind all lottery winners and the first P post-T0 joiners. BR-Q1 (each lobby member equally likely per
  lane) and BR-Q3 (idempotent re-join returns the same token and rank) hold for both lanes.
- **AC-12** Given `enforce` and BLOCK, then 403 `/problems/request-denied`, no token is issued, any existing slot of
  that identity for the event is revoked (status of the old token → `UNKNOWN`), and later joins within 24 h are
  BLOCK with tag `STICKY` without re-evaluating rules.
- **AC-13** Given `shadow`, then every join proceeds as PASS while the computed decision is persisted and counted
  with `mode="shadow"`; given `off`, the guard is not wired and the join is byte-identical to develop.
- **AC-14** Given any decision with score >= 0.30 (and a 1% sample of PASS), then one `bot_decisions` row is written
  asynchronously with `fired_rules` listing every fired rule ID plus tags (BR-B1, FR-B3); the request never waits for
  Postgres; a full buffer drops the row and increments `antibot_decisions_dropped_total`. Every BLOCK row has a
  non-empty rule list (or `STICKY`).
- **AC-15** Given any stored row, Redis key or log line, then it contains no raw fingerprint, no full IP, no UA and no
  raw signal values (fp = 32-hex HMAC, IP = network prefix).

**Config, observability, quality bar**
- **AC-16** Given `ANTIBOT_MODE` != off and an invalid weight, threshold, tier order, mode, fallback or a missing /
  short `ANTIBOT_HASH_KEY` in production (`APP_ENV=production`), then waitingroom fails at startup naming the
  variable; in dev a missing key uses a fixed dev key with a warning (same pattern as `QUEUE_TOKEN_SECRET`).
- **AC-17** Given the metrics in contracts 6, then they are exported on waitingroom `/metrics`, and alerts exist for
  degraded rate > 1% over 5 min, decide p99 > 5 ms over 5 min, and BLOCK share > 20% of joins over 5 min.
- **AC-18** Given the offline corpus (plan 3: >= 1000 bot sessions in 5 families, >= 5000 synthetic human sessions
  incl. hard cases), then with default config >= 95% of bot sessions end in SLOW_LANE or BLOCK, < 1% of human sessions
  end in SLOW_LANE or BLOCK, and 100% of BLOCK results list >= 1 fired rule. The test prints the per-family and
  per-rule confusion table.
- **AC-19** Given the sample bot script run against the waitingroom handler + real Redis (integration), then >= 95% of
  its joins are SLOW_LANE or BLOCK in `enforce`, and a human-replay run from the corpus stays under 1% demoted.

## 5. Non-goals

Turnstile (EV-176), appeal/unblock UI and API (EV-177), AI rule proposals (EV-178), a payment-card writer for R6,
scoring of status/stream polls or hold/checkout requests, IP reputation/ASN feeds, changing the gateway.

## 6. Risks

| Risk | Mitigation |
|---|---|
| False positives in the on-sale minute | Shadow by default (D8), CHALLENGE falls back to pass (D4), weak R5 signs need 2, high R4-network threshold, corpus FP gate, BLOCK-share alert, `off` as instant rollback |
| Old cached web bundles send `{}` → R3 + R5 → BLOCK under enforce | Enforce only after the web change is deployed and shadow metrics are clean; `{}` is visible as `signals_invalid{reason="missing"}` |
| Signals are client-supplied, a capable bot forges them | Server-side R1, R4, R5-UA, R7 and sticky BLOCK remain; the 95% target is stated against the corpus families in plan 3, not adversarial bots (Q-1) |
| Lua changes break lottery fairness or idempotency | Existing waitingroom tests stay green; new chi-square fairness test per lane; idempotency tests per lane |
| Redis latency spike adds to join latency | 4 ms hard budget on a dedicated client, DEGRADED path, p99 alert |
| bot_decisions write load in the peak | Async batch, 1% PASS sample, bounded buffer with drop metric |
| k6 / demo script send `{"signals":{}}` | They run with `shadow`/`off`; documented in plan |

## 7. Open questions (human)

- **Q-1** Is "bot caught" = SLOW_LANE **or** BLOCK acceptable for the >= 95% criterion (slow lane is the docs'
  shadow-ban)? Default: yes. If BLOCK only is required, default weights must rise and the FP risk grows.
- **Q-2** Encode mode/degraded as tags inside `fired_rules` (no migration), or approve a small migration adding
  `mode`/`degraded` columns? Default: tags, no migration.
- **Q-3** R6 has no data source at join (no card data before payment). OK to ship it dormant (defined, tested, never
  fires) and wire payment-svc later? Default: yes.
- **Q-4** CHALLENGE fallback default `pass` (lowest FP) or `slow_lane` (stricter)? Default: `pass`.
- **Q-5** Tier boundaries lower-inclusive (`>= 0.85` = BLOCK) instead of the docs' `> 0.85`? Default: yes.

## Human approval

- Approver: TranDinhVo, in the Claude Code session on 2026-11-22.
- Spec approved as written, with the plan's defaults for the open questions:
  - Q-1: slow lane counts as "bot caught" for the ≥95% criterion.
  - Q-2: mode and degraded state are recorded as tags in `fired_rules`; no migration.
  - Q-3: R6 ships defined and tested but dormant; payment wiring later.
  - Q-4: CHALLENGE falls back to `pass` (configurable to `slow_lane`).
  - Q-5: tier boundaries are lower-inclusive (score ≥ 0.85 is BLOCK).

## Amendment 1 (draft, needs human approval)

Source: `plan-review.md` round 1 (FAIL). Sections 1-7 above stay as approved; where this section differs, it wins.

New and changed decisions:
- **D11 BLOCK needs a positive sign.** BLOCK is only reachable when at least one fired rule is a positive sign:
  R1, R2, R4D, R6, R7, or R5 from a non-missing strong sign (UA deny, empty UA, `wd`). Otherwise the tier is capped
  at SLOW_LANE (tag `CAPPED`). So missing signals alone (old cached bundle, direct link) cost a demotion, never a
  ban (P-1).
- **D12 Slot-aware decisions.** `observe.lua` also reads `HEXISTS wr:{ev}:tok identity`. If the identity already
  holds a slot, only a block-eligible BLOCK (D11) changes it; anything else returns the existing slot unchanged
  (BR-Q3) and the row gets tag `REJOIN`. R1-server records only joins made without a slot (P-1).
- **D13 R4 split** into `R4D` (same device fp, weight 0.60) and `R4N` (same network prefix, weight 0.25: a
  contributor, never a tier on its own). R4N is not evaluated for a prefix holding > 5% of the event's joins in the
  window (one LB/NAT IP in front of everyone); that is counted in `antibot_net_suppressed_total` (P-6).
- **D14 Discrete events only for rhythm and replay.** `iv` and `sq` use only pointerdown, keydown, touchstart and
  the first scroll of a burst (sampled pointermove/scroll only feed the counts). `sq` needs >= 12 events and >= 4
  distinct buckets and includes the `rt` bucket. R7 weight 0.50, so R7 alone is CHALLENGE (P-2).
- **D15 Lenient decoding.** Each signal field is decoded on its own; a wrong type makes only that field absent;
  finite numbers are rounded to the nearest integer. "Missing" means only: over-cap body, bad JSON, `signals` not an
  object, or bad/absent `v` (P-8).
- **D7 (reworded)** Config is validated at startup (AC-16); a runtime panic → PASS. Redis is a dedicated client
  without retries; after 20 degraded calls in 1 s a breaker skips Redis for 3 s (reason `breaker_open`) (P-9, P-11).
- **D6 (changed)** `revoke.lua` sets the 24 h block flag itself, in the same script; `join.lua` on the antibot
  path refuses a new slot while the flag exists. The flag is ignored in `off`/`shadow` (P-12).
- **D16 No auto-join without a snapshot.** If the waiting room has neither a queue token nor a signal snapshot
  (new tab, direct link, restarted browser), it shows a "Join queue" button and collects signals on that page
  instead of joining on mount. The 401 re-join keeps the stored snapshot (P-1).

Changed acceptance criteria:
- **AC-1** Snapshot is kept for re-joins (cleared only on leaving the event or checkout); without token and snapshot
  the waiting room shows a join button (D16). Reason: P-1.
- **AC-2** `iv`/`sq` from discrete events only, entropy floor for `sq` (D14). Reason: P-2.
- **AC-3** Per-field lenient decoding; numbers rounded; shared web/Go fixture decodes identically (D15). Reason: P-8.
- **AC-4** Rules are R1, R2, R3, R4D, R4N, R5, R5W, R6, R7. R4N prefix suppression (D13); `fc` is not evaluated
  for a mobile UA (Android/iOS); only valid time zones are cached. Reason: P-6, P-18, P-21.
- **AC-5** Adds a concurrent p99 (64 goroutines) < 5 ms and a load gate: `tests/load/opening-spike.js` against
  develop vs this branch in `shadow`: join p99 +<= 5 ms, Redis CPU on the event shard +<= 50%. Reason: P-9.
- **AC-7** Adds the breaker (D7). Reason: P-9.
- **AC-8** Tier is computed from the rounded score; BLOCK eligibility cap (D11). Reason: P-1, P-21.
- **AC-11** Shuffle places the slow lobby also when the main lobby is empty; post-T0 slow score
  `1e12 + seq + P + m` (m = slow lobby size, stored at shuffle), so every pre-T0 slow member stays ahead of every
  post-T0 slow joiner. Reason: P-5, P-13.
- **AC-12** BLOCK is atomic with revoke + flag (D6); with an existing slot, missing signals alone never revoke
  (D11, D12). Reason: P-1, P-12.
- **AC-14** Non-UUID event or identity ids are stored as NULL; STICKY rows store score 1.000; tags add `REJOIN`,
  `CAPPED`. Reason: P-14.
- **AC-16** Adds k8s: `ANTIBOT_HASH_KEY` from Secret `eventflow-auth`, `ANTIBOT_MODE=shadow` in app config, both
  overlays build and pass `deploy/k8s/policy`. Reason: P-3.
- **AC-18 / AC-19** The synthetic corpus is a **regression check**, not proof of a low FP rate. Generator
  parameters are fixed independently of the thresholds (sourced in `testdata/corpus/README.md`), humans sit on both
  sides of every threshold, a separate set of 50-100 sessions recorded by the team with the real collector must
  stay < 1% demoted, and a ±20% threshold sensitivity table is printed. The real FP gate is the shadow demotion
  rate on real traffic (D8), required before enforce. Reason: P-7.
- **AC-20 (new)** Enforce go/no-go: `ANTIBOT_HASH_KEY` exists in the Secret before apply; waitingroom and
  waitingroom-controller run the same image (new Lua scripts); shadow demotion rate < 1% over a rehearsal on-sale.
  Revoked slots are not restored by `off` or a revert (the buyer re-joins at the tail). Reason: P-3, P-5, P-19.

Not adopted: **P-17** (key R4D on the session device id). `devices.id` is unique per (identity, fingerprint)
(migrations/0001_core.sql:17-24), so two identities never share it and R4D could never fire. The body `fp` stays;
a bot that rotates it is still seen by R4N and R1. A signed fp hash from identity is a follow-up.

### Amendment 1 approval

- Approved by TranDinhVo in the Claude Code session on 2026-11-22, all items as written (including the "Join queue" button when no snapshot exists and the K8s `ANTIBOT_HASH_KEY` changes).

### Amendment 1, revision after plan review round 2 (approved)

- Approved by TranDinhVo in the Claude Code session on 2026-11-22.
- Item 3 is WITHDRAWN: no "Join queue" button. The waiting room keeps auto-joining; with no stored snapshot the client sends `{"v":1}`, which decodes as valid-but-empty and scores 0.40 (challenge → pass), or returns the existing slot (REJOIN). The 5 frozen web waiting-room tests stay unchanged. Trade-off recorded (N-3): a client sending `{"v":1}` with a browser UA passes.
- K8s TCR approved (N-5): add `ANTIBOT_HASH_KEY` for waitingroom to the policy contract §8 secret table, `check_policy.py`, `rules.md` and the regenerated fixtures; additions only, existing checks unchanged.

## Amendment 2 (approved after review and challenge, 2026-11-22)

- Approved by TranDinhVo in the Claude Code session on 2026-11-22.
- X-4: a strong automation sign (R5: webdriver, headless, scripted UA such as curl or python-requests) weighs 0.85, so it alone reaches BLOCK.
- X-5: generic HTTP-library user agents (okhttp, Go-http-client, axios, node-fetch) move from the R5 deny list to the weak list (R5W); they alone never block.
- X-6: R4D weight 0.55, so a shared device alone is CHALLENGE (pass); it demotes only with another sign.
- X-3: accepted limitation — a bot can forge `fp`/`sq` to evade R4D/R7; there is no stronger server-side device signal yet. Addressed by Turnstile (EV-176) in phase 2.

### Amendment 2, revision (approved 2026-11-22)

- Approved by TranDinhVo in the Claude Code session on 2026-11-22.
- X-6 is WITHDRAWN: R4D stays 0.60 (the 0.55 variant dropped bots caught to 91.0%, below the 95% gate).
- X-4 (R5 strong = 0.85) and X-5 (generic HTTP-library UAs → R5W) stay, conditional on the corpus still reaching ≥95% bots caught and <1% humans demoted; if not, stop and report.
- TCR-6 approved for exactly the frozen assertions that encode the old R5 weight and deny list (score_test.go / rules_test.go), as listed in test-change-request.md, updated for R4D = 0.60.
- R-6 accepted: one AC-18 policy exception (`OPTIONAL_SECRET_OK`) for the optional `ANTIBOT_HASH_KEY` ref; every other secret ref stays non-optional.
