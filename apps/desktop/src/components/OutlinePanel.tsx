import { useEffect, useRef, useState } from "react";
import type { TreeNode } from "../hooks/useFirstLeaf";
import "./OutlinePanel.css";

interface Props {
  tree: TreeNode[];
  currentId: string;
  onSelect: (node: TreeNode) => void;
}

const HOT_ZONE_PX = 16;
const RETRACT_AFTER_MS = 3000;

export function OutlinePanel({ tree, currentId, onSelect }: Props) {
  const [open, setOpen] = useState(false);
  const retractTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (e.clientX <= HOT_ZONE_PX) {
        setOpen(true);
        if (retractTimer.current) {
          window.clearTimeout(retractTimer.current);
          retractTimer.current = undefined;
        }
      }
    };
    window.addEventListener("mousemove", onMove);
    return () => window.removeEventListener("mousemove", onMove);
  }, []);

  const handleMouseLeave = () => {
    if (retractTimer.current) window.clearTimeout(retractTimer.current);
    retractTimer.current = window.setTimeout(() => setOpen(false), RETRACT_AFTER_MS);
  };

  const handleMouseEnter = () => {
    if (retractTimer.current) {
      window.clearTimeout(retractTimer.current);
      retractTimer.current = undefined;
    }
  };

  return (
    <>
      <div className="outline-hot-zone" aria-hidden />
      <aside
        className={`outline-panel${open ? " open" : ""}`}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
      >
        <header className="outline-head">아웃라인</header>
        <ul className="outline-tree">
          {tree.map((root) => (
            <OutlineRow key={root.id} node={root} depth={0} currentId={currentId} onSelect={onSelect} />
          ))}
        </ul>
      </aside>
    </>
  );
}

function OutlineRow({
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
  const active = node.id === currentId;
  return (
    <>
      <li
        className={`outline-row${active ? " active" : ""}${node.kind === "container" ? " container" : ""}`}
        style={{ paddingLeft: 0.75 + depth * 1.1 + "rem" }}
        onClick={() => onSelect(node)}
        role="button"
      >
        <span className="outline-label">{node.label}</span>
        {node.title && <span className="outline-title">. {node.title}</span>}
      </li>
      {node.children.map((c) => (
        <OutlineRow key={c.id} node={c} depth={depth + 1} currentId={currentId} onSelect={onSelect} />
      ))}
    </>
  );
}
