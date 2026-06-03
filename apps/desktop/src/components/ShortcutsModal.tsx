import { useEffect } from "react";
import { useI18n } from "../lib/i18n";
import "./ShortcutsModal.css";

interface Shortcut {
  keys: string;
  labelKey: string;
}

const SHORTCUTS: Shortcut[] = [
  { keys: "⌘P", labelKey: "shortcuts.commandPalette" },
  { keys: "⌘S", labelKey: "shortcuts.manualSnapshot" },
  { keys: "⌘.", labelKey: "shortcuts.exitZenDialog" },
  { keys: "esc", labelKey: "shortcuts.escape" },
  { keys: "⌘⇧F", labelKey: "shortcuts.focusToggle" },
  { keys: "⌘Z", labelKey: "shortcuts.undoBody" },
  { keys: "⌘⇧Z", labelKey: "shortcuts.redoBody" },
  { keys: "@", labelKey: "shortcuts.mentionSearch" },
  { keys: "esc (✱)", labelKey: "shortcuts.closeNote" },
];

interface Props {
  open: boolean;
  onClose: () => void;
}

export function ShortcutsModal({ open, onClose }: Props) {
  const { t } = useI18n();
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="backdrop center" onClick={onClose}>
      <div
        className="modal sc-modal"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={t("shortcuts.helpLabel")}
      >
        <h2>{t("shortcuts.title")}</h2>
        <p className="modal-sub">{t("shortcuts.subtitle")}</p>
        <div className="sc-grid">
          {SHORTCUTS.map((s) => (
            <div className="sc-item" key={s.keys}>
              <span>{t(s.labelKey)}</span>
              <span className="kbd">{s.keys}</span>
            </div>
          ))}
        </div>
        <div className="modal-actions">
          <button type="button" className="btn accent" onClick={onClose}>{t("common.close")}</button>
        </div>
      </div>
    </div>
  );
}
