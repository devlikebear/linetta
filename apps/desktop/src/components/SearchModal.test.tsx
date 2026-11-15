import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import type { SearchResult } from "../lib/types";
import { SearchModal } from "./SearchModal";

const mocks = vi.hoisted(() => ({
  searchQuery: vi.fn(),
  settingsGet: vi.fn(),
  manuscriptSearch: vi.fn(),
  semanticStatus: vi.fn(),
  semanticEnable: vi.fn(),
}));

vi.mock("../hooks/useEngineEvent", () => ({ useEngineEvent: () => {} }));
vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
  search: {
    query: mocks.searchQuery,
  },
  manuscript: { search: mocks.manuscriptSearch },
  semanticSearch: { status: mocks.semanticStatus, enable: mocks.semanticEnable, disable: vi.fn() },
}));

const model = { installed: true, downloading: false, received: 0, total: 141 * 1024 * 1024 };

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

describe("SearchModal by meaning (#384)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    mocks.searchQuery.mockResolvedValue([]);
  });

  function renderInWork(onReveal = vi.fn()) {
    render(
      <I18nProvider>
        <SearchModal open onClose={vi.fn()} onSelect={vi.fn()} projectId="p1" onReveal={onReveal} />
      </I18nProvider>,
    );
    return { onReveal };
  }

  it("finds a paragraph by meaning and opens it there", async () => {
    const user = userEvent.setup();
    mocks.semanticStatus.mockResolvedValue({ supported: true, enabled: true, model, index: { project_id: "p1", done: 10, total: 10 } });
    mocks.manuscriptSearch.mockResolvedValue([
      { node_id: "s2", breadcrumb: "12화 › 씬 2", snippet: "건반 위에 올려진 누나의 손가락이…", paragraph_from: 4, paragraph_to: 4 },
    ]);
    const { onReveal } = renderInWork();
    await user.click(screen.getByTestId("search-mode-meaning"));
    await user.type(screen.getByPlaceholderText("작품 전체 검색…"), "누나가 피아노 치던 기억");
    expect(await screen.findByText("건반 위에 올려진 누나의 손가락이…", {}, { timeout: 2000 })).toBeInTheDocument();
    expect(mocks.manuscriptSearch).toHaveBeenLastCalledWith("p1", "누나가 피아노 치던 기억", 20, "meaning");
    expect(screen.getByText("¶4")).toBeInTheDocument();
    await user.keyboard("{Enter}");
    expect(onReveal).toHaveBeenCalledWith({ nodeId: "s2", paragraphFrom: 4, paragraphTo: 4 });
    expect(window.localStorage.getItem("linetta.search.mode")).toBe("meaning");
  });

  it("offers the model download before meaning search is on, and says how far the index is", async () => {
    const user = userEvent.setup();
    mocks.semanticStatus.mockResolvedValue({ supported: true, enabled: false, model: { ...model, installed: false } });
    mocks.semanticEnable.mockResolvedValue({ supported: true, enabled: true, model: { ...model, installed: false, downloading: true, received: 0 } });
    renderInWork();
    await user.click(screen.getByTestId("search-mode-meaning"));
    const offer = await screen.findByTestId("search-meaning-offer");
    expect(offer.textContent).toContain("141MB");
    await user.click(screen.getByRole("button", { name: "받기" }));
    expect(mocks.semanticEnable).toHaveBeenCalled();
    expect(mocks.manuscriptSearch).not.toHaveBeenCalled();
  });

  it("stays a word search without an open work", async () => {
    render(
      <I18nProvider>
        <SearchModal open onClose={vi.fn()} onSelect={vi.fn()} />
      </I18nProvider>,
    );
    await screen.findByPlaceholderText("작품 전체 검색…");
    expect(screen.queryByTestId("search-mode-meaning")).toBeNull();
  });
});
