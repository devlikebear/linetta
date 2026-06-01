import { useCallback, useEffect, useState } from "react";
import type { Beat, Thread, UpdateThreadInput } from "../lib/types";
import { beats as beatsApi, threads as threadsApi } from "../lib/rpc";
import { X, Plus } from "../lib/icons";
import "./ThreadSheet.css";

const PALETTE = [
  "var(--t-sienna)",
  "var(--t-teal)",
  "var(--t-blue)",
  "var(--t-plum)",
  "var(--t-olive)",
  "#c08a3e",
  "#d35d6e",
  "#666",
];

interface Props {
  threadId: string | null;
  onClose: () => void;
  onSaved?: (thread: Thread) => void;
}

function IntensityPicker({ value, onPick }: { value: number; onPick: (lvl: number) => void }) {
  return (
    <span className="intensity-pick">
      {[1, 2, 3].map((lvl) => (
        <button key={lvl} type="button" className={value === lvl ? "sel" : ""} onClick={() => onPick(lvl)}>
          {lvl}
        </button>
      ))}
    </span>
  );
}

export function ThreadSheet({ threadId, onClose, onSaved }: Props) {
  const [thread, setThread] = useState<Thread | null>(null);
  const [draft, setDraft] = useState<UpdateThreadInput | null>(null);
  const [beatList, setBeatList] = useState<Beat[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async (id: string) => {
    const [th, bs] = await Promise.all([threadsApi.get(id), beatsApi.listByThread(id)]);
    setThread(th);
    setDraft({ id: th.id, name: th.name, color: th.color, summary: th.summary });
    setBeatList(bs);
  }, []);

  useEffect(() => {
    if (!threadId) return;
    setError(null);
    reload(threadId).catch((e) => setError(String(e)));
  }, [threadId, reload]);

  if (!threadId) return null;

  const onSave = async () => {
    if (!draft) return;
    setSaving(true);
    setError(null);
    try {
      const saved = await threadsApi.update(draft);
      setThread(saved);
      onSaved?.(saved);
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const addBeat = async () => {
    try {
      const created = await beatsApi.create({ thread_id: threadId, label: "" });
      setBeatList((prev) => [...prev, created]);
    } catch (e) {
      setError(String(e));
    }
  };

  const updateBeat = async (b: Beat, patch: { label?: string; description?: string; intensity?: number }) => {
    const next = { ...b, ...patch };
    setBeatList((prev) => prev.map((x) => (x.id === b.id ? next : x)));
    try {
      await beatsApi.update({ id: b.id, label: next.label, description: next.description, intensity: next.intensity });
    } catch (e) {
      setError(String(e));
    }
  };

  const deleteBeat = async (b: Beat) => {
    try {
      await beatsApi.delete(b.id);
      setBeatList((prev) => prev.filter((x) => x.id !== b.id));
    } catch (e) {
      setError(String(e));
    }
  };

  const closeThread = async () => {
    try {
      await threadsApi.close(threadId);
      onClose();
    } catch (e) {
      setError(String(e));
    }
  };

  return (
    <aside className="panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl">
          <span
            className="beat-dot"
            style={{ "--bc": draft?.color ?? thread?.color ?? "var(--muted)", width: 12, height: 12 } as React.CSSProperties}
            aria-hidden
          />
          스토리라인
        </span>
        <button type="button" className="panel-close" onClick={onClose} aria-label="닫기">
          <X size={16} />
        </button>
      </div>

      {error && <p className="ts-error">{error}</p>}
      {!thread && !error && <p className="ts-loading">불러오는 중…</p>}

      {thread && draft && (
        <>
          <div className="panel-scroll">
            <div className="sec es-field">
              <h4>이름</h4>
              <input
                value={draft.name ?? ""}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                placeholder="스토리라인 이름"
              />
              <div className="thread-color-row" style={{ marginTop: 14 }}>
                {PALETTE.map((c) => (
                  <button
                    key={c}
                    type="button"
                    aria-label={c}
                    className={"tc-swatch" + (draft.color === c ? " sel" : "")}
                    style={{ background: c }}
                    onClick={() => setDraft({ ...draft, color: c })}
                  />
                ))}
              </div>
            </div>

            <div className="sec es-field">
              <h4>메모</h4>
              <textarea
                value={draft.summary ?? ""}
                onChange={(e) => setDraft({ ...draft, summary: e.target.value })}
                rows={3}
              />
            </div>

            <div className="sec">
              <h4>마디 <span style={{ color: "var(--muted-2)" }}>{beatList.length}</span></h4>
              {beatList.length === 0 && <p className="sec-empty">아직 마디가 없어요</p>}
              {beatList.map((b) => (
                <div className="beat-list-item ts-beat" key={b.id}>
                  <span className="scn">#{b.ordinal}</span>
                  <div className="ts-beat-fields">
                    <input
                      className="ts-beat-label"
                      value={b.label}
                      onChange={(e) => updateBeat(b, { label: e.target.value })}
                      placeholder="마디 제목"
                    />
                    <textarea
                      className="ts-beat-desc"
                      value={b.description}
                      onChange={(e) => updateBeat(b, { description: e.target.value })}
                      placeholder="무슨 일이 일어나는지"
                      rows={2}
                    />
                  </div>
                  <IntensityPicker value={b.intensity} onPick={(lvl) => updateBeat(b, { intensity: lvl })} />
                  <button type="button" className="attr-del" onClick={() => deleteBeat(b)} aria-label="삭제">
                    <X size={13} />
                  </button>
                </div>
              ))}
              <button type="button" className="add-beat" onClick={addBeat}>
                <Plus size={13} /> 새 마디 추가
              </button>
            </div>
          </div>

          <div className="panel-foot">
            <button type="button" className="btn ghost sm" onClick={closeThread}>
              스토리라인 닫기(보관)
            </button>
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
