import Link from "next/link";

import { DomainGallery } from "@/components/showcase/domain-gallery";
import { PrimitiveGallery } from "@/components/showcase/primitive-gallery";
import { TokenGallery } from "@/components/showcase/token-gallery";
import { Alert } from "@/components/ui";

export const metadata = {
  title: "Design system — EventFlow",
  description: "Bộ token và thành phần giao diện dùng chung của EventFlow. Toàn bộ số liệu là dữ liệu mẫu.",
};

export default function ShowcasePage() {
  return (
    <div className="flex flex-col gap-xl">
      <div className="flex flex-col gap-sm">
        <h1 className="text-headline-xl text-fg">Design system</h1>
        <p className="text-body-lg text-fg-muted">
          Những gì đã dựng cho giao diện EventFlow: token, thành phần dùng chung, và các thành phần
          nghiệp vụ của phòng chờ, chọn vé, thanh toán.
        </p>
      </div>

      {/* Handoff muc 8: moi con so trong prototype phai duoc gan nhan la du lieu mau.
        * Dat o dau trang va la Alert (co role) nen screen reader doc duoc, khong chi la mau sac. */}
      <Alert variant="warning" title="Toàn bộ số liệu trên trang này là dữ liệu mẫu">
        Tên sự kiện, giá vé, số thứ tự hàng đợi, thời gian ước tính và các đồng hồ đếm ngược đều là
        dữ liệu minh hoạ. Đây không phải một sự kiện đang mở bán, và không có yêu cầu nào được gửi
        tới máy chủ từ trang này.
      </Alert>

      <TokenGallery />
      <PrimitiveGallery />
      <DomainGallery />

      <p className="text-body-md">
        <Link href="/" className="ef-focus-ring rounded-control text-primary">
          ← Về trang chủ
        </Link>
      </p>
    </div>
  );
}
