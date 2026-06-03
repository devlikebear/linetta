import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EngineGate } from "./EngineGate";
import { I18nProvider } from "../lib/i18n";

const mocks = vi.hoisted(() => ({
  engineStatus: vi.fn(),
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  engineStatus: mocks.engineStatus,
  settings: {
    get: mocks.settingsGet,
  },
}));

describe("EngineGate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
  });

  const renderGate = () => render(
    <I18nProvider>
      <EngineGate><div>Library loaded</div></EngineGate>
    </I18nProvider>,
  );

  it("renders children after a healthy engine status", async () => {
    mocks.engineStatus.mockResolvedValue({ ok: true, version: "0.0.1" });

    renderGate();

    expect(await screen.findByText("Library loaded")).toBeInTheDocument();
  });

  it("shows diagnostics and retries after an engine failure", async () => {
    const user = userEvent.setup();
    mocks.engineStatus
      .mockResolvedValueOnce({ ok: false, error: "engine missing" })
      .mockResolvedValueOnce({ ok: true, version: "0.0.1" });

    renderGate();

    expect(await screen.findByText("엔진을 시작하지 못했습니다")).toBeInTheDocument();
    expect(screen.getByText("engine missing")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "다시 시도" }));

    await waitFor(() => expect(screen.getByText("Library loaded")).toBeInTheDocument());
    expect(mocks.engineStatus).toHaveBeenCalledTimes(2);
  });
});
