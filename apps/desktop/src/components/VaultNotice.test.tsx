import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ status: vi.fn() }));

vi.mock("../lib/rpc", () => ({ vaultApi: { status: rpc.status } }));
vi.mock("../lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
  localeForLanguage: () => "ko-KR",
}));

import { VaultNotice } from "./VaultNotice";

const ITEM = { item_id: "i1", project_id: "p1", title: "비밀 작품", head_revision: 3, base_revision: 2 };

function renderNotice(available = true) {
  return render(<MemoryRouter><VaultNotice available={available} /></MemoryRouter>);
}

describe("VaultNotice", () => {
  beforeEach(() => vi.clearAllMocks());

  it("asks nothing on a build without the vault", () => {
    const { container } = renderNotice(false);
    expect(container).toBeEmptyDOMElement();
    expect(rpc.status).not.toHaveBeenCalled();
  });

  it("names a work with a newer version and links to it", async () => {
    rpc.status.mockResolvedValue({ items: [ITEM] });
    renderNotice();
    expect(await screen.findByRole("link", { name: "비밀 작품" })).toHaveAttribute("href", "/workspace/p1");
    expect(screen.getByTestId("vault-notice")).toHaveTextContent("vault.newVersions");
  });

  it("says nothing when every work is current", async () => {
    rpc.status.mockResolvedValue({ items: [{ ...ITEM, head_revision: 2 }] });
    const { container } = renderNotice();
    await waitFor(() => expect(rpc.status).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });

  it("stays quiet when the vault cannot be reached", async () => {
    rpc.status.mockRejectedValue(new Error("offline"));
    const { container } = renderNotice();
    await waitFor(() => expect(rpc.status).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });
});
