"""Suy bien co tang cho tro ly AI (BR-A7, L7 trong docs/02).

Nguyen tac: khach trong phong cho KHONG BAO GIO duoc thay loi 429 hay "AI dang
ban". Tro ly co the tra loi kem hon, nhung phai luon tra loi duoc mot dieu gi do
huu ich.

Bon nac, chuyen tu dong theo do tre hang doi va ngan sach token, hoac do Ops bat
tay trong war room.
"""

from __future__ import annotations

import asyncio
import enum
import logging
from collections.abc import Callable
from dataclasses import dataclass

from redis.asyncio import Redis

log = logging.getLogger(__name__)


KEY_QUEUE_LAG = "ai:signals:queue_lag_seconds"
KEY_BUDGET_RATIO = "ai:signals:budget_used_ratio"
KEY_CIRCUIT_OPEN = "ai:signals:circuit_open"
KEY_MANUAL_OVERRIDE = "ai:signals:manual_override"


class Mode(enum.StrEnum):
    NORMAL = "NORMAL"       # day du LLM
    SAVING = "SAVING"       # chi FAQ + cache, rut ngan dau ra
    FAQ_ONLY = "FAQ_ONLY"   # 100% template, khong goi Gemini
    OFF = "OFF"             # an tro ly, hien kenh CSKH


@dataclass(frozen=True)
class Signals:
    """Tin hieu quyet dinh nac suy bien."""

    chat_queue_lag_seconds: float
    budget_used_ratio: float      # 0..1
    gemini_circuit_open: bool
    manual_override: Mode | None = None


# Nguong chuyen nac. Dat o day thay vi rai rac trong code de Ops doc duoc va
# doi duoc ma khong phai doc Python.
LAG_SAVING = 30.0
LAG_FAQ_ONLY = 120.0
BUDGET_SAVING = 0.80
BUDGET_EXHAUSTED = 1.0


def decide(s: Signals) -> Mode:
    """Chon nac suy bien.

    Ops ghi de luon thang moi tin hieu tu dong: trong mot su co that, nguoi truc
    biet nhung dieu ma metric chua kip phan anh.
    """
    if s.manual_override is not None:
        return s.manual_override

    if s.gemini_circuit_open:
        return Mode.OFF

    if s.budget_used_ratio >= BUDGET_EXHAUSTED or s.chat_queue_lag_seconds >= LAG_FAQ_ONLY:
        return Mode.FAQ_ONLY

    if s.budget_used_ratio >= BUDGET_SAVING or s.chat_queue_lag_seconds >= LAG_SAVING:
        return Mode.SAVING

    return Mode.NORMAL


@dataclass(frozen=True)
class Policy:
    """Chinh sach cu the ung voi mot nac."""

    call_llm: bool
    max_output_tokens: int
    allow_open_questions: bool   # cau hoi tu do vs chi FAQ
    user_notice: str | None


_POLICIES: dict[Mode, Policy] = {
    Mode.NORMAL: Policy(
        call_llm=True, max_output_tokens=512, allow_open_questions=True, user_notice=None,
    ),
    Mode.SAVING: Policy(
        call_llm=True, max_output_tokens=192, allow_open_questions=False,
        user_notice="Tro ly dang uu tien cac cau hoi thuong gap de phuc vu nhanh hon.",
    ),
    Mode.FAQ_ONLY: Policy(
        call_llm=False, max_output_tokens=0, allow_open_questions=False,
        user_notice="Tro ly dang o che do cau hoi thuong gap. "
                    "Neu can them ho tro, vui long lien he CSKH.",
    ),
    Mode.OFF: Policy(
        call_llm=False, max_output_tokens=0, allow_open_questions=False,
        user_notice="Tro ly tam thoi khong kha dung. Vui long lien he CSKH.",
    ),
}


def policy_for(mode: Mode) -> Policy:
    return _POLICIES[mode]


def _parse_float(val: bytes | str | None, default: float = 0.0) -> float:
    if val is None:
        return default
    try:
        if isinstance(val, bytes):
            val = val.decode()
        return float(val)
    except (ValueError, TypeError):
        return default


def _parse_bool(val: bytes | str | None, default: bool = False) -> bool:
    if val is None:
        return default
    try:
        if isinstance(val, bytes):
            val = val.decode()
        return val.strip().lower() in ("1", "true", "yes", "open")
    except (ValueError, TypeError, AttributeError):
        return default


def _parse_mode(val: bytes | str | None) -> Mode | None:
    if val is None:
        return None
    try:
        if isinstance(val, bytes):
            val = val.decode()
        cleaned = val.strip().upper()
        if cleaned in ("", "AUTO", "NONE"):
            return None
        return Mode(cleaned)
    except (ValueError, TypeError, AttributeError):
        return None


class SignalReader:
    """Doc cac tin hieu suy bien tu Redis (EV-164)."""

    def __init__(self, redis: Redis) -> None:
        self._redis = redis

    async def fetch(self, default_override: Mode | None = None) -> Signals:
        try:
            lag_raw, budget_raw, circuit_raw, override_raw = await asyncio.gather(
                self._redis.get(KEY_QUEUE_LAG),
                self._redis.get(KEY_BUDGET_RATIO),
                self._redis.get(KEY_CIRCUIT_OPEN),
                self._redis.get(KEY_MANUAL_OVERRIDE),
                return_exceptions=True,
            )
        except (RuntimeError, ConnectionError, OSError) as exc:
            log.warning("doc signals tu redis that bai: %s", exc)
            return Signals(
                chat_queue_lag_seconds=0.0,
                budget_used_ratio=0.0,
                gemini_circuit_open=False,
                manual_override=default_override,
            )

        lag = _parse_float(lag_raw if not isinstance(lag_raw, BaseException) else None, 0.0)
        budget = _parse_float(budget_raw if not isinstance(budget_raw, BaseException) else None, 0.0)
        circuit = _parse_bool(circuit_raw if not isinstance(circuit_raw, BaseException) else None, False)
        override = _parse_mode(override_raw if not isinstance(override_raw, BaseException) else None)
        if override is None:
            override = default_override

        return Signals(
            chat_queue_lag_seconds=lag,
            budget_used_ratio=budget,
            gemini_circuit_open=circuit,
            manual_override=override,
        )


class SignalCache:
    """Bo nho dem tin hieu suy bien duoc cap nhat ngam (EV-164).

    Giup cac consumer dong bo hoac khong can await moi lan xu ly job
    van luon co tin hieu gan nhat.
    """

    def __init__(self, reader: SignalReader | None = None, initial: Signals | None = None) -> None:
        self._reader = reader
        self._current: Signals = initial or Signals(
            chat_queue_lag_seconds=0.0,
            budget_used_ratio=0.0,
            gemini_circuit_open=False,
            manual_override=None,
        )
        self._task: asyncio.Task | None = None

    @property
    def current(self) -> Signals:
        return self._current

    def get_signals(self) -> Signals:
        return self._current

    async def refresh(self, default_override: Mode | None = None) -> Signals:
        if self._reader is not None:
            self._current = await self._reader.fetch(default_override=default_override)
        return self._current

    def start_background_refresh(
        self,
        interval_seconds: float = 5.0,
        override_supplier: Callable[[], Mode | None] | None = None,
    ) -> None:
        async def _loop() -> None:
            while True:
                try:
                    override = override_supplier() if override_supplier else None
                    await self.refresh(default_override=override)
                except asyncio.CancelledError:
                    break
                except (RuntimeError, ConnectionError, OSError) as exc:
                    log.warning("loi refresh signals cache: %s", exc)
                try:
                    await asyncio.sleep(interval_seconds)
                except asyncio.CancelledError:
                    break

        self._task = asyncio.create_task(_loop())

    async def stop_background_refresh(self) -> None:
        if self._task is not None:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None
