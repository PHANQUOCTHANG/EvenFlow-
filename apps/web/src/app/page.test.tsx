/** Component test mau: render trang chu bang React Testing Library. */
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import Home from "./page";

describe("Home", () => {
  it("hien ten nen tang", () => {
    render(<Home />);
    expect(screen.getByRole("heading", { level: 1, name: "EventFlow" })).toBeInTheDocument();
  });
});
