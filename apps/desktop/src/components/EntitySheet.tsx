import { useEffect, useState } from "react";
import type { Entity, EntityKind, UpdateEntityInput } from "../lib/types";
import { entities } from "../lib/rpc";
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

  useEffect(() => {
    if (!entityId) return;
    setEntity(null);
    setError(null);
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

  return (
    <aside className="entity-sheet" onMouseDown={(e) => e.stopPropagation()}>
      <header className="entity-head">
        <span>엔티티 편집</span>
        <button type="button" className="entity-close" onClick={onClose} aria-label="닫기">×</button>
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
                  >×</button>
                </div>
              ))}
              <button
                type="button"
                className="attr-add"
                onClick={() => setAttrRows([...attrRows, { key: "", value: "" }])}
              >+ 속성 추가</button>
            </div>
          </section>

          <section className="entity-section relations">
            <h5>관계</h5>
            <p className="entity-empty">(post-MVP)</p>
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
