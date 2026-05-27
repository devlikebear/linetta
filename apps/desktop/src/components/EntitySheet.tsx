import { useCallback, useEffect, useState } from "react";
import type { Entity, EntityKind, Relationship, UpdateEntityInput } from "../lib/types";
import { entities, relationships } from "../lib/rpc";
import { RelationshipPicker } from "./RelationshipPicker";
import { X, Plus } from "../lib/icons";
import "./EntitySheet.css";

interface Props {
  entityId: string | null;
  onClose: () => void;
  onSaved?: (entity: Entity) => void;
}

const KIND_LABEL: Record<EntityKind, string> = {
  character: "인물",
  place: "장소",
  item: "물건",
  concept: "개념",
};

export function EntitySheet({ entityId, onClose, onSaved }: Props) {
  const [entity, setEntity] = useState<Entity | null>(null);
  const [draft, setDraft] = useState<UpdateEntityInput | null>(null);
  const [attrRows, setAttrRows] = useState<{ key: string; value: string }[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [rels, setRels] = useState<Relationship[]>([]);
  const [relTargets, setRelTargets] = useState<Record<string, Entity>>({});
  const [pickerOpen, setPickerOpen] = useState(false);

  const refreshRels = useCallback(async (eid: string) => {
    const list = await relationships.listByEntity(eid);
    setRels(list);
    const ids = Array.from(new Set(list.map((r) => r.to_id)));
    setRelTargets((cur) => {
      // skip ids we already have
      const need = ids.filter((id) => !cur[id]);
      if (need.length === 0) return cur;
      // fire-and-forget fetch the missing ones; functional updater wraps the merge
      Promise.all(need.map((id) => entities.get(id))).then((fetched) => {
        setRelTargets((cur2) => {
          const next = { ...cur2 };
          for (const e of fetched) next[e.id] = e;
          return next;
        });
      });
      return cur;
    });
  }, []);

  useEffect(() => {
    if (!entityId) return;
    setEntity(null);
    setError(null);
    setRels([]);
    entities.get(entityId).then((e) => {
      setEntity(e);
      setDraft({
        id: e.id,
        kind: e.kind,
        name: e.name,
        role: e.role,
        summary: e.summary,
        attributes: e.attributes,
      });
      setAttrRows(Object.entries(e.attributes).map(([key, value]) => ({ key, value })));
    }).catch((e) => setError(String(e)));
    refreshRels(entityId).catch((e) => setError(String(e)));
  }, [entityId, refreshRels]);

  if (!entityId) return null;

  const onSave = async () => {
    if (!draft) return;
    setSaving(true);
    setError(null);
    try {
      const attributes: Record<string, string> = {};
      for (const row of attrRows) {
        if (row.key.trim() !== "") attributes[row.key.trim()] = row.value;
      }
      const saved = await entities.update({ ...draft, attributes });
      setEntity(saved);
      if (onSaved) onSaved(saved);
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <aside className="entity-sheet" onMouseDown={(e) => e.stopPropagation()}>
      <header className="entity-head">
        <span>엔티티 편집</span>
        <button type="button" className="entity-close" onClick={onClose} aria-label="닫기">
          <X size={16} />
        </button>
      </header>

      {error && <p className="entity-error">{error}</p>}
      {!entity && !error && <p className="entity-loading">불러오는 중…</p>}

      {entity && draft && (
        <div className="entity-body">
          <div className="entity-id-row">
            <div className="entity-avatar">{(draft.name ?? entity.name).slice(0, 1)}</div>
            <div className="entity-id-text">
              <input
                className="entity-name"
                value={draft.name ?? ""}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                placeholder="이름"
              />
              <div className="entity-kind-row">
                <select
                  value={draft.kind ?? entity.kind}
                  onChange={(e) => setDraft({ ...draft, kind: e.target.value as EntityKind })}
                >
                  {(Object.keys(KIND_LABEL) as EntityKind[]).map((k) => (
                    <option key={k} value={k}>{KIND_LABEL[k]}</option>
                  ))}
                </select>
                <input
                  className="entity-role"
                  value={draft.role ?? ""}
                  onChange={(e) => setDraft({ ...draft, role: e.target.value })}
                  placeholder="역할 (예: POV)"
                />
              </div>
            </div>
          </div>

          <section className="entity-section">
            <h5>요약</h5>
            <textarea
              value={draft.summary ?? ""}
              onChange={(e) => setDraft({ ...draft, summary: e.target.value })}
              rows={3}
            />
          </section>

          <section className="entity-section">
            <h5>속성</h5>
            <div className="attr-table">
              {attrRows.map((row, i) => (
                <div className="attr-row" key={i}>
                  <input
                    className="attr-key"
                    value={row.key}
                    placeholder="키 (예: 나이)"
                    onChange={(e) => {
                      const next = [...attrRows];
                      next[i] = { ...row, key: e.target.value };
                      setAttrRows(next);
                    }}
                  />
                  <input
                    className="attr-value"
                    value={row.value}
                    placeholder="값 (예: 32)"
                    onChange={(e) => {
                      const next = [...attrRows];
                      next[i] = { ...row, value: e.target.value };
                      setAttrRows(next);
                    }}
                  />
                  <button
                    type="button"
                    className="attr-del"
                    onClick={() => setAttrRows(attrRows.filter((_, j) => j !== i))}
                    aria-label="삭제"
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
              <button
                type="button"
                className="attr-add"
                onClick={() => setAttrRows([...attrRows, { key: "", value: "" }])}
              >
                <Plus size={14} />
                <span>속성 추가</span>
              </button>
            </div>
          </section>

          <section className="entity-section relations">
            <h5>관계</h5>
            {rels.length === 0 && (
              <p className="entity-empty">아직 관계가 없어요</p>
            )}
            {rels.length > 0 && (
              <ul className="relation-list">
                {rels.map((r) => {
                  const target = relTargets[r.to_id];
                  return (
                    <li className="relation-row" key={r.id}>
                      <span className="relation-target">
                        {target ? target.name : r.to_id.slice(0, 6)}
                      </span>
                      <span className="relation-dash"> — </span>
                      <span className="relation-label">{r.label}</span>
                      <button
                        type="button"
                        className="relation-del"
                        aria-label="삭제"
                        onClick={async () => {
                          await relationships.delete(r.id);
                          if (entity) await refreshRels(entity.id);
                        }}
                      >
                        <X size={14} />
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
            <button
              type="button"
              className="relation-add"
              onClick={() => setPickerOpen(true)}
            >
              <Plus size={14} />
              <span>관계 추가</span>
            </button>
            {pickerOpen && entity && (
              <RelationshipPicker
                projectId={entity.project_id}
                fromEntityId={entity.id}
                onClose={() => setPickerOpen(false)}
                onCreated={() => {
                  if (entity) refreshRels(entity.id);
                }}
              />
            )}
          </section>

          <div className="entity-actions">
            <button type="button" onClick={onClose} disabled={saving}>취소</button>
            <button type="button" className="primary" onClick={onSave} disabled={saving}>
              {saving ? "저장 중…" : "저장"}
            </button>
          </div>
        </div>
      )}
    </aside>
  );
}
