import { Alert } from "@/components/ui";

export interface LobbyNoticeProps {
  className?: string;
}

/** Thong bao co che phong cho ngau nhien LOBBY (BR-Q1).
 *
 * "Moi nguoi vao truoc sale_start_at duoc gom vao LOBBY khong co so thu tu.
 * Dung T0 he thong xao tron ngau nhien roi moi cap rank.
 * Vao som 2 tieng hay 2 giay deu co co hoi nhu nhau."
 *
 * Phai noi ro de khach khong ngo nhan rang vao som se co so thu tu truoc. */
export function LobbyNotice({ className }: LobbyNoticeProps) {
  return (
    <Alert
      variant="info"
      title="Quy chế phòng chờ công bằng (Fair Queue)"
      className={className}
    >
      <p className="mt-xs text-body-sm">
        Hệ thống EvenFlow áp dụng cơ chế <strong>xáo trộn ngẫu nhiên tại thời điểm mở bán (T0)</strong>.
        Mọi người truy cập phòng chờ trước giờ mở bán đều có cơ hội nhận lượt như nhau.
        Vào sớm không tạo lợi thế ưu tiên thứ tự.
      </p>
    </Alert>
  );
}
