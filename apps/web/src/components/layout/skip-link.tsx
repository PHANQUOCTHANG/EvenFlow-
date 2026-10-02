/** Link bo qua nav. Phai la phan tu focus duoc DAU TIEN trong trang.
 *
 * Khong duoc an bang `display: none` hay `visibility: hidden` — nhu vay ban phim khong
 * bao gio toi duoc no va link thanh vo dung. Cach dung la dua ra ngoai vung nhin
 * (`sr-only`) roi tra ve vi tri that khi focus. */
export function SkipLink({ targetId = "main" }: { targetId?: string }) {
  return (
    <a
      href={`#${targetId}`}
      className="sr-only focus:not-sr-only focus:absolute focus:left-gutter focus:top-gutter focus:z-50 focus:rounded-control focus:border focus:border-border-strong focus:bg-surface focus:px-md focus:py-sm focus:text-label-lg focus:text-fg focus:shadow-float"
    >
      Bỏ qua điều hướng, tới nội dung chính
    </a>
  );
}
