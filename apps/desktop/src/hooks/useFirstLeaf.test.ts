import { describe, expect, it } from "vitest";
import type { NodeRow } from "../lib/types";
import { buildTree, countEpisodeStatus, findFirstLeaf, flatten, leafNeighbors, sumLeafChars } from "./useFirstLeaf";

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

function rowWithCount(id: string, kind: NodeRow["kind"], ordinal: number, word_count: number, parent_id?: string): NodeRow {
  return {
    ...row(id, kind, ordinal, parent_id),
    word_count,
  };
}

function rowWithStatus(id: string, kind: NodeRow["kind"], ordinal: number, status: NodeRow["status"], parent_id?: string): NodeRow {
  return {
    ...row(id, kind, ordinal, parent_id),
    status,
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

  it("sums a single leaf character count", () => {
    const tree = buildTree([rowWithCount("scene-a", "leaf", 0, 321)]);

    expect(sumLeafChars(tree[0])).toBe(321);
  });

  it("sums leaf character counts through nested containers", () => {
    const tree = buildTree([
      row("part", "container", 0),
      rowWithCount("scene-a", "leaf", 0, 1200, "part"),
      row("chapter", "container", 1, "part"),
      rowWithCount("scene-b", "leaf", 0, 800, "chapter"),
      rowWithCount("scene-c", "leaf", 1, 50, "chapter"),
    ]);

    expect(sumLeafChars(tree[0])).toBe(2050);
  });

  it("returns zero for an empty container", () => {
    const tree = buildTree([row("chapter", "container", 0)]);

    expect(sumLeafChars(tree[0])).toBe(0);
  });

  it("counts published and stock episodes from direct children", () => {
    const tree = buildTree([
      row("arc", "container", 0),
      rowWithStatus("episode-published", "container", 0, "published", "arc"),
      rowWithStatus("episode-final", "container", 1, "final", "arc"),
      rowWithStatus("episode-leaf-final", "leaf", 2, "final", "arc"),
      rowWithStatus("episode-draft", "leaf", 3, "draft", "arc"),
      row("scene-under-episode", "leaf", 0, "episode-final"),
    ]);

    expect(countEpisodeStatus(tree)).toEqual({ published: 1, stock: 2 });
  });
});
