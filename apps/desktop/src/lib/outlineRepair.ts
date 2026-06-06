import type { TreeNode } from "../hooks/useFirstLeaf";
import type { NodeRow } from "./types";

type OutlineKind = "leaf" | "container";

type RepairNode = {
  id: string;
  parent_id?: string;
  ordinal: number;
  kind: OutlineKind;
  label: string;
  title: string;
  word_count: number;
};

export type OutlineRepairRPC = {
  createChild: (parentId: string, kind: OutlineKind, label: string, title: string) => Promise<NodeRow>;
  createSibling: (referenceId: string, kind: OutlineKind, label: string, title: string) => Promise<NodeRow>;
  moveToParent: (id: string, parentId: string) => Promise<{ ok: true }>;
  moveToRoot: (id: string) => Promise<{ ok: true }>;
  convertToContainer: (id: string) => Promise<{ ok: true }>;
  delete?: (id: string) => Promise<{ ok: true }>;
  rename: (id: string, label: string, title: string) => Promise<{ ok: true }>;
};

type Labeler = (key: string, values?: Record<string, string | number>) => string;
type OutlineRole = "part" | "chapter" | "scene";

type OutlineLevelSpec = {
  numberKey: string;
  nameKey: string;
  structuralPatterns: RegExp[];
  titlePatterns: RegExp[];
};

export type OutlineStructurePreset = {
  id: "novel" | "webnovel";
  nameKey: string;
  levels: Record<OutlineRole, OutlineLevelSpec>;
};

const boundary = String.raw`(?:\s|$|[-—–·:])`;

const novelPartPatterns = [
  new RegExp(String.raw`^(?:제\s*)?\d+\s*(?:부|部)${boundary}`, "i"),
  new RegExp(String.raw`^(?:부|部)\s*\d+${boundary}`, "i"),
  new RegExp(String.raw`^part\s*\d+${boundary}`, "i"),
];

const novelChapterPatterns = [
  new RegExp(String.raw`^(?:제\s*)?\d+\s*(?:장|章)${boundary}`, "i"),
  new RegExp(String.raw`^(?:장|章)\s*\d+${boundary}`, "i"),
  new RegExp(String.raw`^(?:chapter|ch)\s*\d+${boundary}`, "i"),
];

const scenePatterns = [
  /^(?:씬|scene|シーン)\s*\d+(?:\s|$|[-—–·:])/i,
];

export const OUTLINE_PRESETS = {
  novel: {
    id: "novel",
    nameKey: "workspace.outlinePreset.novel",
    levels: {
      part: {
        numberKey: "workspace.partNumber",
        nameKey: "workspace.outlineLevel.part",
        structuralPatterns: novelPartPatterns,
        titlePatterns: [
          /^(?:제\s*)?\d+\s*(?:부|部)\s*[-—–·:]?\s*(.+)$/i,
          /^(?:부|部)\s*\d+\s*[-—–·:]?\s*(.+)$/i,
          /^part\s*\d+\s*[-—–·:]?\s*(.+)$/i,
        ],
      },
      chapter: {
        numberKey: "workspace.chapterNumber",
        nameKey: "workspace.outlineLevel.chapter",
        structuralPatterns: novelChapterPatterns,
        titlePatterns: [
          /^(?:제\s*)?\d+\s*(?:장|章)\s*[-—–·:]?\s*(.+)$/i,
          /^(?:장|章)\s*\d+\s*[-—–·:]?\s*(.+)$/i,
          /^(?:chapter|ch)\s*\d+\s*[-—–·:]?\s*(.+)$/i,
        ],
      },
      scene: {
        numberKey: "workspace.sceneNumber",
        nameKey: "workspace.outlineLevel.scene",
        structuralPatterns: scenePatterns,
        titlePatterns: [
          /^(?:씬|scene|シーン)\s*\d+\s*[-—–·:]?\s*(.+)$/i,
        ],
      },
    },
  },
  webnovel: {
    id: "webnovel",
    nameKey: "workspace.outlinePreset.webnovel",
    levels: {
      part: {
        numberKey: "workspace.webNovelPartNumber",
        nameKey: "workspace.outlineLevel.webNovelPart",
        structuralPatterns: [
          ...novelPartPatterns,
          new RegExp(String.raw`^(?:제\s*)?\d+\s*(?:권|卷|아크)${boundary}`, "i"),
          new RegExp(String.raw`^(?:권|卷|아크)\s*\d+${boundary}`, "i"),
          new RegExp(String.raw`^arc\s*\d+${boundary}`, "i"),
        ],
        titlePatterns: [
          /^(?:제\s*)?\d+\s*(?:권|卷|아크|부|部)\s*[-—–·:]?\s*(.+)$/i,
          /^(?:권|卷|아크|부|部)\s*\d+\s*[-—–·:]?\s*(.+)$/i,
          /^(?:arc|part)\s*\d+\s*[-—–·:]?\s*(.+)$/i,
        ],
      },
      chapter: {
        numberKey: "workspace.webNovelChapterNumber",
        nameKey: "workspace.outlineLevel.webNovelChapter",
        structuralPatterns: [
          ...novelChapterPatterns,
          new RegExp(String.raw`^(?:제\s*)?\d+\s*(?:화|話|회)${boundary}`, "i"),
          new RegExp(String.raw`^(?:화|話|회)\s*\d+${boundary}`, "i"),
          new RegExp(String.raw`^(?:episode|ep)\s*\d+${boundary}`, "i"),
        ],
        titlePatterns: [
          /^(?:제\s*)?\d+\s*(?:화|話|회|장|章)\s*[-—–·:]?\s*(.+)$/i,
          /^(?:화|話|회|장|章)\s*\d+\s*[-—–·:]?\s*(.+)$/i,
          /^(?:episode|ep|chapter|ch)\s*\d+\s*[-—–·:]?\s*(.+)$/i,
        ],
      },
      scene: {
        numberKey: "workspace.sceneNumber",
        nameKey: "workspace.outlineLevel.scene",
        structuralPatterns: scenePatterns,
        titlePatterns: [
          /^(?:씬|scene|シーン)\s*\d+\s*[-—–·:]?\s*(.+)$/i,
        ],
      },
    },
  },
} satisfies Record<OutlineStructurePreset["id"], OutlineStructurePreset>;

export type OutlinePresetId = keyof typeof OUTLINE_PRESETS;

const DEFAULT_OUTLINE_PRESET: OutlineStructurePreset = OUTLINE_PRESETS.novel;

export function outlinePresetById(id?: string): OutlineStructurePreset {
  return id === "webnovel" ? OUTLINE_PRESETS.webnovel : DEFAULT_OUTLINE_PRESET;
}

export function outlineRoleName(preset: OutlineStructurePreset, role: OutlineRole, t: Labeler): string {
  return t(preset.levels[role].nameKey);
}

export function outlineNumberLabel(preset: OutlineStructurePreset, role: OutlineRole, number: number, t: Labeler): string {
  return t(preset.levels[role].numberKey, { number });
}

function matchesRoleLabel(label: string, role: OutlineRole, preset: OutlineStructurePreset = DEFAULT_OUTLINE_PRESET): boolean {
  const normalized = label.trim().toLowerCase();
  return preset.levels[role].structuralPatterns.some((pattern) => pattern.test(normalized));
}

export function isStructuralChapterLabel(label: string, preset: OutlineStructurePreset = DEFAULT_OUTLINE_PRESET): boolean {
  return matchesRoleLabel(label, "chapter", preset);
}

export function isStructuralPartLabel(label: string, preset: OutlineStructurePreset = DEFAULT_OUTLINE_PRESET): boolean {
  return matchesRoleLabel(label, "part", preset);
}

export function isSceneLabel(label: string, preset: OutlineStructurePreset = DEFAULT_OUTLINE_PRESET): boolean {
  return matchesRoleLabel(label, "scene", preset);
}

type LabelPlan = {
  label: string;
  title: string;
};

type OutlineNameNode = {
  id: string;
  kind: OutlineKind;
  label: string;
  title: string;
  children?: OutlineNameNode[];
};

function cleanupTitle(value: string): string {
  const trimmed = value.trim().replace(/^[\s\-—–·:]+/, "").trim();
  const parts = trimmed.split(/\s+[-—–]\s+/).map((part) => part.trim()).filter(Boolean);
  if (parts.length > 1 && parts.every((part) => part === parts[0])) return parts[0];
  return trimmed;
}

function embeddedTitle(label: string, role: OutlineRole, preset: OutlineStructurePreset): string {
  const trimmed = label.trim();
  for (const pattern of preset.levels[role].titlePatterns) {
    const match = trimmed.match(pattern);
    if (match?.[1]) return cleanupTitle(match[1]);
  }
  if (role === "part" && !isStructuralPartLabel(trimmed, preset)) return cleanupTitle(trimmed);
  if (role === "chapter" && !isStructuralChapterLabel(trimmed, preset)) return cleanupTitle(trimmed);
  if (role === "scene" && !isSceneLabel(trimmed, preset)) return cleanupTitle(trimmed);
  return "";
}

function plannedName(node: OutlineNameNode, role: OutlineRole, number: number, t: Labeler, preset: OutlineStructurePreset): LabelPlan {
  const title = cleanupTitle(node.title) || embeddedTitle(node.label, role, preset);
  const label = outlineNumberLabel(preset, role, number, t);
  return { label, title };
}

export type OutlineLabelIssue = {
  id: string;
  label: string;
};

export function collectOutlineLabelIssues(tree: TreeNode[], t: Labeler, preset: OutlineStructurePreset = DEFAULT_OUTLINE_PRESET): OutlineLabelIssue[] {
  const issues: OutlineLabelIssue[] = [];
  const addIfNeeded = (node: OutlineNameNode, role: OutlineRole, number: number) => {
    const plan = plannedName(node, role, number, t, preset);
    if (node.label !== plan.label || cleanupTitle(node.title) !== plan.title) {
      issues.push({ id: node.id, label: node.label });
    }
  };
  tree.filter((node) => node.kind === "container").forEach((part, partIndex) => {
    addIfNeeded(part, "part", partIndex + 1);
    part.children.filter((node) => node.kind === "container").forEach((chapter, chapterIndex) => {
      addIfNeeded(chapter, "chapter", chapterIndex + 1);
      chapter.children.filter((node) => node.kind === "leaf").forEach((scene, sceneIndex) => {
        addIfNeeded(scene, "scene", sceneIndex + 1);
      });
    });
  });
  return issues;
}

export async function repairOutlineTree(tree: TreeNode[], rpc: OutlineRepairRPC, t: Labeler, preset: OutlineStructurePreset = DEFAULT_OUTLINE_PRESET): Promise<void> {
  const records = new Map<string, RepairNode>();
  const seed = (nodes: TreeNode[]) => {
    for (const n of nodes) {
      records.set(n.id, {
        id: n.id,
        parent_id: n.parent_id,
        ordinal: n.ordinal,
        kind: n.kind,
        label: n.label,
        title: n.title,
        word_count: n.word_count,
      });
      seed(n.children);
    }
  };
  seed(tree);

  const active = () => Array.from(records.values());
  const childrenOf = (parentID?: string) =>
    active()
      .filter((n) => (n.parent_id ?? "") === (parentID ?? ""))
      .sort((a, b) => a.ordinal - b.ordinal || a.id.localeCompare(b.id));
  const depthOf = (node: RepairNode) => {
    let depth = 0;
    let cur = node;
    const seen = new Set<string>();
    while (cur.parent_id && !seen.has(cur.id)) {
      seen.add(cur.id);
      const parent = records.get(cur.parent_id);
      if (!parent) break;
      depth += 1;
      cur = parent;
    }
    return depth;
  };
  const chainOf = (node: RepairNode) => {
    const chain: RepairNode[] = [node];
    let cur = node;
    const seen = new Set<string>();
    while (cur.parent_id && !seen.has(cur.id)) {
      seen.add(cur.id);
      const parent = records.get(cur.parent_id);
      if (!parent) break;
      chain.unshift(parent);
      cur = parent;
    }
    return chain;
  };
  const nextOrdinal = (parentID: string) => {
    const siblings = childrenOf(parentID);
    return siblings.length === 0 ? 0 : Math.max(...siblings.map((n) => n.ordinal)) + 1;
  };
  const addCreated = (row: NodeRow): RepairNode => {
    const rec: RepairNode = {
      id: row.id,
      parent_id: row.parent_id,
      ordinal: row.ordinal,
      kind: row.kind,
      label: row.label,
      title: row.title,
      word_count: row.word_count,
    };
    records.set(rec.id, rec);
    return rec;
  };
  const moveToParent = async (node: RepairNode, parent: RepairNode) => {
    if (node.parent_id === parent.id) return;
    await rpc.moveToParent(node.id, parent.id);
    node.parent_id = parent.id;
    node.ordinal = nextOrdinal(parent.id);
  };
  const moveToRoot = async (node: RepairNode) => {
    if (!node.parent_id) return;
    const ordinal = nextRootOrdinal();
    await rpc.moveToRoot(node.id);
    node.parent_id = undefined;
    node.ordinal = ordinal;
  };
  const convertToContainer = async (node: RepairNode) => {
    if (node.kind === "container" || node.word_count !== 0) return;
    await rpc.convertToContainer(node.id);
    node.kind = "container";
  };
  const rename = async (node: RepairNode, role: OutlineRole, number: number) => {
    const plan = plannedName(node, role, number, t, preset);
    if (node.label === plan.label && cleanupTitle(node.title) === plan.title) return;
    await rpc.rename(node.id, plan.label, plan.title);
    node.label = plan.label;
    node.title = plan.title;
  };
  const nextRootOrdinal = () => {
    const roots = childrenOf();
    return roots.length === 0 ? 0 : Math.max(...roots.map((n) => n.ordinal)) + 1;
  };
  const ensureChapter = async (part: RepairNode): Promise<RepairNode> => {
    const existing = childrenOf(part.id).find((n) => n.kind === "container");
    if (existing) return existing;
    const created = await rpc.createChild(part.id, "container", outlineNumberLabel(preset, "chapter", 1, t), "");
    return addCreated(created);
  };
  const hasContainerChild = (node: RepairNode) => childrenOf(node.id).some((child) => child.kind === "container");
  const isNestedPartLike = (node: RepairNode) =>
    node.kind === "container" &&
    Boolean(node.parent_id) &&
    !isStructuralChapterLabel(node.label, preset) &&
    hasContainerChild(node);
  const nextStructuralChapterSibling = (siblings: RepairNode[], index: number) =>
    siblings.slice(index + 1).find((sibling) => isStructuralChapterLabel(sibling.label, preset));
  const previousStructuralChapterSibling = (siblings: RepairNode[], index: number) =>
    [...siblings.slice(0, index)].reverse().find((sibling) => isStructuralChapterLabel(sibling.label, preset));
  const isPartMarkerLeaf = (siblings: RepairNode[], index: number, node: RepairNode) =>
    node.kind === "leaf" &&
    node.word_count === 0 &&
    !isSceneLabel(node.label, preset) &&
    !isStructuralChapterLabel(node.label, preset) &&
    Boolean(previousStructuralChapterSibling(siblings, index)) &&
    Boolean(nextStructuralChapterSibling(siblings, index));
  const sceneTargetFor = async (container: RepairNode) => {
    if (isStructuralChapterLabel(container.label, preset)) return container;
    if (hasContainerChild(container)) return ensureChapter(container);
    return container;
  };
  const attachScenesToPrecedingContainer = async () => {
    const parents = active()
      .filter((node) => node.kind === "container" && hasContainerChild(node))
      .sort((a, b) => depthOf(b) - depthOf(a) || a.ordinal - b.ordinal);
    for (const parent of parents) {
      let currentContainer: RepairNode | null = null;
      for (const child of childrenOf(parent.id)) {
        if (child.kind === "container") {
          currentContainer = child;
          continue;
        }
        if (!currentContainer) continue;
        const target = await sceneTargetFor(currentContainer);
        await moveToParent(child, target);
      }
    }
  };
  const normalizeMarkerLeaves = async () => {
    const parents = active()
      .filter((node) => node.kind === "container")
      .sort((a, b) => depthOf(b) - depthOf(a) || a.ordinal - b.ordinal);
    for (const parent of parents) {
      const parentOfParent = parent.parent_id ? records.get(parent.parent_id) : undefined;
      const chapterHost = isStructuralChapterLabel(parent.label, preset) && parentOfParent?.kind === "container" ? parentOfParent : parent;
      let currentPart: RepairNode | null = null;
      let currentChapter: RepairNode | null = null;
      const children = childrenOf(parent.id);
      for (let index = 0; index < children.length; index += 1) {
        const child = children[index];
        if (child.kind === "leaf" && isStructuralChapterLabel(child.label, preset) && child.word_count === 0) {
          await convertToContainer(child);
          const host = currentPart ?? chapterHost;
          await moveToParent(child, host);
          currentChapter = child;
          continue;
        }
        if (isPartMarkerLeaf(children, index, child)) {
          await convertToContainer(child);
          await moveToRoot(child);
          currentPart = child;
          currentChapter = null;
          continue;
        }
        if (child.kind === "container") {
          if (isStructuralChapterLabel(child.label, preset)) {
            currentChapter = child;
          } else if (hasContainerChild(child)) {
            currentPart = child;
            currentChapter = null;
          }
          continue;
        }
        if (currentChapter) {
          await moveToParent(child, currentChapter);
        }
      }
    }
  };

  await normalizeMarkerLeaves();
  await attachScenesToPrecedingContainer();

  for (const part of active().filter(isNestedPartLike).sort((a, b) => depthOf(a) - depthOf(b) || a.ordinal - b.ordinal)) {
    await moveToRoot(part);
  }

  let rootContainers = childrenOf().filter((n) => n.kind === "container");
  const rootChapters = rootContainers.filter((n) => isStructuralChapterLabel(n.label, preset));
  let rootParts = rootContainers.filter((n) => !isStructuralChapterLabel(n.label, preset));
  if (rootChapters.length > 0) {
    let part = rootParts[0];
    if (!part) {
      part = addCreated(await rpc.createSibling(rootChapters[0].id, "container", outlineNumberLabel(preset, "part", 1, t), ""));
    }
    for (const chapter of rootChapters) {
      await moveToParent(chapter, part);
    }
  }

  rootContainers = childrenOf().filter((n) => n.kind === "container");
  rootParts = rootContainers.filter((n) => !isStructuralChapterLabel(n.label, preset));
  const rootLeaves = childrenOf().filter((n) => n.kind === "leaf");
  if (rootParts.length > 0 && rootLeaves.length > 0) {
    const chapter = await ensureChapter(rootParts[0]);
    for (const leaf of rootLeaves) {
      await moveToParent(leaf, chapter);
    }
  }

  for (const part of childrenOf().filter((n) => n.kind === "container" && !isStructuralChapterLabel(n.label, preset))) {
    const directLeaves = childrenOf(part.id).filter((n) => n.kind === "leaf");
    if (directLeaves.length === 0) continue;
    const chapter = await ensureChapter(part);
    for (const leaf of directLeaves) {
      await moveToParent(leaf, chapter);
    }
  }

  for (const container of active().filter((n) => n.kind === "container").sort((a, b) => depthOf(b) - depthOf(a))) {
    if (depthOf(container) <= 1) continue;
    const chain = chainOf(container);
    const part = chain[0];
    if (!part || part.kind !== "container" || part.id === container.id) continue;
    await moveToParent(container, part);
  }

  for (const [partIndex, part] of childrenOf().filter((n) => n.kind === "container").entries()) {
    await rename(part, "part", partIndex + 1);
    for (const [chapterIndex, chapter] of childrenOf(part.id).filter((n) => n.kind === "container").entries()) {
      await rename(chapter, "chapter", chapterIndex + 1);
      for (const [sceneIndex, scene] of childrenOf(chapter.id).filter((n) => n.kind === "leaf").entries()) {
        await rename(scene, "scene", sceneIndex + 1);
      }
    }
  }
}
