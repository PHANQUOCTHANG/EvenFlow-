# Test change request: ai-anthropic

## TCR-1: convert the old Gemini-shaped fakes and expectations in pre-existing tests

Status: **open, awaiting human approval.** Raised at plan round 3 (tests frozen at 0763f2a).

### Why

These files existed before the freeze, so the freeze filter locks them. `traceability.yaml` lists them under
`mechanical_renames`, but a pure rename (imports, class names) is not enough to make them run against the new
code, for two reasons:

1. Generation no longer goes through `google.genai` (spec D1). The old fakes expose
   `sdk.aio.models.generate_content(...)` returning `.text` / `.usage_metadata.total_token_count`; `LlmClient`
   calls `sdk.messages.create(...)` and reads `content` blocks / `usage.input_tokens + output_tokens` /
   `stop_reason`. Old genai `errors.ClientError` 429/400 are no longer what the client handles.
2. `guarded_generate` now reserves `estimate_tokens(prompt, client.sent_max_tokens(budget))` (spec AC-3, plan
   P-3: no `getattr` default). Hand-written fakes without `sent_max_tokens` raise `AttributeError`.

The changes below swap fake shapes and the error/message details tied to the provider. No test is deleted, no
assertion is removed or weakened, and each test keeps its intent (same behaviour under test, same refund /
retry / fallback rules).

### Items

`tests/test_llm_client.py` (renamed from `tests/test_gemini_client.py`; helper `_client` and `FakeModels`
replaced by a fake with `messages.create(**kw)` recording kwargs and returning
`SimpleNamespace(content=[SimpleNamespace(type="text", text=...)], usage=..., stop_reason="end_turn")`;
`_client` builds `LlmClient("k", mock=False, sdk=SimpleNamespace(messages=fake), chat_model="claude-test",
thinking_headroom_tokens=0, gemini_api_key="")`):

| # | Test | Change |
|---|---|---|
| 1 | `test_Generate_TraTokenThatVaGioiHanDauRa` | prompt `"prompt"` -> `Prompt(system="s", user="prompt")`; fake usage `input_tokens=300, output_tokens=21` (sum 321, same expected `total_tokens == 321`); `gen.model` / `call["model"] == "claude-test"` (was `gemini-test`); `call["contents"] == "prompt"` -> `call["messages"] == [{"role": "user", "content": "prompt"}]`; `call["config"].max_output_tokens == 192` -> `call["max_tokens"] == 192` (headroom 0 in `_client`, so the expected value is unchanged). |
| 2 | `test_Generate_ThieuUsageThiTokenLaNone` | fake `usage=None`; prompt as item 1; assertion unchanged (`total_tokens is None`). |
| 3 | `test_Generate_QuaTimeoutThiLoi` | fake with `delay=1.0`; prompt as item 1; assertion unchanged (`TimeoutError`). |
| 4 | `test_Generate_429ThanhRateLimitedVaTangCounter` | `errors.ClientError(429, ...)` -> `anthropic.RateLimitError("quota", response=httpx.Response(429, request=req), body=None)`; metric name `ai_llm_429_total` (rename); assertions unchanged (`RateLimited`, counter +1). |
| 5 | `test_Generate_Loi4xxKhacKhongPhaiRateLimited` | `errors.ClientError(400, ...)` -> `anthropic.BadRequestError(... 400 ...)`; `pytest.raises(errors.ClientError)` -> `pytest.raises(anthropic.BadRequestError)` (same intent: a non-429 4xx propagates, not `RateLimited`). |
| 6 | `test_Embed_That` | embedding goes through `embed_sdk` (spec AC-12): `LlmClient("k", mock=False, sdk=<unused fake>, gemini_api_key="g", embed_model="text-embedding-test", embed_sdk=SimpleNamespace(aio=SimpleNamespace(models=FakeModels())))`; assertions unchanged (vector, recorded `embed_model` / `contents`). |
| 7 | `test_ThatMaThieuKey_TuChoi` | `LlmClient(api_key="", mock=False)`; `match="GEMINI_API_KEY"` -> `match="ANTHROPIC_API_KEY"` (spec D4 / AC-14). |

Unchanged in that file except the mechanical rename: `test_Embed_MockTatDinhVaBoDau`,
`test_mock_embedding_CauKhacNhauThiVectorKhac`, `test_mock_embedding_CauRong`,
`test_KhongThamSo_VanDungDuocOCheDoMock` (keeps `GEMINI_MOCK=1`: the alias still works, spec C1). The import
`from google.genai import errors` is dropped; `anthropic` and `httpx` are imported.

`tests/test_guarded.py`:

| # | Test(s) | Change |
|---|---|---|
| 8 | `FakeGen` (used by `test_ChuaDuocPhep_KhongGoiGeminiVaKhongHoan`, `test_DatChoTruocRoiHoanPhanThua`, `test_429_HoanHetVaNemLai`, `test_LoiKhac_KhongHoan`, `test_KhongBietTokenThat_KhongHoan`) | add `def sent_max_tokens(self, budget): return budget`. Reservations stay identical to today, so every assertion is unchanged. |
| 9 | `FakeModels` + `_real_gemini` (used by `test_KiemDuyetThat_DiQuaLimiter`, `test_KiemDuyetThat_LimiterTuChoi_KhongGoiGemini`, `test_KiemDuyet_ChuaLanCuoi_LimiterTuChoiThiRetry`, `test_KiemDuyet_LanCuoiVanKhongGoiDuoc_ChuyenNguoiDuyet`) | `FakeModels.generate_content` -> `FakeMessages.create(**kw)` that keeps the integer `calls` counter and returns one text block with the same JSON, `usage=SimpleNamespace(input_tokens=70, output_tokens=7)` (sum 77), `stop_reason="end_turn"`; `_real_gemini` builds `LlmClient("k", mock=False, sdk=SimpleNamespace(messages=fake))` and returns `(client, fake)`. Assertions unchanged (`models.calls == 1/0`, refund `77`, verdicts). |

`test_KiemDuyet_LanCuoi429_CungChuyenNguoiDuyet` and `test_KiemDuyetMock_VanChayKhongCanLimiter` need only the
mechanical rename (the first replaces `generate` before any SDK use).

`tests/test_chat_handler.py`:

| # | Test(s) | Change |
|---|---|---|
| 10 | `FakeGemini` (used by every test that reaches `guarded_generate`: `test_429_*`, `test_GeminiLoiKhac_*`, `test_KhongBietTokenThat_KhongHoan`, `test_CoTokenThat_HoanPhanThua`, `test_LanCuoi429_*`, ...) | add `def sent_max_tokens(self, budget): return budget`. Reservations unchanged, assertions unchanged. |

`tests/test_config.py`:

| # | Test | Change |
|---|---|---|
| 11 | `test_GeminiThatMaThieuKeyThiTuChoi` | `Settings(_env_file=None, gemini_mock=False, gemini_api_key="")` -> `Settings(_env_file=None, ai_mock=False, anthropic_api_key="")`; `match="GEMINI_API_KEY"` -> `match="ANTHROPIC_API_KEY"` (spec D4 / AC-14). Test name kept. |

### Not changed

All frozen files in `tests/llm/`, `tests/test_llm_sdk_contract.py`, `tests/test_wiring.py`, `tests/conftest.py`,
`deploy/observability/alerts_test.yml`. Pure renames in `test_moderation_consumer.py`,
`test_classify_real_errors.py` and the two integration files are mechanical (plan 2.2), not part of this request.

### Check after applying

`git diff -M 0763f2a -- services/ai-worker/tests` shows only the plan 2.2 renames and items 1-11;
`uv run pytest -q` is green.

## Approval

Pending (human).
