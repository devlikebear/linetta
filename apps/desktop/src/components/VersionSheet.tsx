import { useEffect, useState } from "react";
import { snapshots } from "../lib/rpc";
import type { NodeRow, SnapshotEntry } from "../lib/types";
import { X } from "../lib/icons";
import "./VersionSheet.css";

interface Props {
  nodeId: string | null;
  onClose: () => void;
  onRestored: (node: NodeRow) => void;
}

const REASON_LABEL: Record<SnapshotEntry["reason"], string> = {
  manual: "수동 저장",
  "ai-replace": "AI 교체 전",
  autosave: "자동 저장",
};

function formatTime(ts: number): string {
  const d = new Date(ts);
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  const hh = String(d.getHours()).padStart(2, "0");
  const mi = String(d.getMinutes()).padStart(2, "0");
  return `${yyyy}-${mm}-${dd} ${hh}:${mi}`;
}

export function VersionSheet({ nodeId, onClose, onRestored }: Props) {
  const [entries, setEntries] = useState<SnapshotEntry[] | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [restoring, setRestoring] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!nodeId) return;
    setEntries(null);
    setError(null);
    snapshots.listForNode(nodeId)
      .then((list) => {
        setEntries(list);
        setSelectedId(list[0]?.id ?? null);
      })
      .catch((e) => setError(String(e)));
  }, [nodeId]);

  if (!nodeId) return null;
  const selected = entries?.find((e) => e.id === selectedId) ?? null;

  const onRestore = async () => {
    if (!selected) return;
    setRestoring(true);
    setError(null);
    try {
      const node = await snapshots.restore(selected.id);
      onRestored(node);
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setRestoring(false);
    }
  };

  // Group: "주요" (manual + ai-replace) on top, then autosaves grouped by YYYY-MM-DD.
  const major: SnapshotEntry[] = [];
  const auto: SnapshotEntry[] = [];
  (entries ?? []).forEach((e) => {
    if (e.reason === "autosave") auto.push(e);
    else major.push(e);
  });
  const autoByDay: { day: string; rows: SnapshotEntry[] }[] = [];
  for (const e of auto) {
    const day = new Date(e.created_at).toISOString().slice(0, 10);
    const last = autoByDay[autoByDay.length - 1];
    if (last && last.day === day) last.rows.push(e);
    else autoByDay.push({ day, rows: [e] });
  }

  return (
    <aside className="panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl">이전 버전</span>
        <button type="button" className="panel-close" onClick={onClose} aria-label="닫기">
          <X size={16} />
        </button>
      </div>

      {error && <p className="vs-error">{error}</p>}
      {!entries && !error && <p className="vs-loading">불러오는 중…</p>}
      {entries && entries.length === 0 && (
        <div className="panel-scroll">
          <div className="sec"><p className="sec-empty">아직 저장된 버전이 없습니다.</p></div>
        </div>
      )}

      {entries && entries.length > 0 && (
        <>
          <div className="panel-scroll">
            <div className="sec">
              <h4>타임라인</h4>
              <div className="vs-timeline">
                {major.length > 0 && (
                  <div className="vs-group">
                    <p className="vs-group-head">주요 저장</p>
                    {major.map((e) => (
                      <button
                        key={e.id}
                        type="button"
                        className={"vs-row" + (e.id === selectedId ? " sel" : "")}
                        onClick={() => setSelectedId(e.id)}
                      >
                        <span className="vs-reason">{REASON_LABEL[e.reason]}</span>
                        <span className="vs-time">{formatTime(e.created_at)}</span>
                      </button>
                    ))}
                  </div>
                )}
                {autoByDay.map((g) => (
                  <div className="vs-group" key={g.day}>
                    <p className="vs-group-head">자동 저장 · {g.day}</p>
                    {g.rows.map((e) => (
                      <button
                        key={e.id}
                        type="button"
                        className={"vs-row" + (e.id === selectedId ? " sel" : "")}
                        onClick={() => setSelectedId(e.id)}
                      >
                        <span className="vs-time">{formatTime(e.created_at)}</span>
                      </button>
                    ))}
                  </div>
                ))}
              </div>
            </div>

            <div className="sec">
              <h4>미리보기</h4>
              <pre className="vs-preview">{selected?.doc_preview || "(빈 본문)"}</pre>
            </div>
          </div>

          <div className="panel-foot">
            <span className="spacer" />
            <button type="button" className="btn ghost sm" onClick={onClose} disabled={restoring}>취소</button>
            <button
              type="button"
              className="btn accent sm"
              onClick={onRestore}
              disabled={restoring || !selected}
            >
              {restoring ? "복원 중…" : "이 버전으로 복원"}
            </button>
          </div>
        </>
      )}
    </aside>
  );
}
