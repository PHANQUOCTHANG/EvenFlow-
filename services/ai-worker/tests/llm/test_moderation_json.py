"""AC-11: phan tich JSON kiem duyet tu Claude (co the boc mot code fence), loi -> NEEDS_HUMAN."""

from __future__ import annotations

import json

import pytest

from app.domain.moderation import AiVerdict, calculate_verdict

from .conftest import FakeAnthropic, make_client, mod_job, text_block

PARSE_FAIL_REASON = "Không phân tích được phản hồi từ AI, chuyển người duyệt"

_PAYLOAD = {
    "score": 0.9,
    "axes": {"forbidden_content": 0.9, "misleading": 0.2, "copyright": 0.0, "incomplete_info": 0.1},
    "reasons": ["Noi dung cam"],
}


async def _scan(text: str):
    return await make_client(FakeAnthropic(content=[text_block(text)])).scan_moderation(mod_job())


@pytest.mark.parametrize(
    "wrap",
    [
        lambda s: s,
        lambda s: f"```json\n{s}\n```",
        lambda s: f"```\n{s}\n```",
        lambda s: f"\n```json\n{s}\n```\n",
    ],
    ids=["plain", "json-fence", "bare-fence", "fence-with-whitespace"],
)
async def test_AC11_JsonHopLe_PhanTichVaApDungNguong(wrap) -> None:
    res = await _scan(wrap(json.dumps(_PAYLOAD)))
    assert res.score == 0.9
    assert res.verdict is calculate_verdict(0.9) is AiVerdict.AUTO_REJECTED
    assert res.axes == _PAYLOAD["axes"]
    assert res.reasons == _PAYLOAD["reasons"]


@pytest.mark.parametrize("score", [0.1, 0.5])
async def test_AC11_NguongVerdictGiuNguyen(score) -> None:
    res = await _scan(json.dumps({"score": score, "axes": {}, "reasons": []}))
    assert res.verdict is calculate_verdict(score)


@pytest.mark.parametrize(
    "text",
    [
        "Su kien nay co ve on, diem 0.1",
        "[0.1, 0.2]",
        '"chi la chuoi"',
        '{"score": "cao", "axes": {}, "reasons": []}',
        '{"score": null}',
        '{"score": {"x": 1}}',
        "```json\nkhong phai json\n```",
    ],
    ids=["prose", "json-list", "json-string", "score-text", "score-null", "score-object", "fenced-garbage"],
)
async def test_AC11_KhongPhanTichDuoc_NeedsHuman_KhongNem(text) -> None:
    res = await _scan(text)
    assert res.verdict is AiVerdict.NEEDS_HUMAN
    assert res.score == 0.5
    assert res.reasons == [PARSE_FAIL_REASON]
