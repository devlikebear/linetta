import { useEffect, useMemo, useRef, useState } from "react";
import { Command as CommandIcon, Circle } from "lucide-react";
import { useI18n } from "../lib/i18n";
import "./CommandPalette.css";

export interface Command {
  id: string;
  section: string;
  label: string;
  hint?: string;          // right-side text (shortcut hint, etc.)
  disabled?: boolean;
  run: () => void | Promise<void>;
}

interface Props {
  open: boolean;
  onClose: () => void;
  commands: Command[];
}

export function CommandPalette({ open, onClose, commands }: Props) {
  const { t } = useI18n();
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
    <div className="backdrop top" onClick={onClose}>
      <div className="palette" onClick={(e) => e.stopPropagation()} onKeyDown={onKeyDown}>
        <div className="palette-input-wrap">
          <span className="ic"><CommandIcon size={17} /></span>
          <input
            ref={inputRef}
            className="palette-input"
            placeholder={t("workspace.command.searchPlaceholder")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <div ref={listRef} className="palette-list">
          {groups.length === 0 && <p className="palette-empty">{t("workspace.command.noResults")}</p>}
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
                    <span className="pic"><Circle size={8} /></span>
                    <span className="palette-label">{c.label}</span>
                    {c.hint && <span className="palette-hint">{c.hint}</span>}
                  </button>
                );
              })}
            </div>
          ))}
        </div>
        <div className="palette-foot">
          <span><span className="kbd">↑↓</span> {t("workspace.command.move")}</span>
          <span><span className="kbd">↵</span> {t("workspace.command.run")}</span>
          <span><span className="kbd">esc</span> {t("common.close")}</span>
        </div>
      </div>
    </div>
  );
}
