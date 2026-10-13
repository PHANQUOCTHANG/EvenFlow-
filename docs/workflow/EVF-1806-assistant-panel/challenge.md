# EVF-1806 — Challenge (step 8) — `AssistantPanel`

Challenger: a separate agent that did not write the code. It read only `spec-plan.md` and `assistant-panel.tsx`
(plus `toEpochMs` in `src/lib/server-time.ts` and the `Alert` role mapping).
Method: 43 probes in a scratch file `src/components/assistant/challenge.scratch.test.tsx`, run with
`npx vitest run src/components/assistant/challenge.scratch.test.tsx` (React 18.3.1, RTL 16.1.0).
Result: **18 failed / 25 passed**. The scratch file **has been deleted**. No other file was changed.

Imports common to every repro below:

```tsx
import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AssistantPanel, type AssistantMessage, type AssistantMode } from "./assistant-panel";
afterEach(() => vi.restoreAllMocks());
```

## Summary

| # | Severity | Type | Finding |
|---|---|---|---|
| C-1 | **major** | spec violation (AC-2) | A `sentAt` number outside the Date range (`1e20`, `8.64e15+1`) throws `RangeError` → the whole panel crashes |
| C-2 | **major** | goes against the intent of BR-A1 / §2.4 / BR-A2 (AC-4/AC-5 do not cover it) | The "Gửi lại" (retry) button is not blocked by pending, SAVING, FAQ_ONLY or exhausted quota → it gets around the gate on open questions |
| C-3 | minor | hardening (affects Vietnamese users) | Enter during IME composition (`isComposing`) still sends |
| C-4 | minor | hardening | Two synchronous Enter presses (or Enter+click) in the same task send twice, because there is no ref lock |
| C-5 | minor | hardening (fail-open) | `quotaRemaining` is negative or the string `"0"` at runtime → open questions are **not** blocked |
| C-6 | minor | hardening | `quotaRemaining > quotaLimit` (11/10) still shows "Còn 11/10" |
| C-7 | minor | hardening | `supportHref="javascript:..."` is rendered as a real `href` (React 18 only warns) |
| C-8 | minor | hardening | Escape during IME composition closes the panel |
| C-9 | minor | hardening | A `mode` outside the union → the input is blocked (fail-closed, good) but there is **no** notice explaining why |
| C-10 | minor | hardening | A `kind` outside the union → the label is an empty string (the user does not know what they are reading, §2.7) |
| C-11 | minor | hardening | Text made only of zero-width spaces (`​`) is sent |

**No** violation found in: technical words (§2.5) across SAVING/FAQ_ONLY/OFF + failed + quota exhausted (including
`aria-label`/`title`), HTML injection via `text`/FAQ `label`, Infinity/NaN/-0 quota, `status` undefined ⇒ `done`,
pending on the assistant role (correctly ignored per the contract), chips allowed/blocked in the right states,
OFF hides log/group and has exactly one `role="status"`, Escape propagates when there is no `onClose`, `sentAt` `0`/
negative/ISO without timezone/garbage/empty/invalid date with `Z`, duplicate message ids, `mode` undefined (fail-closed),
`onSend` throwing (the draft is kept).

---

## C-1 — major — `sentAt` outside the Date range crashes the panel

**Spec clause:** AC-2: "`sentAt` thiếu hoặc không parse được ⇒ **không** hiện giờ và **không throw**" (if `sentAt` is missing or cannot be parsed ⇒ show no time and do not throw).

```tsx
for (const sentAt of [1e20, 8.64e15 + 1]) {
  it(`sentAt ${sentAt} does not throw`, () => {
    expect(() =>
      render(<AssistantPanel mode="NORMAL" messages={[{ id: "1", role: "user", text: "x", sentAt }]} />),
    ).not.toThrow();
  });
}
```

- **Observed:** `RangeError: Invalid time value`. `toEpochMs` only checks `Number.isFinite`, so `1e20` passes.
  `formatHm` creates an Invalid Date (it would print `NaN:NaN`), and `new Date(epoch).toISOString()` in `<time dateTime>`
  throws. Without an error boundary, one bad timestamp from the backend takes down the whole panel (and possibly the
  whole page).
- **Expected:** no time shown, no throw.
- **Suggested fix:** in `MessageItem`, treat `Number.isNaN(new Date(epoch).getTime())` as `null` (or tighten `toEpochMs`
  to `Math.abs(value) <= 8.64e15`). Add a regression test to `assistant-panel.test.tsx`.

## C-2 — major — "Gửi lại" gets around the pending / degradation / quota gates

**Spec clauses:** §2.2 / BR-A1 "một người một job tại một thời điểm" (one person, one job at a time); §2.4 SAVING/FAQ_ONLY
"câu hỏi mở: **chặn**" (open questions: **blocked**); §2.3 `quotaRemaining === 0` ⇒ "chặn gửi câu hỏi mở" (block
sending open questions). AC-4 only lists input + Gửi + chips, and AC-5 does not mention gating. So this is a **gap in
the AC**, but the behavior goes against the intent of the business rule: retrying a failed message re-sends an
**open question** (it calls Gemini) and creates a new job.

```tsx
it("retry is blocked while another message is pending", () => {
  const onRetry = vi.fn();
  const { getByRole } = render(
    <AssistantPanel mode="NORMAL" onSend={() => {}} onRetry={onRetry}
      messages={[
        { id: "u1", role: "user", text: "a", status: "failed" },
        { id: "u2", role: "user", text: "b", status: "pending" },
      ]} />,
  );
  fireEvent.click(getByRole("button", { name: "Gửi lại" }));
  expect(onRetry).not.toHaveBeenCalled();           // FAIL: called 1 time
});

for (const mode of ["SAVING", "FAQ_ONLY"] as const) {
  it(`retry blocked in ${mode}`, () => {
    const onRetry = vi.fn();
    const { getByRole } = render(
      <AssistantPanel mode={mode} onRetry={onRetry}
        messages={[{ id: "u1", role: "user", text: "a", status: "failed" }]} />,
    );
    fireEvent.click(getByRole("button", { name: "Gửi lại" }));
    expect(onRetry).not.toHaveBeenCalled();         // FAIL: called 1 time
  });
}

it("retry blocked when quota exhausted", () => {
  const onRetry = vi.fn();
  const { getByRole } = render(
    <AssistantPanel mode="NORMAL" quotaRemaining={0} quotaLimit={10} onRetry={onRetry}
      messages={[{ id: "u1", role: "user", text: "a", status: "failed" }]} />,
  );
  fireEvent.click(getByRole("button", { name: "Gửi lại" }));
  expect(onRetry).not.toHaveBeenCalled();           // FAIL: called 1 time
});
```

- **Observed:** `onRetry` is called in all 4 cases.
- **Expected (per the BR):** the retry button is `disabled` (or hidden) under the same conditions as the open-question
  input. The simplest fix is `disabled={!openQuestionsAllowed}` (with the `Boolean(onSend)` part dropped from that
  condition for retry). Ask the spec author to add this to AC-4/AC-5 so the test can lock it in.

## C-3 — minor — Enter during IME composition still sends (hardening)

Not in the spec. It matters for Vietnamese users, though: some IMEs (macOS Vietnamese, Windows IMEs, Unikey in
pre-edit mode) use composition, and pressing Enter to confirm a word would send half a sentence.

```tsx
it("IME composition Enter must not send", () => {
  const onSend = vi.fn();
  const { getByRole } = render(<AssistantPanel mode="NORMAL" messages={[]} onSend={onSend} />);
  const ta = getByRole("textbox");
  fireEvent.change(ta, { target: { value: "Mở bán" } });
  fireEvent.keyDown(ta, { key: "Enter", isComposing: true, keyCode: 229 });
  expect(onSend).not.toHaveBeenCalled();            // FAIL: called 1 time
});
```

Fix: in `handleTextareaKeyDown`, `if (event.nativeEvent.isComposing || event.keyCode === 229) return;`.

## C-4 — minor — double submit within one synchronous task (hardening)

```tsx
it("two native Enter in one task send once", async () => {
  const onSend = vi.fn();
  const { getByRole } = render(<AssistantPanel mode="NORMAL" messages={[]} onSend={onSend} />);
  const ta = getByRole("textbox");
  fireEvent.change(ta, { target: { value: "hi" } });
  ta.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }));
  ta.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }));
  await act(async () => {});
  expect(onSend).toHaveBeenCalledTimes(1);          // FAIL: 2 times
});
```

- `submit()` reads `draft` from the render closure, so if a second event arrives before React re-renders, it sends again.
  Two separate events (each `fireEvent` flushed on its own) send only once, which **passes**. A real user can rarely
  hit this, because each native input is its own task, but it is still reachable through scripts or automation. Also,
  between `onSend` and the moment the parent adds the `pending` message, the component has no local lock, so a user
  can type quickly and send a second question if the parent only adds pending after the 202 response.
- Fix: a `submittingRef` / read `textarea.value` from the ref, and lock locally until the `messages` prop changes.

## C-5 — minor — runtime quota is negative or a string → gate fails open (hardening)

The spec only says "không hiện số" (do not show the number) for invalid values. It does not say whether to block.
Today an invalid value is treated as "not exhausted":

```tsx
it("negative remaining fails open", () => {
  const { getByRole } = render(
    <AssistantPanel mode="NORMAL" messages={[]} quotaRemaining={-1} quotaLimit={10} onSend={() => {}} />,
  );
  expect(getByRole("textbox")).toBeDisabled();      // FAIL: enabled
});
it("string '0' at runtime fails open", () => {
  const { getByRole } = render(
    <AssistantPanel mode="NORMAL" messages={[]} quotaRemaining={"0" as unknown as number}
      quotaLimit={10} onSend={() => {}} />,
  );
  expect(getByRole("textbox")).toBeDisabled();      // FAIL: enabled
});
```

The spec's own reasoning in §2.4 ("default NORMAL = mở cửa gọi Gemini đúng lúc cạn ngân sách", i.e. defaulting to
NORMAL opens the door to Gemini calls right when the budget runs out) applies here too. Suggestion: `quotaRemaining <= 0`
(number) ⇒ exhausted. A string from JSON should be fixed at the API mapping layer.

## C-6 — minor — `remaining > limit` still shows the number (hardening)

```tsx
it("remaining > limit hides quota", () => {
  const { container } = render(
    <AssistantPanel mode="NORMAL" messages={[]} quotaRemaining={11} quotaLimit={10} onSend={() => {}} />,
  );
  expect(container.querySelector('[data-slot="quota"]')).toBeNull(); // FAIL: "Còn 11/10 lượt hỏi"
});
```

AC-7 only requires an integer ≥ 0, so this is **not** a violation. Still, "11/10" is contradictory data (handoff §5
principle 3: do not show numbers you cannot trust). `0/0` also shows "Còn 0/0 lượt hỏi", which is harmless.

## C-7 — minor — `supportHref` accepts `javascript:` (hardening)

```tsx
it("supportHref javascript: is not rendered", () => {
  const { container } = render(<AssistantPanel mode="OFF" messages={[]} supportHref="javascript:alert(1)" />);
  expect(container.querySelector("a")?.getAttribute("href") ?? "").not.toMatch(/^javascript:/i); // FAIL
});
```

React 18.3.1 only prints a console warning and still renders the `href`. `supportHref` is server or config owned, not
user input, so the risk is low. A cheap fix is to accept only `https:`, `http:`, `mailto:`, `tel:` or relative paths,
and otherwise treat it as missing (generic wording).

## C-8 — minor — Escape during IME composition closes the panel (hardening)

```tsx
it("Escape during composition does not close", () => {
  const onClose = vi.fn();
  const { getByRole } = render(<AssistantPanel mode="NORMAL" messages={[]} onSend={() => {}} onClose={onClose} />);
  fireEvent.keyDown(getByRole("textbox"), { key: "Escape", isComposing: true });
  expect(onClose).not.toHaveBeenCalled();           // FAIL: called 1 time
});
```

Escape during composition is how the user cancels the pre-edit text. Closing the panel at that moment loses the draft.

## C-9 — minor — unknown `mode` → blocked but no explanation (hardening)

```tsx
it("unknown mode explains", () => {
  const { getByRole, container } = render(
    <AssistantPanel mode={"DEGRADED" as AssistantMode} messages={[]} onSend={() => {}} />,
  );
  expect(getByRole("textbox")).toBeDisabled();      // PASS (fail-closed, good)
  expect(container.querySelector('[role="status"]')).not.toBeNull(); // FAIL: no notice
});
```

If the backend adds a new level (or sends lowercase `faq_only`), the input is disabled for no visible reason.
Suggestion: fall back to the `FAQ_ONLY` notice for any value that is not `NORMAL`/`SAVING`/`OFF`. `mode={undefined}` is
also fail-closed (PASS).

## C-10 — minor — unknown `kind` → empty label (hardening)

```tsx
it("unknown kind has a label", () => {
  const { container } = render(
    <AssistantPanel mode="NORMAL" messages={[{ id: "1", role: "assistant", text: "x", kind: "tool" as never }]} />,
  );
  expect(container.querySelector('[data-slot="message"] span')?.textContent?.trim()).not.toBe(""); // FAIL
});
```

`KIND_LABEL[kind ?? "ai"]` only falls back for null/undefined. Suggestion: `KIND_LABEL[kind] ?? KIND_LABEL.ai`, and
`data-kind` should also be normalized.

## C-11 — minor — text made only of zero-width spaces is sent (hardening)

```tsx
it("zero-width space only is not sent", () => {
  const onSend = vi.fn();
  const { getByRole } = render(<AssistantPanel mode="NORMAL" messages={[]} onSend={onSend} />);
  const ta = getByRole("textbox");
  fireEvent.change(ta, { target: { value: "​" } });
  fireEvent.keyDown(ta, { key: "Enter" });
  expect(onSend).not.toHaveBeenCalled();            // FAIL: called with "​"
});
```

`String.prototype.trim` does not remove U+200B, so this wastes a quota turn on an empty question. There is also no
`maxLength` on the textarea, which is out of scope for this spec.

---

## Other observations (not findings)

- Duplicate message ids: React prints a key warning, but both messages still render, including after a re-render with
  an extra message. `onRetry(id)` would be ambiguous, but that is the backend's responsibility.
- `onSend` throwing: the exception propagates out of the handler and the draft is **kept**, which is reasonable.
- `sentAt: 0` shows `07:00` (epoch 1970, ICT). The contract treats it as valid, so this is not a bug.
- `sentAt` ISO without a timezone: hidden, plus a `console.warn` from `toEpochMs`, which is correct per the contract.
