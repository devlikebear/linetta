import { CheckSquare, Square } from "lucide-react";
import { useMemo, useState } from "react";
import { useI18n } from "../../lib/i18n";
import type { ApplyReplaceResult, ReplacePlan } from "../../lib/types";
import "./BatchReplaceReview.css";

interface Props {
  plan: ReplacePlan;
  applying?: boolean;
  result?: ApplyReplaceResult | null;
  onApply: (candidateIds: string[]) => void;
}

export function BatchReplaceReview({ plan, applying, result, onApply }: Props) {
  const { t } = useI18n();
  const [selected, setSelected] = useState<Set<string>>(() => new Set(plan.candidates.filter((c) => c.selected).map((c) => c.id)));

  const selectedIds = useMemo(() => plan.candidates.filter((c) => selected.has(c.id)).map((c) => c.id), [plan.candidates, selected]);

  const toggle = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectAll = () => setSelected(new Set(plan.candidates.map((c) => c.id)));
  const clearAll = () => setSelected(new Set());

  return (
    <section className="batch-review" aria-label={t("contextual.candidates.title")}>
      <div className="batch-review-head">
        <h4>{t("contextual.candidates.title")}</h4>
        <span>{t("contextual.candidates.count", { count: String(plan.candidates.length) })}</span>
      </div>

      {plan.candidates.length === 0 ? (
        <p className="contextual-empty">{t("contextual.candidates.empty")}</p>
      ) : (
        <>
          <div className="batch-review-tools">
            <button type="button" onClick={selectAll}>{t("contextual.selectAll")}</button>
            <button type="button" onClick={clearAll}>{t("contextual.deselectAll")}</button>
          </div>
          <div className="batch-candidates">
            {plan.candidates.map((candidate) => {
              const isSelected = selected.has(candidate.id);
              return (
                <button
                  key={candidate.id}
                  type="button"
                  className={`batch-candidate${isSelected ? " is-selected" : ""}`}
                  onClick={() => toggle(candidate.id)}
                  aria-pressed={isSelected}
                >
                  <span className="batch-candidate-check" aria-hidden="true">
                    {isSelected ? <CheckSquare size={15} /> : <Square size={15} />}
                  </span>
                  <span className="batch-candidate-body">
                    <span className="batch-candidate-top">
                      <span className="batch-candidate-path">{candidate.breadcrumb}</span>
                      <span className="batch-candidate-count">{t("contextual.occurrences", { count: String(candidate.occurrences) })}</span>
                    </span>
                    <span className="batch-diff">
                      <span className="batch-before">{candidate.before}</span>
                      <span className="batch-after">{candidate.after}</span>
                    </span>
                  </span>
                </button>
              );
            })}
          </div>
          <button
            type="button"
            className="batch-apply"
            onClick={() => onApply(selectedIds)}
            disabled={applying || selectedIds.length === 0}
          >
            {applying ? t("contextual.apply.applying") : t("contextual.applySelected")}
          </button>
        </>
      )}

      {result && (
        <p className={result.failures.length > 0 ? "batch-result has-failures" : "batch-result"}>
          {result.failures.length > 0
            ? t("contextual.apply.failed", {
              applied: String(result.applied),
              failed: String(result.failures.length),
            })
            : t("contextual.apply.applied", { count: String(result.applied) })}
        </p>
      )}
    </section>
  );
}
