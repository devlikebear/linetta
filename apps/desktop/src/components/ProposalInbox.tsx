import { useCallback, useEffect, useMemo, useState } from "react";
import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { proposals as proposalsApi } from "../lib/rpc";
import type {
  ExtractionProgress,
  ProposalOp,
  ProposalOpType,
  SuggestionBatch,
  SuggestionDuplicate,
  SuggestionItem,
} from "../lib/types";
import { useI18n } from "../lib/i18n";
import { useToast } from "./ToastProvider";
import { rpcErrorMessage } from "../lib/rpcMessage";
import { X } from "../lib/icons";
import "./ProposalInbox.css";

/** The suggestions inbox (story-assist Phase 3): an agent's
 *  linetta_propose_story_ops candidates, kept out of Story World until the
 *  writer accepts them one item at a time. Nothing an agent does here
 *  applies anything — proposals.accept is the one path that does, and it
 *  runs the chosen items through the same storyops applier every other
 *  write goes through. */

interface Props {
  projectId: string;
  onClose: () => void;
  /** Opens a scene named by an item's evidence citation. */
  onOpenNode?: (nodeId: string) => void;
}

type GroupKind = "character" | "place" | "item" | "concept" | "relationship" | "fact" | "thread" | "beat" | "other";

const GROUP_ORDER: GroupKind[] = ["character", "place", "item", "concept", "relationship", "fact", "thread", "beat", "other"];

function groupKindFor(op: { op: ProposalOpType; kind?: string }): GroupKind {
  switch (op.op) {
    case "create_entity":
    case "update_entity":
      switch (op.kind) {
        case "place":
          return "place";
        case "item":
          return "item";
        case "concept":
          return "concept";
        default:
          return "character";
      }
    case "create_relationship":
      return "relationship";
    case "create_fact_card":
      return "fact";
    case "create_thread":
    case "update_thread":
      return "thread";
    case "add_beat":
    case "update_beat":
      return "beat";
    default:
      return "other";
  }
}

/** The field(s) a lightweight in-place edit offers, per op type — just
 *  enough to fix a name, role, summary or label without leaving the row. */
function editableFields(opType: ProposalOpType): Array<{ key: keyof ProposalOp; labelKey: string; multiline?: boolean }> {
  switch (opType) {
    case "create_entity":
    case "update_entity":
      return [
        { key: "name", labelKey: "proposals.edit.name" },
        { key: "role", labelKey: "proposals.edit.role" },
        { key: "summary", labelKey: "proposals.edit.summary", multiline: true },
      ];
    case "create_relationship":
      return [{ key: "label", labelKey: "proposals.edit.label" }];
    case "create_fact_card":
      return [
        { key: "claim", labelKey: "proposals.edit.claim" },
        { key: "result", labelKey: "proposals.edit.result", multiline: true },
      ];
    case "create_thread":
    case "update_thread":
      return [
        { key: "name", labelKey: "proposals.edit.name" },
        { key: "summary", labelKey: "proposals.edit.summary", multiline: true },
      ];
    case "add_beat":
    case "update_beat":
      return [
        { key: "label", labelKey: "proposals.edit.label" },
        { key: "description", labelKey: "proposals.edit.summary", multiline: true },
      ];
    default:
      return [];
  }
}

export function ProposalInbox({ projectId, onClose, onOpenNode }: Readonly<Props>) {
  const { t } = useI18n();
  const { showToast } = useToast();
  const [batches, setBatches] = useState<SuggestionBatch[] | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [revision, setRevision] = useState(0);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [editing, setEditing] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<string, Partial<ProposalOp>>>({});
  const [busy, setBusy] = useState(false);
  const [batchFilter, setBatchFilter] = useState<string>("all");
  const [progress, setProgress] = useState<ExtractionProgress | null>(null);
  // story-assist Phase 4 (4.A.5/4.C.2): the most recent accept call's
  // structural undo batches, if any — purpose=forecast's outline-reflection
  // items only. An array, not a single id: with batchFilter=="all" the
  // writer can select structural items from more than one proposal before
  // clicking "모두 적용" (toggleSelect/visibleItemsByGroup place no
  // per-batch limit on `selected`), and accept.go's byProposal loop then
  // creates one undo batch PER proposal — keeping only the first would
  // leave every batch after it with no way to revert through this UI at
  // all (review finding, 2026-10-25). Each id is cleared independently
  // (undoBatch below) once used; the whole array is cleared on the next
  // accept/reject, so the buttons never offer to revert a batch the writer
  // already moved past.
  const [lastUndoBatchIds, setLastUndoBatchIds] = useState<string[]>([]);

  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.project_id && event.project_id !== projectId) return;
    if (event.tool === "linetta_propose_story_ops") setRevision((n) => n + 1);
  });
  useEngineEvent<{ project_id?: string }>("proposals-changed", (event) => {
    if (event.project_id && event.project_id !== projectId) return;
    setRevision((n) => n + 1);
  });

  useEffect(() => {
    let cancelled = false;
    setError(null);
    proposalsApi
      .list(projectId)
      .then((res) => {
        if (cancelled) return;
        setBatches(res.proposals);
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e);
      });
    return () => {
      cancelled = true;
    };
  }, [projectId, revision]);

  const hasExtraction = useMemo(
    () => (batches ?? []).some((b) => b.purpose === "import_extraction"),
    [batches],
  );
  useEffect(() => {
    if (!hasExtraction) {
      setProgress(null);
      return;
    }
    let cancelled = false;
    proposalsApi
      .extractionProgress(projectId)
      .then((p) => {
        if (!cancelled) setProgress(p);
      })
      .catch(() => {
        /* progress is a nicety; a failed fetch just hides it */
      });
    return () => {
      cancelled = true;
    };
  }, [projectId, hasExtraction, revision]);

  const allItems = useMemo(() => (batches ?? []).flatMap((b) => b.items), [batches]);
  const itemsById = useMemo(() => new Map(allItems.map((it) => [it.id, it])), [allItems]);
  const pendingCount = useMemo(() => allItems.filter((it) => it.status === "pending").length, [allItems]);

  const visibleBatches = useMemo(
    () => (batchFilter === "all" ? batches ?? [] : (batches ?? []).filter((b) => b.id === batchFilter)),
    [batches, batchFilter],
  );
  // Shown statuses are "pending" (awaiting a decision), "failed" (the
  // writer tried to accept it and storyops refused — the row stays so the
  // error and an edit-and-retry are both still reachable), and
  // "reverting" (proposals.Undo has committed to revert this item but has
  // not yet confirmed the outcome — see proposals.StatusReverting's own
  // doc comment on the engine side). "accepted" and "rejected" are done;
  // they disappear from the inbox once they are.
  const visibleItemsByGroup = useMemo(() => {
    const groups = new Map<GroupKind, SuggestionItem[]>();
    for (const b of visibleBatches) {
      for (const it of b.items) {
        if (it.status !== "pending" && it.status !== "failed" && it.status !== "reverting") continue;
        const k = groupKindFor(it.op);
        const list = groups.get(k) ?? [];
        list.push(it);
        groups.set(k, list);
      }
    }
    return groups;
  }, [visibleBatches]);

  const toggleSelect = useCallback(
    (item: SuggestionItem) => {
      // Reverting is never a selectable outcome — proposals.Accept
      // refuses it outright (the engine's own duplicate-creation guard),
      // and the checkbox is disabled for exactly this reason; this guard
      // is defense in depth against anything that calls toggleSelect
      // directly.
      if (item.status === "reverting") return;
      setSelected((prev) => {
        const next = new Set(prev);
        if (next.has(item.id)) {
          next.delete(item.id);
          return next;
        }
        next.add(item.id);
        // A dependency not yet accepted is auto-selected too, so checking
        // a relationship candidate without its entity does not silently
        // fail at apply time — the writer sees both checked together.
        for (const depId of item.depends_on) {
          const dep = itemsById.get(depId);
          if (dep && (dep.status === "pending" || dep.status === "failed")) next.add(depId);
        }
        return next;
      });
    },
    [itemsById],
  );

  const convertToAlias = useCallback((item: SuggestionItem, dup: SuggestionDuplicate) => {
    if (!dup.entity_id) return;
    setDrafts((prev) => ({
      ...prev,
      [item.id]: { op: "update_entity", entity_id: dup.entity_id, aliases: [item.op.name ?? ""] },
    }));
    setSelected((prev) => new Set(prev).add(item.id));
  }, []);

  const setDraftField = useCallback((itemId: string, key: keyof ProposalOp, value: string) => {
    setDrafts((prev) => ({ ...prev, [itemId]: { ...prev[itemId], [key]: value } }));
  }, []);

  const resolvedOp = useCallback(
    (item: SuggestionItem): ProposalOp | null => {
      const draft = drafts[item.id];
      if (!draft) return null;
      if (draft.op && draft.op !== item.op.op) {
        // A changed op type (the alias-conversion button) is a complete
        // replacement, not a field patch: carrying the original op's other
        // fields along (name, kind, role, …) could apply them where they
        // do not belong — e.g. renaming the EXISTING entity to the
        // candidate's name via update_entity's optional name field.
        return draft as ProposalOp;
      }
      return { ...item.op, ...draft };
    },
    [drafts],
  );

  async function applySelected() {
    if (selected.size === 0) return;
    setBusy(true);
    try {
      const itemIds = [...selected];
      const edits: Record<string, ProposalOp> = {};
      for (const id of itemIds) {
        const item = itemsById.get(id);
        if (!item) continue;
        const op = resolvedOp(item);
        if (op) edits[id] = op;
      }
      const res = await proposalsApi.accept(projectId, itemIds, Object.keys(edits).length > 0 ? edits : undefined);
      const succeeded = res.items.filter((it) => it.status === "accepted").length;
      const failed = res.items.filter((it) => it.status === "failed").length;
      showToast(t("proposals.toast.applied", { succeeded, failed }));
      setSelected(new Set());
      setDrafts({});
      setLastUndoBatchIds(res.undo_batch_ids ?? []);
      setRevision((n) => n + 1);
    } catch (e) {
      showToast(rpcErrorMessage(e, t));
    } finally {
      setBusy(false);
    }
  }

  async function rejectSelected() {
    if (selected.size === 0) return;
    setBusy(true);
    try {
      await proposalsApi.reject(projectId, [...selected]);
      showToast(t("proposals.toast.rejected", { count: selected.size }));
      setSelected(new Set());
      setLastUndoBatchIds([]);
      setRevision((n) => n + 1);
    } catch (e) {
      showToast(rpcErrorMessage(e, t));
    } finally {
      setBusy(false);
    }
  }

  // story-assist Phase 4: reverts ONE structural batch applySelected just
  // created (purpose=forecast's create_outline_node/create_scene items,
  // plus any add_beat that depended on one) — the outline, and the beats,
  // together. Scoped to one batch id, not "undo everything selected":
  // 4.A.5's own undo is already scoped that way. Removes only this one id
  // from lastUndoBatchIds on success, leaving any other still-pending
  // batch's own revert button in place.
  async function undoBatch(batchId: string) {
    setBusy(true);
    try {
      await proposalsApi.undo(projectId, batchId);
      showToast(t("proposals.toast.undone"));
      setLastUndoBatchIds((prev) => prev.filter((id) => id !== batchId));
      setRevision((n) => n + 1);
    } catch (e) {
      showToast(rpcErrorMessage(e, t));
    } finally {
      setBusy(false);
    }
  }

  const groupLabel = (k: GroupKind) => t(`proposals.group.${k}`);

  return (
    <aside className="panel proposal-inbox" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl">{t("proposals.title", { count: pendingCount })}</span>
        <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}>
          <X size={16} />
        </button>
      </div>

      {(batches ?? []).length > 1 && (
        <div className="pi-filter">
          <select value={batchFilter} onChange={(e) => setBatchFilter(e.target.value)} aria-label={t("proposals.filterLabel")}>
            <option value="all">{t("proposals.filterAll")}</option>
            {(batches ?? []).map((b) => (
              <option key={b.id} value={b.id}>
                {b.summary || b.id}
              </option>
            ))}
          </select>
        </div>
      )}

      {progress && (
        <div className="pi-progress" data-testid="proposal-inbox-progress">
          {t("proposals.progress", { covered: progress.covered_episodes, total: progress.total_episodes })}
        </div>
      )}

      {lastUndoBatchIds.length > 0 && (
        <div className="pi-undo">
          {lastUndoBatchIds.map((batchId) => (
            <button
              key={batchId}
              type="button"
              className="chip"
              disabled={busy}
              onClick={() => undoBatch(batchId)}
              data-testid="proposal-inbox-undo"
            >
              {t("proposals.undoBatch")}
            </button>
          ))}
        </div>
      )}

      {error != null && (
        <p className="pi-error" role="alert">
          {rpcErrorMessage(error, t)}
        </p>
      )}
      {!batches && error == null && <p className="pi-loading">{t("common.loading")}</p>}
      {batches && allItems.length === 0 && (
        <div className="panel-scroll">
          <div className="sec">
            <p className="sec-empty">{t("proposals.empty")}</p>
          </div>
        </div>
      )}

      {batches && allItems.length > 0 && (
        <>
          <div className="panel-scroll pi-body">
            {GROUP_ORDER.filter((k) => (visibleItemsByGroup.get(k) ?? []).length > 0).map((k) => (
              <div className="sec pi-group" key={k}>
                <h4>
                  {groupLabel(k)} <span className="pi-group-count">{visibleItemsByGroup.get(k)?.length}</span>
                </h4>
                {(visibleItemsByGroup.get(k) ?? []).map((item) => {
                  const unmetDep = item.depends_on
                    .map((id) => itemsById.get(id))
                    .find((dep) => dep && dep.status !== "accepted");
                  const draft = drafts[item.id];
                  const isEditing = editing === item.id;
                  return (
                    <div key={item.id} className="pi-row" data-testid={`proposal-item-${item.id}`}>
                      <label className="pi-row-head">
                        <input
                          type="checkbox"
                          checked={selected.has(item.id)}
                          disabled={item.status === "reverting"}
                          onChange={() => toggleSelect(item)}
                          aria-label={item.summary}
                        />
                        <span className="pi-summary">{item.summary}</span>
                      </label>

                      {item.status === "reverting" && (
                        <p className="pi-note pi-note-reverting" data-testid={`proposal-item-reverting-${item.id}`}>
                          {t("proposals.item.reverting")}
                        </p>
                      )}
                      {item.blocked_by_rejected && (
                        <p className="pi-note pi-note-blocked">{t("proposals.item.blockedByRejected")}</p>
                      )}
                      {!item.blocked_by_rejected && unmetDep && (
                        <p className="pi-note">{t("proposals.item.dependsOn", { name: unmetDep.summary })}</p>
                      )}

                      {item.evidence.length > 0 && (
                        <ul className="pi-evidence">
                          {item.evidence.map((ev, i) => (
                            <li key={i}>
                              <span className="pi-quote">“{ev.quote}”</span>
                              {ev.breadcrumb && <span className="pi-breadcrumb">{ev.breadcrumb}</span>}
                              {onOpenNode && (
                                <button type="button" className="pi-link" onClick={() => onOpenNode(ev.node_id)}>
                                  {t("proposals.openScene")}
                                </button>
                              )}
                            </li>
                          ))}
                        </ul>
                      )}

                      {item.duplicates.length > 0 && (
                        <div className="pi-duplicates">
                          {item.duplicates.map((dup, i) => (
                            <div key={i} className="pi-duplicate">
                              <span>
                                {t("proposals.duplicate.maybeSame", {
                                  name: dup.name,
                                  kind: t(`entity.kind.${dup.kind}` as never),
                                })}
                              </span>
                              {dup.entity_id && (
                                <button type="button" className="pi-link" onClick={() => convertToAlias(item, dup)}>
                                  {t("proposals.duplicate.useAsAlias", { name: dup.name })}
                                </button>
                              )}
                            </div>
                          ))}
                        </div>
                      )}

                      {item.result.code === "name_taken" ? (
                        <p className="pi-note pi-note-error">{t("proposals.item.nameTaken", { name: item.op.name ?? "" })}</p>
                      ) : (
                        // dependency_missing is the unmet-dependency note above, in the writer's language.
                        item.result.error &&
                        item.result.code !== "dependency_missing" && (
                          <p className="pi-note pi-note-error">{item.result.error}</p>
                        )
                      )}

                      <div className="pi-row-actions">
                        <button
                          type="button"
                          className="pi-link"
                          onClick={() => setEditing(isEditing ? null : item.id)}
                        >
                          {isEditing ? t("proposals.edit.close") : t("proposals.edit.open")}
                        </button>
                      </div>

                      {isEditing && (
                        <div className="pi-edit-form">
                          {editableFields(item.op_type).map((f) => {
                            const value = (draft?.[f.key] as string | undefined) ?? (item.op[f.key] as string | undefined) ?? "";
                            return (
                              <label key={String(f.key)} className="pi-edit-field">
                                <span>{t(f.labelKey as never)}</span>
                                {f.multiline ? (
                                  <textarea
                                    value={value}
                                    onChange={(e) => setDraftField(item.id, f.key, e.target.value)}
                                  />
                                ) : (
                                  <input
                                    type="text"
                                    value={value}
                                    onChange={(e) => setDraftField(item.id, f.key, e.target.value)}
                                  />
                                )}
                              </label>
                            );
                          })}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            ))}
          </div>

          <div className="pi-footer">
            <button
              type="button"
              className="btn"
              disabled={busy || selected.size === 0}
              onClick={() => void rejectSelected()}
            >
              {t("proposals.rejectSelected")}
            </button>
            <button
              type="button"
              className="btn primary"
              disabled={busy || selected.size === 0}
              onClick={() => void applySelected()}
            >
              {t("proposals.acceptSelected")}
            </button>
          </div>
        </>
      )}
    </aside>
  );
}
