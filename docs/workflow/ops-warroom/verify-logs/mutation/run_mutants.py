"""Mutation run for ops-warroom (v3 step 6). Run from the worktree root: python docs/workflow/ops-warroom/verify-logs/mutation/run_mutants.py [ID ...]

Each mutant replaces exactly one occurrence of `old` with `new`, runs the owning suite, records killed (non-zero
exit) or survived, then reverts with `git checkout -- services apps`.
"""
import json, os, subprocess, sys, pathlib

ROOT = pathlib.Path(__file__).resolve().parents[5]
OUT = pathlib.Path(__file__).resolve().parent
WR = "services/waitingroom/internal/app/ops_override.go"
WH = "services/waitingroom/internal/adapter/http/ops_handler.go"
TH = "services/ticketing/internal/adapter/http/ops_handler.go"
TA = "services/ticketing/internal/app/ops_queries.go"
TR = "services/ticketing/internal/adapter/postgres/ops_repository.go"
GR = "services/gateway/internal/proxy/routes.go"
PY = "services/ai-worker/app/main.py"
WS = "apps/web/src/lib/ops-service.ts"
HS = "apps/web/src/hooks/use-organizer-sales.ts"

SUITES = {
    "wr": ("services/waitingroom", ["go", "test", "./internal/...", "-count=1"]),
    "tk": ("services/ticketing", ["go", "test", "./internal/...", "./cmd/...", "-count=1"]),
    "gw": ("services/gateway", ["go", "test", "./...", "-count=1"]),
    "py": ("services/ai-worker", ["uv", "run", "pytest", "-q", "-x"]),
    "web": ("apps/web", ["npx", "vitest", "run", "src/lib", "src/hooks", "src/components/ops",
                         "src/components/organizer", "src/app/(ops)", "src/app/(organizer)"]),
}

M = [
    # id, file, old, new, suite, description
    ("M01", WR, "rate <= MaxOverrideRate &&", "rate <= MaxOverrideRate+1 &&", "wr", "ValidRate upper bound 10000 -> 10001"),
    ("M02", WR, "rate >= 0 &&", "rate >= -1 &&", "wr", "ValidRate lower bound 0 -> -1"),
    ("M03", WR, "&& rate == math.Trunc(rate)", "", "wr", "ValidRate accepts fractional rates"),
    ("M04", WH, "if c < '0' || c > '9' {", "if c < '0' && c > '9' {", "wr", "parseRate literal digit check disabled (-0 accepted)"),
    ("M05", WR, "if visible < minReasonLen ||", "if visible < minReasonLen-1 ||", "wr", "reason visible-char minimum 3 -> 2"),
    ("M06", WR, "return !unicode.Is(unicode.Cf, c) && !unicode.In", "return !unicode.In", "wr", "Cf chars counted as visible"),
    ("M07", WR, "return r, ErrReasonBidi", "continue", "wr", "bidi controls no longer rejected"),
    ("M08", WR, "utf8.RuneCountInString(r) > maxReasonLen", "utf8.RuneCountInString(r) > maxReasonLen+1", "wr", "reason max 200 -> 201"),
    ("M09", WR, "if isReasonControl(c) {", "if false && isReasonControl(c) {", "wr", "control chars no longer rejected"),
    ("M10", WR, '"rate_per_sec": rate,', '"rate_per_sec": stored,', "wr", "audit records read-back rate instead of commanded rate"),
    ("M11", WR, "action = ActionPaused", "action = ActionOverrideSet", "wr", "rate 0 audited as SET instead of PAUSED"),
    ("M12", WR, 'return OverrideResult{}, fmt.Errorf("ap dung ghi de: %w", err)',
     'o.audit(detached, cmd, ActionOverrideSet, nil)\n\t\treturn OverrideResult{}, fmt.Errorf("ap dung ghi de: %w", err)', "wr",
     "Set audits even when the Redis apply failed (audit before successful apply)"),
    ("M13", WR, """	if err := o.withTimeout(detached, overrideApplyTimeout, func(c context.Context) error {
		return o.store.ClearRateOverride(c, cmd.EventID)
	}); err != nil {""", """	early := o.audit(detached, cmd, ActionOverrideCleared, map[string]any{"rate_per_sec": nil, "previous": prev, "request_id": cmd.RequestID})
	_ = early
	if err := o.withTimeout(detached, overrideApplyTimeout, func(c context.Context) error {
		return o.store.ClearRateOverride(c, cmd.EventID)
	}); err != nil {""", "wr", "Clear audits before apply (order swapped)"),
    ("M14", WR, "if p := recover(); p != nil {", "if p := recover(); false && p != nil {", "wr", "auditor panic reported as recorded"),
    ("M15", WR, "if err == nil {\n\t\treturn AuditRecorded", "if err == nil || err != nil {\n\t\treturn AuditRecorded", "wr", "audit DB error reported as recorded"),
    ("M16", WR, "if o.auditor == nil {\n\t\treturn previousOverride{}, ErrAuditUnavailable", "if false {\n\t\treturn previousOverride{}, ErrAuditUnavailable", "wr", "no audit store no longer fails closed (503)"),
    ("M17", WR, "detached := context.WithoutCancel(ctx)\n\tif err := o.withTimeout(detached, overrideApplyTimeout, func(c context.Context) error {\n\t\treturn o.store.SetRateOverride",
     "detached := ctx\n\tif err := o.withTimeout(detached, overrideApplyTimeout, func(c context.Context) error {\n\t\treturn o.store.SetRateOverride", "wr", "Set apply/audit bound to request context"),
    ("M18", WH, 'if p.Role != "ops" {', 'if p.Role == "nobody" {', "wr", "waitingroom handler role check removed"),
    ("M19", WH, "|| seen[key] {", "{", "wr", "decodeStrict accepts duplicate keys"),
    ("M20", WH, "err != nil || !exactKeys(raw, jsonFieldNames(dst))", "err != nil", "wr", "decodeStrict exact-key check removed (case mismatch + dup)"),
    ("M21", WH, "if _, err := dec.Token(); !errors.Is(err, io.EOF) {", "if false {", "wr", "trailing data after JSON accepted"),
    ("M22", WH, "if len(raw) == 0 || len(raw) > 6 {\n\t\treturn 0, false\n\t}\n\tfor _, c := range raw {",
     "if len(raw) == 0 || len(raw) > 6 {\n\t\treturn 0, false\n\t}\n\tif raw[0] == '\"' { raw = bytes.Trim(raw, `\"`) }\n\tfor _, c := range raw {", "wr", "parseRate accepts quoted string \"12\""),
    ("T01", TH, "(err == nil && !strings.EqualFold(s.OrganizerID, p.ID))", "(err == nil && false)", "tk", "sales owner check removed"),
    ("T02", TH, "!strings.EqualFold(s.OrganizerID, p.ID)", "s.OrganizerID != p.ID", "tk", "owner compare case-sensitive"),
    ("T03", TH, "opsCacheTTL = 2 * time.Second", "opsCacheTTL = 0", "tk", "2s cache disabled"),
    ("T04", TH, "e.val, e.notFound, e.at, e.fill = zero, true, c.now(), true", "e.val, e.notFound, e.at, e.fill = zero, true, c.now(), false", "tk", "not-found no longer cached"),
    ("T05", TH, 'opsPrincipal(w, r, "ops")', 'opsPrincipal(w, r, "organizer")', "tk", "inventory role ops -> organizer"),
    ("T06", TH, "if e.fill && c.now().Sub(e.at) < c.ttl {", "if e.fill && c.now().Sub(e.at) < 10*c.ttl {", "tk", "cache TTL 2s -> 20s (staleness bound)"),
    ("T07", TA, "out.Totals.TicketsSold += t.Sold", "out.Totals.TicketsSold += t.Quota", "tk", "totals not derived from same-snapshot tier rows"),
    ("T08", TA, "rest > 0 || restAmount > 0", "rest > 0 && restAmount > 0", "tk", "unknown-provider reconciliation condition"),
    ("T09", TR, "pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}", "pgx.TxOptions{}", "tk", "sales snapshot tx REPEATABLE READ removed (integration-only)"),
    ("G01", GR, '{Pattern: "PUT /v1/ops/events/{eventID}/admit-override", Upstream: config.Waitingroom, Access: Authenticated, Roles: []string{"ops"}',
     '{Pattern: "PUT /v1/ops/events/{eventID}/admit-override", Upstream: config.Waitingroom, Access: Authenticated, Roles: []string{"ops", "organizer"}', "gw", "PUT override also allowed for organizer"),
    ("G02", GR, 'Roles: []string{"organizer"}, Rate: config.RateDefault},', 'Roles: []string{"organizer", "buyer"}, Rate: config.RateDefault},', "gw", "sales route also allowed for buyer"),
    ("G03", GR, '{Pattern: "GET /v1/ops/ai/mode", Upstream: config.Assistant', '{Pattern: "GET /v1/ops/ai/mode", Upstream: config.Ticketing', "gw", "ai mode routed to wrong upstream"),
    ("G04", GR, '{Pattern: "GET /v1/ops/events/{eventID}/inventory"', '{Pattern: "/v1/ops/events/{eventID}/inventory"', "gw", "inventory route forwards any method"),
    ("P01", PY, 'request.headers.get("x-identity-role", "") != "ops"', 'request.headers.get("x-identity-role", "") not in ("ops", "organizer")', "py", "ai mode allows organizer"),
    ("P02", PY, 'if not _UUID_RE.fullmatch(request.headers.get("x-identity-id", "")):', 'if False:', "py", "ai mode identity (401) check removed"),
    ("P03", PY, 'view["generated_at"] = datetime.now(UTC).isoformat()', 'view["generated_at"] = datetime.now(UTC).isoformat()\n    view["policy"] = {}', "py", "ops ai mode leaks policy field"),
    ("W01", WS, "if (queueErr?.auth) throw new OpsServiceError(401);", "if (failures.some((f) => f?.auth)) throw new OpsServiceError(401);", "web", "401 from any source (not only core) -> sign-in"),
    ("W02", WS, "if (eventErr?.status === 404 || eventErr?.status === 400)", "if (eventErr?.status === 400)", "web", "event 404 not mapped to not-found"),
    ("W03", WS, "if (failures.some((f) => f?.status === 403)) throw new OpsServiceError(403);", "", "web", "source 403 not surfaced as forbidden"),
    ("W04", WS, "if (queueErr && invErr && aiErr) throw new OpsServiceError(503);", "", "web", "all three sources down not 503"),
    ("W05", WS, "rate >= 0 && rate <= MAX_OVERRIDE_RATE", "rate >= 0 && rate <= MAX_OVERRIDE_RATE + 1", "web", "client rate max 10000 -> 10001"),
    ("W06", WS, "return Number.isInteger(rate) &&", "return Number.isFinite(rate) &&", "web", "client accepts fractional rate"),
    ("W07", WS, 'if (isBidiControl(c)) return "BIDI";', "", "web", "client bidi reject removed"),
    ("W08", WS, "const visible = kept.filter(isVisible).length;", "const visible = kept.length;", "web", "client visible count includes invisible chars"),
    ("W09", WS, 'if (res.status === 401) throw new ApiError("AUTH_REQUIRED", 401', 'if (res.status === 4010) throw new ApiError("AUTH_REQUIRED", 401', "web", "write 401 not treated as session lost"),
    ("W10", WS, "    isSample: false,", "    isSample: true,", "web", "war room snapshot labelled sample"),
    ("W11", HS, "initialAutoRefresh = true,", "initialAutoRefresh = false,", "web", "sales auto-refresh off by default"),
    ("W12", HS, "Math.min(pollingIntervalMs, 5000)", "Math.min(pollingIntervalMs, 50000)", "web", "sales polling cap 5s removed"),
    # W11/W12 are equivalent for the only caller (dashboard passes initialAutoRefresh: true, default interval 4000 < cap):
    # stronger mutants on the real call path.
    ("W13", "apps/web/src/components/organizer/sales-dashboard.tsx", "initialAutoRefresh: true,", "initialAutoRefresh: false,", "web", "dashboard auto-refresh off by default"),
    ("W14", HS, "DEFAULT_SALES_POLLING_INTERVAL_MS = 4000;", "DEFAULT_SALES_POLLING_INTERVAL_MS = 4900;", "web", "default polling 4s -> 4.9s"),
]


def run(cmd, cwd):
    env = dict(os.environ)
    env.pop("VITEST_MAX_THREADS", None)
    env.pop("VITEST_MAX_FORKS", None)
    shell = os.name == "nt" and cmd[0] in ("npx", "uv")
    p = subprocess.run(" ".join(f'"{c}"' if "(" in c else c for c in cmd) if shell else cmd, cwd=cwd, env=env,
                       capture_output=True, text=True, encoding="utf-8", errors="replace", shell=shell)
    return p.returncode, (p.stdout + p.stderr)


def main():
    only = set(sys.argv[1:])
    res_path = OUT / "results.json"
    results = json.loads(res_path.read_text()) if res_path.exists() else {}
    for mid, f, old, new, suite, desc in M:
        if only and mid not in only:
            continue
        path = ROOT / f
        src = path.read_text(encoding="utf-8")
        n = src.count(old)
        if n != 1:
            results[mid] = {"desc": desc, "file": f, "status": f"NOT APPLIED (matches={n})"}
            print(mid, "NOT APPLIED", n, flush=True)
            continue
        path.write_text(src.replace(old, new), encoding="utf-8", newline="")
        cwd, cmd = SUITES[suite]
        code, out = run(cmd, ROOT / cwd)
        subprocess.run(["git", "checkout", "--", "services", "apps"], cwd=ROOT, check=True)
        status = "KILLED" if code != 0 else "SURVIVED"
        fails = [l for l in out.splitlines() if l.startswith(("--- FAIL", "FAIL", "FAILED")) or " FAIL " in l or "✗" in l or "×" in l]
        (OUT / f"{mid}.log").write_text("\n".join(fails[:40]) + "\n---- tail ----\n" + "\n".join(out.splitlines()[-15:]), encoding="utf-8")
        results[mid] = {"desc": desc, "file": f, "suite": suite, "status": status, "exit": code}
        print(mid, status, flush=True)
        res_path.write_text(json.dumps(results, indent=1, ensure_ascii=False), encoding="utf-8")
    res_path.write_text(json.dumps(results, indent=1, ensure_ascii=False), encoding="utf-8")


if __name__ == "__main__":
    main()
