// usage: node mutate.js <libDir> <n>
const fs = require("fs");
const path = require("path");
const [, , dir, n] = process.argv;
const M = {
  1: ["api-client.ts", "  if ((await readProblem(res)).type !== TOKEN_EXPIRED) return res;\n", "  /* MUTANT 1: refresh on any 401 type */\n"],
  2: ["api-client.ts", "  return fetch(url, withBearer(base, fresh));\n}", "  /* MUTANT 2: no single-retry guard (recurse) */\n  void fresh;\n  return apiFetch(path, init);\n}"],
  3: ["api-client.ts", "  if (!inflight) {\n    inflight = withRefreshLock", "  if (true /* MUTANT 3: no single-flight reuse */) {\n    inflight = withRefreshLock"],
  4: ["api-client.ts", "      if (epoch === getSessionEpoch() && getSession()?.refreshToken === cur.refreshToken) clearSession();\n", "      /* MUTANT 4: skip clearSession on refresh failure */\n"],
  5: ["payment-client.ts", "  if (response.ok) {\n    const data = (await response.json().catch(() => null)) as Record<string, unknown> | null;\n    if (!data || typeof data.payment_id !== \"string\"",
      "  if (!response.ok) {\n    /* MUTANT 5: fake payment success on error */\n    return { paymentId: `mock_pay_${Date.now()}`, orderId, redirectUrl: effectiveReturnUrl, status: \"PENDING\", providerTxnId: `txn_${Date.now()}` };\n  }\n  if (response.ok) {\n    const data = (await response.json().catch(() => null)) as Record<string, unknown> | null;\n    if (!data || typeof data.payment_id !== \"string\""],
  6: ["payment-client.ts", "  if (res.status === 404) {\n    throw new PaymentClientError(\"Không tìm thấy đơn hàng\", \"NOT_FOUND\", 404);",
      "  if (res.status === 404) {\n    /* MUTANT 6: serve cached order on 404 */\n    const cached = getRecentOrder(orderId);\n    if (cached) return cached;\n    throw new PaymentClientError(\"Không tìm thấy đơn hàng\", \"NOT_FOUND\", 404);"],
  7: ["hold-client.ts", "\"PER_IDENTITY_LIMIT\", 422);", "\"QUANTITY_NOT_ALLOWED\", 422); /* MUTANT 7 */"],
};
const [file, from, to] = M[n];
const p = path.join(dir, file);
const src = fs.readFileSync(p, "utf8").replace(/\r\n/g, "\n");
const crlf = fs.readFileSync(p, "utf8").includes("\r\n");
const count = src.split(from).length - 1;
if (count !== 1) { console.error(`mutation ${n}: anchor found ${count} times in ${file}`); process.exit(2); }
let out = src.replace(from, to);
if (crlf) out = out.replace(/\n/g, "\r\n");
fs.writeFileSync(p, out);
console.log(`mutation ${n} applied to ${file}`);
