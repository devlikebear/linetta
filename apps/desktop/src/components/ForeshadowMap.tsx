import { useCallback, useEffect, useMemo, useState } from "react";
import { Map as MapIcon, X } from "lucide-react";
import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { ledger as ledgerApi } from "../lib/rpc";
import { displayNodeLabel, useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";
import type { LedgerKind, LedgerTimeline, LedgerTimelineEntry, LedgerTimelinePoint, PassageTarget } from "../lib/types";
import "./ForeshadowMap.css";

/** The foreshadowing map (#386 A): every promise, running gag and reader's
 *  question laid on the episode axis — where it was planted, when it is due,
 *  where it was paid — so what is overdue shows at a glance. Read-only: the
 *  ledger stays the place to change an entry. */

interface Props {
  projectId: string;
  /** The scene open in the editor; its episode gets a marker line. */
  nodeId?: string;
  onClose: () => void;
  /** Opens a scene at a quote (planted, paid). */
  onRevealPassage?: (target: PassageTarget) => void;
  /** Opens a scene with nothing to light (a due episode). */
  onOpenNode?: (nodeId: string) => void;
}

const KINDS: LedgerKind[] = ["promise", "gag", "question"];
type Status = "open" | "paid" | "dropped";
const STATUSES: Status[] = ["open", "paid", "dropped"];
type Sort = "overdue" | "open" | "planted";

const LABEL_W = 168;
const ROW_H = 28;
const AXIS_H = 44;
const PAD = 16;

export function ForeshadowMap({ projectId, nodeId, onClose, onRevealPassage, onOpenNode }: Readonly<Props>) {
  const { t, language } = useI18n();
  const [data, setData] = useState<LedgerTimeline | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [kinds, setKinds] = useState<Set<LedgerKind>>(new Set(KINDS));
  const [statuses, setStatuses] = useState<Set<Status>>(new Set<Status>(["open", "paid"]));
  const [partId, setPartId] = useState("");
  const [sort, setSort] = useState<Sort>("overdue");
  const [selected, setSelected] = useState<string | null>(null);

  const load = useCallback(() => {
    setError(null);
    ledgerApi
      .timeline(projectId, nodeId)
      .then(setData)
      .catch(setError);
  }, [projectId, nodeId]);
  useEffect(load, [load]);
  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.project_id && event.project_id !== projectId) return;
    if (event.tool?.includes("ledger")) load();
  });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const part = data?.parts.find((p) => p.node_id === partId);
  const from = part?.from ?? 1;
  const to = part?.to ?? Math.max(1, data?.episodes.length ?? 1);
  const span = to - from + 1;
  const col = Math.max(12, Math.min(40, Math.floor(720 / span)));
  const x = (episode: number) => PAD + (episode - from) * col + col / 2;
  const inRange = (p?: LedgerTimelinePoint) => !!p && p.episode >= from && p.episode <= to;

  const counts = useMemo(() => {
    const all = data?.entries ?? [];
    return {
      open: all.filter((e) => e.status === "open").length,
      paid: all.filter((e) => e.status === "paid").length,
      dropped: all.filter((e) => e.status === "dropped").length,
      overdue: all.filter((e) => e.overdue).length,
    };
  }, [data]);

  const rows = useMemo(() => {
    const list = (data?.entries ?? []).filter(
      (e) => kinds.has(e.kind as LedgerKind) && statuses.has(e.status as Status) && touches(e, from, to),
    );
    const planted = (e: LedgerTimelineEntry) => e.planted?.episode ?? Number.MAX_SAFE_INTEGER;
    return list.sort((a, b) => {
      if (sort === "overdue" && a.overdue !== b.overdue) return a.overdue ? -1 : 1;
      // What is still owed comes before what is settled, then the longest
      // waiting first.
      if (sort !== "planted" && (a.status === "open") !== (b.status === "open")) return a.status === "open" ? -1 : 1;
      if (sort !== "planted" && (a.open_for ?? 0) !== (b.open_for ?? 0)) return (b.open_for ?? 0) - (a.open_for ?? 0);
      return planted(a) - planted(b);
    });
  }, [data, kinds, statuses, sort, from, to]);

  const episodeLabel = (ordinal: number) => {
    const ep = data?.episodes[ordinal - 1];
    return ep ? displayNodeLabel(language, ep.label) : String(ordinal);
  };
  const open = (p: LedgerTimelinePoint | undefined) => {
    if (!p) return;
    if (p.quote && onRevealPassage) {
      // No paragraph number: the ledger keeps the quote, and revealParagraph
      // finds it anywhere in the scene.
      onRevealPassage({ nodeId: p.node_id, paragraphFrom: 0, quote: p.quote });
    } else {
      onOpenNode?.(p.node_id);
    }
    onClose();
  };

  const width = PAD * 2 + span * col;
  const height = AXIS_H + rows.length * ROW_H + 8;
  const tick = col >= 24 ? 1 : col >= 16 ? 5 : 10;
  const chosen = rows.find((r) => r.id === selected) ?? null;

  return (
    <div className="backdrop center" onClick={onClose}>
      <div
        className="modal foreshadow-map"
        role="dialog"
        aria-modal="true"
        aria-labelledby="foreshadow-title"
        data-testid="foreshadow-map"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="fm-head">
          <h2 id="foreshadow-title"><MapIcon size={16} aria-hidden="true" /> {t("foreshadow.title")}</h2>
          <span className="fm-counts" data-testid="foreshadow-counts">
            {t("foreshadow.counts", {
              open: String(counts.open), paid: String(counts.paid),
              dropped: String(counts.dropped), overdue: String(counts.overdue),
            })}
          </span>
          <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}><X size={16} /></button>
        </div>

        <div className="fm-filters">
          <div className="chips" role="group" aria-label={t("foreshadow.kinds")}>
            {KINDS.map((k) => (
              <button key={k} type="button" className={`chip${kinds.has(k) ? " on" : ""}`} aria-pressed={kinds.has(k)}
                data-testid={`foreshadow-kind-${k}`} onClick={() => setKinds(toggle(kinds, k))}>
                {t(`ledger.kind.${k}`)}
              </button>
            ))}
          </div>
          <div className="chips" role="group" aria-label={t("foreshadow.statuses")}>
            {STATUSES.map((s) => (
              <button key={s} type="button" className={`chip${statuses.has(s) ? " on" : ""}`} aria-pressed={statuses.has(s)}
                data-testid={`foreshadow-status-${s}`} onClick={() => setStatuses(toggle(statuses, s))}>
                {t(`foreshadow.status.${s}`)}
              </button>
            ))}
          </div>
          {(data?.parts.length ?? 0) > 0 && (
            <select aria-label={t("foreshadow.part")} value={partId} onChange={(e) => setPartId(e.target.value)} data-testid="foreshadow-part">
              <option value="">{t("foreshadow.allParts")}</option>
              {data?.parts.map((p) => (
                <option key={p.node_id} value={p.node_id}>{displayNodeLabel(language, p.label)}{p.title ? ` ${p.title}` : ""}</option>
              ))}
            </select>
          )}
          <select aria-label={t("foreshadow.sort")} value={sort} onChange={(e) => setSort(e.target.value as Sort)}>
            {(["overdue", "open", "planted"] as const).map((s) => (
              <option key={s} value={s}>{t(`foreshadow.sort.${s}`)}</option>
            ))}
          </select>
        </div>

        {error != null ? (
          <p role="alert" className="fm-empty">{rpcErrorMessage(error, t)}</p>
        ) : !data ? (
          <p className="fm-empty">{t("common.loading")}</p>
        ) : data.episodes.length === 0 || rows.length === 0 ? (
          <p className="fm-empty" data-testid="foreshadow-empty">{t(data.entries.length === 0 ? "foreshadow.empty" : "foreshadow.emptyFiltered")}</p>
        ) : (
          <div className="fm-chart" data-testid="foreshadow-chart">
            <div className="fm-names" style={{ width: LABEL_W, paddingTop: AXIS_H }}>
              {rows.map((r) => (
                <button key={r.id} type="button" className={`fm-name${selected === r.id ? " is-selected" : ""}`}
                  style={{ height: ROW_H }} title={r.body} onClick={() => setSelected(r.id === selected ? null : r.id)}
                  data-testid={`foreshadow-row-${r.id}`}>
                  {r.subject || r.body}
                </button>
              ))}
            </div>
            <svg width={width} height={height} className="fm-svg" role="img" aria-label={t("foreshadow.title")}>
              {data.parts.filter((p) => p.to >= from && p.from <= to).map((p) => (
                <g key={p.node_id}>
                  <line x1={x(Math.max(p.from, from)) - col / 2} x2={x(Math.max(p.from, from)) - col / 2} y1={0} y2={height} className="fm-part-line" />
                  <text x={x(Math.max(p.from, from)) - col / 2 + 4} y={14} className="fm-part-label">{displayNodeLabel(language, p.label)}</text>
                </g>
              ))}
              {Array.from({ length: span }, (_, i) => from + i)
                .filter((ep) => ep === from || ep % tick === 0)
                .map((ep) => (
                  <text key={ep} x={x(ep)} y={AXIS_H - 10} className="fm-tick" textAnchor="middle">{ep}</text>
                ))}
              {data.current ? (
                data.current >= from && data.current <= to ? (
                  <line x1={x(data.current)} x2={x(data.current)} y1={AXIS_H - 4} y2={height} className="fm-current" data-testid="foreshadow-current" />
                ) : null
              ) : null}
              {rows.map((r, i) => (
                <Row key={r.id} entry={r} y={AXIS_H + i * ROW_H + ROW_H / 2} x={x} inRange={inRange}
                  latest={data.latest} from={from} to={to} label={episodeLabel} onOpen={open}
                  selected={selected === r.id} t={t} />
              ))}
            </svg>
          </div>
        )}

        <p className="fm-legend" aria-hidden="true">{t("foreshadow.legend")}</p>

        {chosen ? (
          <div className="fm-detail" data-testid="foreshadow-detail">
            <p>
              <strong>{t(`ledger.kind.${chosen.kind as LedgerKind}`)}{chosen.subject ? ` · ${chosen.subject}` : ""}</strong>
              {chosen.overdue ? <span className="fm-overdue-tag"> ⚠ {t("foreshadow.overdue")}</span> : null}
            </p>
            <p>{chosen.body}</p>
            <div className="fm-detail-actions">
              {chosen.planted ? (
                <button type="button" className="btn sm" onClick={() => open(chosen.planted)}>
                  {t("foreshadow.openPlanted", { label: episodeLabel(chosen.planted.episode) })}
                </button>
              ) : null}
              {chosen.paid ? (
                <button type="button" className="btn sm" onClick={() => open(chosen.paid)}>
                  {t("foreshadow.openPaid", { label: episodeLabel(chosen.paid.episode) })}
                </button>
              ) : null}
              {chosen.status === "open" && chosen.open_for ? (
                <span className="fm-open-for">{t("foreshadow.openFor", { n: String(chosen.open_for) })}</span>
              ) : null}
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}

/** Whether any part of an entry's span falls in the episodes shown. */
function touches(e: LedgerTimelineEntry, from: number, to: number): boolean {
  const points = [e.planted, e.due, e.paid].filter(Boolean) as LedgerTimelinePoint[];
  if (points.length === 0) return false;
  const lo = Math.min(...points.map((p) => p.episode));
  const hi = e.status === "open" ? Number.MAX_SAFE_INTEGER : Math.max(...points.map((p) => p.episode));
  return hi >= from && lo <= to;
}

function toggle<T>(set: Set<T>, value: T): Set<T> {
  const next = new Set(set);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}

function Row({
  entry, y, x, inRange, latest, from, to, label, onOpen, selected, t,
}: Readonly<{
  entry: LedgerTimelineEntry;
  y: number;
  x: (episode: number) => number;
  inRange: (p?: LedgerTimelinePoint) => boolean;
  latest: number;
  from: number;
  to: number;
  label: (ordinal: number) => string;
  onOpen: (p?: LedgerTimelinePoint) => void;
  selected: boolean;
  t: ReturnType<typeof useI18n>["t"];
}>) {
  const start = entry.planted?.episode;
  // Where the line stops: where it was paid, where it was dropped (its due,
  // or where it was planted), or, while open, the latest episode or its due.
  const end =
    entry.status === "paid" ? entry.paid?.episode
      : entry.status === "dropped" ? (entry.due?.episode ?? start)
        : Math.max(latest, entry.due?.episode ?? 0, start ?? 0);
  const clamp = (ep: number) => Math.min(Math.max(ep, from), to);
  const marker = (p: LedgerTimelinePoint | undefined, kind: "planted" | "due" | "paid") => {
    if (!p || !inRange(p)) return null;
    const cx = x(p.episode);
    const title = `${label(p.episode)} · ${t(`foreshadow.point.${kind}`)}${p.quote ? `: “${p.quote}”` : ""}`;
    const shape =
      kind === "due" ? <path d={`M ${cx} ${y - 6} L ${cx + 6} ${y} L ${cx} ${y + 6} L ${cx - 6} ${y} Z`} className="fm-due" />
        : kind === "paid" ? <circle cx={cx} cy={y} r={5.5} className="fm-paid" />
          : <circle cx={cx} cy={y} r={5} className="fm-planted" />;
    return (
      <g
        role="button"
        tabIndex={0}
        className="fm-marker"
        data-testid={`foreshadow-${kind}-${entry.id}`}
        onClick={() => onOpen(p)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onOpen(p);
          }
        }}
      >
        <title>{title}</title>
        {shape}
      </g>
    );
  };
  return (
    <g className={`fm-row fm-${entry.status}${entry.overdue ? " is-overdue" : ""}${selected ? " is-selected" : ""}`}>
      {start !== undefined && end !== undefined && end >= from && start <= to ? (
        <line x1={x(clamp(start))} x2={x(clamp(end))} y1={y} y2={y} className="fm-span" />
      ) : null}
      {entry.status === "open" && end !== undefined && end <= to ? (
        <text x={x(end) + 7} y={y + 4} className="fm-arrow">{entry.overdue ? "→ ⚠" : "→"}</text>
      ) : null}
      {entry.status === "dropped" && end !== undefined && end >= from && end <= to ? (
        <text x={x(end)} y={y + 4} className="fm-dropped" textAnchor="middle">✕</text>
      ) : null}
      {marker(entry.due, "due")}
      {marker(entry.planted, "planted")}
      {marker(entry.paid, "paid")}
    </g>
  );
}
