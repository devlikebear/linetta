import { useState } from "react";
import { ListChecks, Pencil } from "lucide-react";
import type { CompanionChoices } from "../../lib/types";
import { useI18n } from "../../lib/i18n";

interface Props {
  choices: CompanionChoices;
  /** True while a run is streaming — picking is blocked until it settles. */
  disabled?: boolean;
  /** Send the picked option text verbatim as the writer's next reply. */
  onPick: (text: string) => void;
  /** Focus the message input so the writer can type their own answer. */
  onCustom: () => void;
}

// ChoiceCard renders a linetta-choices block as a pick-one button list. A click
// sends the option immediately (single-select); afterward the card locks to the
// chosen option so it reads as a settled answer in the transcript.
export function ChoiceCard({ choices, disabled, onPick, onCustom }: Props) {
  const { t } = useI18n();
  const [picked, setPicked] = useState<string | null>(null);
  const done = picked !== null;

  const pick = (opt: string) => {
    if (done || disabled) return;
    setPicked(opt);
    onPick(opt);
  };

  return (
    <div className={`choice-card${done ? " done" : ""}`}>
      {choices.prompt && (
        <div className="choice-ttl"><ListChecks size={14} /> {choices.prompt}</div>
      )}
      <div className="choice-opts">
        {choices.options.map((opt, i) => (
          <button
            key={i}
            type="button"
            className={`choice-opt${picked === opt ? " picked" : ""}`}
            onClick={() => pick(opt)}
            disabled={done || disabled}
          >
            {opt}
          </button>
        ))}
        {choices.allow_custom && (
          <button
            type="button"
            className="choice-opt custom"
            onClick={onCustom}
            disabled={done || disabled}
          >
            <Pencil size={12} /> {t("companion.choice.custom")}
          </button>
        )}
      </div>
    </div>
  );
}
