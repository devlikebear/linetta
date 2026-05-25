import { useEffect, useMemo, useRef, useState } from "react";
import "./CommandPalette.css";

export interface Command {
  id: string;
  section: string;
  label: string;
  hint?: string;          // right-side text (shortcut, "(곧 추가됨)", etc.)
  disabled?: boolean;
  run: () => void | Promise<void>;
}

interface Props {
  open: boolean;
  onClose: () => void;
  commands: Command[];
}

export function CommandPalette({ open, onClose, commands }: Props) {
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (open) {
      setQuery("");
      setActive(0);
      window.setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [open]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return commands;
    return commands.filter((c) => c.label.toLowerCase().includes(q) || c.section.toLowerCase().includes(q));
  }, [commands, query]);

  useEffect(() => {
    if (active >= filtered.length) setActive(0);
  }, [filtered, active]);

  if (!open) return null;

  const runIndex = (i: number) => {
    const c = filtered[i];
    if (!c || c.disabled) return;
    onClose();
    Promise.resolve(c.run()).catch((e) => console.error("palette command failed:", e));
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((i) => Math.min(filtered.length - 1, i + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((i) => Math.max(0, i - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      runIndex(active);
    }
  };

  // Group filtered commands by section for display.
  const groups: { section: string; items: Command[] }[] = [];
  for (const c of filtered) {
    const last = groups[groups.length - 1];
    if (last && last.section === c.section) {
      last.items.push(c);
    } else {
      groups.push({ section: c.section, items: [c] });
    }
  }

  let globalIdx = -1;

  return (
    <div className="palette-backdrop" onClick={onClose}>
      <div className="palette" onClick={(e) => e.stopPropagation()} onKeyDown={onKeyDown}>
        <input
          ref={inputRef}
          className="palette-input"
          placeholder="검색…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <div ref={listRef} className="palette-list">
          {groups.length === 0 && <p className="palette-empty">결과 없음</p>}
          {groups.map((g) => (
            <div key={g.section} className="palette-group">
              <p className="palette-section">{g.section}</p>
              {g.items.map((c) => {
                globalIdx++;
                const isActive = globalIdx === active;
                const idx = globalIdx;
                return (
                  <button
                    key={c.id}
                    className={`palette-row${isActive ? " active" : ""}${c.disabled ? " disabled" : ""}`}
                    onMouseMove={() => setActive(idx)}
                    onClick={() => runIndex(idx)}
                    disabled={c.disabled}
                  >
                    <span className="palette-label">{c.label}</span>
                    {c.hint && <span className="palette-hint">{c.hint}</span>}
                  </button>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
