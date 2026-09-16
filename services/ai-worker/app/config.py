"""Cau hinh 12-factor. Validate luc khoi dong -- sai thi chet ngay.

Mot service khoi dong duoc voi cau hinh sai roi chet luc 20:00:00 la kich ban
te nhat co the. Tha khong khoi dong duoc con hon.
"""

from __future__ import annotations

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    redis_url: str = "redis://localhost:6379/0"
    amqp_url: str = "amqp://eventflow:eventflow@localhost:5672/"

    gemini_api_key: str = ""
    gemini_mock: bool = True
    gemini_model_fast: str = "gemini-2.0-flash"
    gemini_model_deep: str = "gemini-2.5-pro"

    # Dat THAP hon han muc that ~10% de chua bien an toan (xem gemini/limiter.py).
    gemini_rpm: int = 55
    gemini_tpm: int = 900_000

    # So consumer moi hang doi. Chat duoc uu tien cao nhat vi khach dang doi
    # truc tiep; report chi can 1 vi khong ai ngoi cho no.
    chat_consumers: int = 20
    moderation_consumers: int = 5
    anomaly_consumers: int = 2
    report_consumers: int = 1

    internal_api_base: str = "http://gateway:8080"


settings = Settings()
