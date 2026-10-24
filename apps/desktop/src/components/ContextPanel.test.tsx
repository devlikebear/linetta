import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { NodeRow, Project } from "../lib/types";
import { I18nProvider } from "../lib/i18n";
import { projects } from "../lib/rpc";
import { ContextPanel } from "./ContextPanel";

// The continuity badge (story-assist Phase 2) mounts useEngineEvent for its
// mcp-changed refresh, same as CanonPanel/StyleSection — this stands in for
// Tauri's event bus the same way their test files do.
const events = vi.hoisted(() => ({ listeners: new Map<string, (event: { payload: unknown }) => void>() }));
vi.mock("@tauri-apps/api/event", () => ({
  listen: (name: string, listener: (event: { payload: unknown }) => void) => {
    events.listeners.set(name, listener);
    return Promise.resolve(() => events.listeners.delete(name));
  },
}));

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  projectsUpdate: vi.fn(),
  projectsRewriteSynopsis: vi.fn(),
  projectsClearSynopsis: vi.fn(),
  continuityOpenCounts: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
  projects: {
    update: mocks.projectsUpdate,
    rewriteSynopsis: mocks.projectsRewriteSynopsis,
    clearSynopsis: mocks.projectsClearSynopsis,
  },
  // story-assist Phase 2: the open-observation badge's one RPC call.
  continuity: {
    openCounts: mocks.continuityOpenCounts,
  },
}));

vi.mock("./PlotPanel", () => ({
  PlotPanel: () => <div data-testid="plot-panel" />,
}));

vi.mock("./StatsSection", () => ({
  StatsSection: () => <div data-testid="stats-section" />,
}));

vi.mock("./DirectionSection", () => ({
  DirectionSection: ({
    projectId, nodeId, collapsed, onToggleCollapsed,
  }: { projectId: string; nodeId?: string; collapsed?: boolean; onToggleCollapsed?: (c: boolean) => void }) => (
    <div data-testid="direction-section" data-project-id={projectId} data-node-id={nodeId} data-collapsed={String(collapsed)}>
      <button type="button" onClick={() => onToggleCollapsed?.(!collapsed)}>toggle-direction</button>
    </div>
  ),
}));

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
});

beforeEach(() => {
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
  mocks.continuityOpenCounts.mockResolvedValue({ counts: {} });
});

const project: Project = {
  id: "project-1",
  title: "처음 제목",
  genres: ["판타지"],
  length_target: "novel",
  default_pov: "third_limited",
  style_notes: "",
  outline: "",
  outline_preset: "novel",
  episode_char_target: 5000,
  synopsis: "",
  word_count: 0,
  last_opened_node_id: "scene-1",
  created_at: 1,
  updated_at: 1,
};

const node: NodeRow = {
  id: "scene-1",
  project_id: "project-1",
  ordinal: 0,
  kind: "leaf",
  label: "씬 1",
  title: "",
  status: "draft",
  word_count: 0,
  created_at: 1,
  updated_at: 1,
};

function renderContextPanel(props: Partial<ComponentProps<typeof ContextPanel>> = {}) {
  return render(
    <I18nProvider>
      <ContextPanel
        project={project}
        node={node}
        charCount={0}
        typewriter={false}
        onToggleTypewriter={vi.fn()}
        saveStatus={{ kind: "idle" }}
        mentionedEntities={[]}
        onMentionClick={vi.fn()}
        onOpenThread={vi.fn()}
        {...props}
      />
    </I18nProvider>,
  );
}

describe("ContextPanel", () => {
  it("places the writer's direction section at the top of the panel, wired to the open project and scene", async () => {
    renderContextPanel();
    const section = await screen.findByTestId("direction-section");
    expect(section).toHaveAttribute("data-project-id", "project-1");
    expect(section).toHaveAttribute("data-node-id", "scene-1");

    const scroll = document.querySelector(".panel-scroll");
    expect(scroll?.firstElementChild).toBe(section);
  });

  it("persists the direction section's collapse state to localStorage and restores it on remount", async () => {
    window.localStorage.removeItem("linetta:direction:collapsed");
    const { unmount } = renderContextPanel();
    const section = await screen.findByTestId("direction-section");
    expect(section).toHaveAttribute("data-collapsed", "false");

    fireEvent.click(screen.getByRole("button", { name: "toggle-direction" }));
    await waitFor(() => expect(window.localStorage.getItem("linetta:direction:collapsed")).toBe("1"));
    unmount();

    renderContextPanel();
    expect(await screen.findByTestId("direction-section")).toHaveAttribute("data-collapsed", "true");
    window.localStorage.removeItem("linetta:direction:collapsed");
  });

  it("shows and saves project overview and synopsis as separate fields", async () => {
    vi.useFakeTimers();
    const onProjectChanged = vi.fn();
    vi.mocked(projects.update)
      .mockResolvedValueOnce({ ...project, outline: "새 개요", synopsis: "기존 시놉시스" })
      .mockResolvedValueOnce({ ...project, outline: "새 개요", synopsis: "새 시놉시스" });

    renderContextPanel({
      project: { ...project, outline: "기존 개요", synopsis: "기존 시놉시스" },
      onProjectChanged,
    });

    const overview = screen.getByLabelText("작품 개요");
    const synopsis = screen.getByLabelText("작품 시놉시스");
    expect(overview).toHaveValue("기존 개요");
    expect(synopsis).toHaveValue("기존 시놉시스");

    fireEvent.change(overview, { target: { value: "새 개요" } });
    await act(async () => {
      vi.advanceTimersByTime(650);
      await Promise.resolve();
    });
    expect(projects.update).toHaveBeenCalledWith({ id: "project-1", outline: "새 개요" });

    fireEvent.change(synopsis, { target: { value: "새 시놉시스" } });
    await act(async () => {
      vi.advanceTimersByTime(650);
      await Promise.resolve();
    });
    expect(projects.update).toHaveBeenCalledWith({ id: "project-1", synopsis: "새 시놉시스" });
    expect(onProjectChanged).toHaveBeenLastCalledWith({ ...project, outline: "새 개요", synopsis: "새 시놉시스" });
  });

  it("clears the project synopsis from the sidebar", async () => {
    const user = userEvent.setup();
    const onProjectChanged = vi.fn();
    vi.mocked(projects.clearSynopsis).mockResolvedValue({ ...project, synopsis: "" });

    renderContextPanel({
      project: { ...project, synopsis: "기존 시놉시스" },
      onProjectChanged,
    });

    // Deriving a synopsis from scene summaries needed a model, so that button
    // is gone. Clearing never did, and the field is still typed into by hand.
    expect(screen.queryByRole("button", { name: "재작성" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "클리어" }));
    await waitFor(() => {
      expect(projects.clearSynopsis).toHaveBeenCalledWith("project-1");
    });
    expect(screen.getByLabelText("작품 시놉시스")).toHaveValue("");
  });

  it("lets web novel projects edit the episode character target", async () => {
    const user = userEvent.setup();
    const onProjectChanged = vi.fn();
    vi.mocked(projects.update).mockResolvedValue({ ...project, outline_preset: "webnovel", episode_char_target: 5500 });

    renderContextPanel({
      project: { ...project, outline_preset: "webnovel", episode_char_target: 5000 },
      onProjectChanged,
    });

    const target = screen.getByLabelText("회차 목표");
    expect(target).toHaveValue("5000");

    await user.clear(target);
    await user.type(target, "5500");
    await user.tab();

    await waitFor(() => {
      expect(projects.update).toHaveBeenCalledWith({ id: "project-1", episode_char_target: 5500 });
    });
    expect(onProjectChanged).toHaveBeenCalledWith({ ...project, outline_preset: "webnovel", episode_char_target: 5500 });
  });

  it("shows today's writing progress when loaded", async () => {
    renderContextPanel({ todayChars: 1234 });

    expect(await screen.findByText("오늘 1,234자")).toBeInTheDocument();
  });

  it("shows web novel episode stock counts", async () => {
    renderContextPanel({
      project: { ...project, outline_preset: "webnovel" },
      episodeStock: { published: 1, stock: 2 },
    });

    expect(await screen.findByText("발행 1화 · 비축 2화")).toBeInTheDocument();
  });

  it("lets the project title be edited directly", async () => {
    const user = userEvent.setup();
    const onProjectTitleChange = vi.fn().mockResolvedValue(undefined);
    renderContextPanel({ onProjectTitleChange });

    const input = screen.getByLabelText("소설 제목");
    await user.clear(input);
    await user.type(input, "바뀐 제목");
    await user.tab();

    await waitFor(() => expect(onProjectTitleChange).toHaveBeenCalledWith("바뀐 제목"));
  });

  it("triggers scene mention scanning from the mentioned section", async () => {
    const user = userEvent.setup();
    const onAutoMention = vi.fn();
    renderContextPanel({ onAutoMention });

    await user.click(screen.getByRole("button", { name: /씬 스캔/ }));
    expect(onAutoMention).toHaveBeenCalledOnce();
  });

  it("renders visible writing sidebar labels in English when selected", async () => {
    mocks.settingsGet.mockResolvedValue({ language: "en" });

    renderContextPanel({ project: { ...project, outline: "", synopsis: "" }, charCount: 24, onAutoMention: vi.fn() });

    expect(await screen.findByText("This scene")).toBeInTheDocument();
    expect(screen.getByText("Typewriter mode")).toBeInTheDocument();
    expect(screen.getByLabelText("Project overview")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Logline, theme, main flow")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Scan scene/ })).toBeInTheDocument();
  });

  // story-assist Phase 2, 2.D.2: the badge is this scene's own share of a
  // work-wide map, and it is a button only when the caller wired somewhere
  // for it to open.
  it("shows the open-observation badge for this scene and opens continuity on click", async () => {
    mocks.continuityOpenCounts.mockResolvedValue({ counts: { [node.id]: 2, other: 5 } });
    const onOpenContinuity = vi.fn();
    renderContextPanel({ onOpenContinuity });

    const badge = await screen.findByTestId("continuity-open-badge");
    expect(badge).toHaveTextContent("2");
    await userEvent.click(badge);
    expect(onOpenContinuity).toHaveBeenCalledTimes(1);
  });

  it("shows no badge when this scene has no open observations", async () => {
    mocks.continuityOpenCounts.mockResolvedValue({ counts: { other: 5 } });
    renderContextPanel({ onOpenContinuity: vi.fn() });

    await screen.findByText("이 씬");
    expect(screen.queryByTestId("continuity-open-badge")).not.toBeInTheDocument();
  });

  it("shows no badge when the caller has not wired onOpenContinuity", async () => {
    mocks.continuityOpenCounts.mockResolvedValue({ counts: { [node.id]: 3 } });
    renderContextPanel();

    await waitFor(() => expect(mocks.continuityOpenCounts).toHaveBeenCalled());
    expect(screen.queryByTestId("continuity-open-badge")).not.toBeInTheDocument();
  });
});
