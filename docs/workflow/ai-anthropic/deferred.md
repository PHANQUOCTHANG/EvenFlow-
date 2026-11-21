# Deferred findings: ai-anthropic

Findings from review round 1 that are not fixed on this branch, with the reason and the follow-up.

## X-7 (info): Unicode look-alike closing tags pass through `wrap_untrusted`

`wrap_untrusted` escapes only ASCII `<` and `>`, so full-width / angle-quote look-alikes such as
U+FF1C/U+FF1E, U+2039/U+203A and U+FE64/U+FE65, and a user-typed `&lt;/du_lieu&gt;`, come back
unchanged inside the block. A real `</du_lieu>` still cannot be forged, so the structural boundary
holds, and both system prompts (assistant.v3.md and, since fix R-1, moderation.v1.md) say that nothing
inside `<du_lieu>` is an instruction. Whether the model reads a full-width bracket as a tag could not
be tested without an API key.

Follow-up: NFKC-normalise untrusted text before escaping and escape `&` first, with a test for each
look-alike. This changes the bytes the model sees (and the exact-hash cache key is unaffected, since
that is built from the question, not the wrapped prompt), so it gets its own small change.

## R-7 (minor): anthropic 2.12.0 was locked one day after release

The lock is hashed and the dependency edges were checked as genuine (anthropic -> httpx2/httpcore2 from
github.com/pydantic/httpx2, truststore), so this is not a typosquat, but a release this new has had
little community exposure.

Follow-up: before the next bump, add a release cooldown (`[tool.uv] exclude-newer` set about 7 days
back, or an explicit review step) so new versions are locked only after they have been out for a week.
No change is needed to merge this branch.

## R-4 (minor): alerts gate NOT_RUN

`promtool test rules deploy/observability/alerts_test.yml` needs Docker, which this environment does not
have. It must run (locally or in CI) before human sign-off; the result goes in gate-report.json.
