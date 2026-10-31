import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { changesContinuity } from "../lib/agentTools";
import { continuity as continuityApi } from "../lib/rpc";
import type { ContinuityCheck, ContinuityObservation, ContinuityRef, ContinuityStatus } from "../lib/types";
import { useI18n } from "../lib/i18n";
import type { MessageKey } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";
import "./ContinuitySection.css";

/** An agent's continuity reading of the writer's own scenes (story-assist
 *  Phase 2): places the prose may disagree with the work's own record, or
 *  leave something undecided. Every quote and every ref here was checked by
 *  the engine before it was stored — see internal/continuity.Validate — so
 *  this list is never the model's unverified say-so.
 *
 *  Nothing here edits the manuscript. The writer reads, opens the scene, and
 *  marks each observation dismissed or resolved, or deletes it, by hand or by
 *  asking the agent to settle the list (linetta_update_continuity).
 */

interface Props {
  projectId: string;
  /** The scene open in the editor, for the "this scene" scope. */
  nodeId?: string;
  /** Opens a scene named by an observation or a scene ref. */
  onOpenNode?: (nodeId: string) => void;
  /** Opens the EntitySheet for an entity ref. */
  onOpenEntity?: (entityId: string) => void;
  /** Opens the Fact Book scrolled to and highlighting the fact ref's own card. */
  onOpenFact?: (factId: string) => void;
  /** Bumped by the caller (ContextPanel's open-observations badge, via
   *  Workspace) to pull this section open, scope it to the current scene
   *  and switch to the open tab — so clicking "열린 관찰 N" actually shows
   *  them instead of leaving the section collapsed and scoped to the
   *  whole work. Any new value (not the initial one) re-triggers this;
   *  undefined/null never does. */
  focusRequest?: number | null;
}

type Scope = "scene" | "work";

/** The three statuses a writer can choose: "replaced" is never a tab — see
 *  ContinuitySection's own doc comment on why those observations are
 *  filtered out of every view rather than offered as a fourth choice. */
type SelectableStatus = "open" | "dismissed" | "resolved";

const STATUS_TABS: SelectableStatus[] = ["open", "dismissed", "resolved"];

function timeAgoKey(ms: number, now: number): { key: MessageKey; vars?: Record<string, string> } {
  const minutes = Math.floor((now - ms) / 60000);
  if (minutes < 1) return { key: "continuity.time.justNow" };
  if (minutes < 60) return { key: "continuity.time.minutesAgo", vars: { minutes: String(minutes) } };
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return { key: "continuity.time.hoursAgo", vars: { hours: String(hours) } };
  const days = Math.floor(hours / 24);
  return { key: "continuity.time.daysAgo", vars: { days: String(days) } };
}

/** One scene's worth of checks, merged: the latest check's own metadata
 *  (breadcrumb, staleness, timestamp, note) stands for the group, and every
 *  still-meaningful observation across that scene's checks — open,
 *  dismissed or resolved, never replaced — is shown together. A later
 *  check's empty report does not erase what an earlier one found and the
 *  writer has not acted on yet. */
interface SceneGroup {
  nodeId: string;
  latest: ContinuityCheck;
  observations: ContinuityObservation[];
}

function groupByScene(checks: ContinuityCheck[]): SceneGroup[] {
  const order: string[] = [];
  const groups = new Map<string, SceneGroup>();
  for (const check of checks) {
    let g = groups.get(check.node_id);
    if (!g) {
      g = { nodeId: check.node_id, latest: check, observations: [] };
      groups.set(check.node_id, g);
      order.push(check.node_id);
    }
    for (const obs of check.observations) {
      if (obs.status !== "replaced") g.observations.push(obs);
    }
  }
  return order.map((id) => groups.get(id) as SceneGroup);
}

export function ContinuitySection({
  projectId,
  nodeId,
  onOpenNode,
  onOpenEntity,
  onOpenFact,
  focusRequest,
}: Readonly<Props>) {
  const { t } = useI18n();
  const [scope, setScope] = useState<Scope>(nodeId ? "scene" : "work");
  const [status, setStatus] = useState<SelectableStatus>("open");
  const [checks, setChecks] = useState<ContinuityCheck[] | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  const [open, setOpen] = useState(false);

  // A new focusRequest value (the badge click) pulls the section open,
  // scopes it to the current scene (when one is open) and switches to the
  // open tab, the same reset-on-switch pattern the inspector-slot panels
  // follow in Workspace.tsx. The ref starts at a sentinel distinct from
  // every real focusRequest value on purpose, not at focusRequest itself:
  // Workspace bumps the counter and mounts this component (via CanonPanel's
  // key) in the very same render, so on first mount focusRequest already
  // holds its "new" value — seeding the ref with that same value would make
  // the check below see no change and silently skip the open.
  const lastFocusRequestRef = useRef<number | null | undefined>(undefined);
  useEffect(() => {
    if (focusRequest === lastFocusRequestRef.current) return;
    lastFocusRequestRef.current = focusRequest;
    if (focusRequest == null) return;
    setOpen(true);
    setStatus("open");
    if (nodeId) setScope("scene");
  }, [focusRequest, nodeId]);

  // The data arrives from outside this component — an agent's tool call —
  // so it reloads on the engine's change event rather than only on mount,
  // the same pattern StyleSection's draft follows (StyleSection.tsx).
  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.project_id && event.project_id !== projectId) return;
    if (changesContinuity(event.tool)) setRevision((n) => n + 1);
  });

  useEffect(() => {
    let cancelled = false;
    setError(null);
    continuityApi
      .list(projectId, scope === "scene" ? nodeId : undefined)
      .then((res) => {
        if (cancelled) return;
        setChecks(res.checks);
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e);
      });
    return () => {
      cancelled = true;
    };
  }, [projectId, nodeId, scope, revision]);

  const groups = useMemo(() => groupByScene(checks ?? []), [checks]);

  const setObservationStatus = useCallback(
    async (observationId: string, next: ContinuityStatus) => {
      setBusyId(observationId);
      setError(null);
      try {
        await continuityApi.setStatus(observationId, next);
        setChecks((prev) =>
          prev === null
            ? prev
            : prev.map((c) => ({
                ...c,
                observations: c.observations.map((o) => (o.id === observationId ? { ...o, status: next } : o)),
              })),
        );
      } catch (e) {
        setError(e);
      } finally {
        setBusyId(null);
      }
    },
    [],
  );

  // A refetch, not a local removal: the engine drops a check emptied this way,
  // and a check kept here with no observations would read as a clean one.
  const deleteObservation = useCallback(async (observationId: string) => {
    setBusyId(observationId);
    setError(null);
    try {
      await continuityApi.delete(observationId);
      setRevision((n) => n + 1);
    } catch (e) {
      setError(e);
    } finally {
      setBusyId(null);
    }
  }, []);

  return (
    <details
      className="sec es-field continuity-section"
      open={open}
      onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}
    >
      <summary>{t("continuity.title")}</summary>
      {error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}

      <div className="continuity-controls">
        {nodeId && (
          <div className="continuity-tabs" role="tablist" aria-label={t("continuity.title")}>
            {(["scene", "work"] as const).map((s) => (
              <button
                key={s}
                type="button"
                role="tab"
                aria-selected={scope === s}
                className={scope === s ? "is-active" : ""}
                data-testid={`continuity-scope-${s}`}
                onClick={() => setScope(s)}
              >
                {t(`continuity.scope.${s}`)}
              </button>
            ))}
          </div>
        )}
        <div className="continuity-tabs" role="tablist" aria-label={t("continuity.title")}>
          {STATUS_TABS.map((s) => (
            <button
              key={s}
              type="button"
              role="tab"
              aria-selected={status === s}
              className={status === s ? "is-active" : ""}
              data-testid={`continuity-status-${s}`}
              onClick={() => setStatus(s)}
            >
              {t(`continuity.status.${s}`)}
            </button>
          ))}
        </div>
      </div>

      {checks === null && error == null && <p className="continuity-hint">{t("common.loading")}</p>}
      {checks !== null && groups.length === 0 && <p className="continuity-hint">{t("continuity.emptyWork")}</p>}

      {groups.map((g) => (
        <SceneGroupRow
          key={g.nodeId}
          group={g}
          status={status}
          busyId={busyId}
          onOpenNode={onOpenNode}
          onOpenEntity={onOpenEntity}
          onOpenFact={onOpenFact}
          onSetStatus={setObservationStatus}
          onDelete={deleteObservation}
        />
      ))}
    </details>
  );
}

function SceneGroupRow({
  group,
  status,
  busyId,
  onOpenNode,
  onOpenEntity,
  onOpenFact,
  onSetStatus,
  onDelete,
}: Readonly<{
  group: SceneGroup;
  status: SelectableStatus;
  busyId: string | null;
  onOpenNode?: (nodeId: string) => void;
  onOpenEntity?: (entityId: string) => void;
  onOpenFact?: (factId: string) => void;
  onSetStatus: (observationId: string, next: ContinuityStatus) => void | Promise<void>;
  onDelete: (observationId: string) => void | Promise<void>;
}>) {
  const { t } = useI18n();
  const shown = group.observations.filter((o) => o.status === status);
  // "문제 없음" is about the LATEST check's own report, not about whatever
  // the current status tab happens to show — a scene checked just now with
  // nothing new can still carry an older dismissed observation below it.
  const isClean = group.latest.observations.length === 0;
  const ago = timeAgoKey(group.latest.checked_at, Date.now());

  return (
    <div className="continuity-group">
      <div className="continuity-group-head">
        <span className="continuity-breadcrumb">{group.latest.breadcrumb}</span>
        {group.latest.stale && <span className="continuity-badge stale">{t("continuity.stale")}</span>}
        {onOpenNode && (
          <button type="button" className="btn sm" onClick={() => onOpenNode(group.nodeId)}>
            {t("continuity.action.open")}
          </button>
        )}
      </div>
      <p className="continuity-hint">
        {t(ago.key, ago.vars)}
        {isClean && <> · {t("continuity.clean")}</>}
      </p>
      {group.latest.note && <p className="continuity-note">{t("continuity.noteLabel")}: {group.latest.note}</p>}

      {shown.length === 0 && !isClean && <p className="continuity-hint">{t("continuity.emptyFiltered")}</p>}

      <ul className="continuity-observations">
        {shown.map((o) => (
          <li key={o.id} className="continuity-observation">
            <ObservationRow
              o={o}
              busy={busyId === o.id}
              onOpenNode={onOpenNode}
              onOpenEntity={onOpenEntity}
              onOpenFact={onOpenFact}
              onSetStatus={onSetStatus}
              onDelete={onDelete}
            />
          </li>
        ))}
      </ul>
    </div>
  );
}

function ObservationRow({
  o,
  busy,
  onOpenNode,
  onOpenEntity,
  onOpenFact,
  onSetStatus,
  onDelete,
}: Readonly<{
  o: ContinuityObservation;
  busy: boolean;
  onOpenNode?: (nodeId: string) => void;
  onOpenEntity?: (entityId: string) => void;
  onOpenFact?: (factId: string) => void;
  onSetStatus: (observationId: string, next: ContinuityStatus) => void | Promise<void>;
  onDelete: (observationId: string) => void | Promise<void>;
}>) {
  const { t } = useI18n();
  const paragraph =
    o.paragraph_from === o.paragraph_to
      ? t("continuity.paragraphSingle", { n: String(o.paragraph_from) })
      : t("continuity.paragraphRange", { from: String(o.paragraph_from), to: String(o.paragraph_to) });

  return (
    <div className="continuity-observation-body">
      <div className="continuity-observation-head">
        <span className={`continuity-severity continuity-severity-${o.severity}`} aria-hidden="true" />
        <span className="continuity-category">{t(`continuity.category.${o.category}` as MessageKey)}</span>
        <span className="continuity-kind">{t(`continuity.kind.${o.kind}` as MessageKey)}</span>
      </div>
      <p className="continuity-summary">{o.summary}</p>
      <p className="continuity-quote">
        {paragraph} &ldquo;{o.quote}&rdquo;
      </p>
      {o.suggestion && (
        <p className="continuity-suggestion">
          {t("continuity.suggestionLabel")}: {o.suggestion}
        </p>
      )}
      {o.refs.length > 0 && (
        <div className="continuity-refs">
          {o.refs.map((ref, i) => (
            <RefChip key={`${ref.type}-${ref.id}-${i}`} refItem={ref} onOpenNode={onOpenNode} onOpenEntity={onOpenEntity} onOpenFact={onOpenFact} />
          ))}
        </div>
      )}
      <div className="continuity-actions">
        {o.status !== "dismissed" && (
          <button type="button" className="btn sm" disabled={busy} onClick={() => void onSetStatus(o.id, "dismissed")}>
            {t("continuity.action.dismiss")}
          </button>
        )}
        {o.status !== "resolved" && (
          <button type="button" className="btn accent sm" disabled={busy} onClick={() => void onSetStatus(o.id, "resolved")}>
            {t("continuity.action.resolve")}
          </button>
        )}
        {o.status !== "open" && (
          <button type="button" className="btn sm" disabled={busy} onClick={() => void onSetStatus(o.id, "open")}>
            {t("continuity.action.reopen")}
          </button>
        )}
        <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void onDelete(o.id)}>
          {t("common.delete")}
        </button>
      </div>
    </div>
  );
}

function RefChip({
  refItem,
  onOpenNode,
  onOpenEntity,
  onOpenFact,
}: Readonly<{
  // Named refItem, not ref: React intercepts a JSX prop literally named
  // "ref" on any element, function components included, and never passes
  // it through — a plain data prop needs a different name.
  refItem: ContinuityRef;
  onOpenNode?: (nodeId: string) => void;
  onOpenEntity?: (entityId: string) => void;
  onOpenFact?: (factId: string) => void;
}>) {
  const { t } = useI18n();
  const label = refItem.label || t("continuity.refUnknown");
  if (refItem.type === "entity" && onOpenEntity) {
    return (
      <button type="button" className="chip continuity-ref" onClick={() => onOpenEntity(refItem.id)}>
        {label}
      </button>
    );
  }
  if (refItem.type === "fact" && onOpenFact) {
    return (
      <button type="button" className="chip continuity-ref" onClick={() => onOpenFact(refItem.id)}>
        {label}
      </button>
    );
  }
  if (refItem.type === "scene" && onOpenNode) {
    return (
      <button type="button" className="chip continuity-ref" onClick={() => onOpenNode(refItem.id)}>
        {label}
      </button>
    );
  }
  return <span className="chip continuity-ref is-static">{label}</span>;
}
