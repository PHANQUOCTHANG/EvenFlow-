# Test change request: frontend-debt-r5

No existing test is expected to break legitimately. Checked: no existing test seeds a role outside buyer|organizer|moderator|ops
as a session (the only `role: "admin"` hit is in assistant-panel.test.tsx, unrelated to session storage); no existing test
asserts a non-http(s) redirect_url. Watch item for the implementer (not a request): existing tests re-use the same refresh
string across `it` blocks; the AC-2 record lives in sessionStorage, which `setupEnv()` clears before each test, so none should
be affected. If a failure appears only because of that reuse, report it here rather than weakening AC-2.
