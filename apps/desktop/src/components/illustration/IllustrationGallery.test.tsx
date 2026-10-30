import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  representatives: vi.fn(),
  setRepresentative: vi.fn(),
  requests: vi.fn(),
  requestGet: vi.fn(),
  providers: vi.fn(),
  node: vi.fn(),
  workStatus: vi.fn(),
  publishCover: vi.fn(),
}));

// The gallery and its request cards listen for illustrations-changed; this
// stands in for Tauri's event bus the way CanonPanel's tests do.
const events = vi.hoisted(() => ({ listeners: new Map<string, (event: { payload: unknown }) => void>() }));
vi.mock("@tauri-apps/api/event", () => ({
  listen: (name: string, listener: (event: { payload: unknown }) => void) => {
    events.listeners.set(name, listener);
    return Promise.resolve(() => events.listeners.delete(name));
  },
}));

vi.mock("../../lib/rpc", () => ({
  illustrations: {
    list: rpc.list,
    get: rpc.get,
    update: rpc.update,
    delete: rpc.remove,
    representatives: rpc.representatives,
    setRepresentative: rpc.setRepresentative,
    suggestPrior: vi.fn().mockResolvedValue([]),
  },
  illustrationRequests: { list: rpc.requests, get: rpc.requestGet, approve: vi.fn(), reject: vi.fn() },
  imageProviders: { list: rpc.providers },
  nodes: { get: rpc.node },
  publishApi: { workStatus: rpc.workStatus, publishCover: rpc.publishCover },
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
    rpc.representatives.mockResolvedValue({});
    rpc.requests.mockResolvedValue([]);
    rpc.providers.mockResolvedValue([]);
    rpc.node.mockResolvedValue({ id: "s1", label: "씬 1", title: "" });
    rpc.workStatus.mockRejectedValue(new Error("not available in this build"));
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
    rpc.representatives.mockResolvedValue({});
    rpc.requests.mockResolvedValue([]);
    rpc.providers.mockResolvedValue([]);
    rpc.node.mockResolvedValue({ id: "s1", label: "씬 1", title: "" });
    rpc.workStatus.mockRejectedValue(new Error("not available in this build"));
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

describe("IllustrationGallery, agents and representatives", () => {
  URL.createObjectURL = vi.fn(() => "blob:img");
  URL.revokeObjectURL = vi.fn();
  beforeEach(() => {
    Object.values(rpc).forEach((f) => f.mockReset());
    rpc.get.mockImplementation((id: string) => Promise.resolve({ ...meta(id), data_base64: btoa("jpg") }));
    rpc.list.mockResolvedValue({ items: [meta("a", { caption: "우산" })], count: 1, bytes: 900_000 });
    rpc.representatives.mockResolvedValue({});
    rpc.requests.mockResolvedValue([]);
    rpc.providers.mockResolvedValue([{ id: "gemini-native", active: true, configured: true, consented: true }]);
    rpc.node.mockResolvedValue({ id: "s1", label: "씬 1", title: "" });
    rpc.workStatus.mockRejectedValue(new Error("not available in this build"));
    rpc.setRepresentative.mockResolvedValue({ previous: "" });
  });

  it("lists the requests still waiting, oldest first, as cards", async () => {
    const request = (id: string, created: number) => ({
      id,
      project_id: "p",
      target_node_id: "s1",
      purpose: "insert",
      prompt: `prompt ${id}`,
      character_ids: [],
      include_references: false,
      aspect: "portrait",
      caption: "",
      note: `note ${id}`,
      source: "external",
      status: "pending",
      created_at: created,
    });
    rpc.requests.mockResolvedValue([request("r2", 2), request("r1", 1)]);
    render(<IllustrationGallery projectId="p" onClose={() => {}} onNew={() => {}} />);
    const section = await screen.findByTestId("illustration-requests");
    const cards = section.querySelectorAll('[data-testid="illustration-request"]');
    expect(cards).toHaveLength(2);
    expect(cards[0]).toHaveTextContent("note r1");
    expect(cards[0]).toHaveTextContent("illustrationRequest.fromExternal");
    expect(rpc.requests).toHaveBeenCalledWith("p", ["pending", "generating", "failed"]);
  });

  it("makes a picture stand for the scene or its episode, and takes it back", async () => {
    const changed = vi.fn();
    render(
      <IllustrationGallery
        projectId="p"
        onClose={() => {}}
        onNew={() => {}}
        targets={[{ id: "s1", label: "씬 1" }, { id: "e1", label: "1화" }]}
        nodeLabel={(id) => (id === "e1" ? "1화" : "씬 1")}
        onRepresentativesChanged={changed}
      />,
    );
    fireEvent.click(await screen.findByTestId("illustration-thumb-a"));
    rpc.representatives.mockResolvedValue({ e1: "a" });
    fireEvent.click(screen.getByTestId("illustration-set-representative-e1"));
    await waitFor(() => expect(rpc.setRepresentative).toHaveBeenCalledWith("e1", "a"));
    await waitFor(() => expect(changed).toHaveBeenCalled());
    expect(await screen.findByTestId("illustration-representative-of")).toHaveTextContent("1화");
    fireEvent.click(screen.getByTestId("illustration-clear-representative-e1"));
    await waitFor(() => expect(rpc.setRepresentative).toHaveBeenLastCalledWith("e1", ""));
    // The scene's button stays a "set" button: it is the episode's picture.
    expect(screen.getByTestId("illustration-set-representative-s1")).toBeInTheDocument();
  });

  it("badges a picture that stands for something", async () => {
    rpc.representatives.mockResolvedValue({ s1: "a" });
    render(<IllustrationGallery projectId="p" onClose={() => {}} onNew={() => {}} />);
    await waitFor(() =>
      expect(screen.getByTestId("illustration-thumb-a")).toHaveTextContent("illustration.representative"),
    );
  });
});

describe("IllustrationGallery and the Works cover", () => {
  URL.createObjectURL = vi.fn(() => "blob:img");
  URL.revokeObjectURL = vi.fn();
  const status = (extra: Record<string, unknown> = {}, sheets: Record<string, unknown> = {}) => ({
    project_id: "p",
    connected: true,
    registered: true,
    episodes: [],
    site_only: [],
    sheets: { chosen: 0, remote: true, sheets: [], ...sheets },
    ...extra,
  });
  beforeEach(() => {
    Object.values(rpc).forEach((f) => f.mockReset());
    rpc.get.mockImplementation((id: string) => Promise.resolve({ ...meta(id), data_base64: btoa("jpg") }));
    rpc.list.mockResolvedValue({ items: [meta("a"), meta("b")], count: 2, bytes: 1_800_000 });
    rpc.representatives.mockResolvedValue({});
    rpc.requests.mockResolvedValue([]);
    rpc.providers.mockResolvedValue([]);
    rpc.node.mockResolvedValue({ id: "s1", label: "씬 1", title: "" });
    rpc.workStatus.mockResolvedValue(status());
  });

  const openPicture = async (id: string) => {
    render(<IllustrationGallery projectId="p" onClose={() => {}} onNew={() => {}} />);
    fireEvent.click(await screen.findByTestId(`illustration-thumb-${id}`));
  };

  it("uploads the open picture as the cover, then badges it and reports the review", async () => {
    rpc.publishCover.mockResolvedValue({ illustration_id: "a", state: "pending" });
    await openPicture("a");
    fireEvent.click(await screen.findByTestId("illustration-publish-cover"));
    await waitFor(() => expect(rpc.publishCover).toHaveBeenCalledWith("p", "a"));
    expect(await screen.findByTestId("illustration-site-cover")).toHaveTextContent("publish.state.pending");
    expect(screen.queryByTestId("illustration-publish-cover")).toBeNull();

    fireEvent.click(screen.getByText("illustration.back"));
    expect(screen.getByTestId("illustration-thumb-a")).toHaveTextContent("illustration.cover");
    expect(screen.getByTestId("illustration-thumb-b")).not.toHaveTextContent("illustration.cover");
  });

  it("offers another picture while the site holds a different cover", async () => {
    rpc.workStatus.mockResolvedValue(status({}, { cover_id: "a", cover: { illustration_id: "a", state: "public" } }));
    await openPicture("b");
    expect(await screen.findByTestId("illustration-publish-cover")).toBeInTheDocument();
    expect(screen.queryByTestId("illustration-site-cover")).toBeNull();
  });

  it("badges a chosen cover without an upload offer on a device that is not connected", async () => {
    rpc.workStatus.mockResolvedValue(status({ connected: false, registered: false }, { cover_id: "a", remote: false }));
    render(<IllustrationGallery projectId="p" onClose={() => {}} onNew={() => {}} />);
    await waitFor(() => expect(screen.getByTestId("illustration-thumb-a")).toHaveTextContent("illustration.cover"));
    fireEvent.click(screen.getByTestId("illustration-thumb-a"));
    expect(screen.queryByTestId("illustration-publish-cover")).toBeNull();
  });

  it("does not offer the upload for a work that is not on the site", async () => {
    rpc.workStatus.mockResolvedValue(status({ registered: false }, { remote: false }));
    await openPicture("a");
    await waitFor(() => expect(rpc.workStatus).toHaveBeenCalled());
    expect(screen.queryByTestId("illustration-publish-cover")).toBeNull();
  });

  it("hides the cover where publishing is not available", async () => {
    rpc.workStatus.mockRejectedValue(new Error("not available in this build"));
    await openPicture("a");
    await waitFor(() => expect(rpc.workStatus).toHaveBeenCalled());
    expect(screen.queryByTestId("illustration-cover-actions")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("says why an upload failed and leaves the button to try again", async () => {
    rpc.publishCover.mockRejectedValue({ code: -32000, message: "too big", data: { reason: "publish_image_too_large" } });
    await openPicture("a");
    fireEvent.click(await screen.findByTestId("illustration-publish-cover"));
    expect(await screen.findByText("errors.publishImageTooLarge")).toBeInTheDocument();
    expect(screen.getByTestId("illustration-publish-cover")).toBeEnabled();
  });

  it("follows an agent uploading the cover", async () => {
    render(<IllustrationGallery projectId="p" onClose={() => {}} onNew={() => {}} />);
    await screen.findByTestId("illustration-thumb-a");
    rpc.workStatus.mockResolvedValue(status({}, { cover_id: "b", cover: { illustration_id: "b", state: "public" } }));
    await waitFor(() => expect(events.listeners.has("mcp-changed")).toBe(true));
    events.listeners.get("mcp-changed")?.({ payload: { tool: "linetta_publish_cover", project_id: "p" } });
    await waitFor(() => expect(screen.getByTestId("illustration-thumb-b")).toHaveTextContent("illustration.cover"));
  });
});
