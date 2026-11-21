# Plan: ai-anthropic (round 3, aligned with the frozen tests)

Inputs: `spec.md` (AC-1..AC-20, choices C1-C6; **approved 2026-11-20**), `plan-review.md` (round 1: P-1..P-15,
round 2: Q-1..Q-5), `traceability.yaml`, `red-check.md`. Base develop @ c5fb598. **Tests frozen at 0763f2a**
(`test(ai-worker): add failing tests for the Anthropic switch`): the files in section 2.1 are the contract. Where
this plan and a frozen test disagree, the test wins. One implementer, sequential (files are shared: client,
config, main).

## 1. Files and changes

| # | File | Change | ACs |
|---|---|---|---|
| 1 | `services/ai-worker/pyproject.toml`, `uv.lock` | add `anthropic>=<locked>` with `uv add "anthropic>=X.Y"` (or edit + `uv lock`; **never `--upgrade`**); description "Consumer AI (Anthropic)"; keep `google-genai`. Evidence: `uv --version`, locked `anthropic` version, lock diff only **adds** packages (expected transitive: `distro`, `jiter`, `docstring-parser`, ...) with no version change for existing ones (`httpx 0.28.1`, `pydantic 2.13.5`, `google-genai 2.29.0`). AC-16 "no other dependency changes" = no other direct deps, no bumps. | AC-16 |
| 2 | `app/gemini/` -> `app/llm/` (`git mv`) | `client.py`, `guarded.py`, `limiter.py` keep history; docstrings say "LLM". Test-side renames: section 2.2. | C3 |
| 3 | `app/config.py` | Fields (names fixed by `tests/llm/test_settings_anthropic.py`): `ai_mock: bool = True` (`validation_alias=AliasChoices("AI_MOCK","GEMINI_MOCK")`), `anthropic_api_key: SecretStr`, `anthropic_model_chat` / `anthropic_model_moderation: str = "claude-haiku-5-5"` (`min_length=1`), `ai_timeout_seconds = 10` (`gt=0`, alias `GEMINI_TIMEOUT_SECONDS`), `ai_rpm = 55` / `ai_tpm = 900_000` (`gt=0`, aliases `GEMINI_RPM` / `GEMINI_TPM`), `ai_thinking_headroom_tokens: int = Field(1024, ge=0)`, `gemini_api_key: SecretStr`, `gemini_embed_model = "text-embedding-004"`. Removed as field names: `gemini_model_fast/deep`, `gemini_mock`, `gemini_timeout_seconds`, `gemini_rpm/tpm` (`extra="ignore"` keeps dropped env vars harmless). Validator: mock off + empty Anthropic key -> `ValueError` whose message contains `ANTHROPIC_API_KEY` and never a key value (`test_AC15_LoiValidate_KhongLoKey`). `model_config` uses `validate_by_name=True` (pydantic >= 2.11 name, not `populate_by_name`) so kwargs like `Settings(ai_mock=False)` work. Helper `deprecated_aliases_used(env_file=".env") -> list[str]`: scans `os.environ` **and** `dotenv_values(env_file)` (skipped when `env_file is None`), case-insensitive; an alias counts when it is set and its new name is unset in the same merged view. Shared helper `resolve_mock_flag(env: Mapping[str,str]) -> bool` (`AI_MOCK` over `GEMINI_MOCK`, default `True`), used by `LlmClient()` defaults. Precedence note (accepted, documented in `.env.example`): pydantic-settings gives process env priority over `.env`, so `GEMINI_MOCK` in the process env beats `AI_MOCK` in `.env`; within one source the new name wins. | AC-14, AC-15 |
| 4 | `app/llm/client.py` | **Exported names** (tests import them): `LlmClient`, `Generation`, `RateLimited`, `LlmIncomplete`, `mock_embedding`, `wrap_untrusted`. **Constructor, exactly:** `LlmClient(api_key=None, *, mock=None, sdk=None, chat_model=None, moderation_model=None, timeout_seconds=None, thinking_headroom_tokens=None, gemini_api_key=None, embed_model=None, embed_sdk=None)`. Each `None` falls back to an env-derived default, as at c5fb598: `mock` = `resolve_mock_flag(os.environ)` (AI_MOCK wins over GEMINI_MOCK; nothing set -> mock); `api_key` = `ANTHROPIC_API_KEY`; `gemini_api_key` = `GEMINI_API_KEY`; models / timeout / headroom / embed model = the `Settings` defaults (`claude-haiku-5-5`, 10 s, 1024, `text-embedding-004`). `LlmClient()` with no args and no env -> mock, no SDK built. **Real mode with an empty Anthropic key raises `RuntimeError("AI_MOCK=0 nhung thieu ANTHROPIC_API_KEY")` in `__init__`** (`test_AC1_AI_MOCK0_ThieuKey_TuChoi`), before any SDK exists. SDK construction stays lazy: on the first real `generate` with `sdk=None`, build `AsyncAnthropic(api_key=<explicit key>, timeout=timeout_seconds, max_retries=0)` once; resolve the class at call time so the test's patch of `anthropic.AsyncAnthropic` (and of `app.llm.client.AsyncAnthropic` if that name exists) is honoured. **`sent_max_tokens(budget) -> int`**: `budget + thinking_headroom_tokens` in real mode, `budget` in mock; the single headroom source. `build_prompt(job, tools) -> Prompt` (`Prompt(system, user)` dataclass, `__len__ = len(system)+len(user)`; system = `assistant.v3.md` text exactly, user = wrapped event context + wrapped question; opaque to callers). `generate(prompt, *, max_output_tokens, model=None) -> Generation`: mock path as today (`model == "mock"`, `total_tokens = len(system+user)//4 + 64`); real: `asyncio.wait_for(sdk.messages.create(model=model or chat_model, max_tokens=self.sent_max_tokens(max_output_tokens), system=<str>, messages=[{"role":"user","content":<str>}], output_config={"effort":"low"}), timeout)`; never `temperature` / `top_p` / `top_k` / `tools`. Exceptions: catch **only** `anthropic.RateLimitError` -> `ai_llm_429_total` +1, **exactly one ERROR log record** (no prompt / question / key text), `raise RateLimited(...) from exc`; `anthropic.APITimeoutError` and `asyncio.TimeoutError` -> `raise TimeoutError(...) from exc`; everything else (`APIConnectionError`, 400/401/500/529 `APIStatusError`) propagates as the same object. Check `stop_reason` before content: `refusal` / `max_tokens` -> `LlmIncomplete(reason)`; join `type=="text"` blocks (thinking ignored); blank -> `LlmIncomplete("empty")`; each increments `ai_llm_incomplete_total{reason}`. `LlmIncomplete(reason)` takes the reason positionally, exposes `.reason`, and its args / message carry only the reason. `total_tokens` = in+out when both are int, else `None`. No log record at any level contains the prompt, the question or a key. `scan_moderation(job, *, generate=None)`: mock = keyword scan as today; real: `Prompt(moderation.v1.md, wrapped title+desc)`, `await (generate or self.generate)(prompt, max_output_tokens=512, model=self._moderation_model)`, so the moderation model is used on the direct path and on the `functools.partial(guarded_generate, limiter, client)` path; `LlmIncomplete` -> NEEDS_HUMAN with reason `"AI không đưa ra kết luận, chuyển người duyệt"`; strip one code fence, `json.loads`, require dict, fallback as today (+ `TypeError` / `AttributeError`). **`embed(text) -> list[float]` is kept** (Q-1) and delegates to `self.embedder` (row 5): mock -> `mock_embedding(text)`; real + Gemini key -> `embed_sdk.aio.models.embed_content(model=embed_model, contents=text)` under `asyncio.wait_for(..., timeout_seconds)`; real without Gemini key -> `self.embedder is None` and `embed()` raises `RuntimeError` (never wired: the cache gets `None`, row 10). `genai.Client` is never built in mock, with an empty Gemini key, or before the first `embed`. | AC-1..AC-12 |
| 5 | `app/llm/embedder.py` (new; `test_AC12_ModuleEmbedderTonTai` imports it) | `Embedder` Protocol (`async embed(text) -> list[float]`); `MockEmbedder` (wraps `mock_embedding`, which stays importable from `app.llm.client`); `GeminiEmbedder(api_key, model, timeout_seconds, sdk=None)` (lazy `google.genai.Client(api_key=...)` on first call, `aio.models.embed_content`, `wait_for` timeout); `make_embedder(*, mock, gemini_api_key, embed_model, timeout_seconds, embed_sdk=None) -> Embedder | None` (mock -> `MockEmbedder()`; real + key -> `GeminiEmbedder`; real + empty key -> `None`). `LlmClient.__init__` calls it and stores the result on the public attribute `llm.embedder`. | AC-1, AC-12, AC-13 |
| 6 | `app/pipeline/semantic_cache.py` | `SemanticCache(redis, embedder: Embedder | None, ...)` (positional; `similarity_threshold` kwarg kept). `None`: `get` returns after L1, `put` writes the exact key only (no `hset`), no log at any level (`test_AC13_CacheKhongEmbedder_L1VanChay_KhongLog`: 100 get+put, no record mentioning "embed", none >= WARNING). Any object with `async embed()` still works (existing `FakeEmbedder`, mock `LlmClient`). No other change. | AC-13 |
| 7 | `app/llm/guarded.py` | Signature as at c5fb598: `guarded_generate(limiter, client, prompt, *, max_output_tokens)`, plus a pass-through keyword `model=None` that is forwarded to `client.generate` **only when not `None`** (chat passes none, so fakes whose `generate` has no `model` parameter keep working). `est = estimate_tokens(prompt, client.sent_max_tokens(max_output_tokens))`. `estimate_tokens` body / formula unchanged (`len(prompt)//3 + max_tokens + 256`), hint `str | Prompt`. Permit == sent `max_tokens` + `len(prompt)//3 + 256` (`test_AC3_GiayPhepGuarded_BangDungEstimateTrenSentMaxTokens`: permits for budgets 512 and 192 differ by exactly 320). Refunds unchanged: in+out refund; `None` -> no refund; `RateLimited` -> refund all; `TimeoutError` / `LlmIncomplete` -> no refund. No `getattr(client, "sent_max_tokens", ...)` default; the old hand-written fakes get the method through TCR-1. | AC-3, AC-6, AC-7, AC-8, AC-10 |
| 8 | `app/metrics.py` | `RATE_LIMITED` -> `ai_llm_429_total`; add `INCOMPLETE = Counter("ai_llm_incomplete_total", ..., ["reason"])` | C6, AC-7, AC-10 |
| 9 | `app/consumers/chat.py`, `moderation.py` | imports `app.llm.*`; constructor param / attr `gemini` -> `llm`, position unchanged (tests call `ChatConsumer(limiter, cache, faq, flight, llm, tools, signals, publish)` positionally and `ModerationConsumer(llm, api_base, callback_fn=..., limiter=...)`); `_call_gemini` -> `_call_llm`; chat keeps catching non-`Retry` exceptions -> FAQ fallback with no `cache.put` and no partial text published (covers `LlmIncomplete`, `TimeoutError`); chat passes no `model`. Moderation keeps `functools.partial(guarded_generate, limiter, llm)` (no `model=` bound) or `None` without a limiter, passed as `scan_moderation(job, generate=...)`; `LlmIncomplete` -> NEEDS_HUMAN delivered once, no retry. Log texts "goi gemini" -> "goi llm". | AC-8, AC-10 |
| 10 | `app/llm/wiring.py` (new) + `app/main.py` | `build_llm(settings) -> tuple[LlmClient, Embedder | None]`: `LlmClient(settings.anthropic_api_key.get_secret_value(), mock=settings.ai_mock, chat_model=..., moderation_model=..., timeout_seconds=settings.ai_timeout_seconds, thinking_headroom_tokens=settings.ai_thinking_headroom_tokens, gemini_api_key=settings.gemini_api_key.get_secret_value(), embed_model=settings.gemini_embed_model)`, returns `(llm, llm.embedder)`; builds no SDK client (anthropic or genai). `log_startup(settings, embedder, *, deprecated: list[str]) -> None`, texts fixed by `tests/test_wiring.py`: (a) `embedder is None` -> exactly one INFO or WARNING record whose message contains `semantic cache L3 disabled (GEMINI_API_KEY empty)`; none when an embedder exists; (b) one ready record whose text contains `ai_mock`, `anthropic` (provider), both model ids and `semantic_l3`; (c) per name in `deprecated`, exactly one WARNING whose message names the old and the new variable (e.g. `GEMINI_MOCK is deprecated, use AI_MOCK`); none for an empty list; (d) no key value in any message or record attribute. Lifespan only (+ import lines 31-32): `llm, embedder = build_llm(settings)`, `log_startup(settings, embedder, deprecated=deprecated_aliases_used(...))`, `SemanticCache(redis, embedder)` (the cache never receives `llm` in production), limiter from `ai_rpm/ai_tpm`. Do **not** touch `/admin/mode`, `_signals_now`, `gemini_circuit_open` (ops-warroom hunks). | AC-1, AC-12..AC-15 |
| 11 | `deploy/compose/docker-compose.yml`, `.env.example` | env per AC-17, with fallbacks: `AI_MOCK: "${AI_MOCK:-${GEMINI_MOCK:-1}}"`, `AI_RPM: "${AI_RPM:-55}"`, `AI_TPM: "${AI_TPM:-900000}"`; header comment line 4 updated. Check with `docker compose config` (default, `GEMINI_MOCK=0` only, `AI_MOCK=0`). `.env.example` stays sourceable by bash (no unquoted spaces), see section 5. | AC-17 |
| 12 | `deploy/observability/alerts.yml` | rename only: `GeminiRateLimited` -> `LlmRateLimited`, `ai_gemini_429_total` -> `ai_llm_429_total` (+ comments lines 46, 61). `alerts_test.yml` is frozen; its `LlmRateLimited` case is red now and green with the rename (red-check). | AC-18 |
| 13 | `README.md` | lines 19, 43, 82, 99, 131, 133 | AC-19 |
| 14 | `docs/workflow/ai-anthropic/*` | traceability, evidence (locked SDK version, lock diff check, gate output, **real-key smoke result**). Comment-only Gemini mentions (P-12) updated to "LLM": `app/api/assistant.py:3`, `app/mq/protocol.py:85`, `app/pipeline/singleflight.py:4,27`, `app/pipeline/degradation.py:33` (comment only), compose header line 4. Left as is: `app/scripts/limiter.lua` header (spec forbids touching Lua), `tests/mq/test_runtime.py:60` string (frozen test). | - |

Not touched (logic): `app/scripts/*.lua`, `app/mq/protocol.py` logic (AC-9: `is_permanent` changes only if
`test_classify_anthropic_errors.py` fails; red-check expects it to pass unchanged), `app/pipeline/degradation.py`,
`app/api/*`, `apps/web`, `deploy/k8s` (absent on develop), design docs.

## 2. Tests (frozen at 0763f2a; no network, no keys)

### 2.1 Frozen test files (the contract)

| File | Covers | ACs |
|---|---|---|
| `tests/conftest.py` | root autouse `_clear_ai_env`: deletes every env var whose upper-cased name starts with `AI_`, `GEMINI_` or `ANTHROPIC_` (prefix scan, so Q-4 is already met). Applies to every test, incl. `test_config.py` and integration. | AC-20 |
| `tests/llm/conftest.py` | fakes: `FakeAnthropic` / `FakeMessages` (records `create(**kw)`, returns `content` / `usage` / `stop_reason` or raises), `fake_genai` (`embed_content`; `generate_content` raises), `make_client` (the row 4 constructor), `FakeLimiter`, `status_error` (real `anthropic` classes on `httpx.Response`). Its explicit-list env fixture is redundant with the root one and stays. | - |
| `tests/llm/test_client_mock.py` | mock generate shape + token formula, mock moderation keywords, `embed()` == `mock_embedding`, `LlmClient()` with `AI_MOCK` / `GEMINI_MOCK` / nothing, AI_MOCK wins, `AI_MOCK=0` without key raises with `ANTHROPIC_API_KEY`; no SDK built in mock | AC-1 |
| `tests/llm/test_client_anthropic.py` | call shape per role, forbidden params, lazy `AsyncAnthropic(api_key, timeout, max_retries=0)` built once; `max_tokens` = budget + headroom (configurable), `sent_max_tokens`, permit >= and == estimate on the sent `max_tokens`; BR-A7 wrapping in the user turn only; text-block join, thinking ignored; in+out tokens, missing usage -> `None`; guarded refunds; 429 -> `RateLimited` from exc + counter + one ERROR record; timeout (hard and `APITimeoutError`, with chain); other errors propagate unchanged; refusal / max_tokens / empty -> `LlmIncomplete(.reason)` + counter; no secret in logs | AC-2..AC-10 |
| `tests/llm/test_consumers_anthropic.py` | real `ChatConsumer` / `ModerationConsumer` with `make_client`: NORMAL budget, 429 -> `Retry`, timeout / incomplete -> fallback not cached, moderation via limiter 512+headroom, incomplete -> NEEDS_HUMAN once with the fixed reason, 429 first attempt -> retry | AC-2, AC-3, AC-7, AC-8, AC-10 |
| `tests/llm/test_classify_anthropic_errors.py` | `is_permanent` on real anthropic classes: 4xx permanent; 408/429/5xx, connection, timeout temporary; wrapped `RateLimited` / `TimeoutError` follow the cause; `LlmIncomplete("max_tokens")` not permanent | AC-9 |
| `tests/llm/test_moderation_json.py` | JSON / fenced / prose / list / bad score -> verdict or NEEDS_HUMAN, thresholds unchanged | AC-11 |
| `tests/llm/test_embeddings.py` | `app.llm.embedder` importable; embed via `embed_sdk` with `embed_model`, same timeout; generation never calls Gemini; no `genai.Client` with empty Gemini key; `SemanticCache(redis, None)` 100x zero-log | AC-12, AC-13 |
| `tests/llm/test_settings_anthropic.py` | defaults, key required (message), empty model rejected, numeric validation, headroom 0 valid, new env read, aliases (`GEMINI_MOCK`, `GEMINI_TIMEOUT_SECONDS/RPM/TPM`) honoured and new names win, dropped vars ignored, `SecretStr` repr, validation error hides the key | AC-14, AC-15 |
| `tests/test_llm_sdk_contract.py` | (P-2a) every kwarg sent by the real `generate` / `scan_moderation` exists in `anthropic.resources.messages.AsyncMessages.create` (which has no `**kwargs`); `AsyncAnthropic.__init__` accepts `api_key`, `timeout`, `max_retries` | AC-2 |
| `tests/test_wiring.py` | `build_llm` (embedder `None` / Gemini / mock, no SDK built) and `log_startup` texts (row 10) | AC-1, AC-12..AC-15 |
| `deploy/observability/alerts_test.yml` | `LlmRateLimited` silent at 2m, fires at 4m, silent at 11m | AC-18 |

Not in the frozen set, so not a gate: the P-2(b) `httpx.MockTransport` round trip through the real SDK. SDK
parsing of `usage` / `stop_reason` / content is covered by the required human smoke (section 3 step 7). The
implementer may add it as a new, extra test file.

`red-check.md`: 41 failed, 2 passed, 5 collection errors (127 cases once collectable), all "code does not exist
yet"; the existing suite stays at 203 passed with the root conftest active.

**Lint:** `ruff check tests` reports I001 on the 4 test files that import `app.llm` until `app/llm/` exists (ruff
classifies a missing package as third-party). The implementer must **not** reorder those imports; the warning
clears once row 2 lands (checked on a scratch copy in red-check).

### 2.2 Mechanical renames in existing tests (implementer edits, allowed under AC-20)

From `traceability.yaml` `mechanical_renames`, restricted to: import paths, class / parameter / attribute names,
the file rename, metric names. Not allowed under this heading: any change to an assertion, an expected value, a
fake's return shape or an added fake method; those are TCR-1 (2.3).

| File | Mechanical edit |
|---|---|
| `tests/test_gemini_client.py` | `git mv` -> `tests/test_llm_client.py`; `app.gemini.client` -> `app.llm.client`; `GeminiClient` -> `LlmClient`; `ai_gemini_429_total` -> `ai_llm_429_total`. The rest of this file is TCR-1 items 1-7. |
| `tests/test_guarded.py` | `app.gemini.*` -> `app.llm.*`; `GeminiClient` -> `LlmClient`; local names `gemini` may stay. The fakes are TCR-1 items 8-9. |
| `tests/test_chat_handler.py` | `app.gemini.*` -> `app.llm.*`; `ChatConsumer(..., gemini=...)` keyword -> `llm=`. The fake is TCR-1 item 10. |
| `tests/test_moderation_consumer.py` | `GeminiClient` -> `LlmClient` from `app.llm.client`; keyword `gemini=` -> `llm=` |
| `tests/test_classify_real_errors.py` | `app.gemini.*` -> `app.llm.*`; genai cases stay (google-genai remains installed) |
| `tests/test_config.py` | field names `gemini_timeout_seconds` / `gemini_rpm` / `gemini_tpm` -> `ai_*` (same values, same assertions). `test_GeminiThatMaThieuKeyThiTuChoi` is TCR-1 item 11. |
| `tests/integration/test_assistant_integration.py` | `app.gemini.*` -> `app.llm.*`; `GeminiClient(mock=True)` -> `LlmClient(mock=True)`; `gemini=` -> `llm=`. `SemanticCache(redis, llm)` stays (mock `LlmClient` still has `embed()`); no switch to `MockEmbedder`. |
| `tests/integration/test_consumers_integration.py` | same as above; the fake error string `"gemini tam thoi"` stays |

The freeze filter locks files that existed at 0763f2a. `tests/test_llm_client.py` is the locked
`test_gemini_client.py` under a new name: `git diff -M 0763f2a` must show it as a rename whose content changes are
only the edits above plus the approved TCR-1 edits.

### 2.3 TCR-1 (test change, needs human approval)

File: `docs/workflow/ai-anthropic/test-change-request.md`, status **open, awaiting human**. The old genai
generation fakes cannot drive an Anthropic client, and `guarded_generate` now calls `client.sent_max_tokens`,
which the old hand-written fakes lack. Converting them changes fake shapes and a few expected values, so it is a
test change, not a rename. Items: 7 Gemini-shaped tests in `test_llm_client.py` (5 generation, `test_Embed_That`, the missing-key message; items 1-7), `FakeGen`
and `FakeModels` in `test_guarded.py`, `FakeGemini` in `test_chat_handler.py`, one error-message match in
`test_config.py`. Until approval the implementer does not edit these tests; the gate lists their failures as
"pending TCR-1" and the task cannot reach verify with them red.

Mutation targets: stop-reason checks, token sum, `max_retries=0`, `sent_max_tokens` headroom, alias precedence,
`.env` alias scan, the single L3-disabled log.

## 3. Order

1. Dependency (row 1) + lock-diff evidence. Commit alone (easy revert, Trivy run).
2. `git mv app/gemini app/llm` (row 2) + the mechanical test edits of 2.2 (incl.
   `git mv tests/test_gemini_client.py tests/test_llm_client.py`) in one commit. Frozen `tests/llm/*` untouched.
3. Config (3) -> metrics (8) -> embedder + cache (5, 6) -> client (4) -> guarded (7) -> consumers (9) ->
   wiring + main (10). Run `uv run pytest -q tests/llm tests/test_wiring.py tests/test_llm_sdk_contract.py` after
   each.
4. After TCR-1 is approved: apply exactly the approved edits; record the approval in the TCR file.
5. Compose / env / alerts / README / comments (11-14).
6. Gates: `uv sync --locked`, `ruff check .`, `uv run pytest -q` (all green: 203 baseline + 127 new),
   `pytest -m integration`, `promtool check rules alerts.yml` + `promtool test rules alerts_test.yml`,
   `docker compose config` (three env variants of row 11),
   `git diff -M --stat 0763f2a -- services/ai-worker/tests deploy/observability/alerts_test.yml` lists only the 2.2
   files and the approved TCR-1 files, rollback check (section 5).
7. **Required pre-merge (human):** smoke with a real `ANTHROPIC_API_KEY`, `AI_MOCK=0`: one chat job and one
   moderation job. Record in `evidence.md`: model ids, `stop_reason`, `usage` (in/out), latency, and for the
   Vietnamese chat prompt `usage.input_tokens` vs `len(prompt)//3` (P-9; ratio > 1 -> open follow-up F-5, the
   formula stays frozen). The key is never committed or logged. PR is not merged without this record.

Commit and push after each step.

## 4. Conflict handling

- ops-warroom: merge it first if it lands earlier; our `main.py` hunks (imports 31-32, lifespan 70-132) are
  disjoint from its hunks (13-25, 166-220). Re-run `git merge-tree` before PR.
- k8s-all: untouched; aliases keep its `GEMINI_*` env working (spec F-1).
- `origin/bugfix/EV-120-ai-verdict-callback` (P-13): last commit 2026-10-22, 3 commits not in develop, touches
  `config.py`, `consumers/moderation.py`, `main.py`, `test_config.py`, adds `test_compose_config.py`.
  **Open question for the human:** abandoned or squash-merged? Not blocking. If live: it merges first and we
  rebase onto it (our rewrite of those files is larger), recording the conflict resolution in evidence.

## 5. Rollback

- Code: revert the merge commit on develop (single PR); no data/migration/queue change, Redis keys unchanged
  (`ai:cache:*`, `ai:vec:*`, limiter keys), so old and new workers can share Redis.
- Ops fast path without redeploying old code: `AI_MOCK=1` (assistant serves FAQ/mock, moderation keyword mock).
- Env: aliases mean an old `.env` still starts the new worker; old code reverted onto the new env sees no
  `GEMINI_MOCK` and defaults to mock (safe); real mode on old code needs `GEMINI_MOCK=0` + `GEMINI_API_KEY` again.
- Rollback check in verify (bash, run from the feature worktree root):
  ```bash
  NEW=$(pwd); git worktree add ../evf-rollback c5fb598 && cd ../evf-rollback
  docker compose -f deploy/compose/docker-compose.yml --env-file "$NEW/.env.example" config >/dev/null && echo compose-ok
  cd services/ai-worker && uv sync --locked
  set -a; . "$NEW/.env.example"; set +a
  uv run python -c "from app.config import Settings; from app.gemini.client import GeminiClient; s=Settings(_env_file=None); c=GeminiClient(); print(s.gemini_mock, c.mock)"
  cd "$NEW" && git worktree remove ../evf-rollback
  ```
  Expect `compose-ok` and `True True` (mock mode, no exception). Record the output in evidence.

## 6. Round 2 changes (resolution of plan-review.md round 1)

| Finding | Resolution |
|---|---|
| P-1 (major) | Lifespan test dropped. Pure `build_llm(settings)` and `log_startup(settings, embedder, deprecated=...)` in `app/llm/wiring.py` (row 10), tested with caplog in `tests/test_wiring.py`. Zero-log case: `SemanticCache(redis, None)` driven 100 times. |
| P-2 (major) | (a) no-network signature test of the real `AsyncMessages.create` for every sent kwarg; (b) MockTransport round trip (round 3: optional, not frozen); (c) real-key human smoke is a **required pre-merge** step (section 3 step 7). |
| P-3 (major) | Single `LlmClient.sent_max_tokens(budget)` used by `generate()` and `guarded_generate`; permit == `len(prompt)//3 + sent + 256`. |
| P-4 (major) | Superseded by Q-1 (`embed()` kept; the cache still never receives `llm` in production). |
| P-5 | Alias scan covers `os.environ` + `dotenv_values(env_file)`, case-insensitive; `validate_by_name=True`; cross-source precedence documented in `.env.example`. |
| P-6 | Compose `AI_MOCK: "${AI_MOCK:-${GEMINI_MOCK:-1}}"`, `AI_RPM` / `AI_TPM` overridable; three env variants checked. |
| P-7 | `uv add` / `uv lock` without `--upgrade`; evidence records `uv --version`, locked `anthropic`, adds-only lock diff. |
| P-8 | Root autouse env-clearing fixture (prefix scan, frozen); explicit `api_key` to the SDK; `resolve_mock_flag` shared helper. |
| P-9 | Smoke records `usage.input_tokens` vs `len(prompt)//3`; ratio > 1 -> follow-up F-5. |
| P-10 | No `model=` in the moderation partial; `scan_moderation` alone passes the moderation model. |
| P-11 | Header says approved; `estimate_tokens` hint `str | Prompt`; `LlmIncomplete` carries only the reason. |
| P-12 | Row 14 lists comment updates and the deliberate leaves. |
| P-13 | Open, non-blocking question in section 4. |
| P-14 | Executable rollback check (section 5); test-file rename in the step 2 commit. |
| P-15 | `generate()` catches only `RateLimitError` and `APITimeoutError` / `asyncio.TimeoutError`. |

## 7. Round 3 changes (resolution of plan-review.md round 2, aligned with the tests frozen at 0763f2a)

| Finding | Resolution |
|---|---|
| Q-1 (blocker) | `LlmClient.embed()` kept (row 4): mock -> `mock_embedding`; real -> `embed_sdk.aio.models.embed_content(model=embed_model, contents=text)` under `timeout_seconds`, through `llm.embedder` built by `make_embedder` in the new `app/llm/embedder.py` (row 5; the tests import the module). `wiring.build_llm(settings)` returns `(llm, embedder)`, embedder `None` in real mode with empty `GEMINI_API_KEY`; main wires `SemanticCache(redis, embedder)`. The "no `embed` attribute" test is removed from the plan. Integration tests keep `SemanticCache(redis, llm)` in mock mode (stated in 2.2). |
| Q-2 (major) | Row 4 constructor written exactly as the tests call it: `LlmClient(api_key, *, mock=, sdk=, chat_model=, moderation_model=, timeout_seconds=, thinking_headroom_tokens=, gemini_api_key=, embed_model=, embed_sdk=)`. `LlmClient()` reads `AI_MOCK` / `GEMINI_MOCK` (AI_MOCK wins); `AI_MOCK=0` without key raises `RuntimeError` at construction with `ANTHROPIC_API_KEY` in the message; SDK stays lazy. `scan_moderation(job, *, generate=None)`; `sent_max_tokens(budget)`; `guarded_generate` signature unchanged, estimate via `client.sent_max_tokens` (row 7); `LlmIncomplete(reason)` with `.reason` in `refusal|max_tokens|empty`; metrics `ai_llm_429_total`, `ai_llm_incomplete_total{reason}`; exactly one ERROR record on 429; `log_startup` texts taken from `test_wiring.py` (row 10). |
| Q-3 (major) | Section 2 rewritten against the real frozen paths (2.1). Mechanical renames from `traceability.yaml` listed as implementer edits under AC-20, names / imports only, no assertion changes (2.2). The old genai generation tests in the renamed `test_llm_client.py`, the `test_guarded.py` / `test_chat_handler.py` fakes and one `test_config.py` message are a test change: TCR-1 (2.3, `test-change-request.md`), open, awaiting human. |
| Q-4 (minor) | Already met: the frozen root `tests/conftest.py::_clear_ai_env` deletes by prefix (`AI_`, `GEMINI_`, `ANTHROPIC_`). Noted in 2.1. |
| Q-5 (minor) | Section 5 rollback is a bash block: `uv sync --locked` in the c5fb598 worktree first, then `set -a; . "$NEW/.env.example"; set +a`, then the `uv run` one-liner. |
| Other | P-2(b) MockTransport test is not frozen: optional; SDK parsing is covered by the required human smoke. Lint: do not reorder the `app.llm` imports in tests (ruff I001 clears once `app/llm/` exists). Row 12: `alerts_test.yml` is frozen; only `alerts.yml` changes. Row 7: `model` is forwarded only when not `None`, so existing fakes without a `model` parameter keep working. |
