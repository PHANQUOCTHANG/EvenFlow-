"""Domain types va constants cho kiem duyet noi dung AI (EV-120).

BR-E4 quy dinh 4 truc kiem duyet:
- forbidden_content: noi dung bi cam (chat kich thich, vu khi, bao luc...)
- misleading: thong tin gay hieu lam, lua dao
- copyright: vi pham ban quyen, nhan hieu
- incomplete_info: thong tin qua so sai, khong ro rang

Nguong quyet dinh:
- score < 0.3  -> AUTO_APPROVED
- 0.3 <= score <= 0.8 -> NEEDS_HUMAN
- score > 0.8  -> AUTO_REJECTED
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum


class AiVerdict(str, Enum):
    AUTO_APPROVED = "AUTO_APPROVED"
    NEEDS_HUMAN = "NEEDS_HUMAN"
    AUTO_REJECTED = "AUTO_REJECTED"


AUTO_APPROVE_THRESHOLD = 0.3
AUTO_REJECT_THRESHOLD = 0.8


def calculate_verdict(score: float) -> AiVerdict:
    if score < AUTO_APPROVE_THRESHOLD:
        return AiVerdict.AUTO_APPROVED
    if score <= AUTO_REJECT_THRESHOLD:
        return AiVerdict.NEEDS_HUMAN
    return AiVerdict.AUTO_REJECTED


@dataclass
class ModerationJob:
    job_id: str
    event_id: str
    event_title: str
    event_description: str
    enqueued_at: float


@dataclass
class ModerationResult:
    score: float
    verdict: AiVerdict
    axes: dict[str, float] = field(default_factory=dict)
    reasons: list[str] = field(default_factory=list)
