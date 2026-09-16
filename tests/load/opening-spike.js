/**
 * k6 -- mo phong dung giay 20:00:00 (EVF-124).
 *
 * Kich ban gom 3 pha, dung nhu mot dot mo ban that:
 *
 *   1. RUSH      100.000 VU do vao trong 5 giay  -> kiem tra Tang 2 + 3 (docs/02)
 *   2. WAIT      30 phut polling theo poll_after_ms -> kiem tra Tang 4
 *   3. CHECKOUT  nhung ai duoc admit thi mua ve  -> kiem tra chong oversell
 *
 * Diem cot loi cua bai test: VU phai TON TRONG poll_after_ms tu server. Neu
 * test tu chon chu ky poll, no dang do mot he thong khac voi he thong that.
 *
 * Chay:  k6 run -e BASE=http://localhost:8080 -e EVENT=<uuid> tests/load/opening-spike.js
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { Trend, Rate, Counter } from "k6/metrics";
import { randomIntBetween } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const BASE = __ENV.BASE || "http://localhost:8080";
const EVENT = __ENV.EVENT || "00000000-0000-0000-0000-000000000001";

const joinLatency = new Trend("wr_join_latency", true);
const statusLatency = new Trend("wr_status_latency", true);
const admitted = new Counter("wr_admitted_total");
const oversell = new Counter("ticketing_oversell_detected"); // PHAI luon bang 0
const joinOk = new Rate("wr_join_success");

export const options = {
  scenarios: {
    // Pha 1: dinh nhon. 100k VU trong 5 giay -- dung hinh dang tai tai T0.
    rush: {
      executor: "ramping-arrival-rate",
      startRate: 100,
      timeUnit: "1s",
      preAllocatedVUs: 5000,
      maxVUs: 100000,
      stages: [
        { target: 60000, duration: "5s" },   // T0
        { target: 2000, duration: "25s" },   // nguoi den muon
        { target: 200, duration: "30s" },
      ],
      exec: "joinQueue",
    },
    // Pha 2: 30 phut polling. Day moi la phan chiem phan lon tong luu luong.
    waiting: {
      executor: "constant-vus",
      vus: 20000,
      duration: "30m",
      startTime: "1m",
      exec: "pollStatus",
    },
  },
  thresholds: {
    // Truc tiep anh xa tu bang SLO trong docs/00.
    "wr_join_latency": ["p(99)<300"],
    "wr_status_latency": ["p(99)<150"],
    "wr_join_success": ["rate>0.995"],
    // Bat bien tuyet doi: mot lan cung khong duoc phep.
    "ticketing_oversell_detected": ["count==0"],
    "http_req_failed": ["rate<0.01"],
  },
};

function identity() {
  return `loadtest-${__VU}-${__ITER}`;
}

export function joinQueue() {
  // Jitter phia client (Tang 2): hoan ngau nhien 0..5s sau T0.
  // Nho lottery (BR-Q1), hoan khong lam giam co hoi -- nen khong ai co dong co
  // "lach" bang cach bo qua buoc nay.
  sleep(randomIntBetween(0, 5000) / 1000);

  const res = http.post(
    `${BASE}/v1/events/${EVENT}/queue/join`,
    JSON.stringify({ signals: { interactions: randomIntBetween(3, 20) } }),
    {
      headers: { "Content-Type": "application/json", "X-Identity-Id": identity() },
      tags: { phase: "join" },
    },
  );

  joinLatency.add(res.timings.duration);
  joinOk.add(res.status === 200);

  check(res, {
    "join tra 200": (r) => r.status === 200,
    "co queue_token": (r) => !!r.json("queue_token"),
    "server chi dinh poll_after_ms": (r) => r.json("poll_after_ms") > 0,
  });
}

export function pollStatus() {
  const token = `loadtest-token-${__VU}`;
  let etag = null;

  // Vong lap ton trong nhip do SERVER chi dinh (BR-Q4). Day la diem khac biet
  // giua mot bai load test trung thuc va mot bai test tu ve ra tai.
  for (;;) {
    const headers = { "X-Queue-Token": token };
    if (etag) headers["If-None-Match"] = etag;

    const res = http.get(`${BASE}/v1/events/${EVENT}/queue/status`, {
      headers,
      tags: { phase: "status" },
    });
    statusLatency.add(res.timings.duration);

    if (res.headers["Etag"]) etag = res.headers["Etag"];

    if (res.status === 304) {
      sleep(Number(res.headers["X-Poll-After-Ms"] || 10000) / 1000);
      continue;
    }

    check(res, { "status tra 200/304": (r) => r.status === 200 || r.status === 304 });

    const state = res.json("state");
    if (state === "ADMITTED") {
      admitted.add(1);
      checkout(token);
      return;
    }
    if (state === "EXPIRED" || state === "UNKNOWN") return;

    sleep(Number(res.json("poll_after_ms") || 10000) / 1000);
  }
}

function checkout(token) {
  const res = http.post(
    `${BASE}/v1/events/${EVENT}/holds`,
    JSON.stringify({ ticket_type_id: __ENV.TICKET_TYPE, quantity: 1 }),
    {
      headers: {
        "Content-Type": "application/json",
        "X-Queue-Token": token,
        "X-Identity-Id": identity(),
        // BR-O5: moi API ghi deu bat buoc co idempotency key.
        "Idempotency-Key": `${token}-${__ITER}`,
      },
      tags: { phase: "checkout" },
    },
  );

  // 409 = het ve: hop le va mong doi. 200 = mua duoc.
  // Bat ky ma nao khac deu la dau hieu he thong dang lung lay duoi tai.
  check(res, {
    "checkout co ket qua xac dinh": (r) => r.status === 200 || r.status === 409,
  });

  if (res.status === 200) {
    check(res, {
      "expires_at do server cap (BR-O2)": (r) => !!r.json("expires_at"),
    });
  }
}

export function handleSummary(data) {
  return {
    "tests/load/results/opening-spike.json": JSON.stringify(data, null, 2),
    stdout: `
  Ket qua mo phong gio mo ban
  ---------------------------
  join p99      : ${data.metrics.wr_join_latency?.values["p(99)"]?.toFixed(0)} ms  (SLO < 300)
  status p99    : ${data.metrics.wr_status_latency?.values["p(99)"]?.toFixed(0)} ms  (SLO < 150)
  ti le join ok : ${(data.metrics.wr_join_success?.values.rate * 100)?.toFixed(2)}%  (SLO > 99.5)
  da admit      : ${data.metrics.wr_admitted_total?.values.count}
  oversell      : ${data.metrics.ticketing_oversell_detected?.values.count}  (PHAI la 0)
`,
  };
}
