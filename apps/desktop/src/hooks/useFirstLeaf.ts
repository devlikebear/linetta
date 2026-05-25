import type { NodeRow } from "../lib/types";

export interface TreeNode extends NodeRow {
  children: TreeNode[];
}

/** Build a tree (parent_id NULL roots + recursive children) from a flat list.
 *  Caller must have sorted by (parent_id, ordinal) — `nodes.list_tree` already does. */
export function buildTree(rows: NodeRow[]): TreeNode[] {
  const byId = new Map<string, TreeNode>();
  for (const r of rows) byId.set(r.id, { ...r, children: [] });
  const roots: TreeNode[] = [];
  for (const r of rows) {
    const node = byId.get(r.id)!;
    if (!r.parent_id) {
      roots.push(node);
    } else {
      byId.get(r.parent_id)?.children.push(node);
    }
  }
  return roots;
}

/** Recurse into a node and return the first leaf descendant (DFS, ordinal order).
 *  Returns the node itself if it is already a leaf. Returns null if no leaf exists. */
export function findFirstLeaf(root: TreeNode): TreeNode | null {
  if (root.kind === "leaf") return root;
  for (const c of root.children) {
    const found = findFirstLeaf(c);
    if (found) return found;
  }
  return null;
}

/** Flatten a tree to a list in DFS order (used by Cmd+K's "search node"). */
export function flatten(roots: TreeNode[]): TreeNode[] {
  const out: TreeNode[] = [];
  const walk = (n: TreeNode) => {
    out.push(n);
    n.children.forEach(walk);
  };
  roots.forEach(walk);
  return out;
}

/** Return [prevLeaf, nextLeaf] relative to currentId in DFS leaf order. */
export function leafNeighbors(roots: TreeNode[], currentId: string): { prev: TreeNode | null; next: TreeNode | null } {
  const leaves: TreeNode[] = [];
  const walk = (n: TreeNode) => {
    if (n.kind === "leaf") leaves.push(n);
    n.children.forEach(walk);
  };
  roots.forEach(walk);
  const idx = leaves.findIndex((l) => l.id === currentId);
  if (idx === -1) return { prev: null, next: null };
  return { prev: idx > 0 ? leaves[idx - 1] : null, next: idx < leaves.length - 1 ? leaves[idx + 1] : null };
}
