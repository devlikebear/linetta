import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SearchResult } from "../lib/types";
import { SearchModal } from "./SearchModal";

const mocks = vi.hoisted(() => ({
  searchQuery: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  search: {
    query: mocks.searchQuery,
  },
}));

describe("SearchModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
    render(<SearchModal open onClose={vi.fn()} onSelect={onSelect} />);

    await user.type(screen.getByPlaceholderText("작품, 씬, 본문 검색"), "열쇠");

    await waitFor(() => expect(mocks.searchQuery).toHaveBeenCalledWith("열쇠", 20));
    await user.click(await screen.findByRole("button", { name: /도시의 밤/ }));

    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ node_id: "node-1" }));
  });
});
