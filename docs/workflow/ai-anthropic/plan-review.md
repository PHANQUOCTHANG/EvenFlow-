# Plan review: ai-anthropic (v3 step 2b, round 1)

Reviewer: plan reviewer (clean context). Inputs: `spec.md` (approved 2026-11-20), `plan.md`, code at
develop @ c5fb598 (`services/ai-worker`, compose, `.env.example`, `deploy/observability/alerts*.yml`),
`git diff origin/develop origin/feature/ops-warroom -- services/ai-worker`.

## Verdict: **FAIL (round 1)**: 0 blockers, 4 majors, 11 minors

The plan is mostly sound. Every AC has a step, the order is safe (dependency, then a pure rename, then tests
and freeze, then code), the rollback path is real and the ops-warroom overlap is disjoint. Four majors affect
whether the High-risk ACs can be tested or will hold in production. Each one needs a plan edit only, and round
2 should be quick.

## Checks performed

| Check | Result |
|---|---|
| AC -> plan step coverage (AC-1..AC-20) | all mapped (rows 1-14 + test table). AC-13/AC-15 test is not feasible as written (P-1) |
| File list vs `grep -rni gemini` | functional files complete. Comment-only mentions not listed (P-12) |
| Order | safe. Rename commit before freeze is fine if the `git mv` of the test file is in step 2 (P-14) |
| `git mv app/gemini -> app/llm` | no importer outside `services/ai-worker/{app,tests}` (repo-wide grep empty). ops-warroom tests import only `app.main` and read `signals.gemini_circuit_open` (kept). OK |
| ops-warroom conflict | branch changes `main.py` hunks @13-25 (imports re/datetime/Request/JSONResponse) and @166-220 (mode API). Plan touches imports 31-32 + lifespan 70-132. Disjoint. OK. A different stale branch overlaps (P-13) |
| pydantic-settings aliases | feasible: pydantic 2.13.5 / pydantic-settings 2.15.0 locked. `AliasChoices("AI_MOCK","GEMINI_MOCK")` gives the first alias found in a source, so the new name wins. Warning detection gap (P-5) |
| Limiter accounting vs Lua | Lua refund only adds `delta = reserved - actual` when > 0 (`limiter.py` refund). Rules unchanged. Headroom has two sources of truth (P-3). Tokenizer under-estimate risk (P-9) |
| Embedding-disabled path | `SemanticCache` change is right, but `LlmClient.embed()` "delegates" contradicts `SemanticCache(redis, embedder)` (P-4) |
| uv.lock approach | `uv lock` is correct. Needs a guard against upgrades (P-7) |
| No real network in tests | fakes injected. Env leakage and SDK env-key fallback not guarded (P-8) |
| Rollback | sound (no Redis/queue/migration change, `AI_MOCK=1` fast path). Verify step vague (P-14) |

## Findings

### P-1 (major): the lifespan test in `test_main.py` cannot run as planned
`lifespan()` does `redis.ping()`, `aio_pika.connect_robust()`, topology declare and `runtime.start()`. The
current `test_main.py` has no lifespan test, and unit tests have no Redis/RabbitMQ. So "lifespan with fakes in
real mode w/o Gemini key: one L3-disabled line, no key in caplog, alias warning once" (AC-13, AC-15) would
need heavy monkeypatching or would slip into integration.
**Fix:** in row 10, extract a pure function, e.g.
`build_llm(settings) -> tuple[LlmClient, Embedder | None]` plus `log_startup(settings, embedder)` (L3-disabled
line + one warning per deprecated alias + "san sang" fields). Lifespan only calls them. Unit-test those
functions with `caplog` (exactly one L3 line, no key value, alias warning once). For "100 chat jobs -> zero
embed log lines", use `SemanticCache(redis, None)` driven 100 times in `test_semantic_cache.py`.

### P-2 (major): the fake SDK hides real-SDK argument errors, and the real call is never exercised
`FakeMessages.create(**kw)` accepts anything. If the locked `anthropic` version rejects `output_config`
(or any kwarg) with `TypeError`, the chat path turns every real request into a silent FAQ fallback
(`except Exception`), and all tests stay green. Moderation would go NEEDS_HUMAN / DLQ. For a High-risk
provider swap, the only real call is an "optional" manual smoke.
**Fix:** (a) add a no-network test that inspects the installed SDK:
`inspect.signature(anthropic.AsyncAnthropic(api_key="x", max_retries=0).messages.create).parameters`
contains every key the client sends (`model, max_tokens, system, messages, output_config`). Constructing the
client makes no request. (b) Optional: a test that sends the kwargs through a real `AsyncAnthropic` with an
`httpx.MockTransport` (`http_client=httpx.AsyncClient(transport=...)`). It asserts the JSON body
(`output_config.effort == "low"`, no `temperature`) and returns a canned Messages response, so SDK parsing of
`usage`/`stop_reason`/content blocks is also covered without network. (c) In verify, make the human smoke with
a real key **required before merge** (one chat + one moderation, evidence: model id, `stop_reason`, usage,
latency), not optional.

### P-3 (major): thinking headroom is added in two places
Row 4 adds headroom inside `generate()`. Row 7 adds `llm.thinking_headroom` again in `guarded_generate` to
estimate the permit. AC-3 requires reservation == sent `max_tokens` (+ prompt estimate). Two independent
additions can drift: a fake without the attribute, mock vs real, a later edit. The existing `FakeGen` in
`test_guarded.py` has no `thinking_headroom`.
**Fix:** one source, e.g. `LlmClient.sent_max_tokens(budget: int) -> int` (returns `budget` in mock,
`budget + headroom` in real). `guarded_generate` calls it for `estimate_tokens(prompt, llm.sent_max_tokens(b))`,
and `generate()` uses it for the `max_tokens` kwarg. Add a test that runs a real `LlmClient` with a fake SDK
through `guarded_generate` and asserts `limiter.acquire(est)` used exactly `len(prompt)//3 + sent + 256` and
`create(max_tokens=sent)`. Fakes in `test_guarded.py` must implement the method explicitly (no
`getattr(..., 0)` default that silently under-reserves).

### P-4 (major): the embedding wiring is contradictory and could spam logs
Row 4 says `LlmClient.embed()` "delegates to the embedder so `SemanticCache(redis, llm)` call sites keep
working". Row 10 wires `SemanticCache(redis, embedder)`. If any call site keeps passing `llm` in real mode
without `GEMINI_API_KEY`, `embed()` has nothing to delegate to. It raises, and `SemanticCache.get` logs
`log.exception("embed that bai ...")` on **every** request, breaking AC-13.
**Fix:** choose one design and state it. Recommended: `SemanticCache` takes `Embedder | None`. Main passes
`make_embedder(settings)`. `LlmClient` drops `embed()`, or keeps it in mock only for the integration tests
(`tests/integration/test_*_integration.py` build `SemanticCache(redis, gemini)`), and those become
`SemanticCache(redis, MockEmbedder())` in the mechanical update. If `embed()` is kept, it must raise only in
mock-less/embedder-less mode and never be wired there. Add a test that `main` wiring with no Gemini key gives
`cache._embedder is None`.

### P-5 (minor): deprecated-alias detection reads `os.environ` only
`Settings` uses `env_file=".env"`, and dev `.env` files still carry `GEMINI_MOCK=` etc. (`.env.example`).
`deprecated_aliases_used()` scanning `os.environ` misses the `.env` case, so no warning appears in local
dev. Also note cross-source precedence: `GEMINI_MOCK` in the process env beats `AI_MOCK` in `.env` (env
source has priority over dotenv). Document this or accept it.
**Fix:** check `os.environ` and `dotenv_values(env_file)` (python-dotenv is already a pydantic-settings dep),
skipping the file when `_env_file=None`, case-insensitive. Test both. Use `validate_by_name=True` (pydantic
>= 2.11 name) rather than `populate_by_name`, or check that no deprecation warning shows under pytest.

### P-6 (minor): compose drops the old flag silently
With `AI_MOCK: "${AI_MOCK:-1}"`, a developer whose root `.env` still has `GEMINI_MOCK=0` gets mock mode with no
warning, because compose no longer forwards `GEMINI_MOCK` and the pydantic alias never sees it. It fails safe
but surprises.
**Fix:** `AI_MOCK: "${AI_MOCK:-${GEMINI_MOCK:-1}}"` (nested defaults are supported by Compose v2) and
`AI_RPM: "${AI_RPM:-55}"`, `AI_TPM: "${AI_TPM:-900000}"` so operators can tune per Q-1 without editing YAML.
Check with `docker compose config`.

### P-7 (minor): pin down the `uv` command and the lock-diff check
`uv lock` after editing `pyproject.toml` is correct. `uv add "anthropic>=X.Y"` is equivalent and also writes
the bound. Never use `--upgrade`. `anthropic` brings new transitive packages (e.g. `distro`, `jiter`,
`docstring-parser`), which is expected.
**Fix:** in evidence, record `uv --version`, the locked `anthropic` version, and a check that the `uv.lock`
diff only **adds** packages: no version change for existing ones (`httpx 0.28.1`, `pydantic 2.13.5`,
`google-genai 2.29.0`). Read AC-16 "no other dependency changes" as "no other direct deps, no bumps".

### P-8 (minor): test hygiene for "no network, no keys"
`LlmClient()` reads env, and `anthropic.AsyncAnthropic(api_key=None)` silently falls back to the
`ANTHROPIC_API_KEY` env var. A developer with a real key exported plus a test that builds the lazy client could
make a real call.
**Fix:** an autouse fixture in `tests/conftest.py` that `monkeypatch.delenv`s `ANTHROPIC_API_KEY`,
`GEMINI_API_KEY`, `AI_MOCK`, `GEMINI_MOCK`, `AI_*`, `GEMINI_*`. The client always passes `api_key` explicitly.
Put the mock-flag resolution (`AI_MOCK` over `GEMINI_MOCK`, default `1`) in one helper shared by `Settings`
docs and `LlmClient()`, so the two rules cannot diverge.

### P-9 (minor): token estimate vs the Claude tokenizer
`estimate_tokens = len//3 + max_out + 256` was tuned for Gemini. If Claude counts Vietnamese text above
`len/3`, real usage exceeds the reservation. Refund then is a no-op (`delta <= 0`) and the limiter over-admits,
which shows up as real 429s. The formula is frozen by the spec, so this is evidence only.
**Fix:** in the required smoke (P-2c), record `usage.input_tokens` vs `len(prompt)//3` for one Vietnamese chat
prompt. If the ratio is > 1, open a follow-up (F-5). `ai_llm_429_total` / `LlmRateLimited` is the production
signal.

### P-10 (minor): moderation `model=` bound twice
Row 9 binds `functools.partial(guarded_generate, limiter, llm, model=...)`, and row 4 has `scan_moderation`
pass `model=moderation_model` too. `partial` lets the call override, so it works, but the role-to-model rule
then lives in two places.
**Fix:** bind no model in the partial. `scan_moderation` always passes `model=self._moderation_model`, and chat
passes nothing (default chat model). The test asserts `create(model=ANTHROPIC_MODEL_MODERATION)` through the
guarded path, with the limiter both present and absent (`limiter=None` -> `self.generate`).

### P-11 (minor): plan text and type nits
(a) The header still says "pending human approval". The spec is approved, so update it. (b)
`estimate_tokens(prompt: str, ...)` takes a string, and row 7's "`estimate_tokens(len(prompt), ...)`-compatible"
is ambiguous. State "pass the `Prompt` (`__len__` = len(system)+len(user)), signature hint becomes
`str | Prompt`", and test that `len(Prompt) == len(system) + len(user)`. (c) `LlmIncomplete(reason)` must not carry the partial text in its message or args,
because chat does `log.exception(...)` (AC-10, AC-15). Add an assertion.

### P-12 (minor): comment-only Gemini mentions not listed
`app/api/assistant.py:3`, `app/mq/protocol.py:85`, `app/pipeline/singleflight.py:4,27`,
`app/pipeline/degradation.py:33`, `app/scripts/limiter.lua` header, `alerts.yml:46,61` comments, compose header
line 4, `tests/mq/test_runtime.py:60` string.
**Fix:** say in row 14 / traceability which are updated (alerts comments + compose header are in AC-17/18
scope) and which are deliberately left (Lua: spec forbids touching it, so leave the comment).

### P-13 (minor): another unmerged branch overlaps
`origin/bugfix/EV-120-ai-verdict-callback` (last commit 2026-10-22, 3 commits not in develop) touches
`config.py`, `consumers/moderation.py`, `main.py`, `test_config.py` and adds `test_compose_config.py`. All of
these are files this plan rewrites.
**Fix:** before implementation, confirm with the human that it is abandoned or squash-merged. If it is live,
record the conflict and merge order in section 4.

### P-14 (minor): rollback check and freeze timing
(a) "checkout c5fb598 tests against the new compose env" is not executable as written.
**Fix:** in a worktree at c5fb598, run `docker compose config` with the new `.env.example`, start ai-worker
alone (or `Settings()` + `GeminiClient()` in a Python one-liner with the new env) and assert mock mode with no
startup failure. (b) Do the `git mv tests/test_gemini_client.py tests/test_llm_client.py` in step 2 (pure
rename commit), before the freeze. Otherwise the freeze filter, which locks only files that existed at the
freeze commit, would treat the renamed file as unlocked.

### P-15 (minor): timeout exception order
`anthropic.APITimeoutError` subclasses `APIConnectionError`, and `RateLimitError` subclasses `APIStatusError`.
**Fix:** in `generate()`, catch `RateLimitError` and `APITimeoutError` explicitly and let everything else
propagate. Do not add a broad `except APIConnectionError` above the timeout clause. The AC-8 test with a real
`APITimeoutError(request=req)` covers it, so keep it.

## Required for round 2
Plan edits for P-1..P-4. Minors may be addressed in the same edit or recorded as accepted.
