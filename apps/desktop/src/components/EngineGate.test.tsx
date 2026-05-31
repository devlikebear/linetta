import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EngineGate } from "./EngineGate";

const mocks = vi.hoisted(() => ({
  engineStatus: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  engineStatus: mocks.engineStatus,
}));

describe("EngineGate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders children after a healthy engine status", async () => {
    mocks.engineStatus.mockResolvedValue({ ok: true, version: "0.0.1" });

    render(<EngineGate><div>Library loaded</div></EngineGate>);

    expect(await screen.findByText("Library loaded")).toBeInTheDocument();
  });

  it("shows diagnostics and retries after an engine failure", async () => {
    const user = userEvent.setup();
    mocks.engineStatus
      .mockResolvedValueOnce({ ok: false, error: "engine missing" })
      .mockResolvedValueOnce({ ok: true, version: "0.0.1" });

    render(<EngineGate><div>Library loaded</div></EngineGate>);

    expect(await screen.findByText("엔진을 시작하지 못했습니다")).toBeInTheDocument();
    expect(screen.getByText("engine missing")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "다시 시도" }));

    await waitFor(() => expect(screen.getByText("Library loaded")).toBeInTheDocument());
    expect(mocks.engineStatus).toHaveBeenCalledTimes(2);
  });
});
