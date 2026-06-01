import { ChevronLeft, Layers } from "lucide-react";
import type { TreeNode } from "../hooks/useFirstLeaf";
import { flatten } from "../hooks/useFirstLeaf";

interface Props {
  tree: TreeNode[];
  currentId: string;
  collapsed: boolean;
  onToggleCollapse: () => void;
  onSelect: (node: TreeNode) => void;
}

export function OutlinePanel({ tree, currentId, collapsed, onToggleCollapse, onSelect }: Props) {
  if (collapsed) {
    const scenes = flatten(tree).filter((n) => n.kind === "leaf");
    return (
      <nav className="rail is-collapsed">
        <div className="rail-head">
          <button type="button" className="rail-collapse" onClick={onToggleCollapse} title="아웃라인 펼치기">
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
              title={`${s.label}${s.title ? ` · ${s.title}` : ""}`}
            />
          ))}
        </div>
      </nav>
    );
  }

  return (
    <nav className="rail">
      <div className="rail-head">
        <span className="lbl">아웃라인</span>
        <button type="button" className="rail-collapse" onClick={onToggleCollapse} title="접기">
          <ChevronLeft size={15} />
        </button>
      </div>
      <div className="rail-tree">
        {tree.map((root) => (
          <RailNode key={root.id} node={root} depth={0} currentId={currentId} onSelect={onSelect} />
        ))}
      </div>
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
}: {
  node: TreeNode;
  depth: number;
  currentId: string;
  onSelect: (n: TreeNode) => void;
}) {
  if (node.kind === "leaf") {
    const active = node.id === currentId;
    return (
      <button
        type="button"
        className={`tree-scene${active ? " active" : ""}`}
        onClick={() => onSelect(node)}
      >
        <span className="sc-label">{node.label}</span>
        <span className="sc-title">{node.title}</span>
        <span className="sc-words">{node.word_count}</span>
      </button>
    );
  }

  // Containers: top-level → part header, otherwise chapter header.
  const header =
    depth === 0 ? (
      <div className="tree-part">
        {node.label}
        {node.title ? ` · ${node.title}` : ""}
      </div>
    ) : (
      <div className="tree-chapter">
        <span className="ch-label">{node.label}</span>
        {node.title && <span className="ch-title">{node.title}</span>}
      </div>
    );

  return (
    <div>
      {header}
      {node.children.map((c) => (
        <RailNode key={c.id} node={c} depth={depth + 1} currentId={currentId} onSelect={onSelect} />
      ))}
    </div>
  );
}
