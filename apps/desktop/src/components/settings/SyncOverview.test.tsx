import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ targets: vi.fn(), setTargets: vi.fn(), restore: vi.fn(), setup: vi.fn() }));

vi.mock("../../lib/rpc", () => ({
  syncApi: { targets: rpc.targets, setTargets: rpc.setTargets, restore: rpc.restore },
  vaultApi: { setup: rpc.setup },
}));
vi.mock("./SyncAutoPanel", () => ({ SyncAutoPanel: () => <div data-testid="sync-auto-stub" /> }));
vi.mock("./VaultItemList", () => ({ VaultItemList: () => <div data-testid="vault-items-stub" /> }));
vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
  localeForLanguage: () => "ko-KR",
}));

import { SyncOverview } from "./SyncOverview";

const target = (id: string, over: Record<string, unknown> = {}) => ({
  id,
  configured: true,
  can_primary: true,
  bytes: 0,
  last_success_at: 0,
  ...over,
});
const VIEW = {
  primary: "works",
  mirrors: [] as string[],
  key_stored: true,
  targets: [target("works"), target("git", { bytes: 3 << 20 }), target("folder")],
};

describe("SyncOverview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.targets.mockResolvedValue(VIEW);
    rpc.setTargets.mockResolvedValue(VIEW);
    rpc.restore.mockResolvedValue({});
    rpc.setup.mockResolvedValue({ key_stored: true });
  });

  it("leaves out the targets when Linetta Works is the only one (iOS), but keeps automatic sync and the items", async () => {
    rpc.targets.mockResolvedValue({ ...VIEW, targets: [target("works")] });
    render(<SyncOverview />);
    expect(await screen.findByTestId("sync-auto-stub")).toBeInTheDocument();
    expect(screen.getByTestId("vault-items-stub")).toBeInTheDocument();
    expect(screen.queryByTestId("sync-overview")).toBeNull();
  });

  it("shows the two-way target, and it cannot also be a mirror", async () => {
    render(<SyncOverview />);
    expect(await screen.findByTestId("sync-primary-works")).toBeChecked();
    expect(screen.getByTestId("sync-mirror-works")).toBeDisabled();
    expect(screen.getByTestId("sync-target-line-git")).toHaveTextContent("3.0 MiB");
  });

  it("turns a target into a mirror", async () => {
    render(<SyncOverview />);
    await userEvent.click(await screen.findByTestId("sync-mirror-folder"));
    expect(rpc.setTargets).toHaveBeenCalledWith("works", ["folder"]);
  });

  it("changes the two-way target and takes it off the mirrors", async () => {
    rpc.targets.mockResolvedValue({ ...VIEW, mirrors: ["folder"] });
    render(<SyncOverview />);
    await userEvent.click(await screen.findByTestId("sync-primary-folder"));
    expect(rpc.setTargets).toHaveBeenCalledWith("folder", []);
  });

  it("warns before GitHub becomes the two-way target", async () => {
    render(<SyncOverview />);
    await userEvent.click(await screen.findByTestId("sync-primary-git"));
    expect(rpc.setTargets).not.toHaveBeenCalled();
    expect(screen.getByTestId("sync-git-confirm")).toHaveTextContent("settings.sync.gitWarning");
    await userEvent.click(screen.getByTestId("sync-git-yes"));
    expect(rpc.setTargets).toHaveBeenCalledWith("git", []);
  });

  it("a build that cannot read the folder offers it as a mirror only", async () => {
    rpc.targets.mockResolvedValue({ ...VIEW, targets: [target("works"), target("folder", { can_primary: false })] });
    render(<SyncOverview />);
    expect(await screen.findByTestId("sync-primary-folder")).toBeDisabled();
    expect(screen.getByTestId("sync-mirror-folder")).toBeEnabled();
    expect(screen.getByTestId("sync-mirror-only-folder")).toBeInTheDocument();
  });

  it("a target that is not set up cannot be chosen", async () => {
    rpc.targets.mockResolvedValue({ ...VIEW, targets: [target("works"), target("git", { configured: false })] });
    render(<SyncOverview />);
    expect(await screen.findByTestId("sync-primary-git")).toBeDisabled();
    expect(screen.getByTestId("sync-target-line-git")).toHaveTextContent("settings.sync.target.notSet");
  });

  it("makes a recovery key with no account, shows it once, and stores it after saving", async () => {
    rpc.targets.mockResolvedValue({ ...VIEW, key_stored: false, targets: [target("works", { configured: false }), target("folder")] });
    rpc.setup.mockResolvedValueOnce({ key_stored: false, recovery_key: "AAAA-BBBB" });
    render(<SyncOverview />);
    expect(await screen.findByTestId("sync-primary-folder")).toBeDisabled();
    await userEvent.click(screen.getByTestId("sync-key-consent"));
    await userEvent.click(screen.getByTestId("sync-key-make"));
    expect(rpc.setup).toHaveBeenCalledWith({ consented: true, target: "folder" });
    expect(await screen.findByTestId("sync-new-key")).toHaveTextContent("AAAA-BBBB");
    await userEvent.click(screen.getByTestId("sync-key-saved"));
    await userEvent.click(screen.getByTestId("sync-key-confirm"));
    await waitFor(() => expect(rpc.setup).toHaveBeenCalledWith({ consented: true, recovery_key: "AAAA-BBBB", target: "folder" }));
  });

  it("restores from a mirror", async () => {
    render(<SyncOverview />);
    await userEvent.click(await screen.findByTestId("sync-restore-folder"));
    expect(rpc.restore).toHaveBeenCalledWith("folder");
    expect(await screen.findByTestId("sync-restored")).toBeInTheDocument();
  });

  it("says why a mirror failed", async () => {
    rpc.targets.mockResolvedValue({ ...VIEW, targets: [target("works"), target("folder", { reason: "sync_target_missing" })] });
    render(<SyncOverview />);
    expect(await screen.findByTestId("sync-target-line-folder")).toHaveTextContent("errors.syncTargetMissing");
  });

  it("keeps the failure of a change on screen", async () => {
    rpc.setTargets.mockRejectedValue(new Error("boom"));
    render(<SyncOverview />);
    await userEvent.click(await screen.findByTestId("sync-mirror-folder"));
    expect(await screen.findByTestId("sync-targets-error")).toBeInTheDocument();
  });
});
