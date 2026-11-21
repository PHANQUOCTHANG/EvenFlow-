"""Lap rap LLM cho worker va log khoi dong -- ham thuan, test khong can Redis/RabbitMQ.

Khong tao client SDK nao o day (Anthropic va genai deu luoi): khoi dong khong goi mang.
"""

from __future__ import annotations

import logging

from app.config import DEPRECATED_ALIASES, Settings
from app.llm.client import LlmClient
from app.llm.embedder import Embedder

log = logging.getLogger(__name__)

L3_DISABLED = "semantic cache L3 disabled (GEMINI_API_KEY empty)"


def build_llm(settings: Settings) -> tuple[LlmClient, Embedder | None]:
    """LlmClient tu Settings, kem embedder cho cache L3 (None khi L3 tat)."""
    llm = LlmClient(
        settings.anthropic_api_key.get_secret_value(),
        mock=settings.ai_mock,
        chat_model=settings.anthropic_model_chat,
        moderation_model=settings.anthropic_model_moderation,
        timeout_seconds=settings.ai_timeout_seconds,
        thinking_headroom_tokens=settings.ai_thinking_headroom_tokens,
        gemini_api_key=settings.gemini_api_key.get_secret_value(),
        embed_model=settings.gemini_embed_model,
    )
    return llm, llm.embedder


def log_startup(settings: Settings, embedder: Embedder | None, *, deprecated: list[str]) -> None:
    """Log cau hinh LLM mot lan luc khoi dong. Khong bao gio in key."""
    for old in deprecated:
        log.warning("%s is deprecated, use %s", old, DEPRECATED_ALIASES.get(old, "the new variable"))
    if embedder is None:
        # Mot lan o day, khong lap lai moi request (SemanticCache im lang khi embedder None).
        log.warning(L3_DISABLED)
    log.info(
        "llm san sang: ai_mock=%s provider=%s chat_model=%s moderation_model=%s semantic_l3=%s",
        settings.ai_mock,
        "anthropic",
        settings.anthropic_model_chat,
        settings.anthropic_model_moderation,
        "on" if embedder is not None else "off",
    )
