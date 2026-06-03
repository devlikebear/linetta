import "./MentionPicker.css";
import type { MentionPickerState } from "./MentionExtension";
import { useI18n } from "../../lib/i18n";

interface Props {
  state: MentionPickerState | null;
}

export function MentionPicker({ state }: Props) {
  const { t } = useI18n();
  if (!state || !state.open) return null;
  return (
    <div
      className="mention-picker"
      style={{ left: state.position.left, top: state.position.top }}
      onMouseDown={(e) => e.preventDefault()} // keep focus in the editor
    >
      {state.items.length === 0 && (
        <p className="mention-picker-empty">{t("mention.noResults")}</p>
      )}
      {state.items.map((item, i) => {
        const active = i === state.selectedIndex;
        return (
          <button
            type="button"
            key={item.id ?? `new-${item.name}`}
            className={`mention-row${active ? " active" : ""}${item.isNew ? " new" : ""}`}
            onClick={() => state.pickAt(i)}
          >
            <span className="mention-name">
              {item.isNew ? t("mention.create", { name: item.name }) : item.name}
            </span>
            {!item.isNew && item.role && <span className="mention-role">{item.role}</span>}
          </button>
        );
      })}
    </div>
  );
}
