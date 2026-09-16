"""Tang FAQ -- tra loi khong ton mot token nao (L2).

Y tuong quan trong nhat o day khong phai la tiet kiem tien, ma la DO CHINH XAC.

Cac cau hoi pho bien nhat trong phong cho deu hoi ve SO LIEU DONG cua he thong:
vi tri cua toi, con bao lau, con ve khong. LLM khong biet nhung so do, va neu
duoc hoi no se doan -- tao ra cau tra loi nghe rat thuyet phuc nhung sai.

Nen: khop y dinh bang luat, lay so lieu bang tool-call vao he thong that, roi
dien vao template. Nhanh hon, re hon, va dung (BR-A5).
"""

from __future__ import annotations

import logging
import re
from dataclasses import dataclass
from typing import Callable, Awaitable

from app.pipeline.semantic_cache import normalize

log = logging.getLogger(__name__)


@dataclass(frozen=True)
class Intent:
    name: str
    patterns: tuple[str, ...]
    needs_live_data: bool


# Tap y dinh. Rut ra tu log cau hoi that; bo sung sau moi dot mo ban.
INTENTS: tuple[Intent, ...] = (
    Intent("queue_position",
           ("toi dang o vi tri nao", "thu tu cua toi", "so may", "vi tri cua toi",
            "bao gio toi luot", "con bao lau", "khi nao toi luot", "cho bao lau"),
           needs_live_data=True),
    Intent("ticket_availability",
           ("con ve khong", "con ve hang", "het ve chua", "con bao nhieu ve"),
           needs_live_data=True),
    Intent("lose_spot",
           ("tat trinh duyet", "mat cho khong", "thoat ra", "refresh", "f5",
            "dong tab", "mat mang", "rot mang"),
           needs_live_data=False),
    Intent("hold_duration",
           ("giu ghe bao lau", "bao nhieu phut de thanh toan", "het gio thanh toan",
            "dem nguoc"),
           needs_live_data=False),
    Intent("max_tickets",
           ("mua toi da", "mua duoc may ve", "gioi han ve"),
           needs_live_data=False),
    Intent("refund_policy",
           ("hoan ve", "tra ve", "doi ve", "huy ve", "hoan tien"),
           needs_live_data=False),
    Intent("payment_methods",
           ("thanh toan bang gi", "phuong thuc thanh toan", "the tin dung", "momo",
            "chuyen khoan"),
           needs_live_data=False),
    Intent("fairness",
           ("vao som co loi khong", "xep hang kieu gi", "co cong bang khong",
            "sao nguoi khac vao truoc"),
           needs_live_data=False),
)

# Template tinh. Nhung cau nay duoc bien tap boi con nguoi va da duyet -- khong
# de LLM viet lai, vi chung la cam ket cua nen tang voi khach.
STATIC_ANSWERS: dict[str, str] = {
    "lose_spot":
        "Ban khong mat cho. Vi tri cua ban gan voi tai khoan chu khong phai voi "
        "tab trinh duyet. Neu mat mang, he thong van giu cho ban trong 5 phut. "
        "Chi can mo lai trang la thay dung vi tri cu.",
    "hold_duration":
        "Sau khi chon ve, ban co 10 phut de hoan tat thanh toan. Dong ho dem "
        "nguoc lay theo gio may chu, nen ban khong can lo lech gio thiet bi.",
    "max_tickets":
        "Moi tai khoan mua toi da {max_per_identity} ve cho su kien nay. "
        "Gioi han nay tinh gop tren tat ca cac don ban da dat.",
    "refund_policy":
        "Ve da thanh toan khong hoan lai, tru truong hop ban to chuc huy su kien. "
        "Khi do toan bo ve se duoc hoan tu dong ve phuong thuc thanh toan goc.",
    "payment_methods":
        "Ban co the thanh toan bang the noi dia, the quoc te, vi dien tu hoac "
        "chuyen khoan ngan hang.",
    "fairness":
        "Vao som khong co loi the. Tat ca nhung ai vao truoc gio mo ban deu duoc "
        "xao tron ngau nhien tai dung thoi diem mo ban, roi moi cap so thu tu. "
        "Nguoi vao truoc 2 tieng va nguoi vao truoc 2 giay co co hoi nhu nhau.",
}

FALLBACK = (
    "Minh chua tra loi duoc cau nay. Ban co the xem thong tin chi tiet o trang "
    "su kien, hoac lien he CSKH de duoc ho tro truc tiep."
)


class FaqMatcher:
    def __init__(self, tools_fetcher: Callable[..., Awaitable[dict]] | None = None) -> None:
        self._compiled = {
            intent.name: re.compile("|".join(re.escape(p) for p in intent.patterns))
            for intent in INTENTS
        }
        self._intents = {i.name: i for i in INTENTS}
        self._tools_fetcher = tools_fetcher

    def detect(self, question: str) -> Intent | None:
        norm = normalize(question)
        for name, rx in self._compiled.items():
            if rx.search(norm):
                return self._intents[name]
        return None

    async def match(self, question: str, event_id: str, queue_token: str, tools) -> str | None:
        """Tra ve cau tra loi FAQ, hoac None de chuyen tiep len LLM."""
        intent = self.detect(question)
        if intent is None:
            return None

        if not intent.needs_live_data:
            template = STATIC_ANSWERS.get(intent.name)
            if template is None:
                return None
            if "{" in template:
                cfg = await tools.event_config(event_id)
                return template.format(**cfg)
            return template

        # Cau hoi ve so lieu dong -> lay tu he thong that, khong doan (BR-A5).
        if intent.name == "queue_position":
            pos = await tools.queue_position(event_id, queue_token)
            if pos["state"] == "ADMITTED":
                return "Da toi luot ban. Hay quay lai tab dat ve de chon ve ngay."
            if pos["state"] == "LOBBY":
                return ("Ban da o trong phong cho. So thu tu se duoc cap ngau nhien "
                        "dung thoi diem mo ban, nen ban khong can lam gi them.")
            if pos["eta_seconds"] and pos["eta_seconds"] > 0:
                return (f"Ban dang o vi tri {pos['rank']:,}. Uoc tinh khoang "
                        f"{pos['eta_seconds'] // 60} phut nua toi luot ban.")
            return (f"Ban dang o vi tri {pos['rank']:,}. He thong chua uoc tinh duoc "
                    f"thoi gian cho, minh se cap nhat ngay khi co.")

        if intent.name == "ticket_availability":
            # CO Y khong noi con chinh xac bao nhieu ve (BR-A4): con so chinh xac
            # vua tao tam ly hoang loan, vua giup bot do duoc toc do ban.
            avail = await tools.availability_bucket(event_id)
            return {
                "MANY": "Cac hang ve van con nhieu. Ban cu binh tinh cho toi luot.",
                "FEW": "Mot so hang ve sap het. Khi toi luot, ban nen chon nhanh.",
                "SOLD_OUT": "Rat tiec, ve da ban het. Ban co the dang ky nhan thong bao "
                            "neu co ve duoc tra lai.",
            }.get(avail, FALLBACK)

        return None

    def fallback(self, notice: str | None) -> str:
        return f"{FALLBACK}\n\n{notice}" if notice else FALLBACK
