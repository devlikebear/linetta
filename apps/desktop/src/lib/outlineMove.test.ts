import { describe, expect, it } from "vitest";
import { buildTree } from "../hooks/useFirstLeaf";
import type { NodeRow } from "./types";
import { planNodeMove } from "./outlineMove";

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

describe("planNodeMove", () => {
  it("plans same-parent reordering before a sibling", () => {
    const tree = buildTree([
      row("scene-1", "leaf", 0),
      row("scene-2", "leaf", 1),
      row("scene-3", "leaf", 2),
    ]);

    expect(planNodeMove(tree, "scene-3", "scene-1", "before")).toEqual({
      parentId: null,
      ordinal: 0,
    });
  });

  it("plans moving a node into a container as its last child", () => {
    const tree = buildTree([
      row("scene-1", "leaf", 0),
      row("part", "container", 1),
      row("scene-a", "leaf", 0, "part"),
    ]);

    expect(planNodeMove(tree, "scene-1", "part", "inside")).toEqual({
      parentId: "part",
      ordinal: 1,
    });
  });

  it("rejects moving a container into its own descendant", () => {
    const tree = buildTree([
      row("part", "container", 0),
      row("chapter", "container", 0, "part"),
    ]);

    expect(planNodeMove(tree, "part", "chapter", "inside")).toBeNull();
  });
});
