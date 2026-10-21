# Human sign-off: frontend-real-api (High-risk)

- Approver: TranDinhVo (repository owner of this branch), in the Claude Code session on 2026-10-21.
- Approved commit (gated code): `06fd7c446a9af78de5a33d4c573e150b50d55890`; docs-only commits on top up to the PR head.
- Decision input: `final-decision.sh` → READY; `evidence.md` → PASS (29/29 AC, mutation 7/7, rollback restores develop tree, E2E 38/38, Vitest 2342/2342).
- Earlier human decisions in this task: spec approved; both backend additions (gateway CORS, `GET /api/v1/events/public`); tokens stay in web storage; `qrcode@1.5.4` dependency; G-9 test change request (`test-change-request-fix2.md`).
- Accepted debt (minor, deferred, see `findings.json`): R4-4 (priority), R4-1, R4-2, R4-3, W-4, W-5, plus earlier deferred items (S-2, S-4, S-5, S-6, S-8, C-6, X-14, X-15, R3-5, F-16/F-17 gaps).
- Merge method: merge commit into `develop` (never squash), done by the human on GitHub.
