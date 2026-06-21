import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { snapshots } from "../lib/rpc";
import { VersionSheet } from "./VersionSheet";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  listForNode: vi.fn(),
  compare: vi.fn(),
  restore: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
  snapshots: {
    listForNode: mocks.listForNode,
    compare: mocks.compare,
    restore: mocks.restore,
  },
}));

beforeEach(() => {
  vi.clearAllMocks();
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
  mocks.listForNode.mockResolvedValue([
    { id: "new", reason: "autosave", created_at: 2000, doc_preview: "첫 문장\n새 문장" },
    { id: "old", reason: "manual", created_at: 1000, doc_preview: "첫 문장\n삭제될 문장" },
  ]);
  mocks.compare.mockResolvedValue({
    left: { id: "old", reason: "manual", created_at: 1000, plaintext: "첫 문장\n삭제될 문장\n" },
    right: { id: "new", reason: "autosave", created_at: 2000, plaintext: "첫 문장\n새 문장\n" },
  });
});

describe("VersionSheet", () => {
  it("renders a wide review area for version preview and comparison", async () => {
    const { container } = render(
      <I18nProvider>
        <VersionSheet nodeId="scene-1" onClose={vi.fn()} onRestored={vi.fn()} />
      </I18nProvider>,
    );

    await waitFor(() => {
      expect(container.querySelector(".vs-preview")).toHaveTextContent("새 문장");
    });

    expect(container.querySelector("aside")).toHaveClass("history-panel");
    expect(container.querySelector(".vs-review-sec")).toBeInTheDocument();
    expect(container.querySelector(".vs-review")).toBeInTheDocument();
    expect(container.querySelector(".vs-preview")).toBeInTheDocument();
  });

  it("compares two selected versions in the side panel", async () => {
    const user = userEvent.setup();
    render(
      <I18nProvider>
        <VersionSheet nodeId="scene-1" onClose={vi.fn()} onRestored={vi.fn()} />
      </I18nProvider>,
    );

    await screen.findAllByRole("button", { name: /비교 A/ });

    await user.click(screen.getAllByRole("button", { name: /비교 A/ })[0]);
    await user.click(screen.getAllByRole("button", { name: /비교 B/ })[1]);
    await user.click(screen.getByRole("tab", { name: "비교" }));

    await waitFor(() => {
      expect(snapshots.compare).toHaveBeenCalledWith("old", "new");
    });
    expect(await screen.findByText("- 삭제될 문장")).toBeInTheDocument();
    expect(screen.getByText("+ 새 문장")).toBeInTheDocument();
  });
});
