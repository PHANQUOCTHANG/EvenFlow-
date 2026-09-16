"""Suy bien co tang cho tro ly AI (BR-A7, L7 trong docs/02).

Nguyen tac: khach trong phong cho KHONG BAO GIO duoc thay loi 429 hay "AI dang
ban". Tro ly co the tra loi kem hon, nhung phai luon tra loi duoc mot dieu gi do
huu ich.

Bon nac, chuyen tu dong theo do tre hang doi va ngan sach token, hoac do Ops bat
tay trong war room.
"""

from __future__ import annotations

import enum
import logging
from dataclasses import dataclass

log = logging.getLogger(__name__)


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
