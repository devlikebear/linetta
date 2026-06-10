import type { TreeNode } from "../hooks/useFirstLeaf";
import { flatten } from "../hooks/useFirstLeaf";

export type DropPosition = "before" | "after" | "inside";

export type MovePlan = {
  parentId: string | null;
  ordinal: number;
};

function childrenForParent(tree: TreeNode[], parentId: string | null): TreeNode[] {
  if (parentId === null) return tree;
  return flatten(tree).find((node) => node.id === parentId)?.children ?? [];
}

function containsNode(root: TreeNode, id: string): boolean {
  return root.children.some((child) => child.id === id || containsNode(child, id));
}

export function planNodeMove(tree: TreeNode[], draggedId: string, targetId: string, position: DropPosition): MovePlan | null {
  if (draggedId === targetId) return null;
  const nodes = flatten(tree);
  const dragged = nodes.find((node) => node.id === draggedId);
  const target = nodes.find((node) => node.id === targetId);
  if (!dragged || !target) return null;
  if (containsNode(dragged, target.id)) return null;

  if (position === "inside") {
    if (target.kind !== "container") return null;
    return { parentId: target.id, ordinal: target.children.length };
  }

  const parentId = target.parent_id ?? null;
  const siblings = childrenForParent(tree, parentId).filter((node) => node.id !== draggedId);
  const targetIndex = siblings.findIndex((node) => node.id === targetId);
  if (targetIndex === -1) return null;
  return {
    parentId,
    ordinal: targetIndex + (position === "after" ? 1 : 0),
  };
}
