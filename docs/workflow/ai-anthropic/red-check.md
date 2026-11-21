# Red check: ai-anthropic (v3 step 2a)

Date: 2026-11-21. Base: `169035e`. Run from `services/ai-worker`, with the SDK added only for this run
(pyproject/uv.lock untouched):

```
uv run --with anthropic pytest -q --continue-on-collection-errors tests/llm tests/test_llm_sdk_contract.py tests/test_wiring.py
-> 41 failed, 2 passed, 5 collection errors (127 test cases once collectable)
```

## Failure reasons (all expected: the code under test does not exist yet)

| Count | Reason | Where |
|---|---|---|
| 5 files | `ModuleNotFoundError: No module named 'app.llm'` at collection | test_client_anthropic, test_client_mock, test_consumers_anthropic, test_classify_anthropic_errors, test_wiring |
| 25 | `ModuleNotFoundError: app.llm` inside `make_client` / `importlib` | test_moderation_json, test_embeddings, test_llm_sdk_contract |
| 9 | `DID NOT RAISE ValidationError` (AI_* env and new validators not read yet) | test_settings_anthropic |
| 7 | `AttributeError: 'Settings' object has no attribute 'ai_mock' / 'ai_rpm' / 'anthropic_api_key' / ...` | test_settings_anthropic |
| 2 | `ValidationError ... GEMINI_API_KEY` (alias GEMINI_MOCK=0 + ANTHROPIC key still rejected by old validator) | test_settings_anthropic |
| 1 | `Regex pattern did not match` (old message names GEMINI_API_KEY, spec requires ANTHROPIC_API_KEY) | test_settings_anthropic |
| 1 | `SemanticCache(redis, None)` logs "embed that bai" ERROR on every miss (AC-13 not implemented) | test_embeddings |

No syntax errors, no fixture errors, no failure caused by test bugs.

## Tests that pass at baseline (characterisation, expected green)

- `test_settings_anthropic.py::test_AC14_BienCuDaBo_BiBoQua` (`extra="ignore"` already drops GEMINI_MODEL_FAST/DEEP).
- `test_llm_sdk_contract.py::test_AC2_ConstructorAsyncAnthropic_NhanTimeoutVaMaxRetries` (SDK signature check only).
- `test_classify_anthropic_errors.py` (11 cases) is red today only because it imports `RateLimited`/`LlmIncomplete`
  from `app.llm.client`; the `is_permanent` assertions are expected to pass without touching `mq/protocol.py`
  (spec AC-9: change `is_permanent` only if they fail).

## Alerts (AC-18)

`deploy/observability/alerts_test.yml` gets a `LlmRateLimited` case.
`promtool test rules alerts_test.yml` (prom/prometheus:v3.5.0, same as CI) -> FAILED: `alertname: LlmRateLimited, time: 4m, exp:[...] got:[]`.
Sanity check: a scratch copy of alerts.yml with only the rename (`GeminiRateLimited`->`LlmRateLimited`,
`ai_gemini_429_total`->`ai_llm_429_total`) -> SUCCESS.

## Existing suite is unaffected

`uv run pytest -q --ignore=tests/llm --ignore=tests/test_wiring.py --ignore=tests/test_llm_sdk_contract.py`
-> 203 passed, 22 deselected (= baseline), with the new root `tests/conftest.py` (autouse, clears every
`AI_*` / `GEMINI_*` / `ANTHROPIC_*` env var by prefix scan) active.

## Lint note

`ruff check tests` currently reports I001 on the 4 files that import `app.llm` (ruff classifies a missing
package as third-party). Checked on a scratch copy where `app/llm/` exists: `ruff check tests` -> All checks passed.
The implementer must NOT reorder these imports; they become clean once `app/llm/` exists.
