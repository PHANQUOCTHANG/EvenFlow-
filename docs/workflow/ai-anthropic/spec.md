# Spec: switch ai-worker text generation from Gemini to Anthropic Claude (ai-anthropic)

Tier: **High-risk** (`tier.json`: new dependency, provider swap, secrets/env, limiter accounting, BR-A7 boundary).
Branch `feature/ai-anthropic` from develop @ c5fb598. Scope: `services/ai-worker` + compose/env/alerts/README.
No migration. No change to queues, topology, limiter Lua logic, degradation modes or the assistant HTTP API.

## 1. Context (state at c5fb598)

- `app/gemini/client.py` `GeminiClient` does three things: `generate()` (chat + moderation, via
  `google.genai` `aio.models.generate_content`), `embed()` (semantic cache L3, `text-embedding-004`) and
  `scan_moderation()` (prompt + `json.loads` + NEEDS_HUMAN fallback). Mock mode (`GEMINI_MOCK=1`, default) does
  no network and needs no key. The SDK is injected via `sdk=` for tests (`tests/test_gemini_client.py`).
- Every real call goes through `app/gemini/guarded.py` (`guarded_generate`): limiter permit
  (`estimate_tokens(prompt, max_output_tokens)`) -> call -> refund to the actual token count; `total_tokens=None`
  means "unknown, do not refund"; `RateLimited` (429) refunds everything.
- Chat (`consumers/chat.py`): `PermitDenied`/`RateLimited` -> `Retry`; any other exception (incl. `TimeoutError`)
  -> FAQ fallback, nothing cached. Moderation: limiter/429/exhausted -> NEEDS_HUMAN via `send_needs_human`.
- `mq/protocol.is_permanent` reads HTTP status by duck typing (`err.response.status_code`, then `err.code`).
- `Settings`: `gemini_api_key`, `gemini_mock`, `gemini_model_fast/deep`, `gemini_embed_model`,
  `gemini_timeout_seconds` (10 s, < singleflight TTL 30 s), `gemini_rpm/tpm` (55 / 900 000). Fail fast when
  mock is off and the key is empty. Metric `ai_gemini_429_total`, alert `GeminiRateLimited`.

## 2. Approved human decisions (inputs, not open)

- **D1** Replace Gemini with Anthropic Claude for **text generation** (chat answers + moderation). No
  `google.genai` call remains on the generation path.
- **D2** Keep the semantic cache embedding layer on Gemini (`text-embedding-004`) when `GEMINI_API_KEY` is set
  (Anthropic has no embedding API). Real mode without `GEMINI_API_KEY`: L3 disabled cleanly (exact-hash L1
  still works), logged **once** at startup, no per-request exception/log, no startup failure. Mock mode keeps
  `mock_embedding`.
- **D3** Model `claude-haiku-5-5` for chat and moderation, each configurable by env.
- **D4** `ANTHROPIC_API_KEY` required when mock is off (fail fast at startup). Limiter logic unchanged.

## 3. Choices made by this spec (human may veto at approval)

- **C1 Mock flag:** `AI_MOCK` (default `1`). `GEMINI_MOCK` accepted as a **deprecated alias** for one release;
  if both are set `AI_MOCK` wins. When only the alias is used, one WARNING at startup names the new variable.
  Reason: `feature/k8s-all` and team `.env` files still set `GEMINI_MOCK`; both directions are safe (missing
  flag -> mock; `GEMINI_MOCK=0` without `ANTHROPIC_API_KEY` -> fail fast).
- **C2 Env names:** `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL_CHAT`, `ANTHROPIC_MODEL_MODERATION` (both default
  `claude-haiku-5-5`), `AI_TIMEOUT_SECONDS` (default 10), `AI_RPM` / `AI_TPM` (defaults unchanged 55 / 900 000),
  `AI_THINKING_HEADROOM_TOKENS` (default 1024, see AC-3). Deprecated aliases accepted: `GEMINI_TIMEOUT_SECONDS`,
  `GEMINI_RPM`, `GEMINI_TPM` (same rule as C1). Kept: `GEMINI_API_KEY`, `GEMINI_EMBED_MODEL` (embeddings only).
  Removed (ignored if present, `extra="ignore"`): `GEMINI_MODEL_FAST`, `GEMINI_MODEL_DEEP`.
- **C3 Module path:** `app/gemini/` -> `app/llm/` via `git mv` (`client.py`, `guarded.py`, `limiter.py`) plus new
  `app/llm/embedder.py`. Class `GeminiClient` -> `LlmClient`. `feature/ops-warroom` touches only `app/main.py`
  (mode endpoints, lines ~13-25 and ~166-220) and its own new tests, none of which import `app.gemini`; our
  `main.py` edits are confined to imports at ~31-32 and the lifespan wiring, so the rename adds no conflict.
  `degradation.Signals.gemini_circuit_open` is **kept** (exposed by ops-warroom contract 5.1).
- **C4 Moderation JSON:** keep the `json.loads` path and NEEDS_HUMAN fallback (current test contract), plus
  stripping one optional surrounding Markdown code fence. No structured-output `format` (less API surface).
- **C5 SDK retries:** `max_retries=0`. The worker already has limiter + delayed-retry queue + singleflight;
  SDK retries would multiply calls under one permit and could outlive the 30 s singleflight TTL.
- **C6 Metric:** `ai_gemini_429_total` -> `ai_llm_429_total`; alert `GeminiRateLimited` -> `LlmRateLimited`
  (same expression/threshold; `alerts_test.yml` has no test for it at c5fb598 -- one is added). New counter `ai_llm_incomplete_total{reason}` (`refusal`, `max_tokens`, `empty`).

## 4. Acceptance criteria

### A. Mock mode

- **AC-1 Mock unchanged.** Given `AI_MOCK=1` (or unset) and no API keys, When the worker starts and handles chat
  and moderation jobs, Then no network call is made, no Anthropic/Gemini client is constructed, `generate()`
  returns the same mock `Generation` shape (`model="mock"`, `total_tokens=len(prompt)//4+64` over the combined
  system+user text), keyword moderation verdicts are unchanged, and `embed()` returns `mock_embedding`.
  `LlmClient()` with no arguments still works in mock mode (reads `AI_MOCK`, alias `GEMINI_MOCK`).

### B. Real generation

- **AC-2 Anthropic call shape.** Given mock off and a key, When chat or moderation generates, Then exactly one
  `await client.messages.create(...)` is made on an `anthropic.AsyncAnthropic(api_key, timeout=AI_TIMEOUT_SECONDS,
  max_retries=0)` (built lazily, or the injected `sdk=`), with `model` = `ANTHROPIC_MODEL_CHAT` for chat and
  `ANTHROPIC_MODEL_MODERATION` for moderation, `system=` = the prompt file (`assistant.v3.md` /
  `moderation.v1.md`), `messages=[{"role":"user","content": <wrapped untrusted blocks>}]`,
  `output_config={"effort":"low"}`. No `temperature`/`top_p`/`top_k`, no assistant prefill, no tools.
- **AC-3 Token budget.** `max_tokens` sent = policy budget (chat 512 NORMAL / 192 SAVING; moderation 512) +
  `AI_THINKING_HEADROOM_TOKENS`. The limiter permit is estimated on the same sent `max_tokens`
  (`estimate_tokens` formula unchanged) so reservation is never below the worst case.
- **AC-4 BR-A7 boundary.** Given a question or event text containing `</du_lieu>`, "ignore the instructions" or
  role tags, Then it appears only inside `wrap_untrusted(...)` blocks in the user message (escaped as today), and
  `system=` contains only the trusted prompt file text. Event context (`tools.event_context`) is wrapped too.
- **AC-5 Text extraction.** Given `stop_reason` in {`end_turn`, `stop_sequence`}, Then `Generation.text` is the
  concatenation of content blocks with `type == "text"` (thinking blocks ignored) and `Generation.model` is the
  model id used.
- **AC-6 Token accounting.** Given `resp.usage` with `input_tokens` and `output_tokens`, Then
  `total_tokens = input_tokens + output_tokens` and the limiter refund uses it. Given `usage` is missing or either
  field is not an int, Then `total_tokens=None` and `guarded_generate` refunds nothing (semantics preserved).

### C. Failure paths

- **AC-7 429.** Given the SDK raises `anthropic.RateLimitError`, Then `LlmClient.generate` increments
  `ai_llm_429_total`, logs one ERROR without the key or prompt, and raises `RateLimited` (chained); guarded
  refunds the whole reservation; chat -> `Retry(0)`, moderation -> existing retry/NEEDS_HUMAN path.
- **AC-8 Timeout.** Given the call exceeds `AI_TIMEOUT_SECONDS` (`asyncio.wait_for` hard cap) or the SDK raises
  `anthropic.APITimeoutError`, Then `TimeoutError` is raised (chained); chat answers with the FAQ fallback and
  caches nothing; no refund (unknown usage).
- **AC-9 Other API errors.** `APIConnectionError` and `APIStatusError` propagate unchanged.
  `mq.protocol.is_permanent` classifies real `anthropic` exception objects: 400/401/403/404 -> permanent;
  408/429/500/529 and connection/timeout errors -> temporary (verified in `test_classify_real_errors.py` with
  real SDK classes, no network). `is_permanent` code changes only if this test fails.
- **AC-10 Incomplete answer.** Given `stop_reason == "refusal"` or `"max_tokens"`, or no text block / blank text,
  Then `generate` raises `LlmIncomplete(reason)` and increments `ai_llm_incomplete_total{reason}`; no refund.
  Chat: FAQ fallback (`source="fallback"`), the partial text is never shown, cached or published to singleflight.
  Moderation: verdict `NEEDS_HUMAN` (score 0.5, reason "AI không đưa ra kết luận, chuyển người duyệt"), delivered
  once, no retry.
- **AC-11 Moderation JSON.** Given text that is a JSON object (optionally wrapped in one ```` ```json ```` fence),
  Then score/axes/reasons are parsed and `calculate_verdict(score)` applied as today. Given non-JSON, a JSON
  non-object, or a non-numeric score, Then NEEDS_HUMAN with the existing "Không phân tích được phản hồi từ AI"
  reason (no exception escapes).

### D. Embeddings (semantic cache L3)

- **AC-12 Gemini embeddings when key present.** Given mock off and `GEMINI_API_KEY` set, Then `embed()` calls
  `google.genai` `embed_content(model=GEMINI_EMBED_MODEL)` with the same timeout and returns the vector; behaviour
  of `SemanticCache` is unchanged (failures still logged per call as today).
- **AC-13 L3 disabled when key absent.** Given mock off and `GEMINI_API_KEY` empty, Then startup succeeds, one
  INFO/WARNING line "semantic cache L3 disabled (GEMINI_API_KEY empty)" is logged once, no Gemini client is
  constructed, `SemanticCache.get` returns L1 hits and otherwise `None` without calling an embedder or logging,
  and `put` writes the exact-hash key but no vector. 100 chat jobs produce zero embed-related log lines.

### E. Configuration and secrets

- **AC-14 Validation.** `Settings` rejects at construction: mock off + empty `ANTHROPIC_API_KEY` (message names
  `ANTHROPIC_API_KEY`); empty model names; `AI_TIMEOUT_SECONDS <= 0`; `AI_RPM/AI_TPM <= 0`;
  `AI_THINKING_HEADROOM_TOKENS < 0`. Mock off without `GEMINI_API_KEY` is **valid** (AC-13). Aliases from C1/C2
  are honoured, new names win over aliases, and alias use is warned once at startup.
- **AC-15 No secrets in logs/repr.** `anthropic_api_key` and `gemini_api_key` are `SecretStr`; `repr(settings)`,
  the startup "san sang" log (now: `ai_mock`, `provider="anthropic"`, chat/moderation model ids, `semantic_l3`
  on/off) and every error log in AC-7..AC-10 contain neither key value nor prompt/user text.

### F. Deployment artefacts and docs

- **AC-16 Dependency.** `anthropic` added to `pyproject.toml` with a lower bound at the version the implementer
  locks (recorded in evidence), `uv.lock` regenerated with `uv lock`; CI `uv sync --all-extras --locked` and
  Trivy pass. `google-genai` stays (embeddings). No other dependency changes.
- **AC-17 Compose/env.** `deploy/compose/docker-compose.yml` ai-worker env: `ANTHROPIC_API_KEY`,
  `AI_MOCK: "${AI_MOCK:-1}"`, `ANTHROPIC_MODEL_CHAT/MODERATION` (with defaults), `GEMINI_API_KEY` (optional,
  embeddings), `AI_RPM`, `AI_TPM`; header comment updated. `.env.example` has an "AI (Anthropic)" section and an
  "Embeddings (Gemini, optional)" section; old `GEMINI_MODEL_*` lines removed. `docker compose config` passes.
- **AC-18 Alerts.** `deploy/observability/alerts.yml` uses `ai_llm_429_total` / `LlmRateLimited`; CI
  `promtool check rules alerts.yml` and `promtool test rules alerts_test.yml` pass, with a new test case for
  `LlmRateLimited` (fires on one 429 in 5 m). No other alert changes.
- **AC-19 Docs.** README config/stack mentions (dev without key, stack table, ai-worker line, limiter line)
  updated to Anthropic + optional Gemini embeddings. Historical workflow docs and design docs
  (`docs/0*.md`) untouched (follow-up F-3).
- **AC-20 Regression.** All existing ai-worker unit tests pass after mechanical updates (imports
  `app.gemini` -> `app.llm`, `GeminiClient` -> `LlmClient`, fake SDK shape); integration tests (mock mode) pass;
  `ruff check` clean. No test asserts against the real API; CI never needs `ANTHROPIC_API_KEY`.

## 5. Definition of Ready

- [x] Decisions D1-D4 recorded; choices C1-C6 listed for veto.
- [x] Inventory done (`grep -rni gemini`): ai-worker app/tests/pyproject/uv.lock, compose, `.env.example`,
      alerts, README. `apps/web` mentions are code comments + a test regex forbidding "gemini" in UI text: no change.
- [x] Overlap checked: ops-warroom (main.py only, disjoint hunks), k8s-all (not edited, F-1).
- [x] Test strategy needs no network/keys (fake SDK injected via `sdk=` / `embed_sdk=`).
- [ ] Human approval of this spec (High-risk).

## 6. Out of scope

- K8s manifests (on `feature/k8s-all`), Grafana dashboard JSON, design docs `docs/0*.md`, web copy.
- Streaming, tool use, prompt caching, structured outputs, Batch API, model routing ("deep" model).
- Changing limiter Lua, degradation thresholds, singleflight TTL, queue topology, prompt wording.
- Renaming `gemini_circuit_open` in degradation signals / ops API.

## 7. Follow-ups (recorded, not done here)

- **F-1** `feature/k8s-all` `deploy/k8s/base/ai-worker/deployment.yaml`: env `GEMINI_MOCK`/`GEMINI_RPM`/`GEMINI_TPM`
  -> `AI_MOCK`/`AI_RPM`/`AI_TPM`; add `ANTHROPIC_API_KEY` secretKeyRef (+ overlay `secrets.dev.env`),
  `ANTHROPIC_MODEL_*`; keep `GEMINI_API_KEY` (embeddings, optional). Aliases (C1/C2) keep k8s-all working until then.
- **F-2** Remove deprecated aliases one release later.
- **F-3** Update architecture docs (`docs/02-kien-truc.md` etc.) and Grafana panels using `ai_gemini_429_total`
  (none found in `deploy/` at c5fb598 besides alerts.yml).
- **F-4** Rename `gemini_circuit_open` after ops-warroom merges.

## 8. Risks

| Risk | Mitigation |
|---|---|
| Thinking tokens make answers hit `max_tokens` (empty/partial answer) | effort low + headroom (AC-3); AC-10 fallback; metric `ai_llm_incomplete_total` to tune headroom |
| Real Anthropic RPM/TPM differ from Gemini defaults (55 / 900 000) | Defaults kept; human sets `AI_RPM/AI_TPM` per org tier (Q-1) |
| SDK exception attributes differ from assumptions | AC-9 test with real SDK classes; classifier is duck-typed |
| Moderation output wrapped in prose -> more NEEDS_HUMAN | fence stripping (C4); prompt asks for JSON only; metric via verdict logs |
| Prompt injection via moved system prompt | AC-4 test on exact `system=`/`messages` payload |
| Env rename breaks a deployment | aliases + warning (C1/C2); compose and `.env.example` updated together |
| Key leak in logs | `SecretStr`, AC-15 caplog test |

## 9. Questions for the human

- **Q-1** Anthropic org rate-limit tier for the demo key: keep `AI_RPM=55`, `AI_TPM=900000`, or set from tier?
- **Q-2** Confirm C1-C6 (especially: aliases kept one release; module rename to `app/llm`; metric rename).
- **Q-3** `AI_THINKING_HEADROOM_TOKENS` default 1024 acceptable (cost ~ +$0.0005 worst case per call at $0.50/1M out)?
