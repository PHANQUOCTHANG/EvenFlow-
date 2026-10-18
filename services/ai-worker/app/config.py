"""Cau hinh 12-factor. Validate luc khoi dong -- sai thi chet ngay.

Mot service khoi dong duoc voi cau hinh sai roi chet luc 20:00:00 la kich ban
te nhat co the. Tha khong khoi dong duoc con hon.
"""

from __future__ import annotations

from pathlib import Path

from pydantic import Field, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


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
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    redis_url: str = "redis://localhost:6379/0"
    amqp_url: str = "amqp://eventflow:eventflow@localhost:5672/"
    ai_topology_file: Path = Field(default_factory=default_topology_file)

    gemini_api_key: str = ""
    gemini_mock: bool = True
    gemini_model_fast: str = "gemini-2.0-flash"
    gemini_model_deep: str = "gemini-2.5-pro"
    gemini_embed_model: str = "text-embedding-004"
    # Ngan hon TTL singleflight (30s): nguoi dan dau phai xong hoac bo cuoc truoc khi
    # khoa het han, neu khong mot cau hoi co the bi goi hai lan.
    gemini_timeout_seconds: float = Field(default=10.0, gt=0)

    # Dat THAP hon han muc that ~10% de chua bien an toan (xem gemini/limiter.py).
    gemini_rpm: int = Field(default=55, gt=0)
    gemini_tpm: int = Field(default=900_000, gt=0)

    # So consumer moi hang doi. Chat duoc uu tien cao nhat vi khach dang doi
    # truc tiep; report chi can 1 vi khong ai ngoi cho no. Anomaly/report dat 0
    # duoc (chua co handler: EV-167, EV-168) -- chat va moderation thi khong.
    chat_consumers: int = Field(default=20, ge=1)
    moderation_consumers: int = Field(default=5, ge=1)
    anomaly_consumers: int = Field(default=2, ge=0)
    report_consumers: int = Field(default=1, ge=0)

    # Thoi gian cho handler dang chay xong khi tat; qua han thi huy, message quay lai hang.
    shutdown_grace_seconds: float = Field(default=20.0, gt=0)

    internal_api_base: str = "http://gateway:8080"

    @model_validator(mode="after")
    def _gemini_that_can_key(self) -> Settings:
        if not self.gemini_mock and not self.gemini_api_key:
            raise ValueError("GEMINI_MOCK=0 nhung thieu GEMINI_API_KEY")
        return self


settings = Settings()
