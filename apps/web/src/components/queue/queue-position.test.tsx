import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { QueuePosition } from "./queue-position";

describe("QueuePosition (EV-182 AC-5)", () => {
  it("BR-Q1: LOBBY không hiển thị rank kể cả khi caller truyền vào", () => {
    render(
      <QueuePosition
        state="LOBBY"
        rank={42}
        etaSeconds={30}
        initialRank={100}
      />
    );

    expect(screen.getByText("Phòng chờ chung")).toBeInTheDocument();
    expect(screen.getByText(/Bạn chưa có số thứ tự/)).toBeInTheDocument();
    expect(screen.queryByText("42")).toBeNull();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("QUEUED: hiển thị số thứ tự và tính ETA khi có dữ liệu từ server", () => {
    render(
      <QueuePosition
        state="QUEUED"
        rank={150}
        etaSeconds={90}
        initialRank={500}
      />
    );

    expect(screen.getByText("Số thứ tự của bạn")).toBeInTheDocument();
    expect(screen.getByText("150")).toBeInTheDocument();
    expect(screen.getByText(/Thời gian ước tính: ~1 phút 30 giây/)).toBeInTheDocument();
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
  });

  it("QUEUED: etaSeconds = null hoặc âm -> hiển thị 'đang cập nhật', không tự bịa số", () => {
    render(
      <QueuePosition
        state="QUEUED"
        rank={80}
        etaSeconds={null}
        initialRank={200}
      />
    );

    expect(screen.getByText("Thời gian ước tính: đang cập nhật")).toBeInTheDocument();
  });

  it("ADMITTED: hiển thị thông báo đã tới lượt", () => {
    render(
      <QueuePosition
        state="ADMITTED"
        rank={0}
        etaSeconds={0}
        initialRank={200}
      />
    );

    expect(screen.getByText("Đã tới lượt bạn")).toBeInTheDocument();
  });
});
