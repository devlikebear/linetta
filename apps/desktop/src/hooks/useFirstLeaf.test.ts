import { describe, expect, it } from "vitest";
import type { NodeRow } from "../lib/types";
import { buildTree, findFirstLeaf, flatten, leafNeighbors } from "./useFirstLeaf";

function row(id: string, kind: NodeRow["kind"], ordinal: number, parent_id?: string): NodeRow {
  return {
    id,
    project_id: "p1",
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

describe("tree helpers", () => {
  it("builds roots, descendants, and preserves DFS order", () => {
    const tree = buildTree([
      row("part", "container", 0),
      row("scene-a", "leaf", 0, "part"),
      row("chapter", "container", 1, "part"),
      row("scene-b", "leaf", 0, "chapter"),
      row("scene-c", "leaf", 1),
    ]);

    expect(tree).toHaveLength(2);
    expect(findFirstLeaf(tree[0])?.id).toBe("scene-a");
    expect(flatten(tree).map((n) => n.id)).toEqual(["part", "scene-a", "chapter", "scene-b", "scene-c"]);
  });

  it("returns adjacent leaves across nested containers", () => {
    const tree = buildTree([
      row("part", "container", 0),
      row("scene-a", "leaf", 0, "part"),
      row("chapter", "container", 1, "part"),
      row("scene-b", "leaf", 0, "chapter"),
      row("scene-c", "leaf", 1),
    ]);

    expect(leafNeighbors(tree, "scene-b")).toMatchObject({
      prev: expect.objectContaining({ id: "scene-a" }),
      next: expect.objectContaining({ id: "scene-c" }),
    });
    expect(leafNeighbors(tree, "missing")).toEqual({ prev: null, next: null });
  });
});
