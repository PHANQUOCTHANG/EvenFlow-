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

## Round 2

Reviewer: plan reviewer (clean context). Inputs: `spec.md` (approved), `plan.md` round 2 (section 6), code at
c5fb598, and the untracked test-designer files in `services/ai-worker/tests/llm/` (`conftest.py`,
`test_client_anthropic.py`, `test_client_mock.py`, `test_consumers_anthropic.py`, `test_embeddings.py`,
`test_moderation_json.py`).

### Verdict: **FAIL (round 2)**: 1 blocker, 2 majors, 2 minors

The round-1 edits are real, not just mentioned: `sent_max_tokens` is the single headroom source in rows 4 and 7
and in the tests, `wiring.py` is a pure and testable split, the compose nested default is valid Compose v2
syntax, and the rollback one-liner runs on c5fb598 (`GeminiClient()` reads `GEMINI_MOCK` with default `"1"`,
and `Settings` has `extra="ignore"`). The P-4 fix, however, goes against the approved spec and the tests that
are already written. The plan and the tests also disagree on the constructor and on the file inventory, and
this must be settled before the freeze.

### Round-1 findings

| Finding | Status | Note |
|---|---|---|
| P-1 | resolved | `build_llm`/`log_startup` are pure. The 100x `SemanticCache(redis, None)` case is planned. |
| P-2 | resolved in plan | (a)/(b)/(c) are specified. No `test_llm_sdk_contract.py` exists yet (see Q-3). |
| P-3 | resolved | One `sent_max_tokens`, used by `generate`, `guarded_generate` and the tests. The designer tests use `>=`, and the plan's exact-equality test is stricter (keep it). |
| P-4 | **reopened as Q-1** | "No `embed()`" contradicts spec AC-1/AC-12 and the designer tests. |
| P-5 | resolved | `os.environ` + `dotenv_values`, case-insensitive, `validate_by_name`. Precedence is documented. |
| P-6 | resolved | `${AI_MOCK:-${GEMINI_MOCK:-1}}` is valid. `:-` treats an empty `AI_MOCK=` as unset, which is intended. |
| P-7 | resolved | |
| P-8 | partial (Q-3) | The fixture exists only in `tests/llm/conftest.py`, not `tests/conftest.py`. |
| P-9, P-10, P-11, P-12, P-13 | resolved | |
| P-14 | resolved | See Q-5 (minor wording). |
| P-15 | resolved | |

### New findings

**Q-1 (blocker): the P-4 design breaks the approved spec.** AC-1 says "`embed()` returns `mock_embedding`".
AC-12 says "`embed()` calls `google.genai` `embed_content` with the same timeout". Spec section 5 says the fakes
are injected via `sdk=` / `embed_sdk=`. The designer tests call `LlmClient(...).embed()` with `gemini_api_key=`,
`embed_model=` and `embed_sdk=` (`test_embeddings.py` AC-12, `test_client_mock.py::test_AC1_MockEmbed_*`).
Plan row 4 says "No `embed()` method", and the test table asserts "`LlmClient` has no `embed` attribute".
**Fix:** keep `LlmClient.embed()`, delegating to an internal `Embedder | None` built in `__init__` from
`gemini_api_key`, `embed_model` and `embed_sdk` (through `make_embedder`). Build `GeminiEmbedder` lazily, so no
`genai.Client` is constructed when the key is empty. Expose `llm.embedder`. `build_llm` returns
`(llm, llm.embedder)`, and main wires `SemanticCache(redis, llm.embedder)`, so the cache never receives `llm`
and the P-4 log-spam risk stays closed. If `embed()` is called with no embedder, it raises `RuntimeError`; it is
never wired that way. Drop the "no `embed` attribute" test. Integration tests may keep `SemanticCache(redis, llm)`
in mock mode, or use `MockEmbedder()`; pick one and state it.

**Q-2 (major): the constructor contract differs from the tests.** The tests (`conftest.make_client`,
`test_AC2_TaoAsyncAnthropicLuoi_*`) use `thinking_headroom_tokens=`, while the plan uses `thinking_headroom`. The
tests also pass `gemini_api_key=`, `embed_model=` and `embed_sdk=`, which are not in row 4.
`test_AC1_AI_MOCK0_ThieuKey_TuChoi` expects `LlmClient()` with `AI_MOCK=0` and no key to raise **at
construction**, with "ANTHROPIC_API_KEY" in the message. Row 4 only raises lazily in `_sdk()`.
**Fix:** write the row 4 signature exactly as the tests use it:
`LlmClient(api_key, *, mock, chat_model, moderation_model, timeout_seconds, thinking_headroom_tokens, gemini_api_key, embed_model, sdk=None, embed_sdk=None)`.
Raise `RuntimeError("... ANTHROPIC_API_KEY ...")` in `__init__` when real mode has an empty key (as the
c5fb598 code does), and keep SDK construction lazy. State that `scan_moderation(job, *, generate=None)` takes
the bound guarded function by keyword (the tests use `generate=functools.partial(guarded_generate, lim, c)`).

**Q-3 (major): the test inventory in plan section 2 does not match the files that exist.** The plan names
`tests/test_llm_client.py`, `test_llm_sdk_contract.py`, `test_llm_prompt.py`, `test_moderation_parse.py`,
`test_embedder.py` and `test_wiring.py`, plus updates to `test_config.py`, `test_guarded.py`,
`test_classify_real_errors.py`, `test_semantic_cache.py` and `test_chat_handler.py`, and an autouse fixture in
`tests/conftest.py`. The test designer wrote `tests/llm/*` instead. No contract (P-2a/b), wiring, config/alias
(AC-14/15), classify (AC-9) or root-conftest tests exist yet. The renamed `tests/test_llm_client.py` (from
`test_gemini_client.py`) would hold genai `generate_content` tests that are now obsolete, next to
`tests/llm/test_client_anthropic.py`. The freeze locks only files that exist at the freeze commit, so any AC test
added after it is unlocked.
**Fix:** rewrite the section 2 table against the real paths, mapping each AC to a `tests/llm/*` file or a named
file still to be written. Before the freeze, require the designer to add the missing files: SDK contract,
wiring, config/aliases, classify, cache 100x, and exact guarded equality. Move the env-clearing autouse fixture
to `tests/conftest.py` so `test_config.py` and the others are covered. In `test-change-request.md`, state which
obsolete cases in `test_gemini_client.py`/`test_llm_client.py` are deleted or replaced.

**Q-4 (minor):** `tests/llm/conftest.py` clears an explicit list of variables, not every `AI_*` / `GEMINI_*` /
`ANTHROPIC_*` variable as the plan says. **Fix:** match by prefix in the root fixture.

**Q-5 (minor):** in the section 5 rollback, "with the new `.env.example` values exported" needs a command.
**Fix:** `set -a; . <worktree>/.env.example; set +a` (bash) before the `uv run` line, and run `uv sync --locked`
in the worktree first.

### Required for round 3
Plan edits for Q-1..Q-3 (rows 4, 5, 10 and the section 2 table). The test designer must align or add the test
files before the freeze. Q-4 and Q-5 can go in the same edit.

## Round 3

Reviewer: plan reviewer (clean context). Inputs: `plan.md` @ 5db4b44 (section 7), `test-change-request.md`
(TCR-1), and the tests frozen at 0763f2a (`tests/conftest.py`, `tests/llm/*`, `tests/test_llm_sdk_contract.py`,
`tests/test_wiring.py`). I also read the pre-existing tests that TCR-1 changes.

Human decision recorded: `origin/bugfix/EV-120-ai-verdict-callback` is abandoned, so **P-13 is closed**.

### Verdict: **PASS (round 3)**: 0 blockers, 0 majors, 2 minors

### Round-2 findings

| Finding | Status | Evidence |
|---|---|---|
| Q-1 (blocker) | resolved | Row 4 keeps `embed()`, which delegates to `llm.embedder` (row 5, built in `__init__` by `make_embedder`). `build_llm` returns `(llm, llm.embedder)`, and main wires `SemanticCache(redis, embedder)`, so production never hands `llm` to the cache. The "no embed attribute" test is gone. This matches spec AC-1/AC-12 and `tests/llm/test_embeddings.py` / `test_client_mock.py`. |
| Q-2 (major) | resolved | The row 4 constructor matches `conftest.make_client` and `test_AC2_TaoAsyncAnthropicLuoi_*`. `RuntimeError` with `ANTHROPIC_API_KEY` is raised in `__init__`, and the SDK stays lazy. `scan_moderation(job, *, generate=None)` passes `model=` on both paths. Row 7 forwards `model` only when it is not `None`, so the old fakes keep working. |
| Q-3 (major) | resolved | Section 2.1 maps every frozen file to its ACs. 2.2 limits mechanical renames to names, imports and metric names, with no assertions, values or fake shapes. 2.3 sends the rest to TCR-1, and the gate stays red until TCR-1 is approved. `git diff -M 0763f2a` is the check. |
| Q-4 | resolved | The root `_clear_ai_env` deletes by prefix (`AI_`, `GEMINI_`, `ANTHROPIC_`) for every test. |
| Q-5 | resolved | The rollback is a bash block: `uv sync --locked`, `set -a; . .env.example; set +a`, then the one-liner. |

### TCR-1 opinion: approve as written (11 items, minimal, no assertion weakened)

- **Items 1 to 5 and 7 are needed.** Generation moves off `google.genai` (spec D1), so the fake shape
  (`messages.create`, content blocks, `usage.input/output_tokens`) and the provider error classes have to change.
  The expected values are kept on purpose:
  - total 321 from 300+21, and 77 from 70+7;
  - `max_tokens == 192` with `thinking_headroom_tokens=0`;
  - `RateLimited`, `TimeoutError` and `None` tokens are unchanged;
  - "a non-429 4xx propagates" is kept, with `BadRequestError` in place of `ClientError`.

  The only literal changes are the model id (`gemini-test` -> `claude-test`) and the key name in the error
  message (`GEMINI_API_KEY` -> `ANTHROPIC_API_KEY`, spec D4/AC-14). Both are identifiers, not looser checks.
- **Item 6 is needed.** AC-12 moves embedding injection to `embed_sdk=`, and the vector and recorded-call
  assertions are kept.
- **Items 8 and 10 are needed** because of P-3 (no `getattr` default). `sent_max_tokens = budget` leaves every
  reservation and refund identical. This is the smallest fake change that can work.
- **Item 9 is needed.** Only `refunds[0][1] == 77`, `len(acquired)`, `models.calls` and the verdicts are
  asserted, never the reserved value, so the 1024 default headroom of the real client changes no expectation.
- **Item 11 is needed.** The field was renamed and the message is now spec-mandated.
- **Nothing is deleted.** The 4 untouched tests in `test_llm_client.py` keep the `GEMINI_MOCK=1` alias path,
  which is still valid under spec C1.
- **The mechanical classification is right.** `test_classify_real_errors.py` genai cases need no TCR, because
  `is_permanent` (`app/mq/protocol.py`) stays untouched and genai remains installed. Renaming the
  `test_config.py` kwargs from `gemini_rpm` to `ai_rpm` is a parameter rename with the same values (2.2). I accept
  both.

### New findings

**R-1 (minor): the alias scan and the mock-flag helper have no frozen test.** Round 2 planned tests in
`test_config.py` for `deprecated_aliases_used` (`os.environ`, a tmp `.env`, case-insensitive, `env_file=None`,
new name set) and for `resolve_mock_flag`. The frozen set covers only `log_startup(deprecated=[...])`, not the
detection itself, and section 2 still lists ".env alias scan" as a mutation target.
**Fix:** before writing row 3, the implementer adds a new test file (for example
`tests/test_alias_scan.py`) with those cases and runs it in the gate. Otherwise, drop the mutation target and
record AC-15 detection as covered only by the smoke. This is not blocking, because new tests may be added after
the freeze and AC-1 already exercises `LlmClient()` alias resolution.

**R-2 (minor): `Prompt` is missing from the row 4 export list.** TCR-1 item 1 builds
`Prompt(system="s", user="prompt")`.
**Fix:** add `Prompt` to "Exported names" (from `app.llm.client`).

### Required before implementation
Human approval of TCR-1. R-1 and R-2 can be handled by the implementer without another plan round.
