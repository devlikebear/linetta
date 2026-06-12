import { useEffect, useState, type DragEvent, type MouseEvent } from "react";
import { AlertTriangle, ChevronLeft, Copy, FilePlus2, FolderPlus, Layers, MoreHorizontal, Pencil, Stethoscope, Trash2, ArrowUp, ArrowDown } from "lucide-react";
import type { TreeNode } from "../hooks/useFirstLeaf";
import { flatten, sumLeafChars } from "../hooks/useFirstLeaf";
import type { NodeStatus } from "../lib/types";
import { displayNodeLabel, useI18n } from "../lib/i18n";
import { InlineEditableText } from "./InlineEditableText";
import { planNodeMove, type DropPosition } from "../lib/outlineMove";
import {
  OUTLINE_PRESETS,
  collectOutlineLabelIssues,
  isSceneLabel,
  isStructuralChapterLabel,
  outlinePresetById,
  outlineRoleName,
  type OutlinePresetId,
  type OutlineStructurePreset,
} from "../lib/outlineRepair";
import "./OutlinePanel.css";

interface Props {
  tree: TreeNode[];
  currentId: string;
  collapsed: boolean;
  onToggleCollapse: () => void;
  onSelect: (node: TreeNode) => void;
  onRename?: (node: TreeNode, title: string) => void | Promise<void>;
  renameRequest?: { id: string; nonce: number } | null;
  onCreateScene?: (node: TreeNode) => void;
  onCreatePart?: (node: TreeNode) => void;
  onCreateChapter?: (node: TreeNode) => void;
  onMoveSceneUp?: (node: TreeNode) => void;
  onMoveSceneDown?: (node: TreeNode) => void;
  onDeleteScene?: (node: TreeNode) => void;
  onCopyText?: (node: TreeNode) => void;
  onMoveNodeUp?: (node: TreeNode) => void;
  onMoveNodeDown?: (node: TreeNode) => void;
  onMoveNode?: (node: TreeNode, parentId: string | null, ordinal: number) => void;
  onDeleteNode?: (node: TreeNode) => void;
  onSetStatus?: (node: TreeNode, status: NodeStatus) => void;
  onRepairOutline?: () => void;
  onUndoRepairOutline?: () => void;
  canUndoRepair?: boolean;
  outlinePresetId?: OutlinePresetId;
  episodeCharTarget?: number;
  onOutlinePresetChange?: (presetId: OutlinePresetId) => void;
  tourTarget?: string;
}

type MenuState = {
  node: TreeNode;
  x: number;
  y: number;
};

const NODE_STATUSES: NodeStatus[] = ["draft", "revision", "final", "published"];

export function OutlinePanel({
  tree,
  currentId,
  collapsed,
  onToggleCollapse,
  onSelect,
  onRename,
  renameRequest,
  onCreateScene,
  onCreatePart,
  onCreateChapter,
  onMoveSceneUp,
  onMoveSceneDown,
  onDeleteScene,
  onCopyText,
  onMoveNodeUp,
  onMoveNodeDown,
  onMoveNode,
  onDeleteNode,
  onSetStatus,
  onRepairOutline,
  onUndoRepairOutline,
  canUndoRepair,
  outlinePresetId,
  episodeCharTarget = 5000,
  onOutlinePresetChange,
  tourTarget,
}: Props) {
  const { language, t } = useI18n();
  const [menu, setMenu] = useState<MenuState | null>(null);
  const [doctorOpen, setDoctorOpen] = useState(false);
  const [editingNodeId, setEditingNodeId] = useState<string | null>(null);
  const [draggingNodeId, setDraggingNodeId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<{ nodeId: string; position: DropPosition } | null>(null);
  const outlinePreset = outlinePresetById(outlinePresetId);
  const showEpisodeProgress = outlinePreset.id === "webnovel";
  const outlineIssues = analyzeOutline(tree, t, outlinePreset);
  const partName = outlineRoleName(outlinePreset, "part", t);
  const chapterName = outlineRoleName(outlinePreset, "chapter", t);
  const canOpenMenu = (node: TreeNode) =>
    Boolean(
      onRename ||
        onCreatePart ||
        onCreateScene ||
        onCreateChapter ||
        onCopyText ||
        onMoveNodeUp ||
        onMoveNodeDown ||
        onDeleteNode ||
        onSetStatus ||
        (node.kind === "leaf" && (onMoveSceneUp || onMoveSceneDown || onDeleteScene)),
    );

  useEffect(() => {
    if (!menu) return undefined;
    const close = () => setMenu(null);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    window.addEventListener("click", close);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("click", close);
      window.removeEventListener("keydown", onKey);
    };
  }, [menu]);

  const openMenu = (e: MouseEvent, node: TreeNode) => {
    if (!canOpenMenu(node)) return;
    e.preventDefault();
    e.stopPropagation();
    setMenu({ node, x: e.clientX, y: e.clientY });
  };

  const runAction = (fn: ((node: TreeNode) => void) | undefined) => {
    if (!menu || !fn) return;
    const node = menu.node;
    setMenu(null);
    fn(node);
  };

  const startRename = () => {
    if (!menu || !onRename) return;
    const node = menu.node;
    setMenu(null);
    setEditingNodeId(node.id);
  };

  const commitRename = async (node: TreeNode, title: string) => {
    if (!onRename) return;
    await onRename(node, title);
    setEditingNodeId(null);
  };

  const runStatusAction = (status: NodeStatus) => {
    if (!menu || !onSetStatus) return;
    const node = menu.node;
    setMenu(null);
    onSetStatus(node, status);
  };

  useEffect(() => {
    if (!renameRequest) return;
    setEditingNodeId(renameRequest.id);
  }, [renameRequest]);

  const dropPositionFor = (e: DragEvent<HTMLElement>, node: TreeNode): DropPosition => {
    const rect = e.currentTarget.getBoundingClientRect();
    if (rect.height === 0) return node.kind === "container" ? "inside" : "before";
    const y = e.clientY - rect.top;
    if (node.kind === "container" && y > rect.height * 0.25 && y < rect.height * 0.75) return "inside";
    return y > rect.height / 2 ? "after" : "before";
  };

  const handleDragStart = (e: DragEvent<HTMLElement>, node: TreeNode) => {
    if (!onMoveNode) return;
    e.stopPropagation();
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", node.id);
    }
    setDraggingNodeId(node.id);
  };

  const handleDragOver = (e: DragEvent<HTMLElement>, node: TreeNode) => {
    if (!onMoveNode || !draggingNodeId) return;
    const position = dropPositionFor(e, node);
    if (!planNodeMove(tree, draggingNodeId, node.id, position)) return;
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer) {
      e.dataTransfer.dropEffect = "move";
    }
    setDropTarget({ nodeId: node.id, position });
  };

  const clearDragState = () => {
    setDraggingNodeId(null);
    setDropTarget(null);
  };

  const handleDrop = (e: DragEvent<HTMLElement>, node: TreeNode) => {
    if (!onMoveNode || !draggingNodeId) return;
    e.preventDefault();
    e.stopPropagation();
    const position = dropTarget?.nodeId === node.id ? dropTarget.position : dropPositionFor(e, node);
    const plan = planNodeMove(tree, draggingNodeId, node.id, position);
    const dragged = flatten(tree).find((item) => item.id === draggingNodeId);
    clearDragState();
    if (!plan || !dragged) return;
    onMoveNode(dragged, plan.parentId, plan.ordinal);
  };

  if (collapsed) {
    const scenes = flatten(tree).filter((n) => n.kind === "leaf");
    return (
      <nav className="rail is-collapsed" data-tour={tourTarget}>
        <div className="rail-head">
          <button type="button" className="rail-collapse" onClick={onToggleCollapse} title={t("workspace.outlineExpand")}>
            <Layers size={16} />
          </button>
        </div>
        <div className="rail-mini">
          {scenes.map((s) => (
            <button
              key={s.id}
              type="button"
              className={`dot-scene${s.id === currentId ? " active" : ""}`}
              onClick={() => onSelect(s)}
              title={`${displayNodeLabel(language, s.label)}${s.title ? ` · ${s.title}` : ""}`}
            />
          ))}
        </div>
      </nav>
    );
  }

  return (
    <nav className="rail" data-tour={tourTarget}>
      <div className="rail-head">
        <span className="lbl">{t("workspace.outline")}</span>
        <div className="rail-actions">
          <button
            type="button"
            className={`rail-doctor${doctorOpen ? " is-active" : ""}`}
            onClick={() => setDoctorOpen((v) => !v)}
            aria-label={t("workspace.outlineDoctor")}
            title={t("workspace.outlineDoctor")}
          >
            <Stethoscope size={14} />
          </button>
          <button type="button" className="rail-collapse" onClick={onToggleCollapse} title={t("workspace.collapse")}>
            <ChevronLeft size={15} />
          </button>
        </div>
      </div>
      {doctorOpen && (
        <section className="outline-doctor" aria-label={t("workspace.outlineDoctor")}>
          <div className="outline-doctor-title">
            <AlertTriangle size={13} />
            <span>{t("workspace.outlineDoctorResult")}</span>
            <strong>{outlineIssues.length}</strong>
          </div>
          <label className="outline-preset">
            <span>{t("workspace.outlinePreset")}</span>
            <select
              value={outlinePreset.id}
              aria-label={t("workspace.outlinePreset")}
              onChange={(e) => onOutlinePresetChange?.(e.currentTarget.value as OutlinePresetId)}
              disabled={!onOutlinePresetChange}
            >
              {Object.values(OUTLINE_PRESETS).map((preset) => (
                <option key={preset.id} value={preset.id}>
                  {t(preset.nameKey)}
                </option>
              ))}
            </select>
          </label>
          {outlineIssues.length === 0 ? (
            <p>{t("workspace.outlineDoctorClean")}</p>
          ) : (
            <ul>
              {outlineIssues.map((issue) => (
                <li key={issue.key}>{issue.text}</li>
              ))}
            </ul>
          )}
          <button type="button" className="outline-repair" onClick={onRepairOutline} disabled={!onRepairOutline || outlineIssues.length === 0}>
            {t("workspace.outlineRepair")}
          </button>
          {canUndoRepair && onUndoRepairOutline && (
            <button type="button" className="outline-undo-repair" onClick={onUndoRepairOutline}>
              {t("workspace.outlineUndoRepair")}
            </button>
          )}
        </section>
      )}
      <div className="rail-tree">
        {tree.map((root) => (
          <RailNode
            key={root.id}
            node={root}
            depth={0}
            currentId={currentId}
            onSelect={onSelect}
            onOpenMenu={openMenu}
            canOpenMenu={canOpenMenu}
            showEpisodeProgress={showEpisodeProgress}
            episodeCharTarget={episodeCharTarget}
            outlinePreset={outlinePreset}
            editingNodeId={editingNodeId}
            onCommitRename={onRename ? commitRename : undefined}
            onCancelRename={() => setEditingNodeId(null)}
            draggingNodeId={draggingNodeId}
            dropTarget={dropTarget}
            onDragStartNode={onMoveNode ? handleDragStart : undefined}
            onDragOverNode={onMoveNode ? handleDragOver : undefined}
            onDropNode={onMoveNode ? handleDrop : undefined}
            onDragEndNode={clearDragState}
          />
        ))}
      </div>
      {menu && (
        <div
          role="menu"
          className="outline-menu"
          style={{ left: menu.x, top: menu.y }}
          onClick={(e) => e.stopPropagation()}
        >
          {onRename && (
            <button type="button" role="menuitem" onClick={startRename}>
              <Pencil size={13} /> {t("workspace.rename")}
            </button>
          )}
          {onCreatePart && (
            <button type="button" role="menuitem" onClick={() => runAction(onCreatePart)}>
              <FolderPlus size={13} /> {t("workspace.newOutlineLevel", { level: partName })}
            </button>
          )}
          {onCreateScene && (
            <button type="button" role="menuitem" onClick={() => runAction(onCreateScene)}>
              <FilePlus2 size={13} /> {t("workspace.newScene")}
            </button>
          )}
          {onCreateChapter && (
            <button type="button" role="menuitem" onClick={() => runAction(onCreateChapter)}>
              <FolderPlus size={13} /> {t("workspace.newOutlineLevel", { level: chapterName })}
            </button>
          )}
          {onCopyText && (
            <button type="button" role="menuitem" onClick={() => runAction(onCopyText)}>
              <Copy size={13} /> {t("workspace.copyText")}
            </button>
          )}
          {onSetStatus && (
            <>
              <div className="outline-menu-sep" role="separator" />
              <div className="outline-menu-label">{t("workspace.statusMenu")}</div>
              {NODE_STATUSES.map((status) => (
                <button
                  key={status}
                  type="button"
                  role="menuitem"
                  className={`status-menu-item is-${status}${menu.node.status === status ? " is-current" : ""}`}
                  title={t("workspace.setStatus", { status: t(`workspace.status.${status}`) })}
                  onClick={() => runStatusAction(status)}
                >
                  <span className={`outline-status-dot is-${status}`} aria-hidden="true" />
                  {t(`workspace.status.${status}`)}
                </button>
              ))}
            </>
          )}
          {(onMoveNodeUp || onMoveNodeDown || onDeleteNode || (menu.node.kind === "leaf" && (onMoveSceneUp || onMoveSceneDown || onDeleteScene))) && (
            <div className="outline-menu-sep" role="separator" />
          )}
          {(onMoveNodeUp || (menu.node.kind === "leaf" && onMoveSceneUp)) && (
            <button type="button" role="menuitem" onClick={() => runAction(onMoveNodeUp ?? onMoveSceneUp)}>
              <ArrowUp size={13} /> {t("workspace.moveUp")}
            </button>
          )}
          {(onMoveNodeDown || (menu.node.kind === "leaf" && onMoveSceneDown)) && (
            <button type="button" role="menuitem" onClick={() => runAction(onMoveNodeDown ?? onMoveSceneDown)}>
              <ArrowDown size={13} /> {t("workspace.moveDown")}
            </button>
          )}
          {(onDeleteNode || (menu.node.kind === "leaf" && onDeleteScene)) && (
            <button type="button" role="menuitem" className="danger" onClick={() => runAction(onDeleteNode ?? onDeleteScene)}>
              <Trash2 size={13} /> {t("workspace.delete")}
            </button>
          )}
        </div>
      )}
    </nav>
  );
}

type OutlineIssue = {
  key: string;
  text: string;
};

function analyzeOutline(tree: TreeNode[], t: ReturnType<typeof useI18n>["t"], preset: OutlineStructurePreset): OutlineIssue[] {
  const issues: OutlineIssue[] = [];
  const issueValues = (label: string) => ({
    label,
    part: outlineRoleName(preset, "part", t),
    chapter: outlineRoleName(preset, "chapter", t),
    scene: outlineRoleName(preset, "scene", t),
  });
  const isDirectWebNovelEpisodeLeaf = (node: TreeNode, depth: number) =>
    preset.id === "webnovel" &&
    depth === 1 &&
    node.kind === "leaf" &&
    isStructuralChapterLabel(node.label, preset);
  const visit = (nodes: TreeNode[], depth: number, parentKey: string) => {
    const seen = new Map<string, TreeNode[]>();
    nodes.forEach((node, index) => {
      const label = node.label.trim();
      const directWebNovelEpisodeLeaf = isDirectWebNovelEpisodeLeaf(node, depth);
      if (label) {
        const list = seen.get(label) ?? [];
        list.push(node);
        seen.set(label, list);
      }
      if (node.kind === "container" && node.children.length === 0) {
        issues.push({ key: `empty-${node.id}`, text: t("workspace.outlineIssue.empty", issueValues(node.label)) });
      }
      if (node.kind === "leaf" && node.word_count === 0 && isStructuralChapterLabel(node.label, preset) && !directWebNovelEpisodeLeaf) {
        issues.push({ key: `chapter-as-scene-${node.id}`, text: t("workspace.outlineIssue.chapterAsScene", issueValues(node.label)) });
      }
      if (
        node.kind === "leaf" &&
        node.word_count === 0 &&
        !isSceneLabel(node.label, preset) &&
        !isStructuralChapterLabel(node.label, preset) &&
        nodes.slice(0, index).some((sibling) => isStructuralChapterLabel(sibling.label, preset)) &&
        nodes.slice(index + 1).some((sibling) => isStructuralChapterLabel(sibling.label, preset))
      ) {
        issues.push({ key: `part-as-scene-${node.id}`, text: t("workspace.outlineIssue.partAsScene", issueValues(node.label)) });
      }
      if (depth === 0 && node.kind === "leaf" && tree.some((root) => root.kind === "container")) {
        issues.push({ key: `root-leaf-${node.id}`, text: t("workspace.outlineIssue.rootLeaf", issueValues(node.label)) });
      }
      if (depth === 0 && node.kind === "container" && isStructuralChapterLabel(node.label, preset)) {
        issues.push({ key: `root-chapter-${node.id}`, text: t("workspace.outlineIssue.rootChapter", issueValues(node.label)) });
      }
      if (
        node.kind === "container" &&
        isStructuralChapterLabel(node.label, preset) &&
        node.children.some((child) => child.kind === "container")
      ) {
        issues.push({ key: `chapter-containers-${node.id}`, text: t("workspace.outlineIssue.chapterContainsContainers", issueValues(node.label)) });
      }
      if (
        depth > 0 &&
        node.kind === "container" &&
        !isStructuralChapterLabel(node.label, preset) &&
        node.children.some((child) => child.kind === "container")
      ) {
        issues.push({ key: `nested-part-${node.id}`, text: t("workspace.outlineIssue.nestedPart", issueValues(node.label)) });
      }
      if (depth === 1 && node.kind === "leaf" && !directWebNovelEpisodeLeaf) {
        issues.push({ key: `part-leaf-${node.id}`, text: t("workspace.outlineIssue.sceneUnderPart", issueValues(node.label)) });
      }
      if (depth > 2) {
        issues.push({ key: `deep-${node.id}`, text: t("workspace.outlineIssue.deep", issueValues(node.label)) });
      }
      visit(node.children, depth + 1, node.id);
    });
    seen.forEach((items, label) => {
      if (items.length > 1) {
        issues.push({ key: `dup-${parentKey}-${label}`, text: t("workspace.outlineIssue.duplicate", issueValues(label)) });
      }
    });
  };
  visit(tree, 0, "root");
  for (const issue of collectOutlineLabelIssues(tree, t, preset)) {
    issues.push({
      key: `label-cleanup-${issue.id}`,
      text: t("workspace.outlineIssue.labelCleanup", issueValues(issue.label)),
    });
  }
  return issues;
}

function StatusDot({ status }: { status: NodeStatus }) {
  const { t } = useI18n();
  const label = t(`workspace.status.${status}`);
  return (
    <span
      className={`outline-status-dot is-${status}`}
      aria-label={t("workspace.statusLabel", { status: label })}
      title={label}
    />
  );
}

/** Recursively renders the project tree against the mockup's rail classes.
 *  Top-level containers → `.tree-part` headers; deeper containers →
 *  `.tree-chapter`; leaves → selectable `.tree-scene` buttons. */
function RailNode({
  node,
  depth,
  currentId,
  onSelect,
  onOpenMenu,
  canOpenMenu,
  showEpisodeProgress,
  episodeCharTarget,
  outlinePreset,
  editingNodeId,
  onCommitRename,
  onCancelRename,
  draggingNodeId,
  dropTarget,
  onDragStartNode,
  onDragOverNode,
  onDropNode,
  onDragEndNode,
}: {
  node: TreeNode;
  depth: number;
  currentId: string;
  onSelect: (n: TreeNode) => void;
  onOpenMenu: (e: MouseEvent, n: TreeNode) => void;
  canOpenMenu: (n: TreeNode) => boolean;
  showEpisodeProgress: boolean;
  episodeCharTarget: number;
  outlinePreset: OutlineStructurePreset;
  editingNodeId: string | null;
  onCommitRename?: (node: TreeNode, title: string) => Promise<void>;
  onCancelRename: () => void;
  draggingNodeId: string | null;
  dropTarget: { nodeId: string; position: DropPosition } | null;
  onDragStartNode?: (e: DragEvent<HTMLElement>, node: TreeNode) => void;
  onDragOverNode?: (e: DragEvent<HTMLElement>, node: TreeNode) => void;
  onDropNode?: (e: DragEvent<HTMLElement>, node: TreeNode) => void;
  onDragEndNode: () => void;
}) {
  const { language, t } = useI18n();
  const hasMenu = canOpenMenu(node);
  const label = displayNodeLabel(language, node.label);
  const editing = editingNodeId === node.id && Boolean(onCommitRename);
  const draggable = Boolean(onDragStartNode) && !editing;
  const dragClass = `${draggingNodeId === node.id ? " is-dragging" : ""}${dropTarget?.nodeId === node.id ? ` is-drop-${dropTarget.position}` : ""}`;
  const titleEditor = (
    <InlineEditableText
      value={node.title}
      ariaLabel={t("workspace.prompt.displayTitle")}
      className="outline-title-input"
      autoFocus
      allowEmpty
      placeholder={t("workspace.prompt.displayTitle")}
      onCommit={async (title) => { await onCommitRename?.(node, title); }}
      onCancel={onCancelRename}
    />
  );

  // Episode-like nodes get the char gauge: non-root containers (화 holding
  // scenes) and leaves whose label matches the preset's episode pattern
  // (leaf 화 created directly under a 권 by the webnovel preset).
  const isEpisode =
    showEpisodeProgress &&
    (node.kind === "leaf"
      ? isStructuralChapterLabel(node.label, outlinePreset)
      : depth > 0);
  const episodeChars = isEpisode ? sumLeafChars(node) : 0;
  const normalizedTarget = Math.max(1, episodeCharTarget);
  const episodePercent = Math.min(100, Math.round((episodeChars / normalizedTarget) * 100));
  const episodeProgress = isEpisode ? (
    <span className="episode-progress">
      <span className="episode-count">
        {episodeChars.toLocaleString(language)} / {normalizedTarget.toLocaleString(language)}
      </span>
      <span className="episode-meter" aria-hidden="true">
        <span
          className={`episode-meter-fill${episodeChars >= normalizedTarget ? " is-complete" : ""}`}
          style={{ width: `${episodePercent}%` }}
        />
      </span>
    </span>
  ) : null;

  if (node.kind === "leaf") {
    const active = node.id === currentId;
    const trailing = episodeProgress ?? <span className="sc-words">{node.word_count}</span>;
    return (
      <div
        className={`tree-scene-row${active ? " active" : ""}${dragClass}`}
        draggable={draggable}
        onDragStart={(e) => onDragStartNode?.(e, node)}
        onDragOver={(e) => onDragOverNode?.(e, node)}
        onDrop={(e) => onDropNode?.(e, node)}
        onDragEnd={onDragEndNode}
        onContextMenu={(e) => onOpenMenu(e, node)}
      >
        {editing ? (
          <div className={`tree-scene${active ? " active" : ""}`}>
            <StatusDot status={node.status} />
            <span className="sc-label">{label}</span>
            {titleEditor}
            {trailing}
          </div>
        ) : (
          <button
            type="button"
            className={`tree-scene${active ? " active" : ""}`}
            onClick={() => onSelect(node)}
          >
            <StatusDot status={node.status} />
            <span className="sc-label">{label}</span>
            <span className="sc-title">{node.title}</span>
            {trailing}
          </button>
        )}
        {hasMenu && (
          <button
            type="button"
            className="tree-menu-btn"
            title={t("workspace.menu")}
            aria-label={t("workspace.menu")}
            onClick={(e) => onOpenMenu(e, node)}
          >
            <MoreHorizontal size={14} />
          </button>
        )}
      </div>
    );
  }

  // Containers: top-level → part header, otherwise chapter header.
  const header =
    depth === 0 ? (
      <div
        className={`tree-part-row${dragClass}`}
        draggable={draggable}
        onDragStart={(e) => onDragStartNode?.(e, node)}
        onDragOver={(e) => onDragOverNode?.(e, node)}
        onDrop={(e) => onDropNode?.(e, node)}
        onDragEnd={onDragEndNode}
        onContextMenu={(e) => onOpenMenu(e, node)}
      >
        <div className="tree-part">
          {label}
          {editing ? (
            <>
              <span className="tree-title-sep"> · </span>
              {titleEditor}
            </>
          ) : node.title ? ` · ${node.title}` : ""}
        </div>
        {hasMenu && (
          <button
            type="button"
            className="tree-menu-btn"
            title={t("workspace.menu")}
            aria-label={t("workspace.menu")}
            onClick={(e) => onOpenMenu(e, node)}
          >
            <MoreHorizontal size={14} />
          </button>
        )}
      </div>
    ) : (
      <div
        className={`tree-chapter-row${dragClass}`}
        draggable={draggable}
        onDragStart={(e) => onDragStartNode?.(e, node)}
        onDragOver={(e) => onDragOverNode?.(e, node)}
        onDrop={(e) => onDropNode?.(e, node)}
        onDragEnd={onDragEndNode}
        onContextMenu={(e) => onOpenMenu(e, node)}
      >
        <div className="tree-chapter">
          {depth > 0 && <StatusDot status={node.status} />}
          <span className="ch-label">{label}</span>
          {editing ? titleEditor : node.title && <span className="ch-title">{node.title}</span>}
          {episodeProgress}
        </div>
        {hasMenu && (
          <button
            type="button"
            className="tree-menu-btn"
            title={t("workspace.menu")}
            aria-label={t("workspace.menu")}
            onClick={(e) => onOpenMenu(e, node)}
          >
            <MoreHorizontal size={14} />
          </button>
        )}
      </div>
    );

  return (
    <div>
      {header}
      {node.children.map((c) => (
        <RailNode
          key={c.id}
          node={c}
          depth={depth + 1}
          currentId={currentId}
          onSelect={onSelect}
          onOpenMenu={onOpenMenu}
          canOpenMenu={canOpenMenu}
          showEpisodeProgress={showEpisodeProgress}
          episodeCharTarget={episodeCharTarget}
          outlinePreset={outlinePreset}
          editingNodeId={editingNodeId}
          onCommitRename={onCommitRename}
          onCancelRename={onCancelRename}
          draggingNodeId={draggingNodeId}
          dropTarget={dropTarget}
          onDragStartNode={onDragStartNode}
          onDragOverNode={onDragOverNode}
          onDropNode={onDropNode}
          onDragEndNode={onDragEndNode}
        />
      ))}
    </div>
  );
}
