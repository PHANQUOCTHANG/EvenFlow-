# Verification evidence: ai-anthropic (v3 step 6, High risk)

- Verifier: independent, read-only on code. Date: 2026-11-21.
- Branch `feature/ai-anthropic`, HEAD `5e01d0c`. Gate commit `254f556`. Base `c5fb598`.
- Environment: Windows 11, uv, Python from `uv sync --locked --all-extras`, `anthropic` 2.12.0 locked.
  Docker CLI is present, but the Docker engine is **down** (see `verify-logs/docker-checks.log`).

## Verdict: PASS (automated scope). Merge is BLOCKED until the pending human items are done.

Everything that can be checked without Docker or a real key passes: 452 unit tests, 30/30 mutants killed,
and the rollback check. Three items are NOT VERIFIED (listed below). One of them, the real-key smoke test
(plan section 3 step 7, P-2c), is a **required pre-merge** step. The PR must not merge until a human
records it here.

## 1. Gate and commit identity

| Check | Result |
|---|---|
| `gate-report.json.commit_sha` | `254f556fad35b3970da9410b218b27775e1c1d90` = the last code commit |
| `git diff --name-only 254f556 HEAD` | only `docs/workflow/ai-anthropic/gate-report.json` and `gate/*.log` (docs only) |
| Gate results | py-ai-worker, lock-diff, compose, tamper, secrets, sast and audit PASS; alerts **NOT_RUN (77)**: Docker |

## 2. Acceptance criteria

Logs: `verify-logs/pytest-full.log` (full suite) and `verify-logs/per-ac.log` (one selection per AC).
The counts match `traceability.yaml`.

| AC | Proving tests / check | Result |
|---|---|---|
| AC-1 Mock unchanged | `tests/llm/test_client_mock.py` (10), `test_wiring.py::test_AC1_*` (1) | 11 passed |
| AC-2 Call shape | `test_client_anthropic.py::test_AC2_*`, `test_llm_sdk_contract.py`, `test_consumers_anthropic.py::test_AC2_AC3_*` | 8 passed; SDK signature contract plus alias scan: 19 passed |
| AC-3 Token budget | `-k AC3_` | 11 passed |
| AC-4 BR-A7 boundary | `-k AC4_` | 3 passed |
| AC-5 Text extraction | `-k AC5_` | 2 passed |
| AC-6 Token accounting | `-k AC6_` | 8 passed |
| AC-7 429 | `-k AC7_` | 4 passed |
| AC-8 Timeout | `-k AC8_` | 5 passed |
| AC-9 Other API errors | `tests/llm/test_classify_anthropic_errors.py`, `tests/test_classify_real_errors.py` (22), client `-k AC9_` (5) | 27 passed |
| AC-10 Incomplete | `-k AC10_` | 12 passed |
| AC-11 Moderation JSON | `test_moderation_json.py`, `test_fix_moderation_parse*.py` | 51 passed |
| AC-12 Gemini embeddings | `-k AC12_` | 5 passed |
| AC-13 L3 disabled | `-k AC13_` (includes the 100-job zero-log case) | 5 passed |
| AC-14 Validation | `-k AC14_` | 21 passed |
| AC-15 No secrets | `-k AC15_`, plus caplog asserts in the AC7/AC8/AC10/AC13 tests and `test_fix_secret_logging.py` | 3 passed (+ inside the others) |
| AC-16 Dependency | `pyproject.toml` diff adds only `anthropic>=2.12.0`; `uv sync --locked --all-extras` OK; lock-diff gate PASS | PASS (Trivy runs in CI) |
| AC-17 Compose/env | `docker compose ... --env-file .env.example config` -> compose-ok; ai-worker env has ANTHROPIC_API_KEY, AI_MOCK=1, both model ids, GEMINI_API_KEY, AI_RPM and AI_TPM (`verify-logs/docker-checks.log`) | PASS |
| AC-18 Alerts | `alerts.yml` diff is only the rename to `LlmRateLimited` / `ai_llm_429_total` (plus comments); a new `alerts_test.yml` case exists | **NOT VERIFIED**: promtool needs the Docker engine |
| AC-19 Docs | README updated (+9/-6) | PASS, with a note: `docs/02-kien-truc.md` changed despite the "docs/0*.md untouched" wording. The change only renames the metric and adds `ai_llm_incomplete_total`, as review finding R-5 asked. I accept this deviation. |
| AC-20 Regression | full suite: `452 passed, 22 deselected`; `ruff check .` clean | PASS (unit). Integration (`-m integration`): **NOT VERIFIED**, Docker |

## 3. Mutation testing

The script is `verify-logs/mutation/run_mutants.py`. For each mutant it replaces one exact, unique
occurrence in the code, runs `pytest -x -q`, then restores the original bytes. The log is
`verify-logs/mutation/mutation.log`. Six mutants first showed a captured log line instead of the failing
test, so I re-ran them with better detail extraction (`mutation-rerun-detail.log`). They were killed by
real test failures. After the run, `git status` showed no changes under `services/`.

**Threshold: all behaviour-changing mutants killed. Result: 30/30 KILLED.**

| # | Mutant | Killed by |
|---|---|---|
| M01 | effort `low` -> `medium` | test_AC2_Chat_MotLanMessagesCreate_DungModelVaSystem |
| M02 | `max_retries=0` -> 2 | test_AC2_TaoAsyncAnthropicLuoi_TimeoutVaMaxRetries0 |
| M03 | `sent_max_tokens` drops the headroom | test_AC3_Chat_MaxTokens_BangNganSachCongHeadroom |
| M04 | stop_reason allowlist accepts `max_tokens` | test_AC10_KhongTronVen_LlmIncompleteVaDem[max_tokens] |
| M05 | blank text is not treated as incomplete | test_AC10_KhongTronVen_LlmIncompleteVaDem[only-thinking] |
| M06 | chat caches when the LLM fails (incomplete or timeout) | test_AC8_Chat_Timeout_TraLoiDuPhong_KhongCache[APITimeoutError] |
| M07 | refund clamp has no upper bound | test_X6_UsageVuotPhanDatCho_KhongHoanAm |
| M08 | refund clamp has no lower bound | test_X6_Guarded_KepActualTrongKhoangDatCho[-900000-0] |
| M09 | 429 no longer refunds everything | test_AC7_Guarded_429_HoanToanBo |
| M10 | 429 metric not incremented | test_AC7_RateLimitError_ThanhRateLimited_DemVaLogMotLan |
| M11 | 429 not mapped to RateLimited | test_AC7_RateLimitError_ThanhRateLimited_DemVaLogMotLan |
| M12 | APITimeoutError not mapped to TimeoutError | test_AC8_APITimeoutError_ThanhTimeoutErrorCoChain |
| M13 | `wait_for` hard timeout removed | test_AC8_QuaTimeoutCung_TimeoutError |
| M14 | score upper bound 1.0 dropped | test_X2_X3_PhanHoiDangNgo_NeedsHuman[{"score": 1.01}] |
| M15 | bool score accepted | test_X2_X3_PhanHoiDangNgo_NeedsHuman[{"score": true}] |
| M16 | missing score defaults to 0 | test_X2_X3_PhanHoiDangNgo_NeedsHuman[{}] |
| M17 | code fence not stripped | test_AC11_JsonHopLe_PhanTichVaApDungNguong[json-fence] |
| M18 | `clean_api_key` does not strip | test_X1_KeyKhoangTrangBaoNgoai_DuocCatMotLan |
| M19 | `clean_api_key` accepts inner whitespace | test_X1_KeyCoKhoangTrangHoacKyTuDieuKhienBenTrong_TuChoi_KhongInKey |
| M20 | `error_fields` logs `str(exc)` | test_X1_Chat_LoiCoChuoiChuaKey_LogChiTenLoi |
| M21 | alias precedence: the old name wins | test_X5_A_AI_MOCKTrongEnvFile_ThangGEMINI_MOCKTrongShell |
| M22 | `SemanticCache.get` embeds when the embedder is None | test_AC13_CacheKhongEmbedder_L1VanChay_KhongLog |
| M23 | mock `generate` goes to the SDK | test_AC1_MockGenerate_HinhDangCu_KhongTaoSdk |
| M24 | mock embedder only when a key is set | test_AC1_MockEmbed_LaMockEmbedding |
| M25 | `wrap_untrusted` does not escape `>` | test_AC4_CauHoiDocHai_ChiNamTrongKhoiBocCuaUser |
| M26 | moderation prompt BR-A7 "not instructions" text removed | test_R1_PromptKiemDuyet_NoiDuLieuKhongPhaiChiThi |
| M27 | moderation LlmIncomplete does not give NEEDS_HUMAN | test_AC10_KiemDuyet_KhongTronVen_NeedsHumanMotLan_KhongRetry[refusal] |
| M28 | incomplete metric label passes the raw stop_reason through | test_X4_StopReasonNgoaiAllowlist_LlmIncomplete[model_context_window_exceeded-other] |
| M29 | negative usage accepted | test_X6_UsageAmHoacKhongPhaiSoNguyen_KhongBiet_KhongHoan[usage0] |
| M30 | user question leaks into `system=` | test_AC2_Chat_MotLanMessagesCreate_DungModelVaSystem |

## 4. Rollback check (plan section 5)

Log: `verify-logs/rollback.log`. I used a temporary worktree `../evf-rollback` at `c5fb598` with the new
`.env.example` loaded, then removed it afterwards (`git worktree list` no longer shows it).

- `docker compose ... --env-file "$NEW/.env.example" config` -> `compose-ok`.
- `uv sync --locked` at c5fb598 -> OK.
- **The plan's one-liner as written fails** with `AttributeError: 'GeminiClient' object has no attribute 'mock'`.
  Old `GeminiClient` stores the flag as `_mock` and has no `mock` property. This is a defect in the check
  command, not in the rollback path. Re-running the same check with `c._mock` printed `True True`
  (rc=0): old code on the new env starts in mock mode without an exception, as expected.
- Result: **PASS**. Follow-up: fix the one-liner in plan.md section 5 to use `c._mock` (doc-only).

## 5. NOT VERIFIED

| Item | Why | Owner / action |
|---|---|---|
| **P-2c real-key smoke (REQUIRED pre-merge)** | Needs a real `ANTHROPIC_API_KEY`; the verifier does not attempt it | **Human**: with `AI_MOCK=0`, run one chat job and one moderation job. Record model ids, `stop_reason`, `usage` in/out, latency, and for the Vietnamese chat prompt `usage.input_tokens` vs `len(prompt)//3` (ratio > 1 -> open F-5). Never commit or log the key. |
| AC-18 `promtool check rules` / `promtool test rules alerts_test.yml` | Docker engine down (`docker info` fails) | CI, or a human with Docker: `docker run --rm -v "$PWD/deploy/observability:/w" -w /w --entrypoint promtool prom/prometheus:v3.5.0 test rules alerts_test.yml` |
| AC-20 integration tests `uv run pytest -m integration` (mock mode) | Needs Docker (testcontainers) | CI, or a human with Docker |
| AC-16 Trivy | Runs in CI only | CI |

### Human real-key smoke record (fill in before merge)

| Job | model | stop_reason | usage in/out | latency | input_tokens vs len(prompt)//3 |
|---|---|---|---|---|---|
| chat | | | | | |
| moderation | | | | | |
