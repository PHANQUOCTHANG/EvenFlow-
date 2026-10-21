import { redirect } from "next/navigation";

/** /moderator chưa có trang tổng quan riêng: vào thẳng hàng đợi thẩm định. */
export default function ModeratorHome() {
  redirect("/moderator/reviews");
}
