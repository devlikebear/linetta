import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  status: vi.fn(),
  preview: vi.fn(),
  setAuto: vi.fn(),
  runNow: vi.fn(),
  emit: null as null | ((payload: unknown) => void),
}));

vi.mock("../../lib/rpc", () => ({
  syncApi: { status: rpc.status, preview: rpc.preview, setAuto: rpc.setAuto, runNow: rpc.runNow },
}));
vi.mock("../../hooks/useEngineEvent", () => ({
  useEngineEvent: (_event: string, handler: (payload: unknown) => void) => {
    rpc.emit = handler;
  },
}));
vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    language: "ko",
    t: (key: string, vars?: Record<string, string>) => (vars ? `${key}:${Object.values(vars).join(",")}` : key),
  }),
  localeForLanguage: () => "ko-KR",
}));

import { SyncAutoPanel } from "./SyncAutoPanel";

const OFF = { enabled: false, state: "off", last_success_at: 0, works: [], archived: [] };
const ON = { ...OFF, enabled: true, state: "idle", last_success_at: 1_800_000_000_000 };

describe("SyncAutoPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.status.mockResolvedValue(OFF);
    rpc.preview.mockResolvedValue({ works: 6, bytes: 14 << 20 });
    rpc.setAuto.mockResolvedValue(ON);
    rpc.runNow.mockResolvedValue(ON);
  });

  it("asks before turning on, with how much will go", async () => {
    render(<SyncAutoPanel editable />);
    await userEvent.click(await screen.findByTestId("sync-enable"));
    const confirm = await screen.findByTestId("sync-enable-confirm");
    expect(confirm).toHaveTextContent("6");
    expect(confirm).toHaveTextContent("14.0 MiB");
    expect(rpc.setAuto).not.toHaveBeenCalled();
    await userEvent.click(screen.getByTestId("sync-enable-yes"));
    await waitFor(() => expect(rpc.setAuto).toHaveBeenCalledWith(true));
    expect(await screen.findByTestId("sync-state")).toHaveTextContent("settings.sync.idle");
  });

  it("cancelling sends nothing", async () => {
    render(<SyncAutoPanel editable />);
    await userEvent.click(await screen.findByTestId("sync-enable"));
    await userEvent.click(await screen.findByTestId("sync-enable-no"));
    expect(rpc.setAuto).not.toHaveBeenCalled();
    expect(screen.getByTestId("sync-enable")).toBeInTheDocument();
  });

  it("cannot be turned on while the account cannot upload", async () => {
    render(<SyncAutoPanel editable={false} />);
    expect(await screen.findByTestId("sync-enable")).toBeDisabled();
  });

  it("says why it stopped", async () => {
    rpc.status.mockResolvedValue({ ...ON, state: "stopped", reason: "vault_terms_outdated" });
    render(<SyncAutoPanel editable />);
    expect(await screen.findByTestId("sync-state")).toHaveTextContent("errors.vaultTermsOutdated");
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("stays quiet about being offline and shows the last success", async () => {
    rpc.status.mockResolvedValue({ ...ON, state: "waiting", reason: "vault_unreachable" });
    render(<SyncAutoPanel editable />);
    expect(await screen.findByTestId("sync-state")).toHaveTextContent("settings.sync.waiting");
    expect(screen.getByTestId("sync-last")).toHaveTextContent("settings.sync.lastSuccess");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("lists works that are not in step, and their problems", async () => {
    rpc.status.mockResolvedValue({
      ...ON,
      works: [
        { project_id: "a", title: "맞음", state: "synced" },
        { project_id: "b", title: "충돌", state: "conflict" },
        { project_id: "c", title: "고장", state: "synced", problem: "vault_damaged" },
      ],
    });
    render(<SyncAutoPanel editable />);
    expect(await screen.findByTestId("sync-work-b")).toHaveTextContent("settings.sync.work.conflict");
    expect(screen.getByTestId("sync-work-c")).toHaveTextContent("errors.vaultDamaged");
    expect(screen.queryByTestId("sync-work-a")).not.toBeInTheDocument();
  });

  it("runs now and can be turned off", async () => {
    rpc.status.mockResolvedValue(ON);
    render(<SyncAutoPanel editable />);
    await userEvent.click(await screen.findByTestId("sync-run-now"));
    expect(rpc.runNow).toHaveBeenCalled();
    rpc.setAuto.mockResolvedValue(OFF);
    await userEvent.click(await screen.findByTestId("sync-disable"));
    await waitFor(() => expect(rpc.setAuto).toHaveBeenCalledWith(false));
    expect(await screen.findByTestId("sync-enable")).toBeInTheDocument();
  });

  it("follows the engine's notifications", async () => {
    rpc.status.mockResolvedValue(ON);
    render(<SyncAutoPanel editable />);
    await screen.findByTestId("sync-state");
    rpc.emit?.({ ...ON, state: "running" });
    expect(await screen.findByText("settings.sync.running")).toBeInTheDocument();
  });
});
