import { AlertTriangle, CheckCircle2, CheckSquare, Square } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { contextual, entities } from "../../lib/rpc";
import type {
  ApplyContextResult,
  ConsistencyReport,
  ContextChangePlan,
  Entity,
  MetadataCandidate,
  ReplaceCandidate,
  ReplacePlan,
  ReviewCandidate,
} from "../../lib/types";
import { useI18n } from "../../lib/i18n";
import "./ContextChangeWizard.css";

interface Props {
  projectId: string;
  initialEntityId?: string | null;
  initialText?: string | null;
  autoCheck?: boolean;
  onApplied?: (changedNodeIds: string[]) => void;
}

type CandidateSelection = Record<string, Set<string>>;

export function ContextChangeWizard({ projectId, initialEntityId, initialText, autoCheck, onApplied }: Props) {
  const { language, t } = useI18n();
  const [oldTerm, setOldTerm] = useState("");
  const [newTerm, setNewTerm] = useState("");
  const [entityMatches, setEntityMatches] = useState<Entity[]>([]);
  const [selectedEntityId, setSelectedEntityId] = useState("");
  const [searchingEntities, setSearchingEntities] = useState(false);
  const [plan, setPlan] = useState<ContextChangePlan | null>(null);
  const [selectedMetadata, setSelectedMetadata] = useState<Set<string>>(new Set());
  const [selectedManuscript, setSelectedManuscript] = useState<CandidateSelection>({});
  const [planning, setPlanning] = useState(false);
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<ApplyContextResult | null>(null);
  const [report, setReport] = useState<ConsistencyReport | null>(null);
  const [autoCheckKey, setAutoCheckKey] = useState("");

  useEffect(() => {
    const text = initialText?.trim() ?? "";
    if (!text) return;
    setOldTerm(text);
  }, [initialText]);

  useEffect(() => {
    const entityId = initialEntityId?.trim() ?? "";
    if (!entityId) return;
    let cancelled = false;
    entities.get(entityId)
      .then((entity) => {
        if (cancelled) return;
        setEntityMatches((prev) => {
          const rest = prev.filter((item) => item.id !== entity.id);
          return [entity, ...rest];
        });
        setSelectedEntityId(entity.id);
        setOldTerm(entity.name);
      })
      .catch((err) => {
        if (!cancelled) setError(String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [initialEntityId]);

  useEffect(() => {
    const term = initialText?.trim() ?? "";
    if (!autoCheck || !term || autoCheckKey === term) return;
    setAutoCheckKey(term);
    setReport(null);
    contextual.checkConsistency({ project_id: projectId, old_terms: [term], language })
      .then((nextReport) => setReport(nextReport))
      .catch((err) => setError(String(err)));
  }, [autoCheck, autoCheckKey, initialText, projectId]);

  useEffect(() => {
    const query = oldTerm.trim();
    setPlan(null);
    setResult(null);
    setReport(null);
    setError("");
    if (!query) {
      setEntityMatches([]);
      setSearchingEntities(false);
      return;
    }
    let cancelled = false;
    const timer = window.setTimeout(() => {
      setSearchingEntities(true);
      entities.search(projectId, query, 8)
        .then((matches) => {
          if (!cancelled) setEntityMatches(matches);
        })
        .catch((err) => {
          if (!cancelled) {
            setEntityMatches([]);
            setError(String(err));
          }
        })
        .finally(() => {
          if (!cancelled) setSearchingEntities(false);
        });
    }, 180);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [oldTerm, projectId]);

  useEffect(() => {
    if (!plan) {
      setSelectedMetadata(new Set());
      setSelectedManuscript({});
      return;
    }
    setSelectedMetadata(new Set(plan.metadata_candidates.filter((c) => c.selected).map((c) => c.id)));
    const next: CandidateSelection = {};
    for (const manuscriptPlan of plan.manuscript_plans) {
      next[manuscriptPlan.id ?? manuscriptPlan.query] = new Set(
        manuscriptPlan.candidates.filter((c) => c.selected).map((c) => c.id),
      );
    }
    setSelectedManuscript(next);
  }, [plan]);

  const selectedEntity = entityMatches.find((entity) => entity.id === selectedEntityId);
  const selectedManuscriptCount = useMemo(() => {
    return Object.values(selectedManuscript).reduce((sum, ids) => sum + ids.size, 0);
  }, [selectedManuscript]);
  const hasSelectedCandidates = selectedMetadata.size > 0 || selectedManuscriptCount > 0;

  const makePlan = () => {
    const from = oldTerm.trim();
    const to = newTerm.trim();
    if ((!selectedEntityId && !from) || !to) return;
    setPlanning(true);
    setError("");
    setResult(null);
    setReport(null);
    contextual.planChange({
      project_id: projectId,
      type: "rename",
      language,
      ...(selectedEntityId
        ? { entity_id: selectedEntityId }
        : { selected_text: from, old_terms: [from] }),
      new_terms: [to],
    })
      .then((nextPlan) => setPlan(nextPlan))
      .catch((err) => {
        setPlan(null);
        setError(String(err));
      })
      .finally(() => setPlanning(false));
  };

  const applyPlan = () => {
    if (!plan || !hasSelectedCandidates) return;
    const manuscript_candidate_ids: Record<string, string[]> = {};
    for (const manuscriptPlan of plan.manuscript_plans) {
      const planId = manuscriptPlan.id ?? manuscriptPlan.query;
      const ids = [...(selectedManuscript[planId] ?? new Set<string>())];
      if (ids.length > 0) manuscript_candidate_ids[planId] = ids;
    }
    setApplying(true);
    setError("");
    contextual.applyChange(plan, {
      metadata_candidate_ids: [...selectedMetadata],
      manuscript_candidate_ids,
    })
      .then(async (applied) => {
        setResult(applied);
        const changedNodeIds = applied.manuscript.changed_node_ids ?? [];
        if (changedNodeIds.length > 0 || applied.metadata_applied > 0) {
          onApplied?.(changedNodeIds);
        }
        const nextReport = await contextual.checkConsistency({
          project_id: plan.project_id,
          old_terms: plan.old_terms,
          new_terms: plan.new_terms,
          changed_entity_ids: plan.target.entity_ids ?? [],
          language,
        });
        setReport(nextReport);
      })
      .catch((err) => setError(String(err)))
      .finally(() => setApplying(false));
  };

  const toggleMetadata = (candidate: MetadataCandidate) => {
    setSelectedMetadata((prev) => toggleSet(prev, candidate.id));
  };

  const toggleManuscript = (manuscriptPlan: ReplacePlan, candidate: ReplaceCandidate) => {
    const planId = manuscriptPlan.id ?? manuscriptPlan.query;
    setSelectedManuscript((prev) => {
      const next = { ...prev };
      next[planId] = toggleSet(next[planId] ?? new Set<string>(), candidate.id);
      return next;
    });
  };

  return (
    <div className="panel-scroll contextual-scroll context-change">
      <section className="contextual-section">
        <h4>{t("contextual.change.title")}</h4>
        <label className="contextual-input">
          <span>{t("contextual.change.oldTerm")}</span>
          <input
            aria-label={t("contextual.change.oldTerm")}
            value={oldTerm}
            placeholder={t("contextual.change.oldPlaceholder")}
            onChange={(event) => {
              setOldTerm(event.target.value);
              setSelectedEntityId("");
            }}
          />
        </label>
        <div className="context-target-list">
          {searchingEntities && <p className="contextual-empty compact">{t("contextual.change.searchingEntities")}</p>}
          {entityMatches.map((entity) => {
            const selected = entity.id === selectedEntityId;
            return (
              <button
                key={entity.id}
                type="button"
                className={selected ? "context-target is-selected" : "context-target"}
                aria-pressed={selected}
                onClick={() => setSelectedEntityId(selected ? "" : entity.id)}
              >
                <span>{entity.name}</span>
                <small>{entity.role || entity.kind}</small>
              </button>
            );
          })}
        </div>
        <label className="contextual-input contextual-project-replacement">
          <span>{t("contextual.change.newTerm")}</span>
          <input
            aria-label={t("contextual.change.newTerm")}
            value={newTerm}
            placeholder={t("contextual.change.newPlaceholder")}
            onChange={(event) => setNewTerm(event.target.value)}
          />
        </label>
        <button
          type="button"
          className="contextual-primary-action"
          disabled={planning || (!selectedEntityId && !oldTerm.trim()) || !newTerm.trim()}
          onClick={makePlan}
        >
          {planning ? t("contextual.change.planning") : t("contextual.change.plan")}
        </button>
        {selectedEntity && (
          <p className="contextual-hint">
            {t("contextual.change.entitySelected", { name: selectedEntity.name })}
          </p>
        )}
      </section>

      {error && <p className="contextual-empty error">{t("contextual.project.failed", { error })}</p>}

      {plan && (
        <>
          <ContextPlanSummary plan={plan} />
          {plan.metadata_candidates.length > 0 && (
            <section className="context-review-section">
              <h4>{t("contextual.change.metadata")}</h4>
              {plan.metadata_candidates.map((candidate) => (
                <CandidateButton
                  key={candidate.id}
                  selected={selectedMetadata.has(candidate.id)}
                  label={candidate.label}
                  before={candidate.before}
                  after={candidate.after}
                  onClick={() => toggleMetadata(candidate)}
                />
              ))}
            </section>
          )}
          {plan.manuscript_plans.length > 0 && (
            <section className="context-review-section">
              <h4>{t("contextual.change.manuscript")}</h4>
              {plan.manuscript_plans.map((manuscriptPlan) => (
                <div key={manuscriptPlan.id ?? manuscriptPlan.query} className="context-manuscript-plan">
                  <p className="context-plan-line">
                    {manuscriptPlan.query} → {manuscriptPlan.replacement}
                  </p>
                  {manuscriptPlan.candidates.map((candidate) => (
                    <CandidateButton
                      key={candidate.id}
                      selected={(selectedManuscript[manuscriptPlan.id ?? manuscriptPlan.query] ?? new Set()).has(candidate.id)}
                      label={`${candidate.breadcrumb} · ${t("contextual.occurrences", { count: String(candidate.occurrences) })}`}
                      before={candidate.before}
                      after={candidate.after}
                      onClick={() => toggleManuscript(manuscriptPlan, candidate)}
                    />
                  ))}
                </div>
              ))}
            </section>
          )}
          {plan.review_candidates.length > 0 && (
            <section className="context-review-section">
              <h4>{t("contextual.change.review")}</h4>
              {plan.review_candidates.map((candidate) => (
                <ReviewItem key={candidate.id} candidate={candidate} />
              ))}
            </section>
          )}
          {plan.warnings?.map((warning) => (
            <p key={warning} className="contextual-empty">{warning}</p>
          ))}
          <section className="context-apply-section">
            <button
              type="button"
              className="context-apply"
              disabled={applying || !hasSelectedCandidates}
              onClick={applyPlan}
            >
              {applying ? t("contextual.apply.applying") : t("contextual.applySelected")}
            </button>
            {result && (
              <p className={result.manuscript.failures.length > 0 || (result.failures?.length ?? 0) > 0 ? "context-result has-failures" : "context-result"}>
                {t("contextual.change.applied", {
                  metadata: String(result.metadata_applied),
                  scenes: String(result.manuscript.applied),
                })}
              </p>
            )}
          </section>
          {report && (
            <section className="context-review-section">
              <h4>{t("contextual.change.consistency")}</h4>
              {report.ok ? (
                <p className="context-ok"><CheckCircle2 size={14} /> {t("contextual.change.consistencyOk")}</p>
              ) : (
                report.issues.map((issue, index) => (
                  <div key={`${issue.kind}-${index}`} className="context-issue">
                    <AlertTriangle size={14} />
                    <span>
                      <strong>{issue.breadcrumb || issue.kind}</strong>
                      <small>{issue.message}</small>
                      {issue.snippet && <em>{issue.snippet}</em>}
                    </span>
                  </div>
                ))
              )}
            </section>
          )}
        </>
      )}
    </div>
  );
}

function ContextPlanSummary({ plan }: { plan: ContextChangePlan }) {
  const { t } = useI18n();
  const manuscriptCount = plan.manuscript_plans.reduce((sum, p) => sum + p.candidates.length, 0);
  return (
    <section className="context-plan-summary">
      <span>{t("contextual.change.planSummaryTarget", { name: plan.target.canonical_name })}</span>
      <span>{t("contextual.change.planSummaryCounts", {
        metadata: String(plan.metadata_candidates.length),
        manuscript: String(manuscriptCount),
        review: String(plan.review_candidates.length),
      })}</span>
    </section>
  );
}

function CandidateButton({
  selected,
  label,
  before,
  after,
  onClick,
}: {
  selected: boolean;
  label: string;
  before: string;
  after: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={selected ? "context-candidate is-selected" : "context-candidate"}
      aria-pressed={selected}
      onClick={onClick}
    >
      <span className="context-check" aria-hidden="true">
        {selected ? <CheckSquare size={15} /> : <Square size={15} />}
      </span>
      <span className="context-candidate-body">
        <strong>{label}</strong>
        <span className="context-diff">
          <span>{before}</span>
          <span>{after}</span>
        </span>
      </span>
    </button>
  );
}

function ReviewItem({ candidate }: { candidate: ReviewCandidate }) {
  return (
    <div className="context-review-item">
      <strong>{candidate.label}</strong>
      <span>{candidate.snippet}</span>
    </div>
  );
}

function toggleSet(source: Set<string>, id: string): Set<string> {
  const next = new Set(source);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  return next;
}
