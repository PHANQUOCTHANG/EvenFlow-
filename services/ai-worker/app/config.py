"""Cau hinh 12-factor. Validate luc khoi dong -- sai thi chet ngay.

Mot service khoi dong duoc voi cau hinh sai roi chet luc 20:00:00 la kich ban
te nhat co the. Tha khong khoi dong duoc con hon.
"""

from __future__ import annotations

import os
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Any

from dotenv import dotenv_values
from pydantic import AliasChoices, Field, SecretStr, field_validator, model_validator
from pydantic.fields import FieldInfo
from pydantic_settings import BaseSettings, PydanticBaseSettingsSource, SettingsConfigDict

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

EnvFiles = str | Path | Sequence[str | Path] | None


def clean_api_key(name: str, value: str) -> str:
    """Bo khoang trang bao ngoai (key doc tu file / k8s secret hay co \n cuoi) mot lan.

    Con khoang trang hay ky tu dieu khien / khong phai ASCII nhin thay o giua thi tu choi:
    SDK dat key vao header HTTP, header loi thi traceback cua httpx in lai nguyen key
    (findings X-1). Thong bao loi KHONG bao gio chua gia tri.
    """
    key = value.strip()
    if any(not (0x21 <= ord(ch) <= 0x7E) for ch in key):
        raise ValueError(f"{name} chua khoang trang hoac ky tu khong hop le (gia tri khong duoc in ra)")
    return key


def raw_env(env_file: EnvFiles = ".env") -> dict[str, str | None]:
    """Bien moi truong tho (khoa viet hoa): file .env (file sau de file truoc), roi os.environ de len.

    Giu nguyen TEN bien da dat -- pydantic-settings thi doi alias ve mot khoa, nen khong
    biet duoc ten moi hay ten cu dang co hieu luc.
    """
    files: list[str | Path]
    if env_file is None:
        files = []
    elif isinstance(env_file, (str, Path)):
        files = [env_file]
    else:
        files = list(env_file)
    merged: dict[str, str | None] = {}
    for f in files:
        if Path(f).is_file():
            merged.update({k.upper(): v for k, v in dotenv_values(f).items()})
    merged.update({k.upper(): v for k, v in os.environ.items()})
    return merged


class _AliasPrecedence(PydanticBaseSettingsSource):
    """Ten moi thang alias cu BAT KE nguon nao (spec C1, findings X-5/R-3).

    Mac dinh pydantic-settings ap thu tu nguon truoc thu tu AliasChoices: GEMINI_MOCK=0
    trong shell thang AI_MOCK=1 trong .env. Nguon nay dung truoc env/.env, chon ten moi neu
    no co o bat ky dau, chi lui ve ten cu khi ten moi khong co o dau ca.
    """

    def __init__(self, settings_cls: type[BaseSettings], env_file: EnvFiles) -> None:
        super().__init__(settings_cls)
        self._env_file = env_file

    def get_field_value(self, field: FieldInfo, field_name: str) -> tuple[Any, str, bool]:
        return None, field_name, False

    def __call__(self) -> dict[str, Any]:
        env = raw_env(self._env_file)
        out: dict[str, Any] = {}
        for old, new in DEPRECATED_ALIASES.items():
            for name in (new, old):
                if name in env and env[name] is not None:
                    # Khoa = ten moi (alias dau tien): de len khoa ma nguon env/.env tra ve.
                    out[new] = env[name]
                    break
        return out


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

    @classmethod
    def settings_customise_sources(
        cls,
        settings_cls: type[BaseSettings],
        init_settings: PydanticBaseSettingsSource,
        env_settings: PydanticBaseSettingsSource,
        dotenv_settings: PydanticBaseSettingsSource,
        file_secret_settings: PydanticBaseSettingsSource,
    ) -> tuple[PydanticBaseSettingsSource, ...]:
        env_file = getattr(dotenv_settings, "env_file", None)
        return (
            init_settings,
            _AliasPrecedence(settings_cls, env_file),
            env_settings,
            dotenv_settings,
            file_secret_settings,
        )

    @field_validator("anthropic_api_key", "gemini_api_key")
    @classmethod
    def _key_sach(cls, v: SecretStr, info: Any) -> SecretStr:
        return SecretStr(clean_api_key(str(info.field_name).upper(), v.get_secret_value()))

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
    Khop voi Settings: ten moi o bat ky nguon nao thi alias khong co hieu luc (_AliasPrecedence).
    """
    merged = raw_env(env_file)
    return [old for old, new in DEPRECATED_ALIASES.items() if old in merged and new not in merged]


settings = Settings()
