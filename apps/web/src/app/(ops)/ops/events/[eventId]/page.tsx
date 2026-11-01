import { OpsWarRoom } from "@/components/ops";

export const metadata = {
  title: "War room — Vận hành EvenFlow",
  description: "Phễu chuyển đổi, hàng chờ, tồn kho và AI của một sự kiện",
};

// So lieu song: khong prerender luc build.
export const dynamic = "force-dynamic";

/** Vo mong: KHONG fetch o server (token nam o trinh duyet, AC-26). Hook cua OpsWarRoom tai so lieu o client;
 *  404 hien OpsNotFoundView ngay trong LiveWarRoom. */
export default async function OpsWarRoomPage({ params }: { params: Promise<{ eventId: string }> }) {
  const { eventId } = await params;
  return <OpsWarRoom key={eventId} eventId={eventId} initialSnapshot={null} />;
}
