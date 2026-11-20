# Plan: ai-anthropic (round 2, step 2b)

Inputs: `spec.md` (AC-1..AC-20, choices C1-C6; **approved 2026-11-20**), `plan-review.md` (round 1: FAIL, P-1..P-15).
Base develop @ c5fb598. One implementer, sequential (files are shared: client, config, main). No production code
before the test freeze.

## 1. Files and changes

| # | File | Change | ACs |
|---|---|---|---|
| 1 | `services/ai-worker/pyproject.toml`, `uv.lock` | add `anthropic>=<locked>` with `uv add "anthropic>=X.Y"` (or edit + `uv lock`; **never `--upgrade`**); description "Consumer AI (Anthropic)"; keep `google-genai`. Evidence: `uv --version`, locked `anthropic` version, lock diff only **adds** packages (expected transitive: `distro`, `jiter`, `docstring-parser`, ...) with no version change for existing ones (`httpx 0.28.1`, `pydantic 2.13.5`, `google-genai 2.29.0`). AC-16 "no other dependency changes" = no other direct deps, no bumps. | AC-16 |
| 2 | `app/gemini/` -> `app/llm/` (`git mv`) | `client.py`, `guarded.py`, `limiter.py` keep history; docstrings say "LLM". Same commit: `git mv tests/test_gemini_client.py tests/test_llm_client.py` + mechanical import updates (pure rename, before freeze, so the freeze filter locks the renamed file). | C3 |
| 3 | `app/config.py` | fields: `ai_mock: bool = True` (`validation_alias=AliasChoices("AI_MOCK","GEMINI_MOCK")`), `anthropic_api_key: SecretStr`, `anthropic_model_chat`/`_moderation: str = "claude-haiku-5-5"` (`min_length=1`), `ai_timeout_seconds` (alias `GEMINI_TIMEOUT_SECONDS`), `ai_rpm`/`ai_tpm` (aliases `GEMINI_RPM/TPM`), `ai_thinking_headroom_tokens: int = Field(1024, ge=0)`, `gemini_api_key: SecretStr`, `gemini_embed_model`; drop `gemini_model_fast/deep`, `gemini_mock`. Validator: mock off + empty Anthropic key -> `ValueError("AI_MOCK=0 nhung thieu ANTHROPIC_API_KEY")`. `model_config` uses `validate_by_name=True` (pydantic >= 2.11 name, not `populate_by_name`); test that pytest shows no pydantic deprecation warning. Helper `deprecated_aliases_used(env_file=".env") -> list[str]`: scans `os.environ` **and** `dotenv_values(env_file)` (skipped when `env_file is None`, i.e. `_env_file=None`), case-insensitive; an alias counts when it is set and its new name is unset in the same merged view. Shared helper `resolve_mock_flag(env: Mapping[str,str]) -> bool` (`AI_MOCK` over `GEMINI_MOCK`, default `True`) used by `LlmClient()` defaults so the rule lives once. Precedence note (accepted, documented in `.env.example` comment): pydantic-settings gives process env priority over `.env`, so `GEMINI_MOCK` in the process env beats `AI_MOCK` in `.env`; within one source the new name wins. | AC-14, AC-15 |
| 4 | `app/llm/client.py` | `Prompt(system: str, user: str)` dataclass, `__len__` = `len(system)+len(user)`. `LlmClient(api_key, *, mock, chat_model, moderation_model, timeout_seconds, thinking_headroom, sdk=None)`. **`sent_max_tokens(budget: int) -> int`**: `budget` in mock, `budget + thinking_headroom` in real; the single source of headroom (P-3). `build_prompt(job, tools) -> Prompt` (system = assistant.v3.md; user = wrapped context + wrapped question). `generate(prompt, *, max_output_tokens, model=None)`: mock path as today; real: `max_tokens = self.sent_max_tokens(max_output_tokens)`, `asyncio.wait_for(sdk.messages.create(model=..., max_tokens=..., system=..., messages=[{"role":"user","content":...}], output_config={"effort":"low"}), timeout)`. Exceptions: catch **only** `anthropic.RateLimitError` (metric + `RateLimited`) and `anthropic.APITimeoutError` / `asyncio.TimeoutError` (-> `TimeoutError`); no broad `except APIConnectionError`/`APIStatusError` (timeout subclasses connection error); everything else propagates. Check `stop_reason` before content: `refusal`/`max_tokens` -> `LlmIncomplete(reason)`; join `type=="text"` blocks, blank -> `LlmIncomplete("empty")`. `LlmIncomplete` args/message carry only the reason, never partial text. `total_tokens` = in+out when both int else `None`. `scan_moderation`: `Prompt(moderation.v1.md, wrapped title+desc)`, always passes `model=self._moderation_model` to the bound generate fn; `LlmIncomplete` -> NEEDS_HUMAN result (AC-10 reason), strip one code fence, `json.loads`, require dict, fallback as today (+ `TypeError`/`AttributeError`). `_sdk()` lazily `anthropic.AsyncAnthropic(api_key=<explicit SecretStr value>, timeout=..., max_retries=0)`; real mode with empty key -> `RuntimeError` before constructing the SDK (no env fallback). **No `embed()` method** (P-4). | AC-1..AC-11 |
| 5 | `app/llm/embedder.py` (new) | `Embedder` Protocol (`async embed(text) -> list[float]`), `MockEmbedder` (wraps `mock_embedding`, moved here and re-exported from client), `GeminiEmbedder(api_key, model, timeout, sdk=None)` (lazy `google.genai.Client`, `aio.models.embed_content`), `make_embedder(settings-like) -> Embedder | None` (mock -> `MockEmbedder()`; real + key -> `GeminiEmbedder`; real + empty key -> `None`). | AC-12, AC-13 |
| 6 | `app/pipeline/semantic_cache.py` | constructor `SemanticCache(redis, embedder: Embedder | None)`. `None`: `get` returns after L1, `put` writes the exact key only; no log at any level. No other change. | AC-13 |
| 7 | `app/llm/guarded.py` | `guarded_generate(limiter, llm, prompt, *, max_output_tokens, model=None)`: `est = estimate_tokens(prompt, llm.sent_max_tokens(max_output_tokens))`. `estimate_tokens` body/formula unchanged; signature hint becomes `str | Prompt` (it uses `len(prompt)`). The same `llm.sent_max_tokens` value is what `generate()` sends, so permit == sent `max_tokens` + `len(prompt)//3 + 256`. Refund rules unchanged; `LlmIncomplete` keeps the "no refund" default. No `getattr(llm, ..., 0)` defaults. | AC-3, AC-6, AC-7 |
| 8 | `app/metrics.py` | `RATE_LIMITED` -> `ai_llm_429_total`; add `INCOMPLETE = Counter("ai_llm_incomplete_total", ..., ["reason"])` | C6, AC-7, AC-10 |
| 9 | `app/consumers/chat.py`, `moderation.py` | imports `app.llm.*`; kwarg/attr `gemini` -> `llm`; `_call_gemini` -> `_call_llm`; chat keeps catching non-`Retry` exceptions -> FAQ fallback (covers `LlmIncomplete`, `TimeoutError`), chat passes no `model` (default chat model); moderation `functools.partial(guarded_generate, limiter, llm)` with **no** `model=` bound (role -> model lives only in `scan_moderation`). With `limiter=None` the bound fn is `llm.generate`. Log texts "goi gemini" -> "goi llm". | AC-8, AC-10 |
| 10 | `app/main.py` + new pure functions in `app/llm/wiring.py` | `build_llm(settings) -> tuple[LlmClient, Embedder | None]` (constructs client + `make_embedder(settings)`); `log_startup(settings, embedder, *, deprecated: list[str]) -> None` (one warning per deprecated alias; if real and `embedder is None` exactly one line "semantic cache L3 disabled (GEMINI_API_KEY empty)"; ready-log fields `ai_mock`, `provider`, models, `semantic_l3`; never logs key values). Lifespan only (+ import lines 31-32) calls `build_llm`, `log_startup(settings, embedder, deprecated=deprecated_aliases_used(...))`, `SemanticCache(redis, embedder)`, limiter from `ai_rpm/ai_tpm`. Do **not** touch `/admin/mode`, `_signals_now`, `gemini_circuit_open` (ops-warroom hunks). | AC-13..AC-15 |
| 11 | `deploy/compose/docker-compose.yml`, `.env.example` | env per AC-17, with fallbacks: `AI_MOCK: "${AI_MOCK:-${GEMINI_MOCK:-1}}"`, `AI_RPM: "${AI_RPM:-55}"`, `AI_TPM: "${AI_TPM:-900000}"`; header comment line 4 updated. Check with `docker compose config` (default, `GEMINI_MOCK=0` only, `AI_MOCK=0`). | AC-17 |
| 12 | `deploy/observability/alerts.yml`, `alerts_test.yml` | rename metric/alert (+ comments lines 46, 61); add a promtool test case | AC-18 |
| 13 | `README.md` | lines 19, 43, 82, 99, 131, 133 | AC-19 |
| 14 | `docs/workflow/ai-anthropic/*` | traceability, evidence (locked SDK version, lock diff check, gate output, **real-key smoke result**). Comment-only Gemini mentions (P-12): updated to "LLM": `app/api/assistant.py:3`, `app/mq/protocol.py:85`, `app/pipeline/singleflight.py:4,27`, `app/pipeline/degradation.py:33` (comment only, no logic), `alerts.yml:46,61`, compose header line 4. Left as is: `app/scripts/limiter.lua` header (spec forbids touching Lua), `tests/mq/test_runtime.py:60` string (test fixture text, not behaviour; frozen test). | - |

Not touched (logic): `app/scripts/*.lua`, `app/pipeline/degradation.py` (comment line only), `app/api/*` (comment
line only), `apps/web`, `deploy/k8s` (absent on develop), design docs.

## 2. Tests (written first, red on c5fb598; no network, no keys)

Hygiene (P-8): autouse fixture in `tests/conftest.py` `monkeypatch.delenv(..., raising=False)` for
`ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `AI_MOCK`, `GEMINI_MOCK` and every `AI_*` / `GEMINI_*` /
`ANTHROPIC_*` var; `Settings` in tests built with `_env_file=None` unless the test is about `.env`. The client
always passes `api_key` explicitly, so the SDK env fallback can never be used.

Fake SDK: `FakeMessages.create(**kw)` records kwargs, returns
`SimpleNamespace(content=[SimpleNamespace(type="thinking", ...), SimpleNamespace(type="text", text=...)],
stop_reason=..., usage=SimpleNamespace(input_tokens=.., output_tokens=..))` or raises; injected as
`sdk=SimpleNamespace(messages=FakeMessages())`. SDK errors built from real classes:
`anthropic.RateLimitError("x", response=httpx.Response(429, request=req), body=None)` (same for
`APIStatusError` 400/401/404/500/529), `anthropic.APITimeoutError(request=req)`,
`anthropic.APIConnectionError(request=req)`. Fake embed SDK as in today's `FakeModels.embed_content`.

Real-SDK contract (P-2), because the fake accepts any kwarg:
- (a) required, no network: `test_llm_sdk_contract.py` builds
  `anthropic.AsyncAnthropic(api_key="x", max_retries=0)` (construction makes no request) and asserts
  `inspect.signature(client.messages.create).parameters` contains every key `LlmClient` sends: `model`,
  `max_tokens`, `system`, `messages`, `output_config`. The sent-key set is taken from a fake-SDK call of the real
  `LlmClient.generate`, not hard-coded, so a new kwarg is checked automatically.
- (b) included: real `AsyncAnthropic(api_key="x", max_retries=0, http_client=httpx.AsyncClient(transport=httpx.MockTransport(handler)))`
  injected as `sdk`. Handler asserts the JSON body (`output_config.effort == "low"`, `max_tokens == sent`, no
  `temperature`/`top_p`, `system` string, one user message) and returns a canned Messages JSON (thinking + text
  blocks, `stop_reason`, `usage`), so SDK parsing of `usage`/`stop_reason`/content is covered. A second handler
  returns 429 -> `RateLimited`.
- (c) required before merge, human: smoke with a real `ANTHROPIC_API_KEY` (see section 3 step 7).

| Test file | Cases | ACs |
|---|---|---|
| `tests/test_llm_client.py` (renamed in step 2) | call kwargs exact (model per role, system, single user message, `output_config`, `max_tokens == sent_max_tokens(budget)` = budget+headroom, no temperature/top_p keys); `sent_max_tokens` = budget in mock, budget+headroom in real; thinking block ignored; in+out tokens; usage None / partial -> None; 429 -> `RateLimited` + counter +1; SDK `APITimeoutError` and `wait_for` timeout -> `TimeoutError`; `APIConnectionError` and 4xx propagate unchanged; refusal / max_tokens / empty -> `LlmIncomplete` + counter; `LlmIncomplete` args/str contain no partial text; missing key real -> `RuntimeError`, SDK never constructed; `LlmClient()` mock via `AI_MOCK` and via alias `GEMINI_MOCK`; mock makes no SDK call (sdk object that raises on access); `LlmClient` has no `embed` attribute; `len(Prompt) == len(system)+len(user)` | AC-1..AC-3, AC-5..AC-8, AC-10 |
| `tests/test_llm_sdk_contract.py` (new) | P-2 (a) signature check, (b) MockTransport body + parse + 429 | AC-1, AC-2, AC-5, AC-7 |
| `tests/test_llm_prompt.py` | injection strings only inside `<du_lieu>` in `messages[0].content`, `system` == prompt file text exactly, for chat and moderation | AC-4 |
| `tests/test_moderation_parse.py` (or extend `test_moderation_consumer.py`) | plain JSON, fenced JSON, prose, JSON list, bad score -> NEEDS_HUMAN; refusal -> NEEDS_HUMAN delivered once, no retry; `create(model=ANTHROPIC_MODEL_MODERATION)` through the guarded path with a limiter **and** with `limiter=None` | AC-10, AC-11 |
| `tests/test_embedder.py` | Gemini embed with fake; `make_embedder` -> `None` in real mode w/o key, `MockEmbedder` in mock, `GeminiEmbedder` with key | AC-12, AC-13 |
| `tests/test_semantic_cache.py` (extend) | `SemanticCache(redis, None)`: L1 hit works, miss returns None, `put` writes key, no vector; driven 100 times (get+put) -> caplog has zero records at any level | AC-13 |
| `tests/test_guarded.py` (update) | fakes implement `sent_max_tokens` explicitly; reservation == `len(prompt)//3 + sent + 256`; refund in+out; None -> no refund; 429 -> full refund; `LlmIncomplete` -> no refund; **real `LlmClient` (real mode, fake SDK) through `guarded_generate`**: `limiter.acquire(est)` with `est == len(prompt)//3 + llm.sent_max_tokens(b) + 256` and `create(max_tokens=llm.sent_max_tokens(b))` | AC-3, AC-6, AC-7 |
| `tests/test_chat_handler.py` (update) | `LlmIncomplete` and `TimeoutError` -> `source="fallback"`, cache.put and flight.publish not called; caplog of the fallback has no partial text | AC-8, AC-10 |
| `tests/test_classify_real_errors.py` (update) | genai cases replaced by anthropic real classes: 400/401/403/404 permanent; 408/429/500/529, timeout, connection temporary | AC-9 |
| `tests/test_config.py` (update) | defaults; Anthropic key required (message); empty model rejected; headroom <0 rejected; aliases honoured; new name wins in one source; real without Gemini key valid; `repr` hides keys; `deprecated_aliases_used` from `os.environ`, from a tmp `.env` file, case-insensitive, empty when `env_file=None`, empty when new name also set; `resolve_mock_flag` cases; no pydantic deprecation warning | AC-14, AC-15 |
| `tests/test_wiring.py` (new; replaces the lifespan test planned in `test_main.py`) | `build_llm` real mode w/o Gemini key -> embedder `None`; mock -> `MockEmbedder`; `log_startup` with caplog: exactly one L3-disabled line in real+None, none in mock or with key; one warning per deprecated alias, none when list empty; no API key value anywhere in caplog; ready-log fields present | AC-13, AC-15 |
| integration tests (mock) | mechanical import/kwarg rename only; `SemanticCache(redis, gemini)` -> `SemanticCache(redis, MockEmbedder())` | AC-20 |
| `deploy/observability/alerts_test.yml` | `LlmRateLimited` fires | AC-18 |

Mechanical updates to existing tests (rename imports/kwargs, `MockEmbedder()` in integration) are listed in
`test-change-request.md` before the freeze. Mutation targets: stop-reason checks, token sum, `max_retries=0`,
`sent_max_tokens` headroom, alias precedence, `.env` alias scan.

## 3. Order

1. Dependency (row 1) + lock-diff evidence. Commit alone (easy revert, Trivy run).
2. `git mv app/gemini app/llm` + `git mv tests/test_gemini_client.py tests/test_llm_client.py` + mechanical
   import updates; all tests green (pure rename commit).
3. Write new/updated tests (section 2), confirm red; freeze.
4. Config (row 3) -> metrics (8) -> embedder + cache (5, 6) -> client (4) -> guarded (7) -> consumers (9) ->
   wiring + main (10).
5. Compose/env/alerts/README/comments (11-14).
6. Gates: `uv sync --locked`, `ruff`, `pytest`, `pytest -m integration`, promtool, `docker compose config`
   (three env variants of row 11), rollback check (section 5).
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
- Rollback check in verify (executable): `git worktree add ../evf-rollback c5fb598`; in it run
  `docker compose -f deploy/compose/docker-compose.yml --env-file <new .env.example> config` (must succeed), then
  `cd services/ai-worker && uv run python -c "from app.config import Settings; from app.gemini.client import GeminiClient; s=Settings(); c=GeminiClient(); print(s.gemini_mock, c.mock)"`
  with the new `.env.example` values exported; expect mock mode and no exception. Record output; remove the
  worktree.

## 6. Round 2 changes (resolution of plan-review.md)

| Finding | Resolution |
|---|---|
| P-1 (major) | Lifespan test dropped. Pure `build_llm(settings)` and `log_startup(settings, embedder, deprecated=...)` in `app/llm/wiring.py` (row 10), tested with caplog in `tests/test_wiring.py`. Zero-log case: `SemanticCache(redis, None)` driven 100 times in `test_semantic_cache.py`. |
| P-2 (major) | (a) no-network `inspect.signature` test of the real `AsyncMessages.create` for every sent kwarg (`model, max_tokens, system, messages, output_config`), key set derived from a real `generate` call; (b) `httpx.MockTransport` test through the real SDK (body + parsing + 429); (c) real-key human smoke is a **required pre-merge** step, result in evidence (section 3 step 7). |
| P-3 (major) | Single `LlmClient.sent_max_tokens(budget)` used by `generate()` and `guarded_generate`; test that permit estimate == `len(prompt)//3 + sent + 256` and `create(max_tokens=sent)` with a real client; fakes implement the method, no `getattr` default. |
| P-4 (major) | `SemanticCache(redis, embedder: Embedder | None)`; main passes `make_embedder(settings)`; `LlmClient` has no `embed()` (tested); integration tests use `MockEmbedder()`; wiring test asserts embedder `None` in real mode w/o Gemini key. |
| P-5 | Fixed: alias scan covers `os.environ` + `dotenv_values(env_file)`, case-insensitive, skipped for `_env_file=None`; `validate_by_name=True`; tests for both sources. Cross-source precedence (process `GEMINI_MOCK` beats `.env` `AI_MOCK`) accepted and documented in `.env.example`. |
| P-6 | Fixed: compose `AI_MOCK: "${AI_MOCK:-${GEMINI_MOCK:-1}}"`, `AI_RPM`/`AI_TPM` overridable; `docker compose config` checked with three env variants. |
| P-7 | Fixed: `uv add`/`uv lock` without `--upgrade`; evidence records `uv --version`, locked `anthropic`, adds-only lock diff with pinned existing versions. |
| P-8 | Fixed: autouse env-clearing fixture in `tests/conftest.py`; explicit `api_key` to the SDK; `resolve_mock_flag` shared helper. |
| P-9 | Fixed as evidence: smoke records `usage.input_tokens` vs `len(prompt)//3`; ratio > 1 -> follow-up F-5; `ai_llm_429_total` / `LlmRateLimited` is the production signal. |
| P-10 | Fixed: no `model=` in the moderation partial; `scan_moderation` alone passes the moderation model; chat passes none; test with limiter present and `None`. |
| P-11 | Fixed: (a) header says approved; (b) `estimate_tokens` receives the `Prompt`, hint `str | Prompt`, `len(Prompt)` tested; (c) `LlmIncomplete` carries only the reason, asserted in client and chat tests. |
| P-12 | Fixed: row 14 lists comment updates and the deliberate leaves (Lua header, frozen test string). |
| P-13 | Recorded in section 4 as an open, non-blocking question for the human, with merge order if the branch is live. |
| P-14 | Fixed: (a) executable rollback check via worktree at c5fb598 (section 5); (b) test-file `git mv` moved into the step 2 rename commit, before the freeze. |
| P-15 | Fixed: `generate()` catches only `RateLimitError` and `APITimeoutError`/`asyncio.TimeoutError`; no broad connection/status catch; real-class timeout and connection tests kept. |
