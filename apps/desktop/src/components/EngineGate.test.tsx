import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EngineGate } from "./EngineGate";
import { I18nProvider } from "../lib/i18n";

const mocks = vi.hoisted(() => ({
  engineStatus: vi.fn(),
  openRecoveryFolder: vi.fn(),
  restoreLatestBackup: vi.fn(),
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  engineStatus: mocks.engineStatus,
  openRecoveryFolder: mocks.openRecoveryFolder,
  restoreLatestBackup: mocks.restoreLatestBackup,
  settings: {
    get: mocks.settingsGet,
  },
}));

describe("EngineGate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    mocks.openRecoveryFolder.mockResolvedValue(undefined);
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

  it("offers backup access and confirmed latest-backup restore when startup fails", async () => {
    const user = userEvent.setup();
    mocks.engineStatus.mockResolvedValue({
      ok: false,
      error: "database is malformed",
      home: "/tmp/linetta",
      db_path: "/tmp/linetta/library.db",
    });
    mocks.restoreLatestBackup.mockResolvedValue({
      backup_path: "/tmp/linetta/backups/2026-07-12/library-090000.db",
      quarantined_path: "/tmp/linetta/library.db.corrupt-123",
    });
    vi.spyOn(window, "confirm").mockReturnValue(true);

    renderGate();

    await user.click(await screen.findByRole("button", { name: "백업 폴더 열기" }));
    expect(mocks.openRecoveryFolder).toHaveBeenCalledOnce();

    await user.click(screen.getByRole("button", { name: "최신 백업으로 복구" }));
    expect(mocks.restoreLatestBackup).toHaveBeenCalledOnce();
    expect(await screen.findByText(/앱을 다시 시작/)).toBeInTheDocument();
  });
});
