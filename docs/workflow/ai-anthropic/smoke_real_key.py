"""Real-key smoke test for ai-anthropic (plan P-2c). Costs well under 1 cent.

Run from services/ai-worker with the key in the repo .env (never pass it on the command line):

    cd services/ai-worker
    uv run python -I ../../docs/workflow/ai-anthropic/smoke_real_key.py

It sends ONE chat request and ONE moderation request with AI_MOCK=0 and prints model, stop
reason, usage, latency and the len(prompt)//3 estimate. It never prints the key.
"""

from __future__ import annotations

import asyncio
import sys
import time
from pathlib import Path
from types import SimpleNamespace

sys.path.insert(0, str(Path.cwd()))

import anthropic  # noqa: E402

from app.config import Settings  # noqa: E402
from app.domain.moderation import ModerationJob  # noqa: E402
from app.llm.client import LlmClient  # noqa: E402

REPO_ENV = Path(__file__).resolve().parents[3] / ".env"


class Recorder:
    """Wraps the real SDK and keeps the raw response of each call."""

    def __init__(self, sdk: anthropic.AsyncAnthropic) -> None:
        self._sdk = sdk
        self.responses: list = []
        self.messages = SimpleNamespace(create=self._create)

    async def _create(self, **kw):
        resp = await self._sdk.messages.create(**kw)
        self.responses.append((kw, resp))
        return resp


class FakeTools:
    async def event_context(self, event_id: str) -> str:
        return (
            "Su kien: Dem nhac Acoustic Sai Gon. Ngay 2026-12-20, 19:30, Nha hat Thanh pho. "
            "Hang ve: Thuong 500.000d (con 120), VIP 1.200.000d (het ve). Mo cua luc 18:45."
        )


def report(label: str, kw: dict, resp, seconds: float) -> None:
    prompt_chars = len(kw.get("system", "")) + sum(len(m["content"]) for m in kw["messages"])
    est = prompt_chars // 3
    used_in = resp.usage.input_tokens
    print(f"[{label}] model={resp.model} stop_reason={resp.stop_reason} "
          f"usage_in={used_in} usage_out={resp.usage.output_tokens} latency={seconds:.2f}s "
          f"estimate_len//3={est} ratio_in/estimate={used_in / max(est, 1):.2f}")


async def main() -> int:
    settings = Settings(_env_file=REPO_ENV, ai_mock=False)
    key = settings.anthropic_api_key.get_secret_value()
    if not key:
        print("ANTHROPIC_API_KEY is empty in .env")
        return 2
    rec = Recorder(anthropic.AsyncAnthropic(api_key=key, timeout=settings.ai_timeout_seconds,
                                            max_retries=0))
    llm = LlmClient(key, mock=False, sdk=rec,
                    chat_model=settings.anthropic_model_chat,
                    moderation_model=settings.anthropic_model_moderation,
                    timeout_seconds=max(settings.ai_timeout_seconds, 30.0),
                    thinking_headroom_tokens=settings.ai_thinking_headroom_tokens,
                    gemini_api_key="")

    job = SimpleNamespace(event_id="evt-smoke", question="Con ve VIP khong? Mấy giờ mở cửa?")
    prompt = await llm.build_prompt(job, FakeTools())
    t0 = time.perf_counter()
    gen = await llm.generate(prompt, max_output_tokens=256)
    report("chat", *rec.responses[-1], time.perf_counter() - t0)
    print(f"[chat] answer_chars={len(gen.text)} total_tokens={gen.total_tokens}")

    mod = ModerationJob(job_id="m-smoke", event_id="evt-smoke", event_title="Dem nhac Acoustic",
                        event_description="Dem nhac acoustic cuoi tuan voi ban nhac tre.",
                        enqueued_at=time.time())
    t0 = time.perf_counter()
    result = await llm.scan_moderation(mod)
    report("moderation", *rec.responses[-1], time.perf_counter() - t0)
    print(f"[moderation] verdict={result.verdict.value} score={result.score}")
    return 0


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main()))
