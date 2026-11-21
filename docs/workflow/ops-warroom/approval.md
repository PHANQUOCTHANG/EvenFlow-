# Human sign-off: ops-warroom (High-risk)

- Approver: TranDinhVo, in the Claude Code session on 2026-11-21.
- Approved code: gate PASS at `cfc630e`; later commits add tests (`7a0f27f`), docs, and a merge of `origin/develop` (Next.js 15.5.28 patch only).
- Decision input: `evidence.md` re-verification PASS (47/48 behaviour-changing mutants killed; T09 needs real Postgres), rollback PASS, three review rounds with no blocker or major.
- Earlier human decisions: spec and contracts approved; CCR-1; TCR-1..TCR-4.
- Accepted at sign-off:
  - R-8: `sales/page.test.tsx` changed beyond its TCR row.
  - R3-1: the web form rejects leading/trailing tab or newline in the reason while the server trims them (web is stricter; no bypass).
  - R2-6: tamper threshold for approved frozen tests raised from 4 to 5 lines for TCR-4.
  - X2-3: a 401 from the public event-detail call is shown as 503 and keeps the war room on screen.
  - R2-5 / T09: Postgres/Redis integration suites not run locally (Docker off). Condition: merge only after the PR's `integration` CI job is green.
- Deferred minors: see `deferred.md`.
- Merge: merge commit into `develop` once all PR CI checks (including `integration`) are green; `main` untouched.
