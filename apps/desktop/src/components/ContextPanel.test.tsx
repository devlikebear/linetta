import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { NodeRow, Project } from "../lib/types";
import { projects } from "../lib/rpc";
import { ContextPanel } from "./ContextPanel";

vi.mock("../lib/rpc", () => ({
  projects: {
    update: vi.fn(),
    rewriteSynopsis: vi.fn(),
    clearSynopsis: vi.fn(),
  },
}));

vi.mock("./PlotPanel", () => ({
  PlotPanel: () => <div data-testid="plot-panel" />,
}));

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
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

describe("ContextPanel", () => {
  it("shows and saves project overview and synopsis as separate fields", async () => {
    vi.useFakeTimers();
    const onProjectChanged = vi.fn();
    vi.mocked(projects.update)
      .mockResolvedValueOnce({ ...project, outline: "새 개요", synopsis: "기존 시놉시스" })
      .mockResolvedValueOnce({ ...project, outline: "새 개요", synopsis: "새 시놉시스" });

    render(
      <ContextPanel
        project={{ ...project, outline: "기존 개요", synopsis: "기존 시놉시스" }}
        node={node}
        charCount={0}
        typewriter={false}
        onToggleTypewriter={vi.fn()}
        saveStatus={{ kind: "idle" }}
        mentionedEntities={[]}
        onMentionClick={vi.fn()}
        onOpenThread={vi.fn()}
        onProjectChanged={onProjectChanged}
      />,
    );

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

    render(
      <ContextPanel
        project={{ ...project, synopsis: "기존 시놉시스" }}
        node={node}
        charCount={0}
        typewriter={false}
        onToggleTypewriter={vi.fn()}
        saveStatus={{ kind: "idle" }}
        mentionedEntities={[]}
        onMentionClick={vi.fn()}
        onOpenThread={vi.fn()}
        onProjectChanged={onProjectChanged}
      />,
    );

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
    render(
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
        onProjectTitleChange={onProjectTitleChange}
      />,
    );

    const input = screen.getByLabelText("소설 제목");
    await user.clear(input);
    await user.type(input, "바뀐 제목");
    await user.tab();

    await waitFor(() => expect(onProjectTitleChange).toHaveBeenCalledWith("바뀐 제목"));
  });

  it("triggers scene mention scanning from the mentioned section", async () => {
    const user = userEvent.setup();
    const onAutoMention = vi.fn();
    render(
      <ContextPanel
        project={project}
        node={node}
        charCount={0}
        typewriter={false}
        onToggleTypewriter={vi.fn()}
        saveStatus={{ kind: "idle" }}
        mentionedEntities={[]}
        onMentionClick={vi.fn()}
        onAutoMention={onAutoMention}
        onOpenThread={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /씬 스캔/ }));
    expect(onAutoMention).toHaveBeenCalledOnce();
  });
});
