/**
 * Test cho RadioGroup — AC-5.
 *
 * Mot nhom radio khong co ten nhom thi screen reader doc tung option roi rac,
 * nguoi dung khong biet dang chon cai gi -> role="radiogroup" + aria-labelledby
 * tro vao legend la bat buoc.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { RadioGroup, type RadioOption } from "./radio";

const LEGEND = "Phương thức nhận vé";

const OPTIONS: RadioOption[] = [
  { value: "email", label: "Gửi qua email" },
  { value: "app", label: "Nhận trong ứng dụng" },
  { value: "counter", label: "Nhận tại quầy", disabled: true },
];

describe("RadioGroup — nhom va ten nhom", () => {
  it("co role=radiogroup", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);
    expect(screen.getByRole("radiogroup")).toBeInTheDocument();
  });

  it("ten doc duoc cua nhom la legend", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);
    expect(screen.getByRole("radiogroup")).toHaveAccessibleName(LEGEND);
  });

  it("aria-labelledby tro vao phan tu chua text legend", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);
    const group = screen.getByRole("radiogroup");
    const labelledBy = group.getAttribute("aria-labelledby");

    expect(labelledBy).toBeTruthy();
    expect(document.getElementById(labelledBy as string)).toHaveTextContent(LEGEND);
  });
});

describe("RadioGroup — option", () => {
  it("render dung so radio tu prop options", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);
    expect(screen.getAllByRole("radio")).toHaveLength(OPTIONS.length);
  });

  it("moi option co nhan rieng, lien ket dung input", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);
    for (const opt of OPTIONS) {
      expect(screen.getByLabelText(opt.label)).toHaveAttribute("type", "radio");
    }
  });

  it("moi radio dung CHUNG mot thuoc tinh name", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);
    const names = screen.getAllByRole("radio").map((r) => r.getAttribute("name"));
    expect(new Set(names)).toEqual(new Set(["delivery"]));
  });

  it("option co disabled: true thi bi disable, option khac thi khong", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);
    expect(screen.getByLabelText("Nhận tại quầy")).toBeDisabled();
    expect(screen.getByLabelText("Gửi qua email")).toBeEnabled();
  });
});

describe("RadioGroup — chon gia tri", () => {
  it("value=\"app\" thi CHI mot radio duoc chon", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} value="app" />);

    expect(screen.getByLabelText("Nhận trong ứng dụng")).toBeChecked();
    expect(screen.getByLabelText("Gửi qua email")).not.toBeChecked();
    expect(screen.getByLabelText("Nhận tại quầy")).not.toBeChecked();
    expect(screen.getAllByRole("radio").filter((r) => (r as HTMLInputElement).checked)).toHaveLength(
      1,
    );
  });

  it("khong truyen value thi khong radio nao duoc chon", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);
    expect(screen.getAllByRole("radio").filter((r) => (r as HTMLInputElement).checked)).toHaveLength(
      0,
    );
  });

  it("bam vao mot option thi onValueChange duoc goi voi dung value", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    render(
      <RadioGroup
        name="delivery"
        legend={LEGEND}
        options={OPTIONS}
        onValueChange={onValueChange}
      />,
    );

    await user.click(screen.getByLabelText("Gửi qua email"));

    expect(onValueChange).toHaveBeenCalledWith("email");
  });

  it("bam vao option disabled thi onValueChange KHONG duoc goi", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    render(
      <RadioGroup
        name="delivery"
        legend={LEGEND}
        options={OPTIONS}
        onValueChange={onValueChange}
      />,
    );

    await user.click(screen.getByLabelText("Nhận tại quầy"));

    expect(onValueChange).not.toHaveBeenCalled();
  });
});

describe("RadioGroup — error / helpText", () => {
  it("khong co error thi khong co phan tu role=alert (truong hop am)", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("co error thi thong bao loi hien voi role=alert", () => {
    render(
      <RadioGroup
        name="delivery"
        legend={LEGEND}
        options={OPTIONS}
        error="Vui lòng chọn phương thức nhận vé"
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Vui lòng chọn phương thức nhận vé");
  });

  it("khong co error thi <fieldset role=radiogroup> KHONG co aria-invalid (truong hop am)", () => {
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);
    expect(screen.getByRole("radiogroup")).not.toHaveAttribute("aria-invalid");
  });

  it("co error thi aria-invalid va aria-describedby nam tren chinh <fieldset role=radiogroup>", () => {
    render(
      <RadioGroup
        name="delivery"
        legend={LEGEND}
        options={OPTIONS}
        error="Vui lòng chọn phương thức nhận vé"
      />,
    );
    const group = screen.getByRole("radiogroup");
    const alert = screen.getByRole("alert");
    const describedBy = (group.getAttribute("aria-describedby") ?? "").split(" ").filter(Boolean);

    expect(group).toHaveAttribute("aria-invalid", "true");
    expect(describedBy).toContain(alert.id);
  });

  it("helpText duoc render", () => {
    render(
      <RadioGroup
        name="delivery"
        legend={LEGEND}
        options={OPTIONS}
        helpText="Có thể đổi sau trong đơn hàng"
      />,
    );
    expect(screen.getByText("Có thể đổi sau trong đơn hàng")).toBeInTheDocument();
  });
});

describe("RadioGroup — ban phim & className", () => {
  it("radio dau tien focus duoc bang Tab (AC-9)", async () => {
    const user = userEvent.setup();
    render(<RadioGroup name="delivery" legend={LEGEND} options={OPTIONS} />);

    await user.tab();

    expect(screen.getByLabelText("Gửi qua email")).toHaveFocus();
  });

  it("className truyen vao duoc merge, khong ghi de class goc", () => {
    render(
      <RadioGroup
        name="delivery"
        legend={LEGEND}
        options={OPTIONS}
        className="lop-rieng-cua-toi"
      />,
    );
    const group = screen.getByRole("radiogroup");
    expect(group).toHaveClass("lop-rieng-cua-toi");
    expect(group.className.trim().split(/\s+/).length).toBeGreaterThan(1);
  });
});
