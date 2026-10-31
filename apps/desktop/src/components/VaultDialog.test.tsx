import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  status: vi.fn(),
  workStatus: vi.fn(),
  push: vi.fn(),
  pull: vi.fn(),
  resolveConflict: vi.fn(),
  progress: { emit: (_: unknown) => {} },
}));

vi.mock("../lib/rpc", () => ({
  vaultApi: {
    status: rpc.status,
    workStatus: rpc.workStatus,
    push: rpc.push,
    pull: rpc.pull,
    resolveConflict: rpc.resolveConflict,
  },
}));

vi.mock("../hooks/useEngineEvent", () => ({
  useEngineEvent: (_event: string, handler: (payload: unknown) => void) => {
    rpc.progress.emit = handler;
  },
}));

vi.mock("../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    language: "ko",
    t: (key: string, vars?: Record<string, string | number>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
  localeForLanguage: () => "ko-KR",
}));

import { VaultDialog } from "./VaultDialog";

const WORK = "p1";
const POINTS = [
  { revision: 3, created: "2026-10-31 08:00:00.000Z", bytes: 3 << 20 },
  { revision: 2, created: "2026-10-30 08:00:00.000Z", bytes: 2 << 20 },
];
const PULLED = { project_id: WORK, item_id: "i1", revision: 3, head_revision: 3, snapshot_path: "/b/x.db", files_restored: true };

function ready(mode = "full", retrieve_until = "") {
  return { connected: true, key_stored: true, key_mismatch: false, account: { mode, retrieve_until }, items: [] };
}

function work(state: string, over: Record<string, unknown> = {}) {
  return { project_id: WORK, state, item_id: "i1", base_revision: 3, head_revision: 3, last_synced_at: 1, restore_points: [], ...over };
}

function renderDialog() {
  const props = { onClose: vi.fn(), onOpenSettings: vi.fn(), onStatus: vi.fn(), onReopen: vi.fn() };
  render(<VaultDialog projectId={WORK} {...props} />);
  return props;
}

describe("VaultDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.status.mockResolvedValue(ready());
    rpc.workStatus.mockResolvedValue(work("synced"));
  });

  it("takes focus, so nothing more is typed into the editor behind it", async () => {
    const editor = document.createElement("textarea");
    document.body.append(editor);
    editor.focus();
    renderDialog();
    expect(document.activeElement).toBe(screen.getByTestId("vault-dialog"));
    editor.remove();
    await screen.findByTestId("vault-state");
  });

  it("sends a device that is not set up to settings, and asks the site nothing more", async () => {
    for (const overview of [
      { connected: false, key_stored: false, key_mismatch: false, account: null, items: [] },
      { ...ready(), key_stored: false },
      { ...ready(), key_mismatch: true },
      ready("none"),
    ]) {
      rpc.status.mockResolvedValue(overview);
      const { onOpenSettings } = renderDialog();
      await userEvent.click(await screen.findByTestId("vault-open-settings"));
      expect(onOpenSettings).toHaveBeenCalledTimes(1);
      document.body.innerHTML = "";
    }
    expect(rpc.workStatus).not.toHaveBeenCalled();
  });

  it("shows each state with only the action it allows", async () => {
    const cases: [string, string, string[]][] = [
      ["not_uploaded", "vault.state.notUploaded", ["vault-push"]],
      ["synced", "vault.state.synced", []],
      ["local_changes", "vault.state.localChanges", ["vault-push"]],
      ["remote_changes", "vault.state.remoteChanges", ["vault-pull"]],
      ["conflict", "vault.state.conflict", ["vault-keep-local", "vault-take-remote"]],
    ];
    const all = ["vault-push", "vault-pull", "vault-keep-local", "vault-take-remote"];
    for (const [state, label, actions] of cases) {
      rpc.workStatus.mockResolvedValue(work(state));
      const { onStatus } = renderDialog();
      expect(await screen.findByTestId("vault-state")).toHaveTextContent(label);
      expect(onStatus).toHaveBeenCalledWith(state);
      for (const id of all) {
        if (actions.includes(id)) expect(screen.getByTestId(id)).toBeEnabled();
        else expect(screen.queryByTestId(id)).not.toBeInTheDocument();
      }
      document.body.innerHTML = "";
    }
  });

  it("uploads, shows progress for this work only, and lands on the new state", async () => {
    rpc.workStatus.mockResolvedValue(work("local_changes"));
    let finish: (value: unknown) => void = () => {};
    rpc.push.mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    const { onClose, onStatus } = renderDialog();
    await userEvent.click(await screen.findByTestId("vault-push"));
    expect(rpc.push).toHaveBeenCalledWith(WORK);

    act(() => rpc.progress.emit({ project_id: "other", item_id: "x", phase: "upload", done: 9, total: 9 }));
    expect(screen.getByTestId("vault-progress")).toHaveTextContent("vault.progress.start");
    act(() => rpc.progress.emit({ project_id: WORK, item_id: "i1", phase: "upload", done: 1, total: 4 }));
    expect(screen.getByTestId("vault-progress")).toHaveTextContent("vault.progress.upload:1,4");
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "25");

    // Leaving mid-upload would lose the answer, and nothing else can start.
    await userEvent.keyboard("{Escape}");
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByTestId("vault-close")).toBeDisabled();

    rpc.workStatus.mockResolvedValue(work("synced"));
    await act(async () => finish({ item_id: "i1", head_revision: 4, unchanged: false }));
    expect(await screen.findByTestId("vault-pushed")).toHaveTextContent("vault.pushed");
    expect(screen.getByTestId("vault-state")).toHaveTextContent("vault.state.synced");
    expect(onStatus).toHaveBeenLastCalledWith("synced");
    expect(screen.queryByTestId("vault-progress")).not.toBeInTheDocument();
  });

  it("says when there was nothing to upload", async () => {
    rpc.workStatus.mockResolvedValue(work("local_changes"));
    rpc.push.mockResolvedValue({ item_id: "i1", head_revision: 3, unchanged: true });
    renderDialog();
    await userEvent.click(await screen.findByTestId("vault-push"));
    expect(await screen.findByTestId("vault-pushed")).toHaveTextContent("vault.unchanged");
  });

  it("a conflict found while uploading brings up the choices beside the reason", async () => {
    rpc.workStatus.mockResolvedValue(work("local_changes"));
    rpc.push.mockRejectedValue({ data: { reason: "vault_conflict" } });
    renderDialog();
    const push = await screen.findByTestId("vault-push");
    rpc.workStatus.mockResolvedValue(work("conflict"));
    await userEvent.click(push);
    expect(await screen.findByTestId("vault-conflict")).toBeInTheDocument();
    expect(screen.getByTestId("vault-dialog-error")).toHaveTextContent("errors.vaultConflict");
  });

  it("a pull ends on a result whose only way out reopens the work", async () => {
    rpc.workStatus.mockResolvedValue(work("remote_changes"));
    rpc.pull.mockResolvedValue(PULLED);
    const { onClose, onReopen } = renderDialog();
    await userEvent.click(await screen.findByTestId("vault-pull"));
    expect(rpc.pull).toHaveBeenCalledWith({ project_id: WORK, revision: undefined });
    expect(await screen.findByTestId("vault-pulled")).toHaveTextContent("vault.pulled");
    expect(screen.getByTestId("vault-snapshot")).toHaveTextContent("/b/x.db");
    expect(screen.queryByTestId("vault-close")).not.toBeInTheDocument();
    expect(screen.queryByTestId("vault-files-missing")).not.toBeInTheDocument();

    await userEvent.keyboard("{Escape}");
    expect(onReopen).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
    await userEvent.click(screen.getByTestId("vault-reopen"));
    expect(onReopen).toHaveBeenCalledTimes(2);
  });

  it("clicking outside after a pull reopens too, never closes onto stale text", async () => {
    rpc.workStatus.mockResolvedValue(work("remote_changes"));
    rpc.pull.mockResolvedValue(PULLED);
    const { onClose, onReopen } = renderDialog();
    await userEvent.click(await screen.findByTestId("vault-pull"));
    await screen.findByTestId("vault-pulled");
    await userEvent.click(screen.getByTestId("vault-dialog").parentElement as HTMLElement);
    expect(onReopen).toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("warns when a pull could not write the work's files", async () => {
    rpc.workStatus.mockResolvedValue(work("remote_changes"));
    rpc.pull.mockResolvedValue({ ...PULLED, files_restored: false });
    renderDialog();
    await userEvent.click(await screen.findByTestId("vault-pull"));
    expect(await screen.findByTestId("vault-files-missing")).toHaveTextContent("vault.filesNotRestored");
  });

  it("a refused pull keeps the dialog open with the reason and the fresh state", async () => {
    rpc.workStatus.mockResolvedValue(work("remote_changes"));
    rpc.pull.mockRejectedValue({ data: { reason: "vault_local_changes" } });
    const { onReopen } = renderDialog();
    const pull = await screen.findByTestId("vault-pull");
    rpc.workStatus.mockResolvedValue(work("conflict"));
    await userEvent.click(pull);
    expect(await screen.findByTestId("vault-dialog-error")).toHaveTextContent("errors.vaultLocalChanges");
    expect(screen.getByTestId("vault-state")).toHaveTextContent("vault.state.conflict");
    expect(onReopen).not.toHaveBeenCalled();
  });

  it("conflict: keeping this device's version uploads over the server", async () => {
    rpc.workStatus.mockResolvedValue(work("conflict"));
    rpc.resolveConflict.mockResolvedValue({ push: { item_id: "i1", head_revision: 5, unchanged: false } });
    const { onReopen } = renderDialog();
    const keep = await screen.findByTestId("vault-keep-local");
    rpc.workStatus.mockResolvedValue(work("synced"));
    await userEvent.click(keep);
    expect(rpc.resolveConflict).toHaveBeenCalledWith({ project_id: WORK, choice: "keep_local" });
    expect(await screen.findByTestId("vault-pushed")).toBeInTheDocument();
    expect(screen.getByTestId("vault-state")).toHaveTextContent("vault.state.synced");
    expect(onReopen).not.toHaveBeenCalled();
  });

  it("conflict: taking the server's version names the copy and says it was kept", async () => {
    rpc.workStatus.mockResolvedValue(work("conflict"));
    rpc.resolveConflict.mockResolvedValue({ pull: { ...PULLED, copy_project_id: "p2" } });
    renderDialog();
    await userEvent.click(await screen.findByTestId("vault-take-remote"));
    expect(rpc.resolveConflict).toHaveBeenCalledWith({
      project_id: WORK,
      choice: "take_remote",
      title_suffix: "vault.copySuffix",
    });
    expect(await screen.findByTestId("vault-copy-kept")).toHaveTextContent("vault.copyKept");
  });

  it("goes back to a restore point only after a second click", async () => {
    rpc.workStatus.mockResolvedValue(work("synced", { restore_points: POINTS }));
    rpc.pull.mockResolvedValue({ ...PULLED, revision: 2 });
    renderDialog();
    const points = await screen.findByTestId("vault-restore-points");
    expect(points).toHaveTextContent("vault.latest");

    const back = screen.getByTestId("vault-restore-2");
    expect(back).toHaveTextContent("vault.restore");
    await userEvent.click(back);
    expect(rpc.pull).not.toHaveBeenCalled();
    expect(back).toHaveTextContent("vault.restoreConfirm");
    await userEvent.click(back);
    expect(rpc.pull).toHaveBeenCalledWith({ project_id: WORK, revision: 2 });
    await waitFor(() => expect(screen.getByTestId("vault-pulled")).toBeInTheDocument());
  });

  // After a pull that could not write the work's files, this is the way back.
  it("takes the latest version again once it is confirmed", async () => {
    rpc.workStatus.mockResolvedValue(work("synced", { restore_points: POINTS }));
    rpc.pull.mockResolvedValue(PULLED);
    renderDialog();
    const again = await screen.findByTestId("vault-restore-3");
    expect(again).toHaveTextContent("vault.pullAgain");
    await userEvent.click(again);
    expect(rpc.pull).not.toHaveBeenCalled();
    await userEvent.click(again);
    expect(rpc.pull).toHaveBeenCalledWith({ project_id: WORK, revision: 3 });
  });

  it("has no second way to take the latest when a newer version is on offer", async () => {
    rpc.workStatus.mockResolvedValue(work("remote_changes", { restore_points: POINTS }));
    renderDialog();
    await screen.findByTestId("vault-pull");
    expect(screen.queryByTestId("vault-restore-3")).not.toBeInTheDocument();
    expect(screen.getByTestId("vault-restore-2")).toBeInTheDocument();
  });

  it("a failed restore asks for confirmation again", async () => {
    rpc.workStatus.mockResolvedValue(work("synced", { restore_points: POINTS }));
    rpc.pull.mockRejectedValue({ data: { reason: "vault_revision_gone" } });
    renderDialog();
    await userEvent.click(await screen.findByTestId("vault-restore-2"));
    await userEvent.click(screen.getByTestId("vault-restore-2"));
    expect(await screen.findByTestId("vault-dialog-error")).toHaveTextContent("errors.vaultRevisionGone");
    expect(screen.getByTestId("vault-restore-2")).toHaveTextContent("vault.restore");
    expect(screen.getByTestId("vault-restore-2")).not.toHaveTextContent("vault.restoreConfirm");
  });

  it("offers no restore while unsent work would be lost", async () => {
    for (const state of ["local_changes", "conflict"]) {
      rpc.workStatus.mockResolvedValue(work(state, { restore_points: POINTS }));
      renderDialog();
      expect(await screen.findByTestId("vault-restore-points")).toHaveTextContent("vault.restoreBlocked");
      expect(screen.queryByTestId("vault-restore-2")).not.toBeInTheDocument();
      document.body.innerHTML = "";
    }
  });

  it("terms outdated or read only: uploading is off, downloading is not", async () => {
    rpc.status.mockResolvedValue(ready("terms_outdated"));
    rpc.workStatus.mockResolvedValue(work("local_changes"));
    renderDialog();
    expect(await screen.findByTestId("vault-push")).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent("settings.vault.termsOutdated");
    document.body.innerHTML = "";

    rpc.status.mockResolvedValue(ready("read_only", "2026-11-30 00:00:00.000Z"));
    rpc.workStatus.mockResolvedValue(work("conflict"));
    renderDialog();
    expect(await screen.findByTestId("vault-keep-local")).toBeDisabled();
    expect(screen.getByTestId("vault-take-remote")).toBeEnabled();
    expect(screen.getByRole("alert")).toHaveTextContent("settings.vault.readOnly");
    document.body.innerHTML = "";

    rpc.workStatus.mockResolvedValue(work("remote_changes"));
    renderDialog();
    expect(await screen.findByTestId("vault-pull")).toBeEnabled();
  });

  it("a failed status check says why and can be closed", async () => {
    rpc.status.mockRejectedValue({ data: { reason: "vault_unreachable" } });
    const { onClose } = renderDialog();
    expect(await screen.findByTestId("vault-dialog-error")).toHaveTextContent("errors.vaultUnreachable");
    await userEvent.click(screen.getByTestId("vault-close"));
    expect(onClose).toHaveBeenCalled();
  });
});
