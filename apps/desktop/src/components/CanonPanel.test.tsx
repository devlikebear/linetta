import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { CanonPanel } from "./CanonPanel";
import type { Entity, EntityKind, Relationship } from "../lib/types";

const events = vi.hoisted(() => ({ listeners: new Map<string, (event: { payload: unknown }) => void>() }));
vi.mock("@tauri-apps/api/event", () => ({
  listen: (name: string, listener: (event: { payload: unknown }) => void) => {
    events.listeners.set(name, listener);
    return Promise.resolve(() => events.listeners.delete(name));
  },
}));

const mocks = vi.hoisted(() => ({
  visuals: {
    getSheet: vi.fn().mockResolvedValue({ entity_id: "entity-1", age_range: "", build: "", hair: "", outfit: "", signature: "", palette: "", notes: "", updated_at: 0 }),
    setSheet: vi.fn().mockResolvedValue({}), listImages: vi.fn().mockResolvedValue([]),
    addImage: vi.fn(), getImage: vi.fn(), deleteImage: vi.fn(),
    getArtStyle: vi.fn().mockResolvedValue({ project_id: "project-1", style: "", negative_prompt: "", updated_at: 0 }),
    setArtStyle: vi.fn(),
  },
  settingsGet: vi.fn(),
  entitiesList: vi.fn(),
  relationshipsList: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  visuals: mocks.visuals,
  // I18nProvider reads the language through the same module.
  settings: { get: mocks.settingsGet },
  entities: { list: mocks.entitiesList },
  relationships: { list: mocks.relationshipsList },
  // The style section mounts inside the panel and loads its rules on mount.
  style: {
    getRules: vi.fn().mockResolvedValue({ project_id: "p1", avoid_phrases: [], max_sentence_chars: 0, updated_at: 0 }),
    setRules: vi.fn(),
    check: vi.fn(),
    getDraft: vi.fn().mockResolvedValue({ pending: false, draft: { project_id: "p1", body: "", author: "", created_at: 0 }, limit: 2200 }),
  },
  projects: { get: vi.fn().mockResolvedValue({ id: "p1", style_notes: "" }) },
}));

function entity(
  id: string,
  name: string,
  kind: EntityKind = "character",
  extra: Partial<Entity> = {},
): Entity {
  return {
    id,
    project_id: "project-1",
    kind,
    name,
    aliases: [],
    role: "",
    summary: "",
    attributes: {},
    created_at: 1,
    updated_at: 1,
    ...extra,
  };
}

function rel(id: string, from: string, to: string): Relationship {
  return { id, project_id: "project-1", from_id: from, to_id: to, label: "친구", notes: "" };
}

const cast = [
  entity("e1", "해윤", "character", { role: "주인공", aliases: ["윤 선생"] }),
  entity("e2", "서린", "character", { summary: "해윤의 옛 동료." }),
  entity("e3", "청운관", "place"),
  entity("e4", "은장도", "item"),
];

function panel(props: Partial<ComponentProps<typeof CanonPanel>> = {}) {
  return (
    <I18nProvider>
      <CanonPanel
        projectId="project-1"
        onOpenEntity={vi.fn()}
        onClose={vi.fn()}
        {...props}
      />
    </I18nProvider>
  );
}

function renderPanel(props: Partial<ComponentProps<typeof CanonPanel>> = {}) {
  return render(panel(props));
}

/** Render, wait for the first load, then type into the search box. */
async function search(term: string) {
  renderPanel();
  await screen.findByText("해윤");
  await userEvent.type(screen.getByRole("searchbox"), term);
}

describe("CanonPanel", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    mocks.entitiesList.mockResolvedValue(cast);
    mocks.relationshipsList.mockResolvedValue([rel("r1", "e1", "e2"), rel("r2", "e2", "e1")]);
  });

  it("saves the work art style", async () => {
    const user = userEvent.setup();
    mocks.visuals.setArtStyle.mockImplementation(async (value) => value);
    renderPanel();
    await user.click(screen.getByText("일러스트 화풍"));
    await user.type(await screen.findByLabelText("화풍"), "수채화");
    await user.type(screen.getByLabelText("네거티브 프롬프트"), "글자");
    await user.click(screen.getByRole("button", { name: "저장" }));
    await waitFor(() => expect(mocks.visuals.setArtStyle).toHaveBeenCalledWith(expect.objectContaining({ project_id: "project-1", style: "수채화", negative_prompt: "글자" })));
  });

  it("lists what the work registered, whatever the kind", async () => {
    renderPanel();
    await screen.findByText("해윤");
    expect(screen.getByText("청운관")).toBeTruthy();
    expect(screen.getByText("은장도")).toBeTruthy();
  });

  it("narrows to one kind when a tab is chosen", async () => {
    renderPanel();
    await screen.findByText("해윤");
    await userEvent.click(screen.getByRole("tab", { name: /장소/ }));
    expect(screen.getByText("청운관")).toBeTruthy();
    expect(screen.queryByText("해윤")).toBeNull();
  });

  it("finds a record by the alias used in the prose, not only its name", async () => {
    // The writer looks for what they typed, which is usually not the canonical
    // name — that is the whole reason aliases exist.
    await search("윤 선생");
    expect(screen.getByText("해윤")).toBeTruthy();
    expect(screen.queryByText("서린")).toBeNull();
  });

  it("searches the summary too", async () => {
    await search("옛 동료");
    expect(screen.getByText("서린")).toBeTruthy();
    expect(screen.queryByText("청운관")).toBeNull();
  });

  it("says so when the search matches nothing, instead of showing an empty box", async () => {
    await search("없는이름");
    expect(screen.getByText(/찾는 이름이 없습니다/)).toBeTruthy();
  });

  it("counts relationships from both ends of the pair", async () => {
    renderPanel();
    await screen.findByText("해윤");
    // r1 and r2 are the two halves of one pair, so each name is in two rows.
    expect(screen.getAllByText("관계 2")).toHaveLength(2);
  });

  it("opens the record the writer clicked", async () => {
    const onOpenEntity = vi.fn();
    renderPanel({ onOpenEntity });
    await screen.findByText("해윤");
    await userEvent.click(screen.getByText("해윤"));
    expect(onOpenEntity).toHaveBeenCalledWith("e1");
  });

  it("tells an empty work where records come from", async () => {
    mocks.entitiesList.mockResolvedValue([]);
    mocks.relationshipsList.mockResolvedValue([]);
    renderPanel();
    expect(await screen.findByText(/아직 등록된 요소가 없습니다/)).toBeTruthy();
  });

  it("shows the failure rather than an empty list that looks like an empty work", async () => {
    mocks.entitiesList.mockRejectedValue(new Error("engine down"));
    renderPanel();
    expect(await screen.findByRole("alert")).toBeTruthy();
  });

  it("refetches when an agent changes the work", async () => {
    // The scenario the issue names: an agent says it added three characters,
    // and the open panel must not keep showing the old cast (#28).
    const { rerender } = renderPanel({ refreshKey: 0 });
    await screen.findByText("해윤");
    expect(mocks.entitiesList).toHaveBeenCalledTimes(1);

    mocks.entitiesList.mockResolvedValue([...cast, entity("e5", "도경")]);
    rerender(panel({ refreshKey: 1 }));
    await waitFor(() => expect(screen.getByText("도경")).toBeTruthy());
  });
});
