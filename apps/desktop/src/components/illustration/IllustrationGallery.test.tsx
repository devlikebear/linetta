import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  illustrations: { list: rpc.list, get: rpc.get, update: rpc.update, delete: rpc.remove },
}));

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    language: "ko",
    t: (key: string, vars?: Record<string, string | number>) =>
      vars
        ? `${key}:${Object.entries(vars)
            .map(([name, value]) => `${name}=${value}`)
            .join(",")}`
        : key,
  }),
}));

import { IllustrationGallery } from "./IllustrationGallery";

const meta = (id: string, extra: Record<string, unknown> = {}) => ({
  id,
  project_id: "p",
  caption: "",
  prompt: `prompt ${id}`,
  provider: "gemini-native",
  model: "gemini-3.1-flash-image",
  mime: "image/jpeg",
  width: 848,
  height: 1264,
  byte_size: 900_000,
  created_at: 1,
  ...extra,
});

describe("IllustrationGallery", () => {
  // jsdom has neither; they stay stubbed for the file, since the cleanup that
  // unmounts each test's thumbnails runs after any afterEach here.
  URL.createObjectURL = vi.fn(() => "blob:img");
  URL.revokeObjectURL = vi.fn();
  beforeEach(() => {
    Object.values(rpc).forEach((f) => f.mockReset());
    rpc.get.mockImplementation((id: string) => Promise.resolve({ ...meta(id), data_base64: btoa("jpg") }));
  });

  it("shows the gallery with what it costs, and says so when empty", async () => {
    rpc.list.mockResolvedValue({ items: [meta("a", { caption: "우산" }), meta("b")], count: 2, bytes: 1_800_000 });
    render(<IllustrationGallery projectId="p" onClose={() => {}} onNew={() => {}} />);
    expect(await screen.findByTestId("illustration-thumb-a")).toHaveTextContent("우산");
    expect(screen.getByTestId("illustration-usage")).toHaveTextContent("count=2,size=1.7 MB");
    await waitFor(() => expect(screen.getByTestId("illustration-thumb-a").querySelector("img")).not.toBeNull());

    rpc.list.mockResolvedValue({ items: [], count: 0, bytes: 0 });
    render(<IllustrationGallery projectId="q" onClose={() => {}} onNew={() => {}} />);
    expect(await screen.findByTestId("illustration-empty")).toBeInTheDocument();
  });

  it("opens one picture with its prompt, renames it, and deletes it on the second press", async () => {
    rpc.list.mockResolvedValueOnce({ items: [meta("a")], count: 1, bytes: 900_000 }).mockResolvedValue({ items: [], count: 0, bytes: 0 });
    rpc.update.mockResolvedValue(meta("a", { caption: "표지" }));
    rpc.remove.mockResolvedValue({ ok: true });
    render(<IllustrationGallery projectId="p" onClose={() => {}} onNew={() => {}} />);

    fireEvent.click(await screen.findByTestId("illustration-thumb-a"));
    expect(screen.getByTestId("illustration-detail")).toHaveTextContent("prompt a");

    const caption = screen.getByTestId("illustration-caption");
    fireEvent.change(caption, { target: { value: "표지" } });
    fireEvent.blur(caption);
    await waitFor(() => expect(rpc.update).toHaveBeenCalledWith("a", "표지"));

    fireEvent.click(screen.getByTestId("illustration-delete"));
    expect(rpc.remove).not.toHaveBeenCalled();
    expect(screen.getByTestId("illustration-delete")).toHaveTextContent("illustration.deleteConfirm");
    fireEvent.click(screen.getByTestId("illustration-delete"));
    await waitFor(() => expect(rpc.remove).toHaveBeenCalledWith("a"));
    expect(await screen.findByTestId("illustration-empty")).toBeInTheDocument();
  });

  it("asks the caller for a new drawing", async () => {
    rpc.list.mockResolvedValue({ items: [], count: 0, bytes: 0 });
    const onNew = vi.fn();
    render(<IllustrationGallery projectId="p" onClose={() => {}} onNew={onNew} />);
    fireEvent.click(await screen.findByTestId("illustration-new"));
    expect(onNew).toHaveBeenCalled();
  });
});

describe("IllustrationGallery and scenes", () => {
  URL.createObjectURL = vi.fn(() => "blob:img");
  URL.revokeObjectURL = vi.fn();
  beforeEach(() => {
    Object.values(rpc).forEach((f) => f.mockReset());
    rpc.get.mockImplementation((id: string) => Promise.resolve({ ...meta(id), data_base64: btoa("jpg") }));
  });

  it("marks a picture in use, names its scenes, and inserts it with the edited caption", async () => {
    rpc.list.mockResolvedValue({
      items: [meta("a", { caption: "우산", used_in: [{ node_id: "n1", label: "3화", title: "비" }] })],
      count: 1,
      bytes: 900_000,
    });
    rpc.update.mockResolvedValue(meta("a", { caption: "표지" }));
    const onInsert = vi.fn();
    render(<IllustrationGallery projectId="p" onClose={() => {}} onNew={() => {}} onInsert={onInsert} />);

    expect(await screen.findByTestId("illustration-thumb-a")).toHaveTextContent("illustration.inUse");
    fireEvent.click(screen.getByTestId("illustration-thumb-a"));
    expect(screen.getByTestId("illustration-used-in")).toHaveTextContent("3화 · 비");

    fireEvent.change(screen.getByTestId("illustration-caption"), { target: { value: " 표지 " } });
    fireEvent.click(screen.getByTestId("illustration-insert"));
    expect(onInsert).toHaveBeenCalledWith(expect.objectContaining({ id: "a", caption: "표지" }));
  });

  it("cannot insert with no scene open, and says why a delete was refused", async () => {
    rpc.list.mockResolvedValue({ items: [meta("a")], count: 1, bytes: 1 });
    rpc.remove.mockRejectedValue({ code: -32602, message: "in use", data: { reason: "illustration_in_use" } });
    render(<IllustrationGallery projectId="p" onClose={() => {}} onNew={() => {}} />);
    fireEvent.click(await screen.findByTestId("illustration-thumb-a"));
    expect(screen.getByTestId("illustration-insert")).toBeDisabled();
    fireEvent.click(screen.getByTestId("illustration-delete"));
    fireEvent.click(screen.getByTestId("illustration-delete"));
    expect(await screen.findByText("errors.illustrationInUse")).toBeInTheDocument();
    expect(screen.getByTestId("illustration-delete")).toHaveTextContent("illustration.delete");
  });
});
