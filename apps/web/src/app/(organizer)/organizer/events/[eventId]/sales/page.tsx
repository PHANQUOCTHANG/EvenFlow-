import { SalesDashboard } from "@/components/organizer/sales-dashboard";
import { Card } from "@/components/ui/card";
import { getOrganizerEventById, getRealtimeSales } from "@/lib/organizer-service";

export const metadata = {
  title: "Doanh số Realtime — Ban tổ chức EvenFlow",
  description: "Theo dõi doanh thu và tiến độ bán vé realtime",
};

export default async function OrganizerSalesPage({
  params,
}: {
  params: Promise<{ eventId: string }>;
}) {
  const { eventId } = await params;
  const event = await getOrganizerEventById(eventId);

  if (!event) {
    return (
      <Card className="p-xl text-center space-y-sm my-lg">
        <h1 className="text-heading-md font-bold text-fg">Không tìm thấy sự kiện</h1>
        <p className="text-body-sm text-fg-muted">
          Sự kiện với mã &ldquo;{eventId}&rdquo; không tồn tại hoặc bạn không có quyền truy cập.
        </p>
      </Card>
    );
  }

  const initialSales = await getRealtimeSales(eventId);

  return (
    <div className="py-md">
      <SalesDashboard event={event} initialSalesData={initialSales} />
    </div>
  );
}
