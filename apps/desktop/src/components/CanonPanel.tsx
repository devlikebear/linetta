import { useCallback, useEffect, useMemo, useState } from "react";
import { Library, Search, X } from "lucide-react";
import { entities as entitiesApi, relationships as relationshipsApi } from "../lib/rpc";
import type { Entity, EntityKind, Relationship } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";
import "./CanonPanel.css";

/** The work's own record: who is in it, where it happens, what its rules are.
 *
 *  This data was always there — an agent writing over MCP creates characters,
 *  places and relationships, and the markdown export carries them — but the
 *  only ways into it were a scene's mention list and a search. A writer told
 *  "I added three characters" had nowhere to go and look (#28).
 *
 *  It deliberately sits beside the Fact Book rather than inside it. The Fact
 *  Book is about the real world — a claim and the source that backs it. This
 *  is about the invented one.
 */

const KINDS: EntityKind[] = ["character", "place", "item", "concept"];

interface Props {
  projectId: string;
  onOpenEntity: (entityId: string) => void;
  onClose: () => void;
  /** Bumped by the caller when an agent or another panel changed the work. */
  refreshKey?: number;
}

export function CanonPanel({ projectId, onOpenEntity, onClose, refreshKey = 0 }: Readonly<Props>) {
  const { t } = useI18n();
  const [all, setAll] = useState<Entity[]>([]);
  const [rels, setRels] = useState<Relationship[]>([]);
  const [kind, setKind] = useState<EntityKind | "all">("all");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [list, relList] = await Promise.all([
        entitiesApi.list(projectId),
        relationshipsApi.list(projectId),
      ]);
      setAll(list);
      setRels(relList);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => { void load(); }, [load, refreshKey]);

  const relCount = useMemo(() => {
    const counts = new Map<string, number>();
    for (const r of rels) {
      counts.set(r.from_id, (counts.get(r.from_id) ?? 0) + 1);
      counts.set(r.to_id, (counts.get(r.to_id) ?? 0) + 1);
    }
    return counts;
  }, [rels]);

  const shown = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return all
      .filter((e) => kind === "all" || e.kind === kind)
      .filter((e) => {
        if (!needle) return true;
        // Aliases matter here: the writer looks for the name they used in the
        // prose, which is often not the canonical one.
        const hay = [e.name, e.role, e.summary, ...(e.aliases ?? [])].join(" ").toLowerCase();
        return hay.includes(needle);
      })
      .sort((a, b) => a.name.localeCompare(b.name));
  }, [all, kind, query]);

  const countFor = (k: EntityKind | "all") =>
    k === "all" ? all.length : all.filter((e) => e.kind === k).length;

  return (
    <aside className="panel canon-panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl">
          <span className="ic"><Library size={16} /></span> {t("canon.title")}
        </span>
        <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}>
          <X size={16} />
        </button>
      </div>

      <div className="canon-controls">
        <p className="sd">{t("canon.description")}</p>
        <label className="canon-search">
          <Search size={13} aria-hidden="true" />
          <input
            type="search"
            value={query}
            aria-label={t("canon.searchLabel")}
            placeholder={t("canon.searchPlaceholder")}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <div className="canon-kinds" role="tablist" aria-label={t("canon.kindsLabel")}>
          {(["all", ...KINDS] as const).map((k) => (
            <button
              key={k}
              type="button"
              role="tab"
              aria-selected={kind === k}
              className={kind === k ? "is-active" : ""}
              onClick={() => setKind(k)}
            >
              {k === "all" ? t("canon.kind.all") : t(`entity.kind.${k}`)} {countFor(k)}
            </button>
          ))}
        </div>
      </div>

      <div className="panel-scroll canon-list">
        {loading && <p className="canon-empty">{t("common.loading")}</p>}
        {error != null && <p className="canon-empty" role="alert">{rpcErrorMessage(error, t)}</p>}
        {!loading && error == null && shown.length === 0 && (
          <p className="canon-empty">{query.trim() ? t("canon.noMatches") : t("canon.empty")}</p>
        )}
        {shown.map((e) => (
          <button
            key={e.id}
            type="button"
            className="canon-row"
            onClick={() => onOpenEntity(e.id)}
          >
            <span className="canon-row-head">
              <span className="canon-name">{e.name}</span>
              <span className="canon-kind">{t(`entity.kind.${e.kind}`)}</span>
            </span>
            {e.role && <span className="canon-role">{e.role}</span>}
            {e.summary && <span className="canon-summary">{e.summary}</span>}
            <span className="canon-meta">
              {(e.aliases?.length ?? 0) > 0 && (
                <span>{t("canon.aliases", { list: (e.aliases ?? []).join(", ") })}</span>
              )}
              {(relCount.get(e.id) ?? 0) > 0 && (
                <span>{t("canon.relationships", { count: relCount.get(e.id) ?? 0 })}</span>
              )}
            </span>
          </button>
        ))}
      </div>
    </aside>
  );
}
