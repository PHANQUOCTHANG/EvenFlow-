import { SalesClient } from "./sales-client";

export const metadata = {
  title: "Doanh số — Ban tổ chức EvenFlow",
  description: "Doanh số mẫu của sự kiện (chưa có API doanh số thật)",
};

export default async function OrganizerSalesPage({
  params,
}: {
  params: Promise<{ eventId: string }>;
}) {
  const { eventId } = await params;
  return <SalesClient eventId={eventId} />;
}
