"""Fix R-2/X-2/X-3: phan tich JSON kiem duyet dong kin -- moi phan hoi dang ngo -> NEEDS_HUMAN.

Cac case lay tu repro cua challenger (test_moderation_parse). Ket qua phai luon ma hoa duoc
thanh body callback (httpx json=...), NaN / vo cuc khong bao gio toi callback.
"""

from __future__ import annotations

import json

import httpx
import pytest

from app.consumers.moderation import ModerationConsumer
from app.domain.moderation import AiVerdict

from .conftest import FakeAnthropic, make_client, mod_body, mod_job, text_block

PARSE_FAIL_REASON = "Không phân tích được phản hồi từ AI, chuyển người duyệt"


async def _scan(text: str):
    return await make_client(FakeAnthropic(content=[text_block(text)])).scan_moderation(mod_job())


def _encodable(res) -> None:
    httpx.Request("PATCH", "http://x/", json={"score": res.score, "verdict": res.verdict.value,
                                              "reasons": res.reasons, "axes": res.axes})


@pytest.mark.parametrize(
    "text",
    [
        "{}",
        '{"verdict":"AUTO_REJECTED","reasons":["vu khi"]}',
        '{"Score": 0.95}',
        '{"score": -5}',
        '{"score": -0.01}',
        '{"score": 1.01}',
        '{"score": 2}',
        '{"score": NaN}',
        '{"score": Infinity}',
        '{"score": -Infinity}',
        '{"score": 1e999}',
        '{"score": true}',
        '{"score": false}',
        '{"score": "0.0"}',
        '{"score": [0.1]}',
        '{"score": 0.1, "reasons": "chuoi khong phai list", "axes": [1,2]}',
        '{"score": 0.1, "axes": [1, 2]}',
        '{"score": 0.1, "axes": {"misleading": "cao"}}',
        '{"score": 0.1, "axes": {"misleading": NaN}}',
        '{"score": 0.1, "axes": {"misleading": 1.5}}',
        '{"score": 0.1, "axes": {"misleading": true}}',
        '{"score": 0.1, "reasons": "mot chuoi"}',
        '{"score": 0.1, "reasons": ["ok", 3]}',
        '{"score": 0.1, "reasons": null}',
        '{"score": 0.1, "axes": null}',
    ],
)
async def test_X2_X3_PhanHoiDangNgo_NeedsHuman(text) -> None:
    res = await _scan(text)
    assert res.verdict is AiVerdict.NEEDS_HUMAN
    assert res.score == 0.5
    assert res.axes == {}
    assert res.reasons == [PARSE_FAIL_REASON]
    _encodable(res)


async def test_X3_QuaNhieuTruc_NeedsHuman() -> None:
    axes = {f"truc_{i}": 0.1 for i in range(50)}
    res = await _scan(json.dumps({"score": 0.1, "axes": axes, "reasons": []}))
    assert res.verdict is AiVerdict.NEEDS_HUMAN


@pytest.mark.parametrize(("score", "verdict"), [(0, AiVerdict.AUTO_APPROVED), (1, AiVerdict.AUTO_REJECTED),
                                                (0.0, AiVerdict.AUTO_APPROVED), (1.0, AiVerdict.AUTO_REJECTED)])
async def test_X2_BienHopLe_VanChapNhan(score, verdict) -> None:
    res = await _scan(json.dumps({"score": score}))
    assert res.verdict is verdict
    assert res.score == float(score)
    assert res.axes == {} and res.reasons == []
    _encodable(res)


async def test_X3_LyDoBiCatSoLuongVaDoDai() -> None:
    reasons = ["x" * 5000] + [f"ly do {i}" for i in range(40)]
    res = await _scan(json.dumps({"score": 0.5, "axes": {"misleading": 0.5}, "reasons": reasons}))
    assert res.verdict is AiVerdict.NEEDS_HUMAN
    assert res.axes == {"misleading": 0.5}
    assert 1 <= len(res.reasons) <= 10
    assert all(len(r) <= 500 for r in res.reasons)


async def test_X3_TrucSoNguyen_ThanhFloat() -> None:
    res = await _scan('{"score": 0.2, "axes": {"copyright": 0, "misleading": 1}, "reasons": ["ok"]}')
    assert res.verdict is AiVerdict.AUTO_APPROVED
    assert res.axes == {"copyright": 0.0, "misleading": 1.0}
    assert all(type(v) is float for v in res.axes.values())


@pytest.mark.parametrize("text", ['{"score": NaN}', '{"score": 1e999}',
                                  '{"score": 0.1, "reasons": "chuoi", "axes": [1,2]}'])
async def test_X3_CallbackThat_GuiNeedsHuman_KhongValueError(text) -> None:
    sent: list[dict] = []

    def handler(req: httpx.Request) -> httpx.Response:
        sent.append(json.loads(req.content))
        return httpx.Response(204)

    client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    consumer = ModerationConsumer(make_client(FakeAnthropic(content=[text_block(text)])), "http://event",
                                  client=client)
    res = await consumer.handle_message(mod_body())
    await client.aclose()
    assert res.verdict is AiVerdict.NEEDS_HUMAN
    assert sent == [{"score": 0.5, "verdict": "NEEDS_HUMAN", "reasons": [PARSE_FAIL_REASON], "axes": {}}]
