/** X-5 (AC-11): fallback branch (no showModal): Escape works even when focus is outside the dialog (ignored while busy),
 *  a minimal Tab focus trap keeps keyboard focus inside, and listeners are removed on unmount. */
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { CancelHoldDialog } from "./cancel-hold-dialog";

const keep = () => screen.getByRole("button", { name: "Giữ lại" });
const confirm = () => screen.getByRole("button", { name: "Huỷ giữ vé" });

function blurToBody() {
  (document.activeElement as HTMLElement | null)?.blur();
  expect(document.activeElement).toBe(document.body);
}

describe("CancelHoldDialog fallback branch: Escape outside the dialog", () => {
  it("focus on body, Escape on body -> onDismiss once", () => {
    expect(typeof (HTMLDialogElement.prototype as { showModal?: unknown }).showModal).not.toBe("function");
    const onDismiss = vi.fn();
    render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={onDismiss} />);
    blurToBody();
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("Escape inside the dialog still dismisses exactly once (no double handling)", () => {
    const onDismiss = vi.fn();
    render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={onDismiss} />);
    fireEvent.keyDown(keep(), { key: "Escape" });
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("while busy (DELETE in flight): Escape on body is ignored", () => {
    const onDismiss = vi.fn();
    render(<CancelHoldDialog busy onConfirm={() => {}} onDismiss={onDismiss} />);
    blurToBody();
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("after unmount the document handler is gone", () => {
    const onDismiss = vi.fn();
    const { unmount } = render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={onDismiss} />);
    unmount();
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onDismiss).not.toHaveBeenCalled();
  });
});

describe("CancelHoldDialog fallback branch: focus trap", () => {
  it("Tab on the last button wraps to the first; Shift+Tab on the first wraps to the last", () => {
    render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={() => {}} />);
    confirm().focus();
    const tab = fireEvent.keyDown(confirm(), { key: "Tab" });
    expect(tab).toBe(false); // default prevented
    expect(document.activeElement).toBe(keep());
    const back = fireEvent.keyDown(keep(), { key: "Tab", shiftKey: true });
    expect(back).toBe(false);
    expect(document.activeElement).toBe(confirm());
  });

  it("Tab while focus is outside the dialog brings focus back inside", () => {
    render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={() => {}} />);
    blurToBody();
    fireEvent.keyDown(document.body, { key: "Tab" });
    expect(document.activeElement).toBe(keep());
  });

  it("Tab between two inner buttons is left to the browser (not prevented)", () => {
    render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={() => {}} />);
    keep().focus();
    expect(fireEvent.keyDown(keep(), { key: "Tab" })).toBe(true);
  });
});
