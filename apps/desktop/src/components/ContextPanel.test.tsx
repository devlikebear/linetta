import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { NodeRow, Project } from "../lib/types";
import { ContextPanel } from "./ContextPanel";

vi.mock("./PlotPanel", () => ({
  PlotPanel: () => <div data-testid="plot-panel" />,
}));

const project: Project = {
  id: "project-1",
  title: "처음 제목",
  genres: ["판타지"],
  length_target: "novel",
  default_pov: "third_limited",
  style_notes: "",
  outline: "",
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
});
