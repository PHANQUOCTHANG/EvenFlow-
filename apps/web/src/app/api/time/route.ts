/** `GET /api/time` — Dong bo gio server cho dong ho dem nguoc (EVF-111, BR-O2, BR-Q1).
 *
 * Tra ve thoi gian server hien tai voi header cache edge 1 giay (s-maxage=1).
 * Giup client lay gio server de tinh offset dong ho may khach ma khong gay bao request len backend. */

export const dynamic = "force-dynamic";

export interface ServerTimeResponse {
  server_time: number;
  rfc3339: string;
}

export function GET(): Response {
  const now = new Date();
  const body: ServerTimeResponse = {
    server_time: now.getTime(),
    rfc3339: now.toISOString(),
  };

  return new Response(JSON.stringify(body), {
    status: 200,
    headers: {
      "content-type": "application/json",
      "cache-control": "public, s-maxage=1, stale-while-revalidate=5",
    },
  });
}
