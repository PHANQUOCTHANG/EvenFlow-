/**
 * Test cho Toast + ToastProvider + useToast — AC-6.
 *
 * Ba thu de sai nhat:
 *   1. toast moi GHI DE toast cu (dung mot slot thay vi hang doi) -> user mat
 *      thong bao quan trong. Test: hai toast cung ton tai.
 *   2. auto-dismiss dung setTimeout nen phai test bang fake timer, khong cho that.
 *   3. useToast ngoai provider tra context undefined roi crash o cho khac
 *      ("cannot read property toast of undefined") -> phai throw loi RO RANG.
 */
import type { ReactNode } from "react";

import { act, fireEvent, renderHook, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ToastProvider, useToast } from "./toast";

const PROVIDER_ERROR = "useToast phải dùng trong ToastProvider";
const DISMISS_LABEL = "Đóng thông báo";

function Wrapper({ children }: { children: ReactNode }) {
  return <ToastProvider>{children}</ToastProvider>;
}

function mountToast() {
  return renderHook(() => useToast(), { wrapper: Wrapper });
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useToast — ngoai ToastProvider (AC-6)", () => {
  it("nem loi ro rang, khong fail im lang", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    expect(() => renderHook(() => useToast())).toThrow(PROVIDER_ERROR);
  });
});

describe("ToastProvider — hien toast", () => {
  it("goi toast() thi title xuat hien", () => {
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Bạn đã được vào hàng chờ" });
    });

    expect(screen.getByText("Bạn đã được vào hàng chờ")).toBeInTheDocument();
  });

  it("render ca description khi duoc truyen", () => {
    const { result } = mountToast();

    act(() => {
      result.current.toast({
        title: "Giữ vé thành công",
        description: "Bạn có 10 phút để thanh toán",
      });
    });

    expect(screen.getByText("Giữ vé thành công")).toBeInTheDocument();
    expect(screen.getByText("Bạn có 10 phút để thanh toán")).toBeInTheDocument();
  });

  it("variant error cho ra thong bao co role=alert", () => {
    const { result } = mountToast();

    act(() => {
      result.current.toast({ variant: "error", title: "Thanh toán thất bại" });
    });

    expect(screen.getByRole("alert")).toHaveTextContent("Thanh toán thất bại");
  });

  it('container co aria-live="polite" va chua toast', () => {
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Bạn đã được vào hàng chờ" });
    });

    const live = document.querySelector('[aria-live="polite"]');
    expect(live).not.toBeNull();
    expect(live).toHaveTextContent("Bạn đã được vào hàng chờ");
  });

  it("chua goi toast thi khong co thong bao nao", () => {
    mountToast();
    expect(screen.queryByText("Bạn đã được vào hàng chờ")).not.toBeInTheDocument();
  });
});

describe("ToastProvider — hang doi nhieu toast (AC-6)", () => {
  it("hai toast cung ton tai, toast sau KHONG ghi de toast truoc", () => {
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Toast thứ nhất" });
    });
    act(() => {
      result.current.toast({ title: "Toast thứ hai" });
    });

    expect(screen.getByText("Toast thứ nhất")).toBeInTheDocument();
    expect(screen.getByText("Toast thứ hai")).toBeInTheDocument();
  });

  it("ba toast lien tiep deu hien day du", () => {
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Một" });
      result.current.toast({ title: "Hai" });
      result.current.toast({ title: "Ba" });
    });

    expect(screen.getByText("Một")).toBeInTheDocument();
    expect(screen.getByText("Hai")).toBeInTheDocument();
    expect(screen.getByText("Ba")).toBeInTheDocument();
  });

  it("moi toast co id rieng, khong trung nhau", () => {
    const { result } = mountToast();
    const ids: string[] = [];

    act(() => {
      ids.push(result.current.toast({ title: "Một" }));
      ids.push(result.current.toast({ title: "Hai" }));
      ids.push(result.current.toast({ title: "Ba" }));
    });

    expect(ids.every((id) => typeof id === "string" && id.length > 0)).toBe(true);
    expect(new Set(ids).size).toBe(3);
  });
});

describe("ToastProvider — dismiss thu cong", () => {
  it("dismiss(id) xoa dung toast do, giu lai toast con lai", () => {
    const { result } = mountToast();
    let firstId = "";

    act(() => {
      firstId = result.current.toast({ title: "Toast thứ nhất" });
      result.current.toast({ title: "Toast thứ hai" });
    });

    act(() => {
      result.current.dismiss(firstId);
    });

    expect(screen.queryByText("Toast thứ nhất")).not.toBeInTheDocument();
    expect(screen.getByText("Toast thứ hai")).toBeInTheDocument();
  });

  it("dismiss voi id khong ton tai thi khong crash va khong xoa gi", () => {
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Toast thứ nhất" });
    });

    expect(() => {
      act(() => {
        result.current.dismiss("id-khong-ton-tai");
      });
    }).not.toThrow();

    expect(screen.getByText("Toast thứ nhất")).toBeInTheDocument();
  });

  it("bam nut dong (aria-label tieng Viet) xoa toast", () => {
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Toast thứ nhất" });
    });

    fireEvent.click(screen.getByRole("button", { name: DISMISS_LABEL }));

    expect(screen.queryByText("Toast thứ nhất")).not.toBeInTheDocument();
  });
});

describe("ToastProvider — auto-dismiss (fake timer, AC-6)", () => {
  it("mac dinh tu dong tat sau 5000ms", () => {
    vi.useFakeTimers();
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Tự tắt sau 5 giây" });
    });
    expect(screen.getByText("Tự tắt sau 5 giây")).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(4999);
    });
    expect(screen.getByText("Tự tắt sau 5 giây")).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(screen.queryByText("Tự tắt sau 5 giây")).not.toBeInTheDocument();
  });

  it("duration tuy chinh duoc ton trong", () => {
    vi.useFakeTimers();
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Tự tắt sau 2 giây", duration: 2000 });
    });

    act(() => {
      vi.advanceTimersByTime(1999);
    });
    expect(screen.getByText("Tự tắt sau 2 giây")).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(screen.queryByText("Tự tắt sau 2 giây")).not.toBeInTheDocument();
  });

  it("duration = 0 thi KHONG tu dong tat (truong hop am)", () => {
    vi.useFakeTimers();
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Toast dính", duration: 0 });
    });

    act(() => {
      vi.advanceTimersByTime(60_000);
    });

    expect(screen.getByText("Toast dính")).toBeInTheDocument();
  });

  it("duration am thi KHONG tu dong tat", () => {
    vi.useFakeTimers();
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Toast dính âm", duration: -1 });
    });

    act(() => {
      vi.advanceTimersByTime(60_000);
    });

    expect(screen.getByText("Toast dính âm")).toBeInTheDocument();
  });

  it("nhieu toast co duration khac nhau tat doc lap, khong keo nhau", () => {
    vi.useFakeTimers();
    const { result } = mountToast();

    act(() => {
      result.current.toast({ title: "Tắt sớm", duration: 1000 });
      result.current.toast({ title: "Tắt muộn", duration: 5000 });
    });

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.queryByText("Tắt sớm")).not.toBeInTheDocument();
    expect(screen.getByText("Tắt muộn")).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(4000);
    });
    expect(screen.queryByText("Tắt muộn")).not.toBeInTheDocument();
  });
});
