# TCR-1 applied

- Approved by TranDinhVo in the Claude Code session on 2026-11-21 (all 11 items as written in test-change-request.md).
- Applied by the coordinator session (the implementer subagent was blocked by the permission system and stopped without editing).
- Items 1-7: `services/ai-worker/tests/test_llm_client.py`. Fake Anthropic `messages.create`, `Prompt(system, user)`, `anthropic.RateLimitError`/`BadRequestError`, `embed_sdk` for Gemini embeddings, message `ANTHROPIC_API_KEY`. Expected values kept (321 tokens, `max_tokens` 192 with headroom 0).
- Item 8: `tests/test_guarded.py` `FakeGen.sent_max_tokens(budget) -> budget`.
- Item 9: `tests/test_guarded.py` `FakeModels` → `FakeMessages.create`, usage 70+7=77, integer `calls` counter kept.
- Item 10: `tests/test_chat_handler.py` `FakeGemini.sent_max_tokens(budget) -> budget`.
- Item 11: `tests/test_config.py` `Settings(ai_mock=False, anthropic_api_key="")`, match `ANTHROPIC_API_KEY`.
- No test deleted, no assertion weakened.
- Result: `uv run pytest -q` 345 passed, 22 deselected (integration); baseline 203. `ruff check .` clean; `uv sync --locked` ok.
