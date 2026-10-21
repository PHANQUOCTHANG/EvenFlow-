// Stub API cho E2E (frontend-real-api WP6, plan 0 B1): phuc vu cac fetch cua Next SERVER
// (SSR/ISR) vi page.route chi thay request cua trinh duyet. Chi dung node:http, khong phu thuoc.
// Cung doc events.json voi fixtures/api-mock.ts de du lieu khop nhau.
import { readFileSync } from "node:fs";
import { createServer } from "node:http";
import { fileURLToPath } from "node:url";

const data = JSON.parse(readFileSync(fileURLToPath(new URL("./events.json", import.meta.url)), "utf8"));
const PORT = Number(process.env.STUB_API_PORT ?? 8080);
const PUBLIC_STATUSES = new Set(["SCHEDULED", "ON_SALE", "SOLD_OUT"]);

function send(res, status, body, contentType = "application/json") {
  res.writeHead(status, { "Content-Type": contentType, "Cache-Control": "no-store" });
  res.end(JSON.stringify(body));
}

function notFound(res, path) {
  send(
    res,
    404,
    { type: "/problems/not-found", title: "Not Found", status: 404, detail: `stub-api: khong co ${path}` },
    "application/problem+json",
  );
}

const server = createServer((req, res) => {
  const path = new URL(req.url ?? "/", "http://stub").pathname;
  if (req.method !== "GET") return notFound(res, path);

  if (path === "/api/v1/events/public") {
    return send(res, 200, { events: data.events.filter((e) => PUBLIC_STATUSES.has(e.status)) });
  }
  let m = path.match(/^\/api\/v1\/events\/slug\/([^/]+)$/);
  if (m) {
    const slug = decodeURIComponent(m[1]);
    const ev = data.events.find((e) => e.slug === slug);
    return ev ? send(res, 200, ev) : notFound(res, path);
  }
  m = path.match(/^\/api\/v1\/events\/([^/]+)$/);
  if (m) {
    const ev = data.events.find((e) => e.id === decodeURIComponent(m[1]));
    return ev ? send(res, 200, ev) : notFound(res, path);
  }
  m = path.match(/^\/v1\/events\/([^/]+)\/availability$/);
  if (m) {
    const av = data.availability[decodeURIComponent(m[1])];
    return av ? send(res, 200, av) : notFound(res, path);
  }
  return notFound(res, path);
});

// Next goi "localhost" co the phan giai ra 127.0.0.1 hoac ::1: nghe ca hai.
// Node tren Windows/Linux khong gop dual-stack khi chi dinh host, nen mo hai server rieng.
for (const host of ["127.0.0.1", "::1"]) {
  const s = host === "127.0.0.1" ? server : createServer(server.listeners("request")[0]);
  s.on("error", (err) => {
    // IPv6 co the khong co tren runner: bo qua; IPv4 loi thi thoat de Playwright bao ro.
    if (host === "::1") return;
    console.error(`stub-api: khong nghe duoc ${host}:${PORT}: ${err.message}`);
    process.exit(1);
  });
  s.listen(PORT, host, () => console.log(`stub-api nghe ${host}:${PORT}`));
}
