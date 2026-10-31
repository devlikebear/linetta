import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  status: vi.fn(),
  setup: vi.fn(),
  pull: vi.fn(),
  deleteItem: vi.fn(),
  connectStatus: vi.fn(),
  openExternalUrl: vi.fn(),
  progress: { emit: (_: unknown) => {} },
}));

vi.mock("../../lib/rpc", () => ({
  vaultApi: { status: rpc.status, setup: rpc.setup, pull: rpc.pull, deleteItem: rpc.deleteItem },
  publishApi: { connectStatus: rpc.connectStatus },
  openExternalUrl: rpc.openExternalUrl,
}));

vi.mock("../../hooks/useEngineEvent", () => ({
  useEngineEvent: (_event: string, handler: (payload: unknown) => void) => {
    rpc.progress.emit = handler;
  },
}));

vi.mock("../../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    language: "ko",
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
  localeForLanguage: () => "ko-KR",
}));

import { VaultSection } from "./VaultSection";

const SITE = "https://works.marvin-42.com";
const KEY = "ABCD-EFGH-JKMN-PQRS";

const ACCOUNT = {
  mode: "full",
  status: "approved",
  retrieve_until: "",
  terms_accepted: true,
  usage: { bytes: 50 << 20, quota_bytes: 200 << 20 },
  limits: { blob_bytes: 4 << 20, restore_points: 5, items: 50, blobs_per_revision: 5000 },
};
const ITEM = {
  item_id: "i1",
  project_id: "p1",
  title: "비밀 작품",
  head_revision: 2,
  base_revision: 2,
  updated: "2026-10-31 08:00:00.000Z",
  bytes: 3 << 20,
  on_this_device: true,
};

function overview(over: Record<string, unknown> = {}, account: Record<string, unknown> = {}) {
  return {
    connected: true,
    key_stored: true,
    key_mismatch: false,
    account: { ...ACCOUNT, ...account },
    items: [],
    ...over,
  };
}

function renderSection() {
  const onOpenPublish = vi.fn();
  const view = render(<VaultSection available onOpenPublish={onOpenPublish} />);
  return { ...view, onOpenPublish };
}

describe("VaultSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.status.mockResolvedValue(overview());
    rpc.connectStatus.mockResolvedValue({ state: "connected", site_url: SITE });
    rpc.openExternalUrl.mockResolvedValue(undefined);
    rpc.setup.mockResolvedValue({ key_stored: true });
  });

  it("renders nothing on a build without the vault, and asks nothing", () => {
    const { container } = render(<VaultSection available={false} onOpenPublish={() => {}} />);
    expect(container).toBeEmptyDOMElement();
    expect(rpc.status).not.toHaveBeenCalled();
  });

  it("not connected: sends the writer to the publish pane", async () => {
    rpc.status.mockResolvedValue({ connected: false, key_stored: false, key_mismatch: false, account: null, items: [] });
    const { onOpenPublish } = renderSection();
    await userEvent.click(await screen.findByTestId("vault-open-publish"));
    expect(onOpenPublish).toHaveBeenCalled();
  });

  it("no access: says where the request stands and opens the site's page", async () => {
    const notes: [string, string][] = [
      ["none", "settings.vault.request"],
      ["closed", "settings.vault.request"],
      ["cancelled", "settings.vault.request"],
      ["pending", "settings.vault.pending"],
      ["denied", "settings.vault.denied"],
      ["revoked", "settings.vault.revoked"],
    ];
    for (const [status, note] of notes) {
      rpc.status.mockResolvedValue(overview({ key_stored: false }, { mode: "none", status }));
      const { unmount } = renderSection();
      expect(await screen.findByTestId("vault-no-access-note")).toHaveTextContent(note);
      expect(screen.queryByTestId("vault-make-key")).not.toBeInTheDocument();
      unmount();
    }

    rpc.status.mockResolvedValue(overview({ key_stored: false }, { mode: "none", status: "none" }));
    renderSection();
    await userEvent.click(await screen.findByTestId("vault-open-site"));
    expect(rpc.openExternalUrl).toHaveBeenCalledWith(`${SITE}/me/vault`);
  });

  it("refresh picks up the operator's approval", async () => {
    rpc.status.mockResolvedValue(overview({ key_stored: false }, { mode: "none", status: "pending" }));
    renderSection();
    await screen.findByTestId("vault-no-access");
    rpc.status.mockResolvedValue(overview({ key_stored: false }));
    await userEvent.click(screen.getByTestId("vault-refresh"));
    expect(await screen.findByTestId("vault-make-key")).toBeInTheDocument();
  });

  // Reading this as "no key" would offer a second key over the first.
  it("a locked secret store is an error with a retry, never the make-a-key step", async () => {
    rpc.status.mockRejectedValue({ data: { reason: "vault_key_unavailable" } });
    renderSection();
    expect(await screen.findByTestId("vault-error")).toHaveTextContent("errors.vaultKeyUnavailable");
    expect(screen.queryByTestId("vault-key-step")).not.toBeInTheDocument();

    rpc.status.mockResolvedValue(overview());
    await userEvent.click(screen.getByTestId("vault-refresh"));
    expect(await screen.findByTestId("vault-ready")).toBeInTheDocument();
    expect(screen.queryByTestId("vault-error")).not.toBeInTheDocument();
  });

  it("a refresh that fails keeps what is on screen and says why", async () => {
    renderSection();
    await screen.findByTestId("vault-ready");
    rpc.status.mockRejectedValue({ data: { reason: "vault_unreachable" } });
    await userEvent.click(screen.getByTestId("vault-refresh"));
    expect(await screen.findByTestId("vault-error")).toHaveTextContent("errors.vaultUnreachable");
    expect(screen.getByTestId("vault-ready")).toBeInTheDocument();
  });

  it("first key: says what is sent and will not start until the writer agrees", async () => {
    rpc.status.mockResolvedValue(overview({ key_stored: false }));
    renderSection();
    const make = await screen.findByTestId("vault-make-key");
    expect(screen.getByTestId("vault-disclosure")).toHaveTextContent("settings.vault.disclosure");
    expect(make).toBeDisabled();

    rpc.setup.mockResolvedValue({ key_stored: false, recovery_key: KEY });
    await userEvent.click(screen.getByTestId("vault-consent"));
    await userEvent.click(make);
    expect(rpc.setup).toHaveBeenCalledWith({ consented: true });
    expect(await screen.findByTestId("vault-recovery-key")).toHaveTextContent(KEY);
  });

  it("the key is stored only after the writer confirms saving it", async () => {
    rpc.status.mockResolvedValue(overview({ key_stored: false }));
    renderSection();
    rpc.setup.mockResolvedValue({ key_stored: false, recovery_key: KEY });
    await userEvent.click(await screen.findByTestId("vault-consent"));
    await userEvent.click(screen.getByTestId("vault-make-key"));

    const confirm = await screen.findByTestId("vault-confirm-saved");
    expect(confirm).toBeDisabled();
    expect(rpc.setup).toHaveBeenCalledTimes(1);

    rpc.setup.mockResolvedValue({ key_stored: true });
    rpc.status.mockResolvedValue(overview());
    await userEvent.click(screen.getByTestId("vault-saved-check"));
    await userEvent.click(confirm);
    await waitFor(() =>
      expect(rpc.setup).toHaveBeenLastCalledWith({ consented: true, recovery_key: KEY }),
    );
    expect(await screen.findByTestId("vault-ready")).toBeInTheDocument();
    expect(screen.queryByText(KEY)).not.toBeInTheDocument();
  });

  it("a failed confirmation keeps the key on screen", async () => {
    rpc.status.mockResolvedValue(overview({ key_stored: false }));
    renderSection();
    rpc.setup.mockResolvedValue({ key_stored: false, recovery_key: KEY });
    await userEvent.click(await screen.findByTestId("vault-consent"));
    await userEvent.click(screen.getByTestId("vault-make-key"));
    await userEvent.click(await screen.findByTestId("vault-saved-check"));

    rpc.setup.mockRejectedValue({ data: { reason: "vault_key_unavailable" } });
    await userEvent.click(screen.getByTestId("vault-confirm-saved"));
    expect(await screen.findByTestId("vault-error")).toHaveTextContent("errors.vaultKeyUnavailable");
    expect(screen.getByTestId("vault-recovery-key")).toHaveTextContent(KEY);
  });

  it("copies the key, and says so when it cannot", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    rpc.status.mockResolvedValue(overview({ key_stored: false }));
    renderSection();
    rpc.setup.mockResolvedValue({ key_stored: false, recovery_key: KEY });
    await userEvent.click(await screen.findByTestId("vault-consent"));
    await userEvent.click(screen.getByTestId("vault-make-key"));
    await screen.findByTestId("vault-copy-key");

    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    await userEvent.click(screen.getByTestId("vault-copy-key"));
    expect(writeText).toHaveBeenCalledWith(KEY);
    expect(await screen.findByTestId("vault-copy-result")).toHaveTextContent("settings.vault.copied");

    writeText.mockRejectedValue(new Error("denied"));
    await userEvent.click(screen.getByTestId("vault-copy-key"));
    await waitFor(() =>
      expect(screen.getByTestId("vault-copy-result")).toHaveTextContent("settings.vault.copyFailed"),
    );
  });

  it("another device's account: only offers entering the key it already has", async () => {
    rpc.status.mockResolvedValue(overview({ key_stored: false, items: [{ ...ITEM, title: "", project_id: "" }] }));
    renderSection();
    expect(await screen.findByTestId("vault-key-note")).toHaveTextContent("settings.vault.enterKeyNeeded");
    expect(screen.queryByTestId("vault-make-key")).not.toBeInTheDocument();

    const use = screen.getByTestId("vault-use-key");
    await userEvent.type(screen.getByTestId("vault-key-input"), ` ${KEY} `);
    expect(use).toBeDisabled();
    await userEvent.click(screen.getByTestId("vault-consent"));
    rpc.status.mockResolvedValue(overview({ items: [ITEM] }));
    await userEvent.click(use);
    expect(rpc.setup).toHaveBeenCalledWith({ consented: true, recovery_key: KEY, replace: false });
    expect(await screen.findByTestId("vault-ready")).toHaveTextContent("비밀 작품");
  });

  it("key mismatch: warns, and entering a key replaces the one on this device", async () => {
    rpc.status.mockResolvedValue(overview({ key_mismatch: true, items: [{ ...ITEM, title: "" }] }));
    renderSection();
    expect(await screen.findByTestId("vault-key-note")).toHaveTextContent("settings.vault.mismatch");
    await userEvent.click(screen.getByTestId("vault-consent"));
    await userEvent.type(screen.getByTestId("vault-key-input"), KEY);
    await userEvent.click(screen.getByTestId("vault-use-key"));
    expect(rpc.setup).toHaveBeenCalledWith({ consented: true, recovery_key: KEY, replace: true });
  });

  it("a wrong key stays in the box with the reason beside it", async () => {
    rpc.status.mockResolvedValue(overview({ key_stored: false, items: [ITEM] }));
    rpc.setup.mockRejectedValue({ data: { reason: "vault_wrong_key" } });
    renderSection();
    await userEvent.click(await screen.findByTestId("vault-consent"));
    await userEvent.type(screen.getByTestId("vault-key-input"), KEY);
    await userEvent.click(screen.getByTestId("vault-use-key"));
    expect(await screen.findByTestId("vault-error")).toHaveTextContent("errors.vaultWrongKey");
    expect(screen.getByTestId("vault-key-input")).toHaveValue(KEY);
  });

  it("ready: shows usage and works, and never a place to read the key back", async () => {
    rpc.status.mockResolvedValue(
      overview({ items: [ITEM, { ...ITEM, item_id: "i2", title: "", on_this_device: false }] }),
    );
    renderSection();
    await screen.findByTestId("vault-ready");
    expect(screen.getByTestId("vault-usage")).toHaveTextContent("50.0 MiB / 200.0 MiB");
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "25");
    const items = screen.getByTestId("vault-items");
    expect(items).toHaveTextContent("비밀 작품");
    expect(items).toHaveTextContent("settings.vault.lockedItem");
    expect(items).toHaveTextContent("settings.vault.notOnDevice");
    expect(screen.queryByTestId("vault-recovery-key")).not.toBeInTheDocument();
  });

  // The second device's first step: nothing else can bring a work here.
  it("takes a work that is not on this device, and shows how far it has got", async () => {
    const elsewhere = { ...ITEM, project_id: "", on_this_device: false };
    rpc.status.mockResolvedValue(overview({ items: [elsewhere] }));
    let finish: (value: unknown) => void = () => {};
    rpc.pull.mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    renderSection();
    await userEvent.click(await screen.findByTestId("vault-pull-i1"));
    expect(rpc.pull).toHaveBeenCalledWith({ item_id: "i1" });

    act(() => rpc.progress.emit({ project_id: "", item_id: "i1", phase: "download", done: 2, total: 8 }));
    expect(await screen.findByTestId("vault-pull-progress")).toHaveTextContent("vault.progress.download:2,8");

    rpc.status.mockResolvedValue(overview({ items: [ITEM] }));
    await act(async () => finish({ project_id: "p1", item_id: "i1", revision: 2, head_revision: 2, files_restored: true }));
    expect(await screen.findByTestId("vault-pull-result")).toHaveTextContent("settings.vault.pulledToLibrary");
    expect(screen.queryByTestId("vault-pull-progress")).not.toBeInTheDocument();
    expect(screen.queryByTestId("vault-pull-i1")).not.toBeInTheDocument();
  });

  it("says so when a pulled work's files could not be written", async () => {
    rpc.status.mockResolvedValue(overview({ items: [{ ...ITEM, on_this_device: false }] }));
    rpc.pull.mockResolvedValue({ files_restored: false });
    renderSection();
    await userEvent.click(await screen.findByTestId("vault-pull-i1"));
    expect(await screen.findByTestId("vault-pull-result")).toHaveTextContent("vault.filesNotRestored");
  });

  it("offers no download for a work this key cannot open, or one already here", async () => {
    rpc.status.mockResolvedValue(
      overview({ items: [ITEM, { ...ITEM, item_id: "i2", title: "", on_this_device: false }] }),
    );
    renderSection();
    await screen.findByTestId("vault-items");
    expect(screen.queryByTestId("vault-pull-i1")).not.toBeInTheDocument();
    expect(screen.queryByTestId("vault-pull-i2")).not.toBeInTheDocument();
  });

  it("a failed download says why and keeps the button", async () => {
    rpc.status.mockResolvedValue(overview({ items: [{ ...ITEM, on_this_device: false }] }));
    rpc.pull.mockRejectedValue({ data: { reason: "vault_damaged" } });
    renderSection();
    await userEvent.click(await screen.findByTestId("vault-pull-i1"));
    expect(await screen.findByTestId("vault-error")).toHaveTextContent("errors.vaultDamaged");
    expect(screen.getByTestId("vault-pull-i1")).toBeEnabled();
  });

  it("deletes a work from the server only after a warning and a second click", async () => {
    rpc.status.mockResolvedValue(overview({ items: [ITEM] }));
    rpc.deleteItem.mockResolvedValue({ ok: true });
    renderSection();
    await userEvent.click(await screen.findByTestId("vault-delete-i1"));
    expect(rpc.deleteItem).not.toHaveBeenCalled();
    expect(screen.getByTestId("vault-delete-confirm")).toHaveTextContent("settings.vault.deleteWarning");

    rpc.status.mockResolvedValue(overview());
    await userEvent.click(screen.getByTestId("vault-delete-yes"));
    expect(rpc.deleteItem).toHaveBeenCalledWith("i1");
    await waitFor(() => expect(screen.queryByTestId("vault-items")).not.toBeInTheDocument());
    expect(screen.queryByTestId("vault-delete-confirm")).not.toBeInTheDocument();
  });

  it("warns harder when the server holds the only copy this device knows of", async () => {
    rpc.status.mockResolvedValue(overview({ items: [{ ...ITEM, on_this_device: false }] }));
    renderSection();
    await userEvent.click(await screen.findByTestId("vault-delete-i1"));
    expect(screen.getByTestId("vault-delete-confirm")).toHaveTextContent("settings.vault.deleteWarningNotHere");
  });

  // Nothing else frees space for a writer whose key no longer opens a work.
  it("can delete a work this key cannot open", async () => {
    rpc.status.mockResolvedValue(overview({ items: [{ ...ITEM, title: "", on_this_device: false }] }));
    renderSection();
    expect(await screen.findByTestId("vault-delete-i1")).toBeEnabled();
  });

  it("cancelling deletes nothing", async () => {
    rpc.status.mockResolvedValue(overview({ items: [ITEM] }));
    renderSection();
    await userEvent.click(await screen.findByTestId("vault-delete-i1"));
    await userEvent.click(screen.getByTestId("vault-delete-no"));
    expect(screen.queryByTestId("vault-delete-confirm")).not.toBeInTheDocument();
    expect(rpc.deleteItem).not.toHaveBeenCalled();
  });

  it("a failed delete says why and leaves the question open", async () => {
    rpc.status.mockResolvedValue(overview({ items: [ITEM] }));
    rpc.deleteItem.mockRejectedValue({ data: { reason: "vault_unreachable" } });
    renderSection();
    await userEvent.click(await screen.findByTestId("vault-delete-i1"));
    await userEvent.click(screen.getByTestId("vault-delete-yes"));
    expect(await screen.findByTestId("vault-error")).toHaveTextContent("errors.vaultUnreachable");
    expect(screen.getByTestId("vault-delete-confirm")).toBeInTheDocument();
  });

  it("does not ask again about a delete that went through when the refresh fails", async () => {
    rpc.status.mockResolvedValue(overview({ items: [ITEM] }));
    rpc.deleteItem.mockResolvedValue({ ok: true });
    renderSection();
    await userEvent.click(await screen.findByTestId("vault-delete-i1"));
    rpc.status.mockRejectedValue({ data: { reason: "vault_unreachable" } });
    await userEvent.click(screen.getByTestId("vault-delete-yes"));
    expect(await screen.findByTestId("vault-error")).toHaveTextContent("errors.vaultUnreachable");
    expect(screen.queryByTestId("vault-delete-confirm")).not.toBeInTheDocument();
  });

  it("terms outdated: says uploads wait for a new agreement on the site", async () => {
    rpc.status.mockResolvedValue(overview({}, { mode: "terms_outdated", terms_accepted: false }));
    renderSection();
    expect(await screen.findByTestId("vault-terms-outdated")).toBeInTheDocument();
    await userEvent.click(screen.getByTestId("vault-open-site"));
    expect(rpc.openExternalUrl).toHaveBeenCalledWith(`${SITE}/me/vault`);
  });

  it("read only: gives the last day to download", async () => {
    rpc.status.mockResolvedValue(
      overview({}, { mode: "read_only", status: "revoked", retrieve_until: "2026-11-30 00:00:00.000Z" }),
    );
    renderSection();
    expect(await screen.findByTestId("vault-read-only")).toHaveTextContent("2026");
  });

  it("read only with nothing to read: asks for no key", async () => {
    rpc.status.mockResolvedValue(overview({ key_stored: false }, { mode: "read_only", status: "revoked" }));
    renderSection();
    expect(await screen.findByTestId("vault-ready")).toBeInTheDocument();
    expect(screen.queryByTestId("vault-key-step")).not.toBeInTheDocument();
  });
});
