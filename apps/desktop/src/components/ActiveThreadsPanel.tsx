import { useCallback, useEffect, useState } from "react";
import type { Thread } from "../lib/types";
import { beats as beatsApi, threads as threadsApi } from "../lib/rpc";
import { Plus } from "../lib/icons";

interface Props {
  projectId: string;
  nodeId: string;
  onOpenThread: (threadId: string) => void;
  onChanged?: () => void;
}

interface Row {
  thread: Thread;
}

export function ActiveThreadsPanel({ projectId, nodeId, onOpenThread, onChanged }: Props) {
  const [rows, setRows] = useState<Row[] | null>(null);
  const [adding, setAdding] = useState<string | null>(null); // threadId currently showing the label-prompt
  const [draftLabel, setDraftLabel] = useState("");

  const reload = useCallback(async () => {
    try {
      const nodeBeats = await beatsApi.listByNode(nodeId);
      const ids = Array.from(new Set(nodeBeats.map((b) => b.thread_id)));
      if (ids.length === 0) {
        setRows([]);
        return;
      }
      const all = await threadsApi.list(projectId, false);
      const map = new Map(all.map((t) => [t.id, t]));
      setRows(ids.map((id) => map.get(id)).filter((t): t is Thread => !!t).map((thread) => ({ thread })));
    } catch {
      setRows([]); // benign
    }
  }, [projectId, nodeId]);

  useEffect(() => { reload(); }, [reload]);

  const submitBeat = async (threadId: string) => {
    if (!draftLabel.trim()) { setAdding(null); return; }
    try {
      await beatsApi.create({ thread_id: threadId, node_id: nodeId, label: draftLabel.trim() });
      setAdding(null);
      setDraftLabel("");
      onChanged?.();
      reload();
    } catch { /* benign */ }
  };

  return (
    <section className="ctx-section">
      <h4>활성 Thread</h4>
      {rows && rows.length === 0 && <p className="ctx-empty">이 씬에 연결된 스토리라인 없음</p>}
      {rows && rows.map(({ thread }) => (
        <div key={thread.id} className="ctx-entity-row" style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
          <button
            type="button"
            className="ctx-entity"
            onClick={() => onOpenThread(thread.id)}
            style={{ flex: 1 }}
          >
            <span
              aria-hidden
              style={{
                display: "inline-block", width: 10, height: 10, borderRadius: "50%",
                backgroundColor: thread.color, marginRight: "0.5rem",
              }}
            />
            <span className="ctx-entity-name">{thread.name}</span>
          </button>
          <button
            type="button"
            aria-label="이 씬에 마디 추가"
            onClick={() => { setAdding(thread.id); setDraftLabel(""); }}
            style={{ background: "none", border: "1px solid #d8d6cf", borderRadius: 4, cursor: "pointer", padding: "0 0.35rem", display: "inline-flex", alignItems: "center", justifyContent: "center" }}
          >
            <Plus size={12} />
          </button>
        </div>
      ))}
      {adding && (
        <input
          autoFocus
          className="attr-value"
          value={draftLabel}
          placeholder="이 씬에서 일어난 마디 (Enter)"
          onChange={(e) => setDraftLabel(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") { e.preventDefault(); submitBeat(adding); }
            else if (e.key === "Escape") { e.preventDefault(); setAdding(null); }
          }}
          onBlur={() => setAdding(null)}
        />
      )}
    </section>
  );
}
