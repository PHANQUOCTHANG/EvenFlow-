"""AC-9: mq.protocol.is_permanent voi exception THAT cua SDK anthropic (khong goi mang).

Day la test dac ta (characterization): duck typing hien tai (`err.response.status_code`) duoc
ky vong la da khop. `is_permanent` chi duoc sua neu file nay do.
"""

from __future__ import annotations

import anthropic
import pytest

from app.llm.client import LlmIncomplete, RateLimited
from app.mq.protocol import is_permanent

from .conftest import http_request, status_error


@pytest.mark.parametrize(
    ("cls", "status"),
    [
        (anthropic.BadRequestError, 400),
        (anthropic.AuthenticationError, 401),
        (anthropic.PermissionDeniedError, 403),
        (anthropic.NotFoundError, 404),
    ],
)
def test_AC9_4xx_LaVinhVien(cls, status) -> None:
    assert is_permanent(status_error(cls, status)) is True


@pytest.mark.parametrize(
    ("cls", "status"),
    [
        (anthropic.APIStatusError, 408),
        (anthropic.RateLimitError, 429),
        (anthropic.InternalServerError, 500),
        (anthropic.APIStatusError, 529),
    ],
)
def test_AC9_TamThoi_408_429_5xx(cls, status) -> None:
    assert is_permanent(status_error(cls, status)) is False


def test_AC9_MatKetNoiVaTimeout_LaTam() -> None:
    assert is_permanent(anthropic.APIConnectionError(request=http_request())) is False
    assert is_permanent(anthropic.APITimeoutError(request=http_request())) is False


def test_AC9_LoiDaBoc_TheoChuoiCause() -> None:
    # RateLimited/TimeoutError duoc raise ... from exc trong LlmClient: van la tam.
    for wrapper, cause in (
        (RateLimited("429"), status_error(anthropic.RateLimitError, 429)),
        (TimeoutError(), anthropic.APITimeoutError(request=http_request())),
    ):
        wrapper.__cause__ = cause
        assert is_permanent(wrapper) is False


def test_AC9_LlmIncomplete_KhongPhaiLoiVinhVien() -> None:
    # Moderation xu ly NEEDS_HUMAN truc tiep; neu lot ra runtime cung khong duoc vao DLQ ngay.
    assert is_permanent(LlmIncomplete("max_tokens")) is False
