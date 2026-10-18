"""Phan loai voi loi THAT cua thu vien (khong goi mang) -- duck typing phai khop httpx va google-genai."""

from __future__ import annotations

import httpx
import pytest
from google.genai import errors as genai_errors

from app.gemini.client import RateLimited
from app.gemini.guarded import PermitDenied
from app.mq.protocol import is_permanent


def _http_error(status: int) -> httpx.HTTPStatusError:
    req = httpx.Request("PATCH", "http://event/internal/v1/events/x/ai-verdict")
    resp = httpx.Response(status, request=req)
    return httpx.HTTPStatusError(f"{status}", request=req, response=resp)


@pytest.mark.parametrize(("status", "permanent"), [(404, True), (409, True), (422, True), (429, False), (503, False)])
def test_httpx_HTTPStatusError(status, permanent) -> None:
    assert is_permanent(_http_error(status)) is permanent


@pytest.mark.parametrize(("code", "permanent"), [(400, True), (403, True), (429, False)])
def test_genai_ClientError(code, permanent) -> None:
    err = genai_errors.ClientError(code, {"error": {"code": code, "message": "x", "status": "X"}})
    assert is_permanent(err) is permanent


def test_genai_ServerError_LaTam() -> None:
    err = genai_errors.ServerError(503, {"error": {"code": 503, "message": "x", "status": "UNAVAILABLE"}})
    assert not is_permanent(err)


def test_httpx_MatKetNoi_LaTam() -> None:
    assert not is_permanent(httpx.ConnectError("refused"))
    assert not is_permanent(httpx.ReadTimeout("slow"))


def test_LimiterVa429_LaTam() -> None:
    assert not is_permanent(PermitDenied(1000))
    assert not is_permanent(RateLimited("429"))
