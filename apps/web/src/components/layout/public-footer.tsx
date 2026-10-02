import { Container } from "./container";

export function PublicFooter() {
  return (
    <footer className="mt-xl border-t border-border bg-surface">
      <Container className="flex flex-col gap-xs py-lg text-body-sm text-fg-muted">
        <p>EventFlow — nền tảng bán vé sự kiện chịu tải đột biến.</p>
        <p>Dự án học tập. Mọi số liệu trong giao diện là dữ liệu mẫu.</p>
      </Container>
    </footer>
  );
}
