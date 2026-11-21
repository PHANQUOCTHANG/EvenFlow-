"""Mot duong duy nhat de goi LLM that: xin phep limiter -> goi -> hoan phan thua (L6).

Truoc day chat tu viet logic nay con kiem duyet thi goi thang `generate` -- bo qua
limiter. Khi bat AI_MOCK=0, moi job kiem duyet la mot request khong ai dem, an vao
dung han muc ma tro ly cua 30.000 khach dang can. Gom ve mot cho de khong con loi nao
"quen" xin phep.
"""

from __future__ import annotations

from typing import Any

from app import metrics
from app.llm.client import Generation, Prompt, RateLimited
from app.llm.limiter import estimate_tokens


class PermitDenied(Exception):
    """Limiter chua cap phep. Nguoi goi dua job vao hang tre, KHONG ngu trong worker."""

    def __init__(self, wait_ms: int) -> None:
        super().__init__(f"limiter chua cho phep, thu lai sau {wait_ms}ms")
        self.wait_ms = wait_ms


async def guarded_generate(
    limiter: Any,
    client: Any,
    prompt: str | Prompt,
    *,
    max_output_tokens: int,
    model: str | None = None,
) -> Generation:
    # Uoc luong tren max_tokens THUC GUI (ngan sach + headroom suy nghi), khong phai ngan
    # sach: giay phep khong bao gio thap hon truong hop xau nhat (spec AC-3).
    permit = await limiter.acquire(estimate_tokens(prompt, client.sent_max_tokens(max_output_tokens)))
    if not permit.granted:
        metrics.PERMIT_DENIED.inc()
        raise PermitDenied(permit.wait_ms)

    # Mac dinh coi nhu da dung het phan dat cho: khong biet chac thi KHONG hoan.
    # Timeout / tra loi khong tron ven chang han -- nha cung cap co the da tinh token du
    # minh khong nhan ket qua; hoan nham la vuot TPM, dung dieu limiter ton tai de ngan.
    actual = permit.reserved_tokens
    try:
        # model chi truyen khi co: fake cu (khong co tham so model) van chay.
        kwargs: dict[str, Any] = {"max_output_tokens": max_output_tokens}
        if model is not None:
            kwargs["model"] = model
        result = await client.generate(prompt, **kwargs)
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
