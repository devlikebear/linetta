import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { NodeRow, Project } from "../lib/types";
import { I18nProvider } from "../lib/i18n";
import { projects } from "../lib/rpc";
import { ContextPanel } from "./ContextPanel";

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  projectsUpdate: vi.fn(),
  projectsRewriteSynopsis: vi.fn(),
  projectsClearSynopsis: vi.fn(),
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
}));

vi.mock("./PlotPanel", () => ({
  PlotPanel: () => <div data-testid="plot-panel" />,
}));

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
});

beforeEach(() => {
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
});

const project: Project = {
  id: "project-1",
  title: "처음 제목",
  genres: ["판타지"],
  length_target: "novel",
  default_pov: "third_limited",
  style_notes: "",
  outline: "",
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

  it("rewrites and clears the project synopsis from the sidebar", async () => {
    const user = userEvent.setup();
    const onProjectChanged = vi.fn();
    vi.mocked(projects.rewriteSynopsis).mockResolvedValue({ ...project, synopsis: "재작성된 시놉시스" });
    vi.mocked(projects.clearSynopsis).mockResolvedValue({ ...project, synopsis: "" });

    renderContextPanel({
      project: { ...project, synopsis: "기존 시놉시스" },
      onProjectChanged,
    });

    await user.click(screen.getByRole("button", { name: "재작성" }));
    await waitFor(() => {
      expect(projects.rewriteSynopsis).toHaveBeenCalledWith("project-1");
    });
    expect(screen.getByLabelText("작품 시놉시스")).toHaveValue("재작성된 시놉시스");

    await user.click(screen.getByRole("button", { name: "클리어" }));
    await waitFor(() => {
      expect(projects.clearSynopsis).toHaveBeenCalledWith("project-1");
    });
    expect(screen.getByLabelText("작품 시놉시스")).toHaveValue("");
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
});
