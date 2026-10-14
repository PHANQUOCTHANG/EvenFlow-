import { T } from "@/components/i18n/t";

import { Container } from "./container";

export function PublicFooter() {
  return (
    <footer className="mt-xl border-t border-border bg-surface">
      <Container className="flex flex-col gap-xs py-lg text-body-sm text-fg-muted">
        <p>
          <T k="footer.tagline" />
        </p>
        <p>
          <T k="footer.disclaimer" />
        </p>
      </Container>
    </footer>
  );
}
