import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ status: vi.fn() }));

vi.mock("../lib/rpc", () => ({ syncApi: { status: rpc.status } }));
vi.mock("../hooks/useEngineEvent", () => ({ useEngineEvent: () => {} }));
vi.mock("../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) => (vars ? `${key}:${Object.values(vars).join(",")}` : key),
  }),
  localeForLanguage: () => "ko-KR",
}));

import { SyncStatusNotice } from "./SyncStatusNotice";

const ON = { enabled: true, state: "idle", last_success_at: 1, works: [], archived: [] };

function renderNotice(available = true) {
  return render(<MemoryRouter><SyncStatusNotice available={available} /></MemoryRouter>);
}

describe("SyncStatusNotice", () => {
  beforeEach(() => vi.clearAllMocks());

  it("asks nothing on a build without sync", () => {
    const { container } = renderNotice(false);
    expect(container).toBeEmptyDOMElement();
    expect(rpc.status).not.toHaveBeenCalled();
  });

  it("says nothing when off, in step, or offline", async () => {
    for (const status of [{ ...ON, enabled: false, state: "off" }, ON, { ...ON, state: "waiting", reason: "vault_unreachable" }]) {
      rpc.status.mockResolvedValue(status);
      const { container, unmount } = renderNotice();
      await waitFor(() => expect(rpc.status).toHaveBeenCalled());
      expect(container).toBeEmptyDOMElement();
      unmount();
    }
  });

  it("says why it stopped", async () => {
    rpc.status.mockResolvedValue({ ...ON, state: "stopped", reason: "vault_quota_exceeded" });
    renderNotice();
    expect(await screen.findByTestId("sync-stopped-notice")).toHaveTextContent("errors.vaultQuotaExceeded");
  });

  it("links to works in conflict", async () => {
    rpc.status.mockResolvedValue({ ...ON, works: [{ project_id: "p1", title: "충돌 작품", state: "conflict" }] });
    renderNotice();
    expect(await screen.findByRole("link", { name: "충돌 작품" })).toHaveAttribute("href", "/workspace/p1");
  });

  it("names works moved to the archive", async () => {
    rpc.status.mockResolvedValue({ ...ON, archived: ["p1"], works: [{ project_id: "p1", title: "지운 작품", state: "synced" }] });
    renderNotice();
    expect(await screen.findByTestId("sync-archived-notice")).toHaveTextContent("지운 작품");
  });
});
