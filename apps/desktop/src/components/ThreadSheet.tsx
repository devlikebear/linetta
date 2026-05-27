import { useCallback, useEffect, useState } from "react";
import type { Beat, Thread, UpdateThreadInput } from "../lib/types";
import { beats as beatsApi, threads as threadsApi } from "../lib/rpc";
import { X, Plus } from "../lib/icons";
import "./ThreadSheet.css";

const PALETTE = ["#c0392b", "#c08a3e", "#b58a00", "#3e8e41", "#2980b9", "#7e57c2", "#d35d6e", "#666"];

interface Props {
  threadId: string | null;
  onClose: () => void;
  onSaved?: (thread: Thread) => void;
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

  const updateBeat = async (b: Beat, patch: { label?: string; intensity?: number }) => {
    const next = { ...b, ...patch };
    setBeatList((prev) => prev.map((x) => (x.id === b.id ? next : x)));
    try {
      await beatsApi.update({ id: b.id, label: next.label, intensity: next.intensity });
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
    <aside className="thread-sheet" onMouseDown={(e) => e.stopPropagation()}>
      <header className="thread-head">
        <span>스토리라인 편집</span>
        <button type="button" className="thread-close" onClick={onClose} aria-label="닫기">
          <X size={16} />
        </button>
      </header>

      {error && <p className="thread-error">{error}</p>}
      {!thread && !error && <p className="thread-loading">불러오는 중…</p>}

      {thread && draft && (
        <div className="thread-body">
          <section className="thread-section">
            <input
              className="thread-name"
              value={draft.name ?? ""}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              placeholder="스토리라인 이름"
            />
          </section>

          <section className="thread-section">
            <h5>색</h5>
            <div className="thread-colors">
              {PALETTE.map((c) => (
                <button
                  key={c}
                  type="button"
                  aria-label={c}
                  className={`thread-color-swatch${draft.color === c ? " sel" : ""}`}
                  style={{ backgroundColor: c }}
                  onClick={() => setDraft({ ...draft, color: c })}
                />
              ))}
            </div>
          </section>

          <section className="thread-section">
            <h5>요약</h5>
            <textarea
              value={draft.summary ?? ""}
              onChange={(e) => setDraft({ ...draft, summary: e.target.value })}
              rows={3}
            />
          </section>

          <section className="thread-section">
            <h5>마디</h5>
            {beatList.length === 0 && <p className="thread-empty">아직 마디 없음</p>}
            {beatList.map((b) => (
              <div className="beat-row" key={b.id}>
                <span className="beat-ordinal">#{b.ordinal}</span>
                <input
                  className="attr-value"
                  value={b.label}
                  onChange={(e) => updateBeat(b, { label: e.target.value })}
                  placeholder="마디 설명"
                />
                <div className="beat-intensity">
                  {[1, 2, 3].map((lvl) => (
                    <button
                      key={lvl}
                      type="button"
                      className={b.intensity === lvl ? "sel" : ""}
                      onClick={() => updateBeat(b, { intensity: lvl })}
                    >{lvl}</button>
                  ))}
                </div>
                <button type="button" className="attr-del" onClick={() => deleteBeat(b)} aria-label="삭제">
                  <X size={14} />
                </button>
              </div>
            ))}
            <button type="button" className="attr-add" onClick={addBeat}>
              <Plus size={14} />
              <span>새 마디 추가</span>
            </button>
          </section>

          <div className="thread-actions">
            <button type="button" className="thread-close-action" onClick={closeThread}>
              이 스토리라인 닫기
            </button>
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
