# Plan: ai-anthropic (draft, step 2b)

Inputs: `spec.md` (AC-1..AC-20, choices C1-C6; **pending human approval**). Base develop @ c5fb598.
One implementer, sequential (files are shared: client, config, main). No production code before approval.

## 1. Files and changes

| # | File | Change | ACs |
|---|---|---|---|
| 1 | `services/ai-worker/pyproject.toml`, `uv.lock` | add `anthropic>=<locked>`; description text "Consumer AI (Anthropic)"; `uv lock`; keep `google-genai` | AC-16 |
| 2 | `app/gemini/` -> `app/llm/` (`git mv`) | `client.py`, `guarded.py`, `limiter.py` keep history; docstrings say "LLM" | C3 |
| 3 | `app/config.py` | fields: `ai_mock: bool = True` (`validation_alias=AliasChoices("AI_MOCK","GEMINI_MOCK")`), `anthropic_api_key: SecretStr`, `anthropic_model_chat`/`_moderation: str = "claude-haiku-5-5"` (`min_length=1`), `ai_timeout_seconds` (alias `GEMINI_TIMEOUT_SECONDS`), `ai_rpm`/`ai_tpm` (aliases `GEMINI_RPM/TPM`), `ai_thinking_headroom_tokens: int = Field(1024, ge=0)`, `gemini_api_key: SecretStr`, `gemini_embed_model`; drop `gemini_model_fast/deep`, `gemini_mock`. Validator: mock off + empty Anthropic key -> `ValueError("AI_MOCK=0 nhung thieu ANTHROPIC_API_KEY")`. Helper `deprecated_aliases_used() -> list[str]` (reads `os.environ`: alias set and new name unset) for the startup warning. `populate_by_name=True` so tests can pass field names. | AC-14, AC-15 |
| 4 | `app/llm/client.py` | `Prompt(system: str, user: str)` dataclass (`__len__` = combined chars, used by `estimate_tokens`). `LlmClient(api_key, *, mock, chat_model, moderation_model, timeout_seconds, thinking_headroom, sdk=None)`. `build_prompt(job, tools) -> Prompt` (system = assistant.v3.md; user = wrapped context + wrapped question). `generate(prompt, *, max_output_tokens, model=None)`: mock path as today; real: `max_tokens = max_output_tokens + headroom`, `asyncio.wait_for(sdk.messages.create(model, max_tokens, system, messages=[user], output_config={"effort":"low"}), timeout)`; map `RateLimitError` -> metric + `RateLimited`, `APITimeoutError` -> `TimeoutError`; check `stop_reason` before content: `refusal`/`max_tokens` -> `LlmIncomplete`; join `type=="text"` blocks, blank -> `LlmIncomplete("empty")`; `total_tokens` = in+out when both int else `None`. `scan_moderation`: build `Prompt(moderation.v1.md, wrapped title+desc)`, `generate(..., model=moderation_model)`, `LlmIncomplete` -> NEEDS_HUMAN result (AC-10 reason), strip one code fence, `json.loads`, require dict, fallback as today (add `TypeError`/`AttributeError` to the caught set). `_sdk()` lazily `anthropic.AsyncAnthropic(api_key=..., timeout=..., max_retries=0)`. `embed()` delegates to the embedder (row 5) so `SemanticCache(redis, llm)` call sites keep working. | AC-1..AC-11 |
| 5 | `app/llm/embedder.py` (new) | `MockEmbedder` (wraps `mock_embedding`, moved here and re-exported from client), `GeminiEmbedder(api_key, model, timeout, sdk=None)` (lazy `google.genai.Client`, `aio.models.embed_content`), `make_embedder(settings-like) -> Embedder | None` (None when real mode and empty key). | AC-12, AC-13 |
| 6 | `app/pipeline/semantic_cache.py` | `embedder` may be `None`: `get` returns after L1, `put` writes the exact key only; no log. No other change. | AC-13 |
| 7 | `app/llm/guarded.py` | `guarded_generate(limiter, llm, prompt, *, max_output_tokens, model=None)`: estimate on `max_output_tokens + llm.thinking_headroom` (0 in mock) via unchanged `estimate_tokens(len(prompt), ...)`-compatible call; refund rules unchanged; `LlmIncomplete` keeps the "no refund" default. | AC-3, AC-6, AC-7 |
| 8 | `app/metrics.py` | `RATE_LIMITED` -> `ai_llm_429_total`; add `INCOMPLETE = Counter("ai_llm_incomplete_total", ..., ["reason"])` | C6, AC-7, AC-10 |
| 9 | `app/consumers/chat.py`, `moderation.py` | imports `app.llm.*`; kwarg/attr `gemini` -> `llm`; `_call_gemini` -> `_call_llm`; chat keeps catching non-`Retry` exceptions -> FAQ fallback (covers `LlmIncomplete`, `TimeoutError`); moderation `functools.partial(guarded_generate, limiter, llm, model=...)`. Log texts "goi gemini" -> "goi llm". | AC-8, AC-10 |
| 10 | `app/main.py` | lifespan only (+ import lines 31-32): build `LlmClient` from settings, `embedder = make_embedder(settings)`; if real and `None` -> log once "semantic cache L3 disabled (GEMINI_API_KEY empty)"; warn once per deprecated alias; `SemanticCache(redis, embedder)`; limiter from `ai_rpm/ai_tpm`; ready log `ai_mock`, `provider`, models, `semantic_l3`. Do **not** touch `/admin/mode`, `_signals_now`, `gemini_circuit_open` (ops-warroom hunks). | AC-13..AC-15 |
| 11 | `deploy/compose/docker-compose.yml`, `.env.example` | env per AC-17 | AC-17 |
| 12 | `deploy/observability/alerts.yml`, `alerts_test.yml` | rename metric/alert; add a promtool test case | AC-18 |
| 13 | `README.md` | lines 19, 43, 82, 99, 131, 133 | AC-19 |
| 14 | `docs/workflow/ai-anthropic/*` | traceability, baseline, evidence (locked SDK version, gate output) | - |

Not touched: `app/scripts/*.lua`, `app/pipeline/degradation.py`, `app/api/*`, `apps/web`, `deploy/k8s` (absent on
develop), design docs.

## 2. Tests (written first, red on c5fb598; no network, no keys)

Fake SDK: `FakeMessages.create(**kw)` records kwargs, returns
`SimpleNamespace(content=[SimpleNamespace(type="thinking", ...), SimpleNamespace(type="text", text=...)],
stop_reason=..., usage=SimpleNamespace(input_tokens=.., output_tokens=..))` or raises; injected as
`sdk=SimpleNamespace(messages=FakeMessages())`. SDK errors built from real classes:
`anthropic.RateLimitError("x", response=httpx.Response(429, request=req), body=None)` (same for
`APIStatusError` 400/401/404/500/529), `anthropic.APITimeoutError(request=req)`,
`anthropic.APIConnectionError(request=req)`. Fake embed SDK as in today's `FakeModels.embed_content`.

| Test file | Cases | ACs |
|---|---|---|
| `tests/test_llm_client.py` (replaces `test_gemini_client.py`, `git mv`) | call kwargs exact (model per role, system, single user message, `output_config`, `max_tokens`=budget+headroom, no temperature/top_p keys); thinking block ignored; in+out tokens; usage None / partial -> None; 429 -> `RateLimited` + counter +1; SDK timeout and `wait_for` timeout -> `TimeoutError`; refusal / max_tokens / empty -> `LlmIncomplete` + counter; 4xx/connection propagate; missing key real -> RuntimeError; `LlmClient()` mock via `AI_MOCK` and via alias `GEMINI_MOCK`; mock makes no SDK call (sdk object that raises on access) | AC-1..AC-3, AC-5..AC-8, AC-10 |
| `tests/test_llm_prompt.py` | injection strings only inside `<du_lieu>` in `messages[0].content`, `system` == prompt file text exactly, for chat and moderation | AC-4 |
| `tests/test_moderation_parse.py` (or extend `test_moderation_consumer.py`) | plain JSON, fenced JSON, prose, JSON list, bad score -> NEEDS_HUMAN; refusal -> NEEDS_HUMAN delivered once, no retry | AC-10, AC-11 |
| `tests/test_embedder.py` | Gemini embed with fake; `make_embedder` None in real mode w/o key; mock -> deterministic | AC-12, AC-13 |
| `tests/test_semantic_cache.py` (extend) | `embedder=None`: L1 hit works, miss returns None, `put` writes key, no vector, caplog empty | AC-13 |
| `tests/test_guarded.py` (update) | reservation uses budget+headroom; refund in+out; None -> no refund; 429 -> full refund; `LlmIncomplete` -> no refund | AC-3, AC-6, AC-7 |
| `tests/test_chat_handler.py` (update) | `LlmIncomplete` and `TimeoutError` -> `source="fallback"`, cache.put and flight.publish not called | AC-8, AC-10 |
| `tests/test_classify_real_errors.py` (update) | genai cases replaced by anthropic real classes: 400/401/403/404 permanent; 408/429/500/529, timeout, connection temporary | AC-9 |
| `tests/test_config.py` (update) | defaults; Anthropic key required (message); empty model rejected; headroom <0 rejected; aliases honoured; new name wins; real without Gemini key valid; `repr` hides keys | AC-14, AC-15 |
| `tests/test_main.py` (extend) | lifespan with fakes in real mode w/o Gemini key: one L3-disabled line, no key in caplog, alias warning once | AC-13, AC-15 |
| integration tests (mock) | mechanical import/kwarg rename only | AC-20 |
| `deploy/observability/alerts_test.yml` | `LlmRateLimited` fires | AC-18 |

Mechanical updates to existing tests (rename imports/kwargs) are listed in `test-change-request.md` before the
test freeze. Mutation targets: stop-reason checks, token sum, `max_retries=0`, headroom addition, alias precedence.

## 3. Order

1. Dependency (row 1) + `uv lock` — commit alone (easy revert, Trivy run).
2. `git mv app/gemini app/llm` + mechanical import updates, all tests green (pure rename commit).
3. Write new/updated tests (section 2), confirm red; freeze.
4. Config (row 3) -> metrics (8) -> embedder + cache (5, 6) -> client (4) -> guarded (7) -> consumers (9) -> main (10).
5. Compose/env/alerts/README (11-13). 6. Gates: `uv sync --locked`, `ruff`, `pytest`, `pytest -m integration`,
   promtool, `docker compose config`; one manual smoke with a real key (human, optional, not in CI).
Commit and push after each step.

## 4. Conflict handling

- ops-warroom: merge it first if it lands earlier; our `main.py` hunks (imports 31-32, lifespan 70-132) are
  disjoint from its hunks (13-25, 166-220). Re-run `git merge-tree` before PR.
- k8s-all: untouched; aliases keep its `GEMINI_*` env working (spec F-1).

## 5. Rollback

- Code: revert the merge commit on develop (single PR); no data/migration/queue change, Redis keys unchanged
  (`ai:cache:*`, `ai:vec:*`, limiter keys), so old and new workers can share Redis.
- Ops fast path without redeploying old code: `AI_MOCK=1` (assistant serves FAQ/mock, moderation keyword mock).
- Env: aliases mean an old `.env` still starts the new worker; old code reverted onto the new env sees no `GEMINI_MOCK`
  and defaults to mock (safe); real mode on old code needs `GEMINI_MOCK=0` + `GEMINI_API_KEY` again.
- Rollback check in verify: checkout c5fb598 tests against the new compose env, confirm the old worker still
  starts in mock mode.
