import type { TreeNode } from "../hooks/useFirstLeaf";
import { flatten } from "../hooks/useFirstLeaf";
import { isStructuralChapterLabel, type OutlineStructurePreset } from "./outlineRepair";

/** Resolve the episode-level node whose chars make up "이번 화" for the given
 *  node. Walks from the node upward and returns the first ancestor (self
 *  included) that is episode-like:
 *  - its label matches the preset's chapter/episode pattern (covers both
 *    leaf 화 directly under a 권 and container 화 holding scenes), or
 *  - it is a non-root container (matches the outline rail's gauge heuristic
 *    for unlabeled mid-level containers).
 *  Falls back to the node itself (e.g. the root seed scene), or null when the
 *  id is not in the tree. */
export function findEpisodeNode(
  tree: TreeNode[],
  nodeId: string,
  preset: OutlineStructurePreset,
): TreeNode | null {
  const byId = new Map(flatten(tree).map((n) => [n.id, n] as const));
  const start = byId.get(nodeId);
  if (!start) return null;
  let cur: TreeNode | undefined = start;
  while (cur) {
    if (isStructuralChapterLabel(cur.label, preset)) return cur;
    if (cur.kind === "container" && cur.parent_id) return cur;
    cur = cur.parent_id ? byId.get(cur.parent_id) : undefined;
  }
  return start;
}
