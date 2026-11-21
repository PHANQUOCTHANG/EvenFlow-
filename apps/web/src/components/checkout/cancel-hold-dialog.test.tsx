/** AC-11 dialog mechanics (plan-review R2-1, R2-2, R2-4): the native showModal() branch (stubbed, jsdom has none) and the
 *  open-attribute fallback branch. */
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CancelHoldDialog } from "./cancel-hold-dialog";

const proto = HTMLDialogElement.prototype as unknown as {
  showModal?: () => void;
  close?: () => void;
};

function stubNative() {
  const showModal = vi.fn(function (this: HTMLDialogElement) {
    if (this.hasAttribute("open")) throw new DOMException("already open", "InvalidStateError");
    this.setAttribute("open", "");
  });
  const close = vi.fn(function (this: HTMLDialogElement) {
    this.removeAttribute("open");
    this.dispatchEvent(new Event("close"));
  });
  proto.showModal = showModal;
  proto.close = close;
  return { showModal, close };
}

afterEach(() => {
  delete proto.showModal;
  delete proto.close;
});

const dialog = () => screen.getByRole("alertdialog");

describe("CancelHoldDialog native branch (showModal)", () => {
  it("opens with showModal once, never via the React open prop; labelled, described, focus on keep", () => {
    const { showModal } = stubNative();
    render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={() => {}} />);
    expect(showModal).toHaveBeenCalledTimes(1);
    const d = dialog();
    expect(d.getAttribute("aria-modal")).toBe("true");
    expect(document.getElementById(d.getAttribute("aria-labelledby") as string)?.textContent).toBe("Huỷ giữ vé?");
    expect(document.getElementById(d.getAttribute("aria-describedby") as string)?.textContent).toBeTruthy();
    expect(document.activeElement?.textContent).toBe("Giữ lại");
  });

  it("native Escape (cancel event) is prevented and routed to onDismiss", () => {
    stubNative();
    const onDismiss = vi.fn();
    render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={onDismiss} />);
    const ev = new Event("cancel", { cancelable: true });
    act(() => {
      dialog().dispatchEvent(ev);
    });
    expect(ev.defaultPrevented).toBe(true);
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("while busy: cancel event, Escape and 'Giữ lại' do not dismiss; confirm does not re-fire", () => {
    stubNative();
    const onDismiss = vi.fn();
    const onConfirm = vi.fn();
    render(<CancelHoldDialog busy onConfirm={onConfirm} onDismiss={onDismiss} />);
    const ev = new Event("cancel", { cancelable: true });
    act(() => {
      dialog().dispatchEvent(ev);
    });
    fireEvent.keyDown(dialog(), { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Giữ lại" }));
    fireEvent.click(screen.getByRole("button", { name: "Huỷ giữ vé" }));
    expect(ev.defaultPrevented).toBe(true);
    expect(onDismiss).not.toHaveBeenCalled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("unmount closes the native dialog without calling onDismiss again", () => {
    const { close } = stubNative();
    const onDismiss = vi.fn();
    const { unmount } = render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={onDismiss} />);
    unmount();
    expect(close).toHaveBeenCalledTimes(1);
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("a close the browser does on its own syncs state through onDismiss", () => {
    stubNative();
    const onDismiss = vi.fn();
    render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={onDismiss} />);
    act(() => {
      dialog().dispatchEvent(new Event("close"));
    });
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});

describe("CancelHoldDialog fallback branch (no showModal)", () => {
  it("sets the open attribute and handles Escape itself", () => {
    const onDismiss = vi.fn();
    render(<CancelHoldDialog busy={false} onConfirm={() => {}} onDismiss={onDismiss} />);
    expect(dialog().hasAttribute("open")).toBe(true);
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("confirm calls onConfirm; keep calls onDismiss", () => {
    const onDismiss = vi.fn();
    const onConfirm = vi.fn();
    render(<CancelHoldDialog busy={false} onConfirm={onConfirm} onDismiss={onDismiss} />);
    fireEvent.click(screen.getByRole("button", { name: "Huỷ giữ vé" }));
    fireEvent.click(screen.getByRole("button", { name: "Giữ lại" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
