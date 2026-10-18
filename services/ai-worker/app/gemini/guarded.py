"""Mot duong duy nhat de goi Gemini that: xin phep limiter -> goi -> hoan phan thua (L6).

Truoc day chat tu viet logic nay con kiem duyet thi goi thang `generate` -- bo qua
limiter. Khi bat GEMINI_MOCK=0, moi job kiem duyet la mot request khong ai dem, an vao
dung han muc ma tro ly cua 30.000 khach dang can. Gom ve mot cho de khong con loi nao
"quen" xin phep.
"""

from __future__ import annotations

from typing import Any

from app import metrics
from app.gemini.client import Generation, RateLimited
from app.gemini.limiter import estimate_tokens


class PermitDenied(Exception):
    """Limiter chua cap phep. Nguoi goi dua job vao hang tre, KHONG ngu trong worker."""

    def __init__(self, wait_ms: int) -> None:
        super().__init__(f"limiter chua cho phep, thu lai sau {wait_ms}ms")
        self.wait_ms = wait_ms


async def guarded_generate(limiter: Any, gemini: Any, prompt: str, *, max_output_tokens: int) -> Generation:
    permit = await limiter.acquire(estimate_tokens(prompt, max_output_tokens))
    if not permit.granted:
        metrics.PERMIT_DENIED.inc()
        raise PermitDenied(permit.wait_ms)

    # Mac dinh coi nhu da dung het phan dat cho: khong biet chac thi KHONG hoan.
    # Timeout chang han -- Gemini co the da tinh token du minh khong nhan ket qua;
    # hoan nham la vuot TPM, dung dieu limiter ton tai de ngan.
    actual = permit.reserved_tokens
    try:
        result = await gemini.generate(prompt, max_output_tokens=max_output_tokens)
        if result.total_tokens is not None:
            actual = result.total_tokens
        return result
    except RateLimited:
        # 429: request bi tu choi, khong ton token -> hoan het.
        actual = 0
        raise
    finally:
        # Bo qua buoc nay thi han muc ro ri dan cho toi khi limiter chan het moi thu.
        await limiter.refund(permit.reserved_tokens, actual)
