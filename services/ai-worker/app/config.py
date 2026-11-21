"""Cau hinh 12-factor. Validate luc khoi dong -- sai thi chet ngay.

Mot service khoi dong duoc voi cau hinh sai roi chet luc 20:00:00 la kich ban
te nhat co the. Tha khong khoi dong duoc con hon.
"""

from __future__ import annotations

import os
from collections.abc import Mapping
from pathlib import Path

from dotenv import dotenv_values
from pydantic import AliasChoices, Field, SecretStr, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

# Bien cu (truoc khi chuyen sang Anthropic) -> ten moi. Van doc duoc trong mot ban phat hanh
# (spec C1/C2); dung bien cu thi khoi dong canh bao mot lan (app.llm.wiring.log_startup).
DEPRECATED_ALIASES: dict[str, str] = {
    "GEMINI_MOCK": "AI_MOCK",
    "GEMINI_TIMEOUT_SECONDS": "AI_TIMEOUT_SECONDS",
    "GEMINI_RPM": "AI_RPM",
    "GEMINI_TPM": "AI_TPM",
}

DEFAULT_MODEL = "claude-haiku-5-5"
_FALSE = {"0", "false", "f", "no", "n", "off"}


def default_topology_file(config_file: Path | None = None) -> Path:
    """Chay tu repo: <repo>/deploy/rabbitmq/definitions.json.

    Trong image (/app/app/config.py) khong co thu muc cha thu 3; Dockerfile dat
    AI_TOPOLOGY_FILE nen gia tri nay chi la duong lui tuong doi.
    """
    parents = (config_file or Path(__file__)).resolve().parents
    if len(parents) > 3:
        return parents[3] / "deploy" / "rabbitmq" / "definitions.json"
    return Path("topology") / "definitions.json"


class Settings(BaseSettings):
    # validate_by_name: Settings(ai_mock=False) van dung duoc du truong co validation_alias.
    # hide_input_in_errors: loi validate khong in lai gia tri dau vao (co the chua key).
    model_config = SettingsConfigDict(
        env_file=".env", extra="ignore", validate_by_name=True, hide_input_in_errors=True,
    )

    redis_url: str = "redis://localhost:6379/0"
    amqp_url: str = "amqp://eventflow:eventflow@localhost:5672/"
    ai_topology_file: Path = Field(default_factory=default_topology_file)

    # Sinh van ban (chat + kiem duyet) qua Anthropic. AI_MOCK=1 (mac dinh): khong goi mang.
    # Thu tu AliasChoices: ten moi truoc, nen dat ca hai thi ten moi thang.
    ai_mock: bool = Field(default=True, validation_alias=AliasChoices("AI_MOCK", "GEMINI_MOCK"))
    anthropic_api_key: SecretStr = SecretStr("")
    anthropic_model_chat: str = Field(default=DEFAULT_MODEL, min_length=1)
    anthropic_model_moderation: str = Field(default=DEFAULT_MODEL, min_length=1)
    # Ngan hon TTL singleflight (30s): nguoi dan dau phai xong hoac bo cuoc truoc khi
    # khoa het han, neu khong mot cau hoi co the bi goi hai lan.
    ai_timeout_seconds: float = Field(
        default=10.0, gt=0, validation_alias=AliasChoices("AI_TIMEOUT_SECONDS", "GEMINI_TIMEOUT_SECONDS"),
    )
    # Dat THAP hon han muc that ~10% de chua bien an toan (xem llm/limiter.py).
    ai_rpm: int = Field(default=55, gt=0, validation_alias=AliasChoices("AI_RPM", "GEMINI_RPM"))
    ai_tpm: int = Field(default=900_000, gt=0, validation_alias=AliasChoices("AI_TPM", "GEMINI_TPM"))
    # Cong them vao max_tokens gui di (va vao giay phep limiter) de phan suy nghi cua model
    # khong an mat cau tra loi (spec AC-3).
    ai_thinking_headroom_tokens: int = Field(default=1024, ge=0)

    # Chi cho embedding cua cache ngu nghia (L3). Rong o che do that: L3 tat, khong loi.
    gemini_api_key: SecretStr = SecretStr("")
    gemini_embed_model: str = "text-embedding-004"

    # So consumer moi hang doi. Chat duoc uu tien cao nhat vi khach dang doi
    # truc tiep; report chi can 1 vi khong ai ngoi cho no. Anomaly/report dat 0
    # duoc (chua co handler: EV-167, EV-168) -- chat va moderation thi khong.
    chat_consumers: int = Field(default=20, ge=1)
    moderation_consumers: int = Field(default=5, ge=1)
    anomaly_consumers: int = Field(default=2, ge=0)
    report_consumers: int = Field(default=1, ge=0)

    # Thoi gian cho handler dang chay xong khi tat; qua han thi huy, message quay lai hang.
    shutdown_grace_seconds: float = Field(default=20.0, gt=0)

    # API cong khai qua gateway (tool tra cuu thu hang, tinh trang ve, su kien).
    internal_api_base: str = "http://gateway:8080"
    # Noi gui ket qua kiem duyet: PATCH /internal/v1/events/{id}/ai-verdict cua event-service.
    # Gateway CHAN /internal/* tu ben ngoai, nen phai goi thang event-service trong mang noi bo.
    # Rong thi dung internal_api_base (giu hanh vi cu).
    moderation_api_base: str = ""

    @property
    def moderation_base(self) -> str:
        return self.moderation_api_base or self.internal_api_base

    @model_validator(mode="after")
    def _that_can_key_anthropic(self) -> Settings:
        if not self.ai_mock and not self.anthropic_api_key.get_secret_value():
            raise ValueError("AI_MOCK=0 nhung thieu ANTHROPIC_API_KEY")
        return self


def resolve_mock_flag(env: Mapping[str, str]) -> bool:
    """Co mock tu mot bang bien moi truong: AI_MOCK thang GEMINI_MOCK, khong co gi thi mock.

    Dung chung cho LlmClient() khong tham so, de no khop voi Settings.ai_mock.
    """
    upper = {k.upper(): v for k, v in env.items()}
    for name in ("AI_MOCK", "GEMINI_MOCK"):
        value = upper.get(name)
        if value is not None and value.strip() != "":
            return value.strip().lower() not in _FALSE
    return True


def deprecated_aliases_used(env_file: str | Path | None = ".env") -> list[str]:
    """Ten bien cu dang co hieu luc: co dat (os.environ hoac file .env) ma ten moi thi khong.

    Khong phan biet hoa thuong, giong pydantic-settings. env_file=None: chi xem os.environ.
    """
    merged: dict[str, str | None] = {}
    if env_file is not None and Path(env_file).is_file():
        merged.update({k.upper(): v for k, v in dotenv_values(env_file).items()})
    merged.update({k.upper(): v for k, v in os.environ.items()})
    return [old for old, new in DEPRECATED_ALIASES.items() if old in merged and new not in merged]


settings = Settings()
