/**
 * Test cho Container — AC-7.
 *
 * Gia tri thuc (1200px / 1440px) khong kiem duoc o jsdom: khong co layout engine
 * va Tailwind khong chay trong test, nen `getComputedStyle().maxWidth` luon rong.
 * Vi vay test o day kiem dieu KIEM DUOC va van bat duoc bug that:
 *   - hai variant phai sinh ra class KHAC NHAU (neu quen map variant thi 2 variant
 *     ra cung class -> "workspace" khong rong hon "public" chut nao)
 *   - mac dinh phai dung voi variant="public"
 *   - `className` nguoi dung truyen vao khong duoc ghi de class goc
 * Phan padding ngang gutter/margin theo breakpoint: xem traceability.md.
 */
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Container, type ContainerVariant } from "./container";

const VARIANTS: ContainerVariant[] = ["public", "workspace"];

function classOf(testId: string): string {
  return screen.getByTestId(testId).getAttribute("class") ?? "";
}

describe("Container — render co ban", () => {
  it("render children", () => {
    render(<Container>Noi dung trang</Container>);
    expect(screen.getByText("Noi dung trang")).toBeInTheDocument();
  });

  it("mac dinh render thanh <div>", () => {
    render(<Container data-testid="ct">Noi dung</Container>);
    expect(screen.getByTestId("ct").tagName).toBe("DIV");
  });

  it("spread rest xuong phan tu goc", () => {
    render(
      <Container data-testid="ct" aria-label="Khung noi dung">
        Noi dung
      </Container>,
    );
    expect(screen.getByTestId("ct")).toHaveAttribute("aria-label", "Khung noi dung");
  });

  it("className truyen vao duoc merge, KHONG ghi de class goc", () => {
    render(
      <Container data-testid="ct" className="lop-rieng-cua-toi">
        Noi dung
      </Container>,
    );
    const el = screen.getByTestId("ct");
    expect(el).toHaveClass("lop-rieng-cua-toi");
    expect(el.className.trim().split(/\s+/).length).toBeGreaterThan(1);
  });
});

describe("Container — prop `as`", () => {
  it('as="section" render thanh <section>', () => {
    render(
      <Container as="section" data-testid="ct">
        Noi dung
      </Container>,
    );
    expect(screen.getByTestId("ct").tagName).toBe("SECTION");
  });

  it('as="main" render thanh <main> va duoc nhan dien la landmark main', () => {
    render(
      <Container as="main" data-testid="ct">
        Noi dung
      </Container>,
    );
    expect(screen.getByTestId("ct").tagName).toBe("MAIN");
    expect(screen.getByRole("main")).toBe(screen.getByTestId("ct"));
  });

  it('as="div" render thanh <div>', () => {
    render(
      <Container as="div" data-testid="ct">
        Noi dung
      </Container>,
    );
    expect(screen.getByTestId("ct").tagName).toBe("DIV");
  });

  it("doi `as` khong lam mat class cua variant", () => {
    const asDiv = render(
      <Container data-testid="ct" variant="workspace">
        Noi dung
      </Container>,
    );
    const divClass = classOf("ct");
    asDiv.unmount();

    render(
      <Container data-testid="ct" as="section" variant="workspace">
        Noi dung
      </Container>,
    );
    expect(classOf("ct")).toBe(divClass);
  });
});

describe("Container — AC-7 variant chieu rong", () => {
  it("moi variant render duoc", () => {
    for (const variant of VARIANTS) {
      const view = render(
        <Container data-testid="ct" variant={variant}>
          Noi dung
        </Container>,
      );
      expect(screen.getByTestId("ct")).toBeInTheDocument();
      view.unmount();
    }
  });

  it('variant="public" va variant="workspace" cho ra class KHAC NHAU', () => {
    const classes = VARIANTS.map((variant) => {
      const view = render(
        <Container data-testid="ct" variant={variant}>
          Noi dung
        </Container>,
      );
      const cls = classOf("ct");
      view.unmount();
      return cls;
    });

    expect(new Set(classes).size).toBe(VARIANTS.length);
  });

  it('khong truyen variant thi giong y variant="public" (mac dinh theo hop dong §5)', () => {
    const implicit = render(<Container data-testid="ct">Noi dung</Container>);
    const implicitClass = classOf("ct");
    implicit.unmount();

    render(
      <Container data-testid="ct" variant="public">
        Noi dung
      </Container>,
    );

    expect(classOf("ct")).toBe(implicitClass);
  });

  it('khong truyen variant thi KHAC variant="workspace"', () => {
    const implicit = render(<Container data-testid="ct">Noi dung</Container>);
    const implicitClass = classOf("ct");
    implicit.unmount();

    render(
      <Container data-testid="ct" variant="workspace">
        Noi dung
      </Container>,
    );

    expect(classOf("ct")).not.toBe(implicitClass);
  });

  it("class khong rong (co that su ap dung style, khong phai <div> tran)", () => {
    for (const variant of VARIANTS) {
      const view = render(
        <Container data-testid="ct" variant={variant}>
          Noi dung
        </Container>,
      );
      expect(classOf("ct").trim().length).toBeGreaterThan(0);
      view.unmount();
    }
  });
});
