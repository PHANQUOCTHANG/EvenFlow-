import { notFound } from "next/navigation";

import { OpsWarRoom } from "@/components/ops";
import {
  OpsServiceError,
  getOpsWarRoom,
  opsErrorStatus,
  type OpsWarRoomSnapshot,
} from "@/lib/ops-service";

export const metadata = {
  title: "War room — Vận hành EvenFlow",
  description: "Phễu chuyển đổi, hàng chờ, tồn kho và AI của một sự kiện",
};

// So lieu song: khong prerender luc build.
export const dynamic = "force-dynamic";

export default async function OpsWarRoomPage({
  params,
}: {
  params: Promise<{ eventId: string }>;
}) {
  const { eventId } = await params;

  let snapshot: OpsWarRoomSnapshot | null = null;
  let initialError: { status: number; message: string } | undefined;
  let missing = false;

  try {
    snapshot = await getOpsWarRoom(eventId);
  } catch (err) {
    const status = opsErrorStatus(err);
    if (status === 404) {
      missing = true;
    } else {
      // Object thuong, khong truyen class qua ranh gioi server/client.
      initialError = {
        status: typeof status === "number" ? status : 500,
        // Chi message cua OpsServiceError ra client; loi la khong lo chi tiet noi bo.
        message:
          err instanceof OpsServiceError ? err.message : "Không tải được số liệu war room.",
      };
    }
  }

  // Goi ngoai try/catch: notFound() nem loi dieu huong, khong duoc nuot.
  if (missing) notFound();

  return (
    <OpsWarRoom
      key={eventId}
      eventId={eventId}
      initialSnapshot={snapshot}
      initialError={initialError}
    />
  );
}
