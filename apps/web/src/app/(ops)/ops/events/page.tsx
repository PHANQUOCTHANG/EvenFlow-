import { OpsEventsClient } from "./events-client";

export const metadata = {
  title: "War room — Vận hành EvenFlow",
  description: "Chọn sự kiện để theo dõi phễu, hàng chờ, tồn kho và AI",
};

// So lieu song, tai o client (token o trinh duyet): khong prerender luc build.
export const dynamic = "force-dynamic";

export default function OpsEventsPage() {
  return (
    <div className="mx-auto max-w-4xl space-y-lg py-md">
      <header className="space-y-xs">
        <h1 className="text-headline-xl font-bold text-fg">War room</h1>
        <p className="text-body-md text-fg-muted">Chọn một sự kiện để mở màn hình vận hành.</p>
      </header>
      <OpsEventsClient />
    </div>
  );
}
