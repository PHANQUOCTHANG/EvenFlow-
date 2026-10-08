import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { LobbyNotice } from "./lobby-notice";

describe("LobbyNotice (BR-Q1, EVF-111)", () => {
  it("render canh bao quy che phong cho cong bang va nhan manh khong uu tien vao som", () => {
    render(<LobbyNotice />);

    expect(screen.getByText(/Quy chế phòng chờ công bằng/i)).toBeInTheDocument();
    expect(screen.getByText(/Vào sớm không tạo lợi thế ưu tiên/i)).toBeInTheDocument();
    expect(screen.getByRole("status")).toBeInTheDocument();
  });
});
