import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import type { SearchResult } from "../lib/types";
import { SearchModal } from "./SearchModal";

const mocks = vi.hoisted(() => ({
  searchQuery: vi.fn(),
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
  search: {
    query: mocks.searchQuery,
  },
}));

function renderSearchModal(onSelect = vi.fn()) {
  render(
    <I18nProvider>
      <SearchModal open onClose={vi.fn()} onSelect={onSelect} />
    </I18nProvider>,
  );
  return { onSelect };
}

describe("SearchModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    mocks.searchQuery.mockResolvedValue([
      {
        project_id: "project-1",
        project_title: "도시의 밤",
        node_id: "node-1",
        node_label: "씬 1",
        node_title: "열쇠",
        node_kind: "leaf",
        preview: "숨은 열쇠를 발견했다.",
        updated_at: 1,
      } satisfies SearchResult,
    ]);
  });

  it("queries and selects a result", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    renderSearchModal(onSelect);

    await user.type(screen.getByPlaceholderText("작품 전체 검색…"), "열쇠");

    await waitFor(() => expect(mocks.searchQuery).toHaveBeenCalledWith("열쇠", 20));
    await user.click(await screen.findByRole("button", { name: /도시의 밤/ }));

    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ node_id: "node-1" }));
  });

  it("renders search chrome in English when selected", async () => {
    mocks.settingsGet.mockResolvedValue({ language: "en" });
    mocks.searchQuery.mockResolvedValue([]);

    renderSearchModal();

    expect(await screen.findByPlaceholderText("Search entire work...")).toBeInTheDocument();
    expect(screen.getByText("Enter a word to search")).toBeInTheDocument();
    expect(screen.getByText("Search titles, scenes, and body")).toBeInTheDocument();
  });
});
