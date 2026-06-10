import type { TreeNode } from "../hooks/useFirstLeaf";
import { flatten } from "../hooks/useFirstLeaf";
import { outlineNumberLabel, type OutlineStructurePreset } from "./outlineRepair";

type Labeler = (key: string, values?: Record<string, string | number>) => string;
type NodeKind = "leaf" | "container";

export type CreateNodeStep =
  | { placement: "child"; parentId: string; kind: NodeKind; label: string; title: string }
  | { placement: "sibling"; referenceId: string; kind: NodeKind; label: string; title: string };

export type ChapterCreationPlan = {
  chapter: CreateNodeStep;
  seedScene: boolean;
  seedSceneLabel: string;
};

function episodeLikeCount(nodes: TreeNode[], preset: OutlineStructurePreset): number {
  if (preset.id === "webnovel") return nodes.filter((node) => node.kind === "container" || node.kind === "leaf").length;
  return nodes.filter((node) => node.kind === "container").length;
}

export function planChapterCreation(tree: TreeNode[], anchor: TreeNode, preset: OutlineStructurePreset, t: Labeler): ChapterCreationPlan {
  const allNodes = flatten(tree);
  const parentContainer = anchor.parent_id
    ? allNodes.find((node) => node.id === anchor.parent_id && node.kind === "container")
    : undefined;
  const chapterKind: NodeKind = preset.id === "webnovel" ? "leaf" : "container";
  const seedScene = preset.id !== "webnovel";
  const labelFor = (count: number) => outlineNumberLabel(preset, "chapter", count + 1, t);
  let chapter: CreateNodeStep;

  if (anchor.kind === "container" && !anchor.parent_id) {
    chapter = {
      placement: "child",
      parentId: anchor.id,
      kind: chapterKind,
      label: labelFor(episodeLikeCount(anchor.children, preset)),
      title: "",
    };
  } else if (anchor.kind === "leaf" && parentContainer && !parentContainer.parent_id) {
    chapter = {
      placement: "child",
      parentId: parentContainer.id,
      kind: chapterKind,
      label: labelFor(episodeLikeCount(parentContainer.children, preset)),
      title: "",
    };
  } else {
    const reference = anchor.kind === "leaf" && parentContainer ? parentContainer : anchor;
    const siblings = allNodes.filter((node) => (node.parent_id ?? null) === (reference.parent_id ?? null));
    chapter = {
      placement: "sibling",
      referenceId: reference.id,
      kind: chapterKind,
      label: labelFor(episodeLikeCount(siblings, preset)),
      title: "",
    };
  }

  return {
    chapter,
    seedScene,
    seedSceneLabel: outlineNumberLabel(preset, "scene", 1, t),
  };
}
