import { useEffect } from "react";
import { useI18n, type MessageKey } from "../lib/i18n";
import "./ShortcutsModal.css";

interface Shortcut {
  keys: string;
  labelKey: MessageKey;
}

// Cmd+I (AI draft) is deliberately absent: that feature was removed with the
// companion and Workspace leaves the key unbound, so listing it here would
// advertise a shortcut that does nothing. Cmd+J used to be absent for the
// same reason; it is not any more, now that it opens the agent panel (#95).
const SHORTCUTS: Shortcut[] = [
  { keys: "⌘P", labelKey: "shortcuts.commandPalette" },
  { keys: "⌘S", labelKey: "shortcuts.manualSnapshot" },
  { keys: "⌘F", labelKey: "shortcuts.globalSearch" },
  { keys: "⌘.", labelKey: "shortcuts.exitZenDialog" },
  { keys: "esc", labelKey: "shortcuts.escape" },
  { keys: "⌘⇧F", labelKey: "workspace.command.contextualEdit" },
  { keys: "⌘J", labelKey: "workspace.command.agentPanel" },
  { keys: "⌘Z", labelKey: "shortcuts.undoBody" },
  { keys: "⌘⇧Z", labelKey: "shortcuts.redoBody" },
  { keys: "@", labelKey: "shortcuts.mentionSearch" },
  { keys: "esc (✱)", labelKey: "shortcuts.closeNote" },
];

interface Props {
  open: boolean;
  onClose: () => void;
  // agent_available: an iPad build can ship with no provider plumbed in at
  // all, and Cmd+J does nothing when it isn't. Advertising the key anyway
  // would send the writer to open a panel that can't open (#95).
  agentAvailable: boolean;
}

export function ShortcutsModal({ open, onClose, agentAvailable }: Props) {
  const { t } = useI18n();
  const shortcuts = agentAvailable ? SHORTCUTS : SHORTCUTS.filter((s) => s.keys !== "⌘J");
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
          {shortcuts.map((s) => (
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
