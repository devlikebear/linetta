import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import "./AIContextChecklist.css";
import type { AIContextKey, AIContextPreview, AIContextSelection, ContextCounts } from "../../lib/types";
import { useI18n } from "../../lib/i18n";

// Re-export so existing consumers that import { ContextCounts } from this file
// continue to work without changes.
export type { ContextCounts };

export const DEFAULT_AI_CONTEXT_SELECTION: AIContextSelection = {
  current_scene: true,
  overview: true,
  synopsis: true,
  nearby_scenes: true,
  related_scenes: true,
  plot: true,
  entities: true,
  relationships: true,
  notes: true,
  project_meta: true,
  style_notes: true,
  facts: true,
  memories: true,
};

interface Props {
  anchor: { top: number; left: number };
  preview: AIContextPreview;
  selection: AIContextSelection;
  onSelectionChange: (next: AIContextSelection) => void;
  onClose: () => void;
}

interface ListProps {
  preview: AIContextPreview;
  selection: AIContextSelection;
  onSelectionChange: (next: AIContextSelection) => void;
  disabled?: boolean;
}

export function AIContextChecklistList({ preview, selection, onSelectionChange, disabled = false }: ListProps) {
  const { t } = useI18n();
  const [openId, setOpenId] = useState<AIContextKey | null>(null);

  return (
    <ul className="ai-checklist">
      {preview.sections.map((section) => {
        const checked = section.present && selection[section.id];
        const canToggle = section.present && !disabled;
        return (
          <li key={section.id} className={section.present ? "" : "off"}>
            <div className="ai-context-row">
              <label className="ai-context-label">
                <input
                  className="ai-context-check"
                  type="checkbox"
                  checked={checked}
                  disabled={!canToggle}
                  onChange={(e) => onSelectionChange({ ...selection, [section.id]: e.target.checked })}
                />
                <span>{section.label}</span>
              </label>
              {section.count > 0 && <span className="n">{formatCount(section, t)}</span>}
              <button
                type="button"
                className="ai-preview-toggle"
                onClick={() => setOpenId((id) => (id === section.id ? null : section.id))}
                disabled={!section.present}
                aria-label={t("ai.context.preview", { label: section.label })}
              >
                {openId === section.id ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
              </button>
            </div>
            {openId === section.id && (
              <pre className="ai-context-preview">
                {section.preview || t("ai.context.empty")}
              </pre>
            )}
          </li>
        );
      })}
    </ul>
  );
}

function formatCount(section: { id: AIContextKey; count: number }, t: ReturnType<typeof useI18n>["t"]) {
  if (section.id === "project_meta") return `${section.count}/3`;
  return t("ai.context.count", { count: section.count });
}

export function AIContextChecklist({ anchor, preview, selection, onSelectionChange, onClose }: Props) {
  return (
    <>
      <div
        className="ai-context-checklist"
        style={{ top: anchor.top, left: anchor.left }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <AIContextChecklistList preview={preview} selection={selection} onSelectionChange={onSelectionChange} />
      </div>
      {/* invisible backdrop to capture outside click */}
      <div
        style={{ position: "fixed", inset: 0, zIndex: 55 }}
        onMouseDown={onClose}
      />
    </>
  );
}

export function totalContextItems(preview: AIContextPreview, selection: AIContextSelection): number {
  return preview.sections.reduce((sum, section) => {
    if (!section.present || !selection[section.id]) return sum;
    return sum + Math.max(section.count, 1);
  }, 0);
}
