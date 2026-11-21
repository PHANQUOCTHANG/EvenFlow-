import { SalesClient } from "./sales-client";

export const metadata = {
  title: "Doanh số — Ban tổ chức EvenFlow",
  description: "Doanh số thời gian thực của sự kiện (cập nhật mỗi 4 giây)",
};

export default async function OrganizerSalesPage({
  params,
}: {
  params: Promise<{ eventId: string }>;
}) {
  const { eventId } = await params;
  return <SalesClient eventId={eventId} />;
}
