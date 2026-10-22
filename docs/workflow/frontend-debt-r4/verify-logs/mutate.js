// usage: node mutate.js <repoRoot> <mutantId>
const fs = require("fs");
const path = require("path");
const [root, id] = process.argv.slice(2);
const AC = "apps/web/src/lib/api-client.ts";
const SS = "apps/web/src/lib/session.ts";
const ED = "apps/web/src/app/(organizer)/organizer/events/[eventId]/edit/edit-event-client.tsx";
const M = {
  1: [
    [AC, "  if (cur && sentUserId !== null && cur.user.id !== sentUserId) throw userChanged();\n", ""],
    [AC, "  if (after.user.id !== cur.user.id) throw userChanged();", ""],
    [AC, "    if (session && session.user.id !== startUserId) throw userChanged();\n", ""],
  ],
  2: [[AC, "  if (existing) return joinInflight(existing);", "  if (existing) return existing;"]],
  3: [[AC, "const abortTimer = setTimeout(() => controller.abort(), REFRESH_POST_TIMEOUT_MS);", "const abortTimer: ReturnType<typeof setTimeout> | undefined = undefined;"]],
  4: [[AC, "      return doRefresh(usedRefresh, sentUserId);\n    }\n    throw networkError();\n  };", "      return doRefresh(usedRefresh, sentUserId);\n    }\n    throw err;\n  };"]],
  5: [[SS,
    "const prev = window.sessionStorage.getItem(K_LAST_UID) ?? readFrom(store(\"local\"))?.user.id ?? readFrom(store(\"session\"))?.user.id;",
    "const prev = readFrom(store(\"local\"))?.user.id ?? readFrom(store(\"session\"))?.user.id ?? window.sessionStorage.getItem(K_LAST_UID);"]],
  6: [
    [AC, "function joinInflight(p: Promise<string>): Promise<string> {\n  return new Promise<string>((resolve, reject) => {\n    const timer = setTimeout(() => reject(networkError()), HARD_DEADLINE_MS);",
         "function joinInflight(p: Promise<string>, onTimeout?: () => void): Promise<string> {\n  return new Promise<string>((resolve, reject) => {\n    const timer = setTimeout(() => { onTimeout?.(); reject(networkError()); }, HARD_DEADLINE_MS);"],
    [AC, "  if (existing) return joinInflight(existing);", "  if (existing) return joinInflight(existing, () => { if (inflight.get(key) === existing) inflight.delete(key); });"],
  ],
  7: [[AC, "      if (isSamePostedSession(epoch, cur)) clearSession();\n      throw new ApiError(\"AUTH_REQUIRED\", 401, {}, \"Phien dang nhap da het han\");",
           "      clearSession();\n      throw new ApiError(\"AUTH_REQUIRED\", 401, {}, \"Phien dang nhap da het han\");"]],
  8: [[ED, "(s) => !referenced.has(s.id) && s.price === tier.price && normName(s.name) === normName(tier.name)",
           "(s) => normName(s.name) === normName(tier.name)"]],
  9: [[AC, "  if (after.user.id !== cur.user.id) throw userChanged();", ""]],
};
for (const [rel, from, to] of M[id]) {
  const f = path.join(root, rel);
  const raw = fs.readFileSync(f, "utf8");
  const crlf = raw.includes("\r\n");
  const src = raw.replace(/\r\n/g, "\n");
  const n = src.split(from).length - 1;
  if (n !== 1) { console.error(`mutant ${id}: expected 1 match in ${rel}, got ${n}`); process.exit(2); }
  const out = src.replace(from, to);
  fs.writeFileSync(f, crlf ? out.replace(/\n/g, "\r\n") : out);
}
console.log(`mutant ${id} applied`);
