import { useCallback, useEffect, useState } from "react";
import type { Entity, EntityKind, Relationship, SceneMention, UpdateEntityInput } from "../lib/types";
import { entities, relationships } from "../lib/rpc";
import { RelationshipPicker } from "./RelationshipPicker";
import { X, Plus, User, MapPin, Box, Lightbulb } from "../lib/icons";
import "./EntitySheet.css";

interface Props {
  entityId: string | null;
  onClose: () => void;
  onSaved?: (entity: Entity) => void;
  onNavigate?: (nodeId: string) => void;
}

const KIND_META: Record<EntityKind, { label: string; color: string; Icon: typeof User }> = {
  character: { label: "인물", color: "var(--t-sienna)", Icon: User },
  place: { label: "장소", color: "var(--t-teal)", Icon: MapPin },
  item: { label: "물건", color: "var(--t-olive)", Icon: Box },
  concept: { label: "개념", color: "var(--t-plum)", Icon: Lightbulb },
};

export function EntitySheet({ entityId, onClose, onSaved, onNavigate }: Props) {
  const [entity, setEntity] = useState<Entity | null>(null);
  const [draft, setDraft] = useState<UpdateEntityInput | null>(null);
  const [attrRows, setAttrRows] = useState<{ key: string; value: string }[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [rels, setRels] = useState<Relationship[]>([]);
  const [relTargets, setRelTargets] = useState<Record<string, Entity>>({});
  const [pickerOpen, setPickerOpen] = useState(false);
  const [scenes, setScenes] = useState<SceneMention[]>([]);

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

  useEffect(() => {
    if (!entityId) {
      setScenes([]);
      return;
    }
    let cancelled = false;
    entities.scenes(entityId)
      .then((s) => { if (!cancelled) setScenes(s); })
      .catch(() => { if (!cancelled) setScenes([]); });
    return () => { cancelled = true; };
  }, [entityId]);

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

  const kind = (draft?.kind ?? entity?.kind ?? "character") as EntityKind;
  const meta = KIND_META[kind];
  const HeadIcon = meta.Icon;

  return (
    <aside className="panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl">
          <span className="ic"><HeadIcon size={16} /></span> 엔티티 편집
        </span>
        <button type="button" className="panel-close" onClick={onClose} aria-label="닫기">
          <X size={16} />
        </button>
      </div>

      {error && <p className="es-error">{error}</p>}
      {!entity && !error && <p className="es-loading">불러오는 중…</p>}

      {entity && draft && (
        <>
          <div className="panel-scroll">
            <div className="sec">
              <div className="es-id">
                <span className="es-av" style={{ "--av": meta.color } as React.CSSProperties}>
                  {(draft.name ?? entity.name).slice(0, 1) || "?"}
                </span>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <input
                    className="es-name"
                    value={draft.name ?? ""}
                    onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                    placeholder="이름"
                  />
                  <div className="es-kindrow">
                    <label className="es-kind-chip">
                      <HeadIcon size={12} />
                      <select
                        value={kind}
                        onChange={(e) => setDraft({ ...draft, kind: e.target.value as EntityKind })}
                      >
                        {(Object.keys(KIND_META) as EntityKind[]).map((k) => (
                          <option key={k} value={k}>{KIND_META[k].label}</option>
                        ))}
                      </select>
                    </label>
                    <input
                      className="es-role"
                      value={draft.role ?? ""}
                      onChange={(e) => setDraft({ ...draft, role: e.target.value })}
                      placeholder="역할 (예: POV)"
                    />
                  </div>
                </div>
              </div>
            </div>

            <div className="sec es-field">
              <h4>요약</h4>
              <textarea
                value={draft.summary ?? ""}
                onChange={(e) => setDraft({ ...draft, summary: e.target.value })}
                rows={5}
              />
            </div>

            <div className="sec es-field">
              <h4>속성</h4>
              <div className="attr-grid">
                {attrRows.map((row, i) => (
                  <div className="attr-row" key={i}>
                    <input
                      className="attr-k"
                      value={row.key}
                      placeholder="키 (예: 나이)"
                      onChange={(e) => {
                        const next = [...attrRows];
                        next[i] = { ...row, key: e.target.value };
                        setAttrRows(next);
                      }}
                    />
                    <input
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
                      <X size={13} />
                    </button>
                  </div>
                ))}
                <button
                  type="button"
                  className="attr-add"
                  onClick={() => setAttrRows([...attrRows, { key: "", value: "" }])}
                >
                  <Plus size={13} /> 속성 추가
                </button>
              </div>
            </div>

            <div className="sec">
              <h4>관계</h4>
              {rels.length === 0 && <p className="sec-empty">아직 관계가 없어요</p>}
              {rels.length > 0 && (
                <ul className="rel-list">
                  {rels.map((r) => {
                    const target = relTargets[r.to_id];
                    return (
                      <li className="rel-row" key={r.id}>
                        <span className="rel-target">
                          {target ? target.name : r.to_id.slice(0, 6)}
                        </span>
                        <span className="gap" />
                        <span className="rel-label">{r.label}</span>
                        <button
                          type="button"
                          className="attr-del"
                          aria-label="삭제"
                          onClick={async () => {
                            await relationships.delete(r.id);
                            if (entity) await refreshRels(entity.id);
                          }}
                        >
                          <X size={13} />
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
              <button
                type="button"
                className="rel-add"
                onClick={() => setPickerOpen(true)}
              >
                <Plus size={13} /> 관계 추가
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
            </div>

            <div className="sec">
              <h4>등장 씬 <span style={{ color: "var(--muted-2)" }}>{scenes.length}</span></h4>
              {scenes.length === 0 ? (
                <p className="sec-empty">아직 등장한 씬이 없어요</p>
              ) : (
                <div className="scene-chips">
                  {scenes.map((s) => (
                    <button
                      key={s.node_id}
                      type="button"
                      className="scene-chip"
                      onClick={() => onNavigate?.(s.node_id)}
                    >
                      {s.label}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="panel-foot">
            <span className="spacer" />
            <button type="button" className="btn ghost sm" onClick={onClose} disabled={saving}>취소</button>
            <button type="button" className="btn accent sm" onClick={onSave} disabled={saving}>
              {saving ? "저장 중…" : "저장"}
            </button>
          </div>
        </>
      )}
    </aside>
  );
}
