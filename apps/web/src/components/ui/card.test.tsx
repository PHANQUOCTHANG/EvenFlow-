/**
 * Test cho Card — AC-8.
 *
 * Diem de sai nhat: render the rong cho slot khong duoc truyen. Mot <div> header
 * rong van chiem padding/border -> layout lech. Nen o day kiem CA truong hop am.
 */
import { createRef } from "react";

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Card } from "./card";

describe("Card — slot body", () => {
  it('children render trong [data-slot="body"]', () => {
    const { container } = render(<Card>Nội dung thẻ</Card>);
    const body = container.querySelector('[data-slot="body"]');
    expect(body).not.toBeNull();
    expect(body).toHaveTextContent("Nội dung thẻ");
  });

  it("children phuc tap van render duoc", () => {
    render(
      <Card>
        <h2>Vé VIP</h2>
        <p>1.500.000 đ</p>
      </Card>,
    );
    expect(screen.getByRole("heading", { name: "Vé VIP" })).toBeInTheDocument();
    expect(screen.getByText("1.500.000 đ")).toBeInTheDocument();
  });
});

describe("Card — slot header / footer khi duoc truyen", () => {
  it('header render trong [data-slot="header"]', () => {
    const { container } = render(<Card header={<h3>Tiêu đề</h3>}>Thân</Card>);
    const header = container.querySelector('[data-slot="header"]');
    expect(header).not.toBeNull();
    expect(header).toHaveTextContent("Tiêu đề");
    expect(screen.getByRole("heading", { name: "Tiêu đề" })).toBeInTheDocument();
  });

  it('footer render trong [data-slot="footer"]', () => {
    const { container } = render(<Card footer={<span>Chân thẻ</span>}>Thân</Card>);
    const footer = container.querySelector('[data-slot="footer"]');
    expect(footer).not.toBeNull();
    expect(footer).toHaveTextContent("Chân thẻ");
  });

  it("ca ba slot cung ton tai khi truyen day du", () => {
    const { container } = render(
      <Card header="Đầu" footer="Cuối">
        Giữa
      </Card>,
    );
    expect(container.querySelector('[data-slot="header"]')).toHaveTextContent("Đầu");
    expect(container.querySelector('[data-slot="body"]')).toHaveTextContent("Giữa");
    expect(container.querySelector('[data-slot="footer"]')).toHaveTextContent("Cuối");
  });
});

describe("Card — KHONG render slot rong (truong hop am)", () => {
  it("khong truyen header thi khong co phan tu header nao", () => {
    const { container } = render(<Card>Chỉ có thân</Card>);
    expect(container.querySelector('[data-slot="header"]')).toBeNull();
  });

  it("khong truyen footer thi khong co phan tu footer nao", () => {
    const { container } = render(<Card>Chỉ có thân</Card>);
    expect(container.querySelector('[data-slot="footer"]')).toBeNull();
  });

  it("chi truyen header thi khong sinh footer, va nguoc lai", () => {
    const onlyHeader = render(<Card header="Đầu">Thân</Card>);
    expect(onlyHeader.container.querySelector('[data-slot="header"]')).not.toBeNull();
    expect(onlyHeader.container.querySelector('[data-slot="footer"]')).toBeNull();
    onlyHeader.unmount();

    const onlyFooter = render(<Card footer="Cuối">Thân</Card>);
    expect(onlyFooter.container.querySelector('[data-slot="footer"]')).not.toBeNull();
    expect(onlyFooter.container.querySelector('[data-slot="header"]')).toBeNull();
  });
});

describe("Card — hop dong chung", () => {
  it("forwardRef tro toi phan tu <div> goc", () => {
    const ref = createRef<HTMLDivElement>();
    render(
      <Card ref={ref} data-testid="the">
        Thân
      </Card>,
    );
    expect(ref.current).toBeInstanceOf(HTMLDivElement);
    expect(ref.current).toBe(screen.getByTestId("the"));
  });

  it("spread rest xuong phan tu goc", () => {
    render(
      <Card data-testid="the" aria-label="Thẻ vé">
        Thân
      </Card>,
    );
    expect(screen.getByTestId("the")).toHaveAttribute("aria-label", "Thẻ vé");
  });

  it("className truyen vao duoc merge, khong ghi de class goc", () => {
    render(
      <Card className="lop-rieng-cua-toi" data-testid="the">
        Thân
      </Card>,
    );
    const el = screen.getByTestId("the");
    expect(el).toHaveClass("lop-rieng-cua-toi");
    expect(el.className.trim().split(/\s+/).length).toBeGreaterThan(1);
  });
});
