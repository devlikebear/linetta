import { useEffect, useState, type MouseEvent } from "react";
import { ChevronLeft, FilePlus2, FolderPlus, Layers, MoreHorizontal, Pencil, Trash2, ArrowUp, ArrowDown } from "lucide-react";
import type { TreeNode } from "../hooks/useFirstLeaf";
import { flatten } from "../hooks/useFirstLeaf";
import { displayNodeLabel, useI18n } from "../lib/i18n";

interface Props {
  tree: TreeNode[];
  currentId: string;
  collapsed: boolean;
  onToggleCollapse: () => void;
  onSelect: (node: TreeNode) => void;
  onRename?: (node: TreeNode) => void;
  onCreateScene?: (node: TreeNode) => void;
  onCreateChapter?: (node: TreeNode) => void;
  onMoveSceneUp?: (node: TreeNode) => void;
  onMoveSceneDown?: (node: TreeNode) => void;
  onDeleteScene?: (node: TreeNode) => void;
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
  onCreateChapter,
  onMoveSceneUp,
  onMoveSceneDown,
  onDeleteScene,
}: Props) {
  const { language, t } = useI18n();
  const [menu, setMenu] = useState<MenuState | null>(null);
  const canOpenMenu = (node: TreeNode) =>
    Boolean(
      onRename ||
        onCreateScene ||
        onCreateChapter ||
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
      <nav className="rail is-collapsed">
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
    <nav className="rail">
      <div className="rail-head">
        <span className="lbl">{t("workspace.outline")}</span>
        <button type="button" className="rail-collapse" onClick={onToggleCollapse} title={t("workspace.collapse")}>
          <ChevronLeft size={15} />
        </button>
      </div>
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
          {onCreateScene && (
            <button type="button" role="menuitem" onClick={() => runAction(onCreateScene)}>
              <FilePlus2 size={13} /> {t("workspace.newScene")}
            </button>
          )}
          {onCreateChapter && (
            <button type="button" role="menuitem" onClick={() => runAction(onCreateChapter)}>
              <FolderPlus size={13} /> {t("workspace.newChapter")}
            </button>
          )}
          {menu.node.kind === "leaf" && (onMoveSceneUp || onMoveSceneDown || onDeleteScene) && (
            <div className="outline-menu-sep" role="separator" />
          )}
          {menu.node.kind === "leaf" && onMoveSceneUp && (
            <button type="button" role="menuitem" onClick={() => runAction(onMoveSceneUp)}>
              <ArrowUp size={13} /> {t("workspace.moveUp")}
            </button>
          )}
          {menu.node.kind === "leaf" && onMoveSceneDown && (
            <button type="button" role="menuitem" onClick={() => runAction(onMoveSceneDown)}>
              <ArrowDown size={13} /> {t("workspace.moveDown")}
            </button>
          )}
          {menu.node.kind === "leaf" && onDeleteScene && (
            <button type="button" role="menuitem" className="danger" onClick={() => runAction(onDeleteScene)}>
              <Trash2 size={13} /> {t("workspace.delete")}
            </button>
          )}
        </div>
      )}
    </nav>
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
