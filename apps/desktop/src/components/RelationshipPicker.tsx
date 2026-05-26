import { useEffect, useRef, useState } from "react";
import { entities, relationships } from "../lib/rpc";
import { LABEL_PRESETS } from "../lib/relationshipPresets";
import type { Entity } from "../lib/types";
import "./RelationshipPicker.css";

interface Props {
  projectId: string;
  fromEntityId: string;
  // Entities to hide from the search results (typically [fromEntityId]).
  excludeIds?: string[];
  onClose: () => void;
  onCreated: () => void; // parent refreshes the list
}

export function RelationshipPicker({
  projectId,
  fromEntityId,
  excludeIds = [],
  onClose,
  onCreated,
}: Props) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Entity[]>([]);
  const [target, setTarget] = useState<Entity | null>(null);
  const [label, setLabel] = useState("");
  const [inverseLabel, setInverseLabel] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const debounceRef = useRef<number | null>(null);

  const hide = new Set([fromEntityId, ...excludeIds]);

  useEffect(() => {
    if (debounceRef.current) window.clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(async () => {
      try {
        const list = await entities.search(projectId, query, 20);
        setResults(list.filter((e) => !hide.has(e.id)));
      } catch (e) {
        setError(String(e));
      }
    }, 200);
    return () => {
      if (debounceRef.current) window.clearTimeout(debounceRef.current);
    };
  }, [query, projectId, fromEntityId]);

  const onSave = async () => {
    if (!target || label.trim() === "") return;
    setSaving(true);
    setError(null);
    try {
      if (inverseLabel.trim() === "") {
        await relationships.createOne({
          project_id: projectId,
          from_id: fromEntityId,
          to_id: target.id,
          label: label.trim(),
        });
      } else {
        await relationships.createPair({
          project_id: projectId,
          from_id: fromEntityId,
          to_id: target.id,
          label: label.trim(),
          inverse_label: inverseLabel.trim(),
        });
      }
      onCreated();
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="rel-picker-backdrop" onMouseDown={onClose}>
      <div
        className="rel-picker"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="rel-picker-head">
          <span>관계 추가</span>
          <button
            type="button"
            className="rel-picker-close"
            onClick={onClose}
            aria-label="닫기"
          >×</button>
        </header>

        {error && <p className="rel-picker-error">{error}</p>}

        <section className="rel-picker-section">
          <h6>대상</h6>
          {!target ? (
            <>
              <input
                className="rel-picker-search"
                placeholder="엔티티 이름 검색"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                autoFocus
              />
              <div className="rel-picker-results">
                {results.length === 0 && (
                  <p className="rel-picker-empty">결과 없음</p>
                )}
                {results.map((e) => (
                  <button
                    type="button"
                    key={e.id}
                    className="rel-picker-result"
                    onClick={() => setTarget(e)}
                  >
                    <span className="rel-picker-result-name">{e.name}</span>
                    {e.role && (
                      <span className="rel-picker-result-role">{e.role}</span>
                    )}
                  </button>
                ))}
              </div>
            </>
          ) : (
            <div className="rel-picker-target">
              <span className="rel-picker-target-name">{target.name}</span>
              <button
                type="button"
                className="rel-picker-change"
                onClick={() => setTarget(null)}
              >변경</button>
            </div>
          )}
        </section>

        <section className="rel-picker-section">
          <h6>관계 (이쪽 → 상대)</h6>
          <div className="rel-picker-chips">
            {LABEL_PRESETS.map((p) => (
              <button
                key={p}
                type="button"
                className={`rel-picker-chip${label === p ? " active" : ""}`}
                onClick={() => setLabel(p)}
              >{p}</button>
            ))}
          </div>
          <input
            className="rel-picker-label"
            placeholder="예: 친구"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
          />
        </section>

        <section className="rel-picker-section">
          <h6>역방향 라벨 (선택)</h6>
          <p className="rel-picker-hint">
            비워두면 단방향 한 줄만 저장됩니다. 입력하면 상대 쪽에도 자동으로 추가됩니다.
          </p>
          <div className="rel-picker-chips">
            {LABEL_PRESETS.map((p) => (
              <button
                key={p}
                type="button"
                className={`rel-picker-chip${inverseLabel === p ? " active" : ""}`}
                onClick={() => setInverseLabel(p)}
              >{p}</button>
            ))}
          </div>
          <input
            className="rel-picker-label"
            placeholder="예: 친구 (또는 비워두기)"
            value={inverseLabel}
            onChange={(e) => setInverseLabel(e.target.value)}
          />
        </section>

        <div className="rel-picker-actions">
          <button type="button" onClick={onClose} disabled={saving}>취소</button>
          <button
            type="button"
            className="primary"
            disabled={saving || !target || label.trim() === ""}
            onClick={onSave}
          >{saving ? "저장 중…" : "저장"}</button>
        </div>
      </div>
    </div>
  );
}
