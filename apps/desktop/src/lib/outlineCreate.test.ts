import { describe, expect, it } from "vitest";
import { buildTree } from "../hooks/useFirstLeaf";
import type { NodeRow } from "./types";
import { OUTLINE_PRESETS } from "./outlineRepair";
import { planChapterCreation } from "./outlineCreate";

function row(id: string, kind: NodeRow["kind"], ordinal: number, parent_id?: string): NodeRow {
  return {
    id,
    project_id: "project-1",
    parent_id,
    ordinal,
    kind,
    label: id,
    title: "",
    status: "draft",
    word_count: 0,
    created_at: 1,
    updated_at: 1,
  };
}

const t = (key: string, values?: Record<string, string | number>) => {
  if (key === "workspace.webNovelChapterNumber") return `${values?.number}화`;
  if (key === "workspace.chapterNumber") return `${values?.number}장`;
  if (key === "workspace.sceneNumber") return `씬 ${values?.number}`;
  return key;
};

describe("planChapterCreation", () => {
  it("creates a web novel chapter as a direct leaf episode without seeding a scene", () => {
    const tree = buildTree([
      row("arc", "container", 0),
      row("episode-1", "leaf", 0, "arc"),
      row("episode-2", "container", 1, "arc"),
    ]);

    const plan = planChapterCreation(tree, tree[0].children[0], OUTLINE_PRESETS.webnovel, t);

    expect(plan).toEqual({
      chapter: {
        placement: "child",
        parentId: "arc",
        kind: "leaf",
        label: "3화",
        title: "",
      },
      seedScene: false,
      seedSceneLabel: "씬 1",
    });
  });

  it("keeps the novel chapter path as a container with a seeded scene", () => {
    const tree = buildTree([
      row("part", "container", 0),
      row("chapter-1", "container", 0, "part"),
    ]);

    const plan = planChapterCreation(tree, tree[0], OUTLINE_PRESETS.novel, t);

    expect(plan).toMatchObject({
      chapter: {
        placement: "child",
        parentId: "part",
        kind: "container",
        label: "2장",
      },
      seedScene: true,
      seedSceneLabel: "씬 1",
    });
  });
});
