import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, Network, X } from "lucide-react";
import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { forceLayout, type Point } from "../lib/forceLayout";
import { displayNodeLabel, entityKindLabel, useI18n } from "../lib/i18n";
import { relationships as relationshipsApi } from "../lib/rpc";
import { rpcErrorMessage } from "../lib/rpcMessage";
import type { EntityKind, PassageTarget, RelationshipGraph as Graph, RelationshipGraphEdge } from "../lib/types";
import "./RelationshipGraph.css";

/** The relationship graph (#386 B2): the cast and how they stand toward each
 *  other at one episode. Moving the slider shows the graph then, the ties
 *  that changed in that episode drawn heavier. Read-only: a node opens the
 *  entity sheet, where relationships are changed. */

interface Props {
  projectId: string;
  /** The scene open in the editor: the graph starts at its episode. */
  nodeId?: string;
  onClose: () => void;
  onOpenEntity?: (entityId: string) => void;
  /** Opens a scene at the line a change rests on. */
  onRevealPassage?: (target: PassageTarget) => void;
  onOpenNode?: (nodeId: string) => void;
}

const W = 900;
const H = 520;
const KINDS: EntityKind[] = ["character", "place", "item", "concept"];
const POSITIONS_KEY = "linetta.relationshipGraph.";

/** Where the writer left the nodes, kept per work in this browser only: a
 *  view of the work, not part of it. */
function readPositions(projectId: string): Record<string, Point> {
  try {
    const raw = window.localStorage.getItem(POSITIONS_KEY + projectId);
    return raw ? (JSON.parse(raw) as Record<string, Point>) : {};
  } catch {
    return {};
  }
}

function writePositions(projectId: string, positions: Record<string, Point>) {
  try {
    window.localStorage.setItem(POSITIONS_KEY + projectId, JSON.stringify(positions));
  } catch {
    // A convenience; the graph lays itself out again without it.
  }
}

export function RelationshipGraph({ projectId, nodeId, onClose, onOpenEntity, onRevealPassage, onOpenNode }: Readonly<Props>) {
  const { t, language } = useI18n();
  const [data, setData] = useState<Graph | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [episode, setEpisode] = useState<number | undefined>(undefined);
  const [kinds, setKinds] = useState<Set<EntityKind>>(new Set<EntityKind>(["character"]));
  const [showCo, setShowCo] = useState(false);
  const [changedOnly, setChangedOnly] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [positions, setPositions] = useState<Record<string, Point>>(() => readPositions(projectId));
  const svgRef = useRef<SVGSVGElement | null>(null);
  const drag = useRef<{ id: string; moved: boolean } | null>(null);

  const load = useCallback(() => {
    setError(null);
    relationshipsApi
      .graph(projectId, episode ? { episode } : { nodeId })
      .then((g) => {
        setData(g);
        if (episode === undefined) setEpisode(g.as_of);
      })
      .catch(setError);
  }, [projectId, nodeId, episode]);
  useEffect(load, [load]);
  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.project_id && event.project_id !== projectId) return;
    load();
  });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Lay out whoever has no place yet, around those who do. The ties come
  // from the latest written episode, so the places suit the whole story and
  // stay put as the slider moves: the eye can follow a tie changing.
  const total = data?.episodes.length ?? 0;
  const latest = data?.latest || total;
  useEffect(() => {
    if (!data) return;
    const ids = data.nodes.map((n) => n.entity_id);
    if (ids.every((id) => positions[id])) return;
    let cancelled = false;
    const ties = data.as_of === latest || latest === 0
      ? Promise.resolve(data)
      : relationshipsApi.graph(projectId, { episode: latest }).catch(() => data);
    void ties.then((g) => {
      if (cancelled) return;
      const next = forceLayout({
        ids,
        edges: [
          ...g.edges.map((e) => ({ from: e.from_id, to: e.to_id, weight: 1 })),
          ...g.co_appearances.map((c) => ({ from: c.from_id, to: c.to_id, weight: Math.min(c.scenes / 10, 0.5) })),
        ],
        width: W,
        height: H,
        fixed: Object.fromEntries(ids.filter((id) => positions[id]).map((id) => [id, positions[id]])),
      });
      setPositions(next);
      writePositions(projectId, next);
    });
    return () => {
      cancelled = true;
    };
  }, [data, positions, projectId, latest]);

  const visible = useMemo(() => {
    const nodes = (data?.nodes ?? []).filter((n) => kinds.has(n.kind));
    const ids = new Set(nodes.map((n) => n.entity_id));
    const edges = (data?.edges ?? []).filter(
      (e) => ids.has(e.from_id) && ids.has(e.to_id) && (!changedOnly || e.changed),
    );
    const co = showCo && !changedOnly
      ? (data?.co_appearances ?? []).filter((c) => ids.has(c.from_id) && ids.has(c.to_id))
      : [];
    return { nodes, edges, co };
  }, [data, kinds, showCo, changedOnly]);

  const names = useMemo(() => new Map((data?.nodes ?? []).map((n) => [n.entity_id, n.name])), [data]);
  const episodeLabel = (ordinal?: number) => {
    const ep = ordinal ? data?.episodes[ordinal - 1] : undefined;
    return ep ? displayNodeLabel(language, ep.label) || ep.title || String(ordinal) : String(ordinal ?? "");
  };
  const asOf = data?.as_of ?? 0;
  const chosen = visible.edges.find((e) => e.id === selected) ?? null;

  const toSvg = (e: React.PointerEvent) => {
    const svg = svgRef.current;
    const m = svg?.getScreenCTM();
    if (!svg || !m) return null;
    const p = new DOMPoint(e.clientX, e.clientY).matrixTransform(m.inverse());
    return { x: Math.round(Math.max(12, Math.min(W - 12, p.x))), y: Math.round(Math.max(12, Math.min(H - 12, p.y))) };
  };

  const go = (step: { since_node_id?: string; quote?: string }) => {
    if (!step.since_node_id) return;
    if (step.quote && onRevealPassage) {
      onRevealPassage({ nodeId: step.since_node_id, paragraphFrom: 0, quote: step.quote });
    } else {
      onOpenNode?.(step.since_node_id);
    }
    onClose();
  };

  return (
    <div className="backdrop center" onClick={onClose}>
      <div
        className="modal relationship-graph"
        role="dialog"
        aria-modal="true"
        aria-labelledby="relationship-graph-title"
        data-testid="relationship-graph"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="rg-head">
          <h2 id="relationship-graph-title"><Network size={16} aria-hidden="true" /> {t("relGraph.title")}</h2>
          {data && (
            <span className="rg-counts" data-testid="relationship-graph-counts">
              {t("relGraph.counts", {
                nodes: String(visible.nodes.length),
                edges: String(visible.edges.length),
                changed: String(visible.edges.filter((e) => e.changed).length),
              })}
            </span>
          )}
          <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}><X size={16} /></button>
        </div>

        {total > 0 && (
          <div className="rg-slider">
            <span className="rg-slider-label">{t("relGraph.episode")}</span>
            <button type="button" className="btn ghost sm" aria-label={t("relGraph.prev")} disabled={asOf <= 1}
              onClick={() => setEpisode(Math.max(1, asOf - 1))}>
              <ChevronLeft size={14} />
            </button>
            <strong className="rg-episode" data-testid="relationship-graph-episode">{episodeLabel(asOf)}</strong>
            <button type="button" className="btn ghost sm" aria-label={t("relGraph.next")} disabled={asOf >= total}
              onClick={() => setEpisode(Math.min(total, asOf + 1))}>
              <ChevronRight size={14} />
            </button>
            <input type="range" min={1} max={total} value={asOf || 1} aria-label={t("relGraph.episode")}
              onChange={(e) => setEpisode(Number(e.target.value))} />
            <span className="rg-range">{episodeLabel(1)} – {episodeLabel(total)}</span>
          </div>
        )}

        <div className="rg-filters">
          <div className="chips" role="group" aria-label={t("relGraph.kinds")}>
            {KINDS.map((k) => (
              <button key={k} type="button" className={`chip${kinds.has(k) ? " on" : ""}`} aria-pressed={kinds.has(k)}
                onClick={() => setKinds(toggle(kinds, k))}>
                {entityKindLabel(language, k)}
              </button>
            ))}
          </div>
          <label className="rg-check">
            <input type="checkbox" checked={showCo} onChange={(e) => setShowCo(e.target.checked)} />
            {t("relGraph.coAppearances")}
          </label>
          <label className="rg-check">
            <input type="checkbox" checked={changedOnly} onChange={(e) => setChangedOnly(e.target.checked)} />
            {t("relGraph.changedOnly")}
          </label>
          <button type="button" className="btn ghost sm" onClick={() => setPositions({})}>
            {t("relGraph.relayout")}
          </button>
        </div>

        {error != null ? (
          <p role="alert" className="rg-empty">{rpcErrorMessage(error, t)}</p>
        ) : !data ? (
          <p className="rg-empty">{t("common.loading")}</p>
        ) : visible.nodes.length === 0 ? (
          <p className="rg-empty" data-testid="relationship-graph-empty">{t("relGraph.empty")}</p>
        ) : (
          <svg ref={svgRef} viewBox={`0 0 ${W} ${H}`} className="rg-svg" role="img" aria-label={t("relGraph.title")}
            data-testid="relationship-graph-svg">
            {visible.co.map((c) => {
              const a = positions[c.from_id];
              const b = positions[c.to_id];
              if (!a || !b) return null;
              return (
                <line key={`co-${c.from_id}-${c.to_id}`} x1={a.x} y1={a.y} x2={b.x} y2={b.y} className="rg-co"
                  strokeWidth={Math.min(1 + c.scenes / 6, 4)}>
                  <title>{t("relGraph.coTitle", { a: names.get(c.from_id) ?? "", b: names.get(c.to_id) ?? "", count: String(c.scenes) })}</title>
                </line>
              );
            })}
            {visible.edges.map((e) => {
              const a = positions[e.from_id];
              const b = positions[e.to_id];
              if (!a || !b) return null;
              const label = !e.inverse_label || e.inverse_label === e.label ? e.label : `${e.label} / ${e.inverse_label}`;
              const mx = a.x + (b.x - a.x) * 0.5;
              const my = a.y + (b.y - a.y) * 0.5;
              const cls = `rg-edge${e.changed ? " is-changed" : ""}${selected === e.id ? " is-selected" : ""}`;
              return (
                <g key={e.id} className={cls} data-testid={`relationship-graph-edge-${e.id}`}
                  onClick={() => setSelected(selected === e.id ? null : e.id)}>
                  <line x1={a.x} y1={a.y} x2={b.x} y2={b.y} className="rg-edge-hit" />
                  <line x1={a.x} y1={a.y} x2={b.x} y2={b.y} className="rg-edge-line" />
                  <text x={mx} y={my - 6} textAnchor="middle" className="rg-edge-label">
                    {e.changed ? `◆ ${label}` : label}
                  </text>
                </g>
              );
            })}
            {visible.nodes.map((n) => {
              const p = positions[n.entity_id];
              if (!p) return null;
              const r = Math.min(8 + Math.sqrt(n.appearances) * 2.5, 24);
              const pending = !n.first_episode;
              return (
                <g key={n.entity_id} className={`rg-node rg-kind-${n.kind}${pending ? " is-pending" : ""}`}
                  transform={`translate(${p.x},${p.y})`} data-testid={`relationship-graph-node-${n.entity_id}`}
                  onPointerDown={(ev) => {
                    (ev.currentTarget as Element).setPointerCapture?.(ev.pointerId);
                    drag.current = { id: n.entity_id, moved: false };
                  }}
                  onPointerMove={(ev) => {
                    if (drag.current?.id !== n.entity_id) return;
                    const q = toSvg(ev);
                    if (!q) return;
                    if (!drag.current.moved && Math.hypot(q.x - p.x, q.y - p.y) < 3) return;
                    drag.current.moved = true;
                    setPositions((cur) => ({ ...cur, [n.entity_id]: q }));
                  }}
                  onPointerUp={() => {
                    const d = drag.current;
                    drag.current = null;
                    if (!d) return;
                    if (d.moved) {
                      setPositions((cur) => {
                        writePositions(projectId, cur);
                        return cur;
                      });
                    } else if (onOpenEntity) {
                      onOpenEntity(n.entity_id);
                      onClose();
                    }
                  }}>
                  <title>
                    {pending
                      ? t("relGraph.notYet", { name: n.name })
                      : t("relGraph.nodeTitle", { name: n.name, count: String(n.appearances), first: episodeLabel(n.first_episode) })}
                  </title>
                  <circle r={r} className="rg-node-dot" />
                  <text y={r + 14} textAnchor="middle" className="rg-node-name">{n.name}</text>
                </g>
              );
            })}
          </svg>
        )}

        <div className="rg-detail" data-testid="relationship-graph-detail">
          {chosen ? (
            <EdgeHistory edge={chosen} names={names} episodeLabel={episodeLabel} onGo={go} t={t} />
          ) : (
            <p className="rg-hint">{t("relGraph.pickEdge")}</p>
          )}
        </div>
      </div>
    </div>
  );
}

function EdgeHistory({ edge, names, episodeLabel, onGo, t }: {
  edge: RelationshipGraphEdge;
  names: Map<string, string>;
  episodeLabel: (ordinal?: number) => string;
  onGo: (step: { since_node_id?: string; quote?: string }) => void;
  t: ReturnType<typeof useI18n>["t"];
}) {
  return (
    <div>
      <h3 className="rg-detail-title">{names.get(edge.from_id)} → {names.get(edge.to_id)}</h3>
      <ul className="rg-steps">
        {edge.steps.map((s, i) => (
          <li key={i} className="rg-step">
            <span className="rg-step-when">
              {s.since_episode ? t("relGraph.since", { episode: episodeLabel(s.since_episode) }) : t("relGraph.fromStart")}
            </span>
            <span className="rg-step-label">
              {s.label}{s.inverse_label && s.inverse_label !== s.label ? ` / ${s.inverse_label}` : ""}
            </span>
            {s.quote && <q className="rg-step-quote">{s.quote}</q>}
            {s.since_node_id && (
              <button type="button" className="btn ghost sm" onClick={() => onGo(s)}>{t("relGraph.go")}</button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

function toggle<T>(set: Set<T>, value: T): Set<T> {
  const next = new Set(set);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}
