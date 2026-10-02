/**
 * Test cho Alert — AC-6.
 *
 * role phai dung theo muc do: "alert" ngat loi screen reader ngay (dung cho
 * warning/error), "status" doc khi ranh (info/success). Dung role="alert" cho
 * moi thu la lam nhieu nguoi dung screen reader.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Alert, type AlertVariant } from "./alert";

const DISMISS_LABEL = "Đóng thông báo";

const ROLE_BY_VARIANT: Array<{ variant: AlertVariant; role: string }> = [
  { variant: "info", role: "status" },
  { variant: "success", role: "status" },
  { variant: "warning", role: "alert" },
  { variant: "error", role: "alert" },
];

describe("Alert — role theo variant (AC-6)", () => {
  for (const { variant, role } of ROLE_BY_VARIANT) {
    it(`variant "${variant}" co role="${role}"`, () => {
      render(<Alert variant={variant}>Nội dung thông báo</Alert>);
      expect(screen.getByRole(role)).toHaveTextContent("Nội dung thông báo");
    });
  }

  it("khong truyen variant thi mac dinh la info -> role=status", () => {
    render(<Alert>Nội dung thông báo</Alert>);
    expect(screen.getByRole("status")).toHaveTextContent("Nội dung thông báo");
  });

  it("variant error KHONG dung role=status (truong hop am)", () => {
    render(<Alert variant="error">Thanh toán thất bại</Alert>);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("variant success KHONG dung role=alert (truong hop am)", () => {
    render(<Alert variant="success">Thanh toán thành công</Alert>);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toBeInTheDocument();
  });
});

describe("Alert — title va body", () => {
  it("render ca title va body", () => {
    render(
      <Alert variant="warning" title="Sắp hết thời gian giữ vé">
        Bạn còn 2 phút để hoàn tất thanh toán.
      </Alert>,
    );

    expect(screen.getByText("Sắp hết thời gian giữ vé")).toBeInTheDocument();
    expect(screen.getByText("Bạn còn 2 phút để hoàn tất thanh toán.")).toBeInTheDocument();
  });

  it("khong truyen title thi van render body", () => {
    render(<Alert>Chỉ có nội dung</Alert>);
    expect(screen.getByRole("status")).toHaveTextContent("Chỉ có nội dung");
  });
});

describe("Alert — dismiss", () => {
  it("co onDismiss thi render nut dismiss voi aria-label tieng Viet", () => {
    render(<Alert onDismiss={() => {}}>Nội dung</Alert>);
    const btn = screen.getByRole("button", { name: DISMISS_LABEL });
    expect(btn).toBeInTheDocument();
    expect(btn).toHaveAccessibleName(DISMISS_LABEL);
  });

  it("bam nut dismiss thi onDismiss duoc goi dung mot lan", async () => {
    const user = userEvent.setup();
    const onDismiss = vi.fn();
    render(<Alert onDismiss={onDismiss}>Nội dung</Alert>);

    await user.click(screen.getByRole("button", { name: DISMISS_LABEL }));

    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("KHONG truyen onDismiss thi KHONG co nut dismiss (truong hop am)", () => {
    render(<Alert>Nội dung</Alert>);
    expect(screen.queryByRole("button", { name: DISMISS_LABEL })).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});

describe("Alert — hop dong chung", () => {
  it("spread rest xuong phan tu goc", () => {
    render(<Alert data-testid="canh-bao">Nội dung</Alert>);
    expect(screen.getByTestId("canh-bao")).toBe(screen.getByRole("status"));
  });

  it("className truyen vao duoc merge, khong ghi de class goc", () => {
    render(
      <Alert className="lop-rieng-cua-toi" data-testid="canh-bao">
        Nội dung
      </Alert>,
    );
    const el = screen.getByTestId("canh-bao");
    expect(el).toHaveClass("lop-rieng-cua-toi");
    expect(el.className.trim().split(/\s+/).length).toBeGreaterThan(1);
  });
});
