import { describe, expect, it } from "vitest";
import type { TreeNode } from "../hooks/useFirstLeaf";
import { OUTLINE_PRESETS } from "./outlineRepair";
import { findEpisodeNode } from "./outlineEpisode";

function node(partial: Partial<TreeNode> & Pick<TreeNode, "id" | "kind" | "label">): TreeNode {
  return {
    project_id: "project-1",
    ordinal: 0,
    title: "",
    status: "draft",
    word_count: 0,
    created_at: 1,
    updated_at: 1,
    children: [],
    ...partial,
  };
}

const webnovel = OUTLINE_PRESETS.webnovel;

describe("findEpisodeNode", () => {
  it("returns the parent episode container for a scene inside a 화", () => {
    const scene = node({ id: "scene-1", kind: "leaf", label: "씬 1", parent_id: "ep-1", word_count: 1200 });
    const episode = node({ id: "ep-1", kind: "container", label: "1화", parent_id: "arc-1", children: [scene] });
    const arc = node({ id: "arc-1", kind: "container", label: "1권", children: [episode] });

    expect(findEpisodeNode([arc], "scene-1", webnovel)?.id).toBe("ep-1");
  });

  it("returns the leaf itself for a leaf 화 directly under a 권", () => {
    const episode = node({ id: "ep-leaf", kind: "leaf", label: "1화", parent_id: "arc-1", word_count: 2500 });
    const sibling = node({ id: "ep-leaf-2", kind: "leaf", label: "2화", parent_id: "arc-1", word_count: 4000 });
    const arc = node({ id: "arc-1", kind: "container", label: "1권", children: [episode, sibling] });

    expect(findEpisodeNode([arc], "ep-leaf", webnovel)?.id).toBe("ep-leaf");
  });

  it("falls back to the node itself for a root-level seed scene", () => {
    const seed = node({ id: "seed-1", kind: "leaf", label: "씬 1", word_count: 300 });

    expect(findEpisodeNode([seed], "seed-1", webnovel)?.id).toBe("seed-1");
  });

  it("treats a non-root container without an episode label as the episode for its scenes", () => {
    const scene = node({ id: "scene-1", kind: "leaf", label: "씬 1", parent_id: "mid-1" });
    const mid = node({ id: "mid-1", kind: "container", label: "막간", parent_id: "arc-1", children: [scene] });
    const arc = node({ id: "arc-1", kind: "container", label: "1권", children: [mid] });

    expect(findEpisodeNode([arc], "scene-1", webnovel)?.id).toBe("mid-1");
  });

  it("returns null when the node id is not in the tree", () => {
    const seed = node({ id: "seed-1", kind: "leaf", label: "씬 1" });

    expect(findEpisodeNode([seed], "missing", webnovel)).toBeNull();
  });
});
