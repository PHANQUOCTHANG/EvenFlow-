"""Embedding cho cache ngu nghia (L3), tach khoi sinh van ban.

Sinh van ban da chuyen sang Anthropic, nhung Anthropic khong co API embedding: L3 van dung
Gemini `text-embedding-004` khi co GEMINI_API_KEY (spec D2). Khong co key o che do that thi
khong co embedder nao (None) -- cache chi con tang exact hash, khoi dong van thanh cong.
"""

from __future__ import annotations

import asyncio
import hashlib
from typing import Any, Protocol

from app.pipeline.semantic_cache import normalize

MOCK_EMBED_DIMS = 64


class Embedder(Protocol):
    async def embed(self, text: str) -> list[float]: ...


def mock_embedding(text: str, dims: int = MOCK_EMBED_DIMS) -> list[float]:
    """Vector bam tat dinh tren cau da chuan hoa: du de cache ngu nghia chay o local/test.

    Khong mang nghia ngu nghia that -- hai cau dong nghia khac chu se khong gan nhau.
    Chi dung khi AI_MOCK=1.
    """
    vec = [0.0] * dims
    for token in normalize(text).split():
        h = hashlib.sha256(token.encode()).digest()
        idx = int.from_bytes(h[:4], "big") % dims
        vec[idx] += 1.0 if h[4] & 1 else -1.0
    norm = sum(x * x for x in vec) ** 0.5
    return [x / norm for x in vec] if norm else vec


class MockEmbedder:
    async def embed(self, text: str) -> list[float]:
        return mock_embedding(text)


class GeminiEmbedder:
    """Gemini embed_content. Client genai chi duoc tao o lan embed dau tien."""

    def __init__(self, api_key: str, model: str, timeout_seconds: float, sdk: Any = None) -> None:
        self._api_key = api_key
        self._model = model
        self._timeout = timeout_seconds
        self._sdk = sdk

    async def embed(self, text: str) -> list[float]:
        resp = await asyncio.wait_for(
            self._models().embed_content(model=self._model, contents=text),
            timeout=self._timeout,
        )
        return list(resp.embeddings[0].values)

    def _models(self) -> Any:
        if self._sdk is None:
            from google import genai

            self._sdk = genai.Client(api_key=self._api_key)
        return self._sdk.aio.models


def make_embedder(
    *,
    mock: bool,
    gemini_api_key: str,
    embed_model: str,
    timeout_seconds: float,
    embed_sdk: Any = None,
) -> Embedder | None:
    if mock:
        return MockEmbedder()
    if not gemini_api_key:
        return None
    return GeminiEmbedder(gemini_api_key, embed_model, timeout_seconds, sdk=embed_sdk)
