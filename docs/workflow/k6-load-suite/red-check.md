# Red check (step 2a) — k6-load-suite

- Base: `3aae74d` (spec approved). Tests written before any implementation.
- Command: `node --test tests/load/lib/*.test.mjs` (Node v22.21.0, built-in runner, no npm dependency).
  Note: on Node 22 `node --test tests/load/lib` (a bare directory) is resolved as a module path and fails with
  MODULE_NOT_FOUND for the directory itself, so the Makefile and gate pass the glob instead (same file set).
- Result: exit 1, 5 test files, 5 failed, 0 passed — every file fails with `ERR_MODULE_NOT_FOUND` for the module it
  tests (`profiles.js`, `jwt.js`, `signals.js`, `flow.js`, `summary.js`): the helpers do not exist yet. This is the
  intended red reason (missing implementation), not a syntax error in the tests.
- Test cases waiting behind those imports: 52 (`test(...)` blocks).

| File | AC | Red reason |
|---|---|---|
| tests/load/lib/profiles.test.mjs | AC-2, AC-3, AC-8 shape, AC-9 shape, AC-1 thresholds | profiles.js missing |
| tests/load/lib/jwt.test.mjs | AC-4 (claims, fixture from Go authx), AC-9 (token lifetime) | jwt.js missing |
| tests/load/lib/flow.test.mjs | AC-4 (identity range, remote guard), AC-5, AC-6 | flow.js, rng.js missing |
| tests/load/lib/signals.test.mjs | AC-7 (contract ranges, uniqueness, size, modes) | signals.js missing |
| tests/load/lib/summary.test.mjs | AC-10, AC-11 | summary.js missing |

Fixture `tests/load/lib/fixtures/access-token.json`: token produced by `authx.SignAccessToken` (and checked with
`authx.VerifyAccessToken`) through a temporary, uncommitted test file in `libs/go/authx`; the generator code and command
are inside the fixture. The secret is the compose DEV default.

Signals contract source: `git show origin/feature/antibot-phase1:docs/workflow/antibot-phase1/contracts.md` §1 (fetched
2026-11-22). The range check lives in `signals.test.mjs` (`contractViolations`); the builder is one module
(`lib/signals.js`) so an amended contract changes one file plus this check.

Raw output (trimmed to the summary):

```
# Error [ERR_MODULE_NOT_FOUND]: Cannot find module 'D:\\Documents\\Nam4-1\\Project1\\EvenFlow-k6\\tests\\load\\lib\\flow.js'
not ok 1 - tests\\load\\lib\\flow.test.mjs
# Error [ERR_MODULE_NOT_FOUND]: Cannot find module 'D:\\Documents\\Nam4-1\\Project1\\EvenFlow-k6\\tests\\load\\lib\\jwt.js'
not ok 2 - tests\\load\\lib\\jwt.test.mjs
# Error [ERR_MODULE_NOT_FOUND]: Cannot find module 'D:\\Documents\\Nam4-1\\Project1\\EvenFlow-k6\\tests\\load\\lib\\profiles.js'
not ok 3 - tests\\load\\lib\\profiles.test.mjs
# Error [ERR_MODULE_NOT_FOUND]: Cannot find module 'D:\\Documents\\Nam4-1\\Project1\\EvenFlow-k6\\tests\\load\\lib\\signals.js'
not ok 4 - tests\\load\\lib\\signals.test.mjs
# Error [ERR_MODULE_NOT_FOUND]: Cannot find module 'D:\\Documents\\Nam4-1\\Project1\\EvenFlow-k6\\tests\\load\\lib\\summary.js'
not ok 5 - tests\\load\\lib\\summary.test.mjs
# tests 5
# suites 0
# pass 0
# fail 5
```
