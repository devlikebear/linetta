import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, ChevronDown, ChevronUp, Compass, Trash2, X } from "lucide-react";

import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { forecast as forecastApi } from "../lib/rpc";
import type { Forecast, ForecastBranch } from "../lib/types";
import { useI18n, type MessageKey } from "../lib/i18n";
import { useToast } from "./ToastProvider";
import { rpcErrorMessage } from "../lib/rpcMessage";
import "./ForecastSheet.css";

/** "Where the story could go" (story-assist Phase 4): possible
 *  continuations an agent drafted with linetta_save_forecast, laid out
 *  side by side for the writer to compare. Never canon — picking a branch
 *  only sets a plan in the brief's Direction section; reflecting it into
 *  the outline goes through the suggestions inbox, via
 *  [개요 반영 제안 요청] below. */

interface Props {
  projectId: string;
  onClose: () => void;
  onOpenEntity?: (entityId: string) => void;
  onOpenFact?: (factId: string) => void;
  onOpenNode?: (nodeId: string) => void;
  /** Fills the agent panel's composer with a ready-to-send request to
   *  reflect the chosen branch into the outline (purpose=forecast). The
   *  writer still presses send, and still applies the resulting proposal
   *  themselves in the suggestions inbox. */
  onRequestOutlineReflection: (promptText: string) => void;
}

export function ForecastSheet({ projectId, onClose, onOpenEntity, onOpenFact, onOpenNode, onRequestOutlineReflection }: Readonly<Props>) {
  const { t } = useI18n();
  const { showToast } = useToast();
  const [forecasts, setForecasts] = useState<Forecast[] | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [revision, setRevision] = useState(0);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  // linetta_save_forecast is the only write; direct select/clear/delete
  // RPCs below update local state themselves and do not need this.
  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.project_id && event.project_id !== projectId) return;
    if (event.tool === "linetta_save_forecast") setRevision((n) => n + 1);
  });

  const load = useCallback(() => {
    setError(null);
    return forecastApi
      .list(projectId)
      .then((res) => setForecasts(res.forecasts))
      .catch((e: unknown) => setError(e));
  }, [projectId]);

  useEffect(() => {
    void load();
  }, [load, revision]);

  const toggleExpanded = useCallback((key: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const chooseBranch = useCallback(
    async (f: Forecast, branch: ForecastBranch) => {
      setBusyId(f.id);
      try {
        const res = await forecastApi.select(f.id, branch.id);
        if (res.stale) {
          showToast(t("forecast.toast.selectedStale"));
        } else {
          showToast(t("forecast.toast.selected"));
        }
        setRevision((n) => n + 1);
      } catch (e) {
        showToast(rpcErrorMessage(e, t));
      } finally {
        setBusyId(null);
      }
    },
    [showToast, t],
  );

  const deselect = useCallback(
    async (f: Forecast) => {
      setBusyId(f.id);
      try {
        await forecastApi.clearSelection(projectId);
        showToast(t("forecast.toast.deselected"));
        setRevision((n) => n + 1);
      } catch (e) {
        showToast(rpcErrorMessage(e, t));
      } finally {
        setBusyId(null);
      }
    },
    [projectId, showToast, t],
  );

  const deleteForecast = useCallback(
    async (f: Forecast) => {
      setBusyId(f.id);
      try {
        await forecastApi.delete(f.id);
        showToast(t("forecast.toast.deleted"));
        setRevision((n) => n + 1);
      } catch (e) {
        showToast(rpcErrorMessage(e, t));
      } finally {
        setBusyId(null);
      }
    },
    [showToast, t],
  );

  const requestReflection = useCallback(
    (f: Forecast, branch: ForecastBranch) => {
      const prompt = t("forecast.reflectInOutlinePrompt", { title: branch.title, id: f.id });
      onRequestOutlineReflection(prompt);
    },
    [onRequestOutlineReflection, t],
  );

  const sorted = useMemo(
    () => (forecasts ?? []).slice().sort((a, b) => b.created_at - a.created_at),
    [forecasts],
  );

  return (
    <aside className="panel forecast-sheet" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl">
          <Compass size={15} className="fs-title-ic" />
          {t("forecast.title")}
        </span>
        <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}>
          <X size={16} />
        </button>
      </div>

      {error ? (
        <p className="fs-error" role="alert">
          {rpcErrorMessage(error, t)}
        </p>
      ) : null}

      {forecasts === null ? (
        <p className="fs-loading">{t("common.loading")}</p>
      ) : sorted.length === 0 ? (
        <p className="fs-empty">{t("forecast.empty")}</p>
      ) : (
        <div className="fs-list" data-testid="forecast-list">
          {sorted.map((f) => (
            <ForecastCard
              key={f.id}
              forecast={f}
              busy={busyId === f.id}
              expanded={expanded}
              onToggleExpanded={toggleExpanded}
              onChoose={(branch) => chooseBranch(f, branch)}
              onDeselect={() => deselect(f)}
              onDelete={() => deleteForecast(f)}
              onRequestReflection={(branch) => requestReflection(f, branch)}
              onOpenEntity={onOpenEntity}
              onOpenFact={onOpenFact}
              onOpenNode={onOpenNode}
            />
          ))}
        </div>
      )}
    </aside>
  );
}

function ForecastCard({
  forecast: f,
  busy,
  expanded,
  onToggleExpanded,
  onChoose,
  onDeselect,
  onDelete,
  onRequestReflection,
  onOpenEntity,
  onOpenFact,
  onOpenNode,
}: Readonly<{
  forecast: Forecast;
  busy: boolean;
  expanded: Set<string>;
  onToggleExpanded: (key: string) => void;
  onChoose: (branch: ForecastBranch) => void;
  onDeselect: () => void;
  onDelete: () => void;
  onRequestReflection: (branch: ForecastBranch) => void;
  onOpenEntity?: (entityId: string) => void;
  onOpenFact?: (factId: string) => void;
  onOpenNode?: (nodeId: string) => void;
}>) {
  const { t } = useI18n();
  return (
    <div className="fs-card" data-testid="forecast-card">
      <div className="fs-card-head">
        <p className="fs-divergence">{f.divergence}</p>
        <span className="fs-badge-noncanon">{t("forecast.notCanon")}</span>
      </div>
      <p className="fs-meta">
        {t("forecast.summary", {
          episode: f.base_episode_label || "—",
          count: f.branches.length,
          horizon: f.horizon,
        })}
      </p>
      {f.stale ? (
        <p className="fs-stale-banner">
          <AlertTriangle size={13} />
          {t("forecast.staleBanner", { reasons: formatReasons(f, t) })}
        </p>
      ) : null}
      <div className="fs-branches" data-testid="forecast-branches">
        {f.branches.map((b) => (
          <BranchCard
            key={b.id}
            branch={b}
            selected={f.selected_branch === b.id}
            busy={busy}
            expandKey={`${f.id}:${b.id}`}
            expanded={expanded}
            onToggleExpanded={onToggleExpanded}
            onChoose={() => onChoose(b)}
            onDeselect={onDeselect}
            onRequestReflection={() => onRequestReflection(b)}
            onOpenEntity={onOpenEntity}
            onOpenFact={onOpenFact}
            onOpenNode={onOpenNode}
          />
        ))}
      </div>
      <div className="fs-card-foot">
        <button
          type="button"
          className="chip fs-delete"
          disabled={busy}
          onClick={onDelete}
          data-testid="forecast-delete"
        >
          <Trash2 size={13} />
          {t("forecast.delete")}
        </button>
      </div>
    </div>
  );
}

function formatReasons(f: Forecast, t: (key: MessageKey, values?: Record<string, string | number>) => string): string {
  return (f.reasons ?? [])
    .map((r) => {
      if (r.group === "episodes") {
        return t("forecast.staleReason.episodes", { labels: (r.episode_labels ?? []).join(", ") });
      }
      return t(`forecast.staleReason.${r.group}` as MessageKey, { count: r.count ?? 0 });
    })
    .join(", ");
}

function BranchCard({
  branch: b,
  selected,
  busy,
  expandKey,
  expanded,
  onToggleExpanded,
  onChoose,
  onDeselect,
  onRequestReflection,
  onOpenEntity,
  onOpenFact,
  onOpenNode,
}: Readonly<{
  branch: ForecastBranch;
  selected: boolean;
  busy: boolean;
  expandKey: string;
  expanded: Set<string>;
  onToggleExpanded: (key: string) => void;
  onChoose: () => void;
  onDeselect: () => void;
  onRequestReflection: () => void;
  onOpenEntity?: (entityId: string) => void;
  onOpenFact?: (factId: string) => void;
  onOpenNode?: (nodeId: string) => void;
}>) {
  const { t } = useI18n();
  const isOpen = expanded.has(expandKey);
  const hasDetails =
    (b.decisions?.length ?? 0) > 0 ||
    (b.changes && Object.values(b.changes).some((v) => (v?.length ?? 0) > 0)) ||
    (b.unknowns?.length ?? 0) > 0 ||
    Boolean(b.direction_fit);

  return (
    <div className={`fs-branch${selected ? " fs-branch-selected" : ""}`} data-testid="forecast-branch">
      <div className="fs-branch-head">
        <h4 className="fs-branch-title">{b.title}</h4>
        {selected ? <span className="fs-badge-selected">{t("forecast.selectedBadge")}</span> : null}
      </div>
      <p className="fs-branch-premise">{b.premise}</p>

      <ol className="fs-beats">
        {b.beats.map((beat, idx) => (
          <li key={idx} className="fs-beat">
            <span className="fs-beat-offset">{t("forecast.episodeOffset", { n: beat.episode_offset })}</span>
            <span className="fs-beat-summary">{beat.summary}</span>
          </li>
        ))}
      </ol>

      {b.risks && b.risks.length > 0 ? (
        <ul className="fs-risks" data-testid="forecast-risks">
          {b.risks.map((risk, idx) => (
            <li key={idx} className="fs-risk" title={risk.description}>
              <span className={`fs-risk-badge fs-risk-${risk.kind}`}>{t(`forecast.riskKind.${risk.kind}` as MessageKey)}</span>
              {risk.entity_id && onOpenEntity ? (
                <button type="button" className="fs-risk-ref" onClick={() => onOpenEntity(risk.entity_id!)}>
                  {risk.entity_name || risk.entity_id}
                </button>
              ) : null}
              {risk.fact_id && onOpenFact ? (
                <button type="button" className="fs-risk-ref" onClick={() => onOpenFact(risk.fact_id!)}>
                  {risk.fact_claim || risk.fact_id}
                </button>
              ) : null}
              {risk.node_id && onOpenNode ? (
                <button type="button" className="fs-risk-ref" onClick={() => onOpenNode(risk.node_id!)}>
                  {risk.node_breadcrumb || risk.node_id}
                </button>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}

      {hasDetails ? (
        <button
          type="button"
          className="fs-details-toggle"
          onClick={() => onToggleExpanded(expandKey)}
          aria-expanded={isOpen}
        >
          {isOpen ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
          {t("forecast.detailsToggle")}
        </button>
      ) : null}

      {isOpen ? (
        <div className="fs-details" data-testid="forecast-details">
          {b.decisions && b.decisions.length > 0 ? (
            <div className="fs-detail-group">
              <h5>{t("forecast.decisionsHeading")}</h5>
              <ul>
                {b.decisions.map((d, idx) => (
                  <li key={idx}>
                    <strong>{d.entity_name || d.entity_id}</strong>: {d.decision}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          {b.changes && Object.values(b.changes).some((v) => (v?.length ?? 0) > 0) ? (
            <div className="fs-detail-group">
              <h5>{t("forecast.changesHeading")}</h5>
              {(["characters", "relationships", "world", "threads"] as const).map((key) =>
                b.changes?.[key] && b.changes[key]!.length > 0 ? (
                  <ul key={key}>
                    {b.changes[key]!.map((line, idx) => (
                      <li key={idx}>{line}</li>
                    ))}
                  </ul>
                ) : null,
              )}
            </div>
          ) : null}
          {b.unknowns && b.unknowns.length > 0 ? (
            <div className="fs-detail-group">
              <h5>{t("forecast.unknownsHeading")}</h5>
              <ul>
                {b.unknowns.map((u, idx) => (
                  <li key={idx}>{u}</li>
                ))}
              </ul>
            </div>
          ) : null}
          {b.direction_fit ? (
            <div className="fs-detail-group">
              <h5>{t("forecast.directionFitHeading")}</h5>
              <p>{b.direction_fit}</p>
            </div>
          ) : null}
        </div>
      ) : null}

      <div className="fs-branch-actions">
        {selected ? (
          <>
            <button type="button" className="chip" disabled={busy} onClick={onDeselect} data-testid="forecast-deselect">
              {t("forecast.deselect")}
            </button>
            <button
              type="button"
              className="chip fs-reflect"
              disabled={busy}
              onClick={onRequestReflection}
              data-testid="forecast-request-reflection"
            >
              {t("forecast.requestReflection")}
            </button>
          </>
        ) : (
          <button type="button" className="chip" disabled={busy} onClick={onChoose} data-testid="forecast-choose">
            {t("forecast.choose")}
          </button>
        )}
      </div>
    </div>
  );
}
