import { useCallback, useEffect, useRef, useState } from "react";
import type { TreeNode } from "../hooks/useFirstLeaf";
import "./OutlinePanel.css";

interface Props {
  tree: TreeNode[];
  currentId: string;
  onSelect: (node: TreeNode) => void;
  /** Fires every time the panel transitions from open to closed (manual or auto). */
  onClose?: () => void;
}

const HOT_ZONE_PX = 16;
const RETRACT_AFTER_MS = 3000;

export function OutlinePanel({ tree, currentId, onSelect, onClose }: Props) {
  const [open, setOpen] = useState(false);
  const retractTimer = useRef<number | undefined>(undefined);
  const closeRef = useRef(onClose);
  useEffect(() => { closeRef.current = onClose; }, [onClose]);

  const closePanel = useCallback(() => {
    if (retractTimer.current) {
      window.clearTimeout(retractTimer.current);
      retractTimer.current = undefined;
    }
    setOpen((prev) => {
      if (prev) closeRef.current?.();
      return false;
    });
  }, []);

  // Hover near the left edge to reveal.
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

  // ESC closes immediately when open.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        closePanel();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, closePanel]);

  // Clicking outside the panel (and outside the hot zone) closes immediately.
  // We do NOT preventDefault so the click still reaches the editor and refocuses it.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      const target = e.target as HTMLElement;
      if (target.closest(".outline-panel") || target.closest(".outline-hot-zone")) return;
      closePanel();
    };
    window.addEventListener("mousedown", onDown);
    return () => window.removeEventListener("mousedown", onDown);
  }, [open, closePanel]);

  const handleMouseLeave = () => {
    if (retractTimer.current) window.clearTimeout(retractTimer.current);
    retractTimer.current = window.setTimeout(() => closePanel(), RETRACT_AFTER_MS);
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
        <header className="outline-head">
          <span>아웃라인</span>
          <button
            type="button"
            className="outline-close"
            onClick={closePanel}
            aria-label="아웃라인 닫기"
            title="닫기 (Esc)"
          >
            ×
          </button>
        </header>
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
