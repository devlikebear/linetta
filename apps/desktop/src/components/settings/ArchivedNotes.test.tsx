import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ list: vi.fn(), restore: vi.fn(), remove: vi.fn() }));

vi.mock("@tauri-apps/api/event", () => ({
  listen: () => Promise.resolve(() => undefined),
}));

vi.mock("../../lib/rpc", () => ({
  memory: { archive: { list: rpc.list, restore: rpc.restore, remove: rpc.remove } },
}));

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
}));

import { ArchivedNotes } from "./ArchivedNotes";

const line = { id: "a1", project_id: "w1", body: "곰배는 조연", archived_at: 1 };

describe("ArchivedNotes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.list.mockResolvedValue([line]);
  });

  it("stays out of the way until a work has archived lines", async () => {
    rpc.list.mockResolvedValue([]);
    render(<ArchivedNotes workId="w1" />);
    await waitFor(() => expect(rpc.list).toHaveBeenCalledWith("w1"));
    expect(screen.queryByTestId("memory-archive")).toBeNull();
  });

  it("restores a line and lists again", async () => {
    rpc.restore.mockResolvedValue({});
    render(<ArchivedNotes workId="w1" />);
    await screen.findByText("곰배는 조연");
    rpc.list.mockResolvedValue([]);
    await userEvent.click(screen.getByText("settings.memory.archive.restore"));
    expect(rpc.restore).toHaveBeenCalledWith("w1", "a1");
    await waitFor(() => expect(screen.queryByText("곰배는 조연")).toBeNull());
  });

  it("deletes a line", async () => {
    rpc.remove.mockResolvedValue({});
    render(<ArchivedNotes workId="w1" />);
    await screen.findByText("곰배는 조연");
    await userEvent.click(screen.getByText("settings.memory.archive.delete"));
    expect(rpc.remove).toHaveBeenCalledWith("w1", "a1");
  });

  it("keeps the line and says why when the notes are too full to take it back", async () => {
    rpc.restore.mockRejectedValue(new Error("over budget"));
    render(<ArchivedNotes workId="w1" />);
    await screen.findByText("곰배는 조연");
    await userEvent.click(screen.getByText("settings.memory.archive.restore"));
    await screen.findByTestId("memory-archive-error");
    expect(screen.getByText("곰배는 조연")).toBeTruthy();
  });

  it("drops a slow answer for the work the writer left", async () => {
    let answer: (v: unknown) => void = () => undefined;
    rpc.list.mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    rpc.list.mockResolvedValueOnce([{ ...line, id: "b1", body: "둘째 작품" }]);
    const { rerender } = render(<ArchivedNotes workId="w1" />);
    rerender(<ArchivedNotes workId="w2" />);
    await screen.findByText("둘째 작품");
    answer([line]);
    await Promise.resolve();
    expect(screen.queryByText("곰배는 조연")).toBeNull();
  });
});
