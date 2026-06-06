import { useEffect, useState, type MouseEvent } from "react";
import { AlertTriangle, ChevronLeft, FilePlus2, FolderPlus, Layers, MoreHorizontal, Pencil, Stethoscope, Trash2, ArrowUp, ArrowDown } from "lucide-react";
import type { TreeNode } from "../hooks/useFirstLeaf";
import { flatten } from "../hooks/useFirstLeaf";
import { displayNodeLabel, useI18n } from "../lib/i18n";
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
  onRename?: (node: TreeNode) => void;
  onCreateScene?: (node: TreeNode) => void;
  onCreatePart?: (node: TreeNode) => void;
  onCreateChapter?: (node: TreeNode) => void;
  onMoveSceneUp?: (node: TreeNode) => void;
  onMoveSceneDown?: (node: TreeNode) => void;
  onDeleteScene?: (node: TreeNode) => void;
  onMoveNodeUp?: (node: TreeNode) => void;
  onMoveNodeDown?: (node: TreeNode) => void;
  onDeleteNode?: (node: TreeNode) => void;
  onRepairOutline?: () => void;
  onUndoRepairOutline?: () => void;
  canUndoRepair?: boolean;
  outlinePresetId?: OutlinePresetId;
  onOutlinePresetChange?: (presetId: OutlinePresetId) => void;
  tourTarget?: string;
}

type MenuState = {
  node: TreeNode;
  x: number;
  y: number;
};

export function OutlinePanel({
  tree,
  currentId,
  collapsed,
  onToggleCollapse,
  onSelect,
  onRename,
  onCreateScene,
  onCreatePart,
  onCreateChapter,
  onMoveSceneUp,
  onMoveSceneDown,
  onDeleteScene,
  onMoveNodeUp,
  onMoveNodeDown,
  onDeleteNode,
  onRepairOutline,
  onUndoRepairOutline,
  canUndoRepair,
  outlinePresetId,
  onOutlinePresetChange,
  tourTarget,
}: Props) {
  const { language, t } = useI18n();
  const [menu, setMenu] = useState<MenuState | null>(null);
  const [doctorOpen, setDoctorOpen] = useState(false);
  const outlinePreset = outlinePresetById(outlinePresetId);
  const outlineIssues = analyzeOutline(tree, t, outlinePreset);
  const partName = outlineRoleName(outlinePreset, "part", t);
  const chapterName = outlineRoleName(outlinePreset, "chapter", t);
  const canOpenMenu = (node: TreeNode) =>
    Boolean(
      onRename ||
        onCreatePart ||
        onCreateScene ||
        onCreateChapter ||
        onMoveNodeUp ||
        onMoveNodeDown ||
        onDeleteNode ||
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
            <button type="button" role="menuitem" onClick={() => runAction(onRename)}>
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
  const visit = (nodes: TreeNode[], depth: number, parentKey: string) => {
    const seen = new Map<string, TreeNode[]>();
    nodes.forEach((node, index) => {
      const label = node.label.trim();
      if (label) {
        const list = seen.get(label) ?? [];
        list.push(node);
        seen.set(label, list);
      }
      if (node.kind === "container" && node.children.length === 0) {
        issues.push({ key: `empty-${node.id}`, text: t("workspace.outlineIssue.empty", issueValues(node.label)) });
      }
      if (node.kind === "leaf" && node.word_count === 0 && isStructuralChapterLabel(node.label, preset)) {
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
      if (depth === 1 && node.kind === "leaf") {
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
}: {
  node: TreeNode;
  depth: number;
  currentId: string;
  onSelect: (n: TreeNode) => void;
  onOpenMenu: (e: MouseEvent, n: TreeNode) => void;
  canOpenMenu: (n: TreeNode) => boolean;
}) {
  const { language, t } = useI18n();
  const hasMenu = canOpenMenu(node);
  const label = displayNodeLabel(language, node.label);

  if (node.kind === "leaf") {
    const active = node.id === currentId;
    return (
      <div className={`tree-scene-row${active ? " active" : ""}`} onContextMenu={(e) => onOpenMenu(e, node)}>
        <button
          type="button"
          className={`tree-scene${active ? " active" : ""}`}
          onClick={() => onSelect(node)}
        >
          <span className="sc-label">{label}</span>
          <span className="sc-title">{node.title}</span>
          <span className="sc-words">{node.word_count}</span>
        </button>
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
      <div className="tree-part-row" onContextMenu={(e) => onOpenMenu(e, node)}>
        <div className="tree-part">
          {label}
          {node.title ? ` · ${node.title}` : ""}
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
      <div className="tree-chapter-row" onContextMenu={(e) => onOpenMenu(e, node)}>
        <div className="tree-chapter">
          <span className="ch-label">{label}</span>
          {node.title && <span className="ch-title">{node.title}</span>}
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
        />
      ))}
    </div>
  );
}
