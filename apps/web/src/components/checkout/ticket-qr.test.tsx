import { render, screen, waitFor } from "@testing-library/react";
import QRCode from "qrcode";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TicketQr } from "./ticket-qr";

async function svgOf(payload: string) {
  const { container } = render(<TicketQr payload={payload} label={`QR ${payload}`} />);
  await waitFor(() => expect(container.querySelector("svg")).not.toBeNull());
  return container.querySelector("svg")!.outerHTML;
}

describe("TicketQr (AC-15)", () => {
  afterEach(() => vi.restoreAllMocks());

  it("ve mot SVG QR that tu payload, giu role img, aria-label va data-qr-payload", async () => {
    const { container } = render(<TicketQr payload="SIGNED.PAYLOAD-1" label="Mã QR vé A1" />);
    const box = await screen.findByRole("img", { name: "Mã QR vé A1" });
    expect(box.getAttribute("data-qr-payload")).toBe("SIGNED.PAYLOAD-1");
    await waitFor(() => expect(container.querySelector("svg path")).not.toBeNull());
    expect(container.textContent).not.toContain("SIGNED.PAYLOAD-1");
  });

  it("payload khac nhau cho ra QR khac nhau, cung payload cho ra cung QR", async () => {
    const a = await svgOf("PAYLOAD-AAA");
    document.body.innerHTML = "";
    const b = await svgOf("PAYLOAD-BBB");
    document.body.innerHTML = "";
    const a2 = await svgOf("PAYLOAD-AAA");
    expect(a).not.toBe(b);
    expect(a).toBe(a2);
  });

  it("moi ve mot QR rieng", async () => {
    render(
      <>
        <TicketQr payload="P1" label="Mã QR vé 1" />
        <TicketQr payload="P2" label="Mã QR vé 2" />
      </>,
    );
    expect(await screen.findAllByRole("img", { name: /Mã QR vé/ })).toHaveLength(2);
    await waitFor(() => expect(document.querySelectorAll("svg")).toHaveLength(2));
  });

  it("tao QR that bai: chi hien payload dang chu du phong, khong co SVG", async () => {
    vi.spyOn(QRCode, "toString").mockRejectedValue(new Error("boom"));
    const { container } = render(<TicketQr payload="FALLBACK-PAYLOAD" label="Mã QR vé F" />);
    await waitFor(() => expect(container.querySelector("code")?.textContent).toBe("FALLBACK-PAYLOAD"));
    expect(container.querySelector("svg")).toBeNull();
    expect(screen.getByRole("img", { name: "Mã QR vé F" })).toBeInTheDocument();
  });
});
