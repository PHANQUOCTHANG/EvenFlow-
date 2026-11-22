# Human sign-off: antibot-phase1 (EV-173, EV-174, EV-175 — High-risk)

- Approver: TranDinhVo, in the Claude Code session on 2026-11-22.
- Approved code: gate PASS at `9291299` (last code commit `43973ec`); later commits are docs only plus a merge of `origin/develop`.
- Decision input: `evidence.md` re-verification PASS (72/72 mutant runs killed); corpus 95.80% bots caught / 0.10% humans demoted; review, challenge and two re-review rounds with no blocker or major left.
- Earlier human decisions: spec + Q-1..Q-5; Amendment 1 (+ join button withdrawn); Amendment 2 + revision (R5 strong 0.85, HTTP-library UAs → R5W, R4D 0.60, X-3 accepted limitation); TCR-1..TCR-6; K8s TCR N-5; R-6 policy exception.
- Ships in `ANTIBOT_MODE=shadow` (records decisions, never blocks).
- NOT VERIFIED here (Docker off): integration suites and promtool (condition: the PR's `integration` job must be green before merge), real-Redis p99 / load gate, AC-18 recorded sessions, AC-20 shadow on real traffic.
- Before switching to `enforce` (human go/no-go): integration + promtool green; real-Redis p99 < 5 ms under the k6 opening spike (`tests/load`, replaces `opening-spike.js`) with shadow vs develop delta within the AC-5/P-9 gate; 50–100 recorded sessions pass AC-18; `ANTIBOT_HASH_KEY` present in the Secret; waitingroom and controller on the same image; shadow demotion < 1% over a rehearsal on-sale.
- Deferred: EV-176 Turnstile, EV-177 appeal UI, EV-178 AI rules; X-3 (forged fp/sq) until Turnstile; X-9 prefix hashing + retention.
- Merge: merge commit into `develop` once all PR CI checks (including `integration`) are green; `main` untouched.
