"""Tool-call lay so lieu THAT tu he thong (BR-A5).

Nguyen tac xuyen suot cua thiet ke AI trong EventFlow:

    LLM DIEN GIAI. HE THONG TINH TOAN.

Moi con so xuat hien trong cau tra loi cua tro ly -- vi tri hang cho, thoi gian
uoc tinh, tinh trang ve, doanh so trong bao cao -- deu phai di qua day. Khong
bao gio de mo hinh tu nghi ra mot con so.

Ly do rat cu the: mot cau tra loi sai kieu "ban o vi tri 200, khoang 5 phut nua"
trong khi that ra khach o vi tri 40.000 se tao ra mot lan khieu nai that, va
hang nghin lan nhu the trong mot dot mo ban thi thanh su co truyen thong.
"""

from __future__ import annotations

import logging

import httpx

log = logging.getLogger(__name__)


class SystemTools:
    """Cac cong cu doc so lieu that. Chi doc, khong bao gio ghi.

    Tro ly AI khong duoc phep thay doi trang thai he thong -- khong huy don,
    khong giu ve, khong dieu chinh hang cho. Quyen ghi khong nam o day de
    mot cu prompt injection thanh cong cung khong lam duoc gi nguy hiem.
    """

    def __init__(self, base_url: str, client: httpx.AsyncClient | None = None) -> None:
        self._base = base_url.rstrip("/")
        self._client = client or httpx.AsyncClient(timeout=2.0)

    async def queue_position(self, event_id: str, queue_token: str) -> dict:
        """Vi tri that trong hang cho."""
        r = await self._client.get(
            f"{self._base}/v1/events/{event_id}/queue/status",
            headers={"X-Queue-Token": queue_token},
        )
        r.raise_for_status()
        return r.json()

    async def availability_bucket(self, event_id: str) -> str:
        """Tinh trang ve dang KHOANG, khong phai con so chinh xac.

        Tra ve MANY / FEW / SOLD_OUT. Con so chinh xac bi giau di co chu y
        (BR-A4): no vua gay tam ly hoang loan, vua cho bot mot kenh do toc do
        ban de canh thoi gian tan cong.
        """
        r = await self._client.get(f"{self._base}/v1/events/{event_id}/availability")
        r.raise_for_status()
        return r.json().get("bucket", "MANY")

    async def event_config(self, event_id: str) -> dict:
        """Cau hinh dung de dien vao template FAQ."""
        r = await self._client.get(f"{self._base}/v1/events/{event_id}")
        r.raise_for_status()
        data = r.json()
        return {
            "max_per_identity": data.get("max_per_identity", 4),
            "hold_minutes": data.get("hold_ttl_seconds", 600) // 60,
        }

    async def event_context(self, event_id: str) -> str:
        """Ngu canh su kien de nhet vao prompt.

        Chi lay cac truong CAN cho viec tra loi. Nhet ca ban ghi vao prompt vua
        ton token vua tang be mat ro ri thong tin noi bo.
        """
        r = await self._client.get(f"{self._base}/v1/events/{event_id}")
        r.raise_for_status()
        d = r.json()
        fields = ("title", "venue", "starts_at", "sale_start_at", "description")
        return "\n".join(f"{k}: {d.get(k, '')}" for k in fields)
