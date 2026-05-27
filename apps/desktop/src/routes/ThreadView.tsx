import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { beats as beatsApi, threads as threadsApi } from "../lib/rpc";
import type { Beat, Thread } from "../lib/types";
import "./ThreadView.css";

const INTENSITY_PX: Record<number, number> = { 1: 14, 2: 22, 3: 30 };

interface Lane {
  thread: Thread;
  beats: Beat[];
}

export function ThreadView() {
  const { projectId } = useParams();
  const navigate = useNavigate();
  const [lanes, setLanes] = useState<Lane[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!projectId) return;
    let cancelled = false;
    (async () => {
      try {
        const ts = await threadsApi.list(projectId, false);
        const lanes = await Promise.all(
          ts.map(async (t) => ({ thread: t, beats: await beatsApi.listByThread(t.id) })),
        );
        if (!cancelled) setLanes(lanes);
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    })();
    return () => { cancelled = true; };
  }, [projectId]);

  const maxOrdinal = useMemo(
    () => Math.max(1, ...((lanes ?? []).flatMap((l) => l.beats.map((b) => b.ordinal)))),
    [lanes],
  );

  if (error) return <main className="shell"><p className="error">{error}</p></main>;
  if (!lanes) return <main className="shell"><p className="hint">불러오는 중…</p></main>;

  const jumpTo = (b: Beat) => {
    if (!b.node_id) return;
    navigate(`/workspace/${projectId}`, { state: { jumpToNodeId: b.node_id } });
  };

  return (
    <main className="thread-view">
      <header className="thread-view-top">
        <Link to={`/workspace/${projectId}`} className="thread-view-back">← 작업실</Link>
        <h1>흐름</h1>
      </header>

      {lanes.length === 0 && <p className="hint">아직 스토리라인이 없어요. Cmd+K → "이 씬을 새 Thread로 표시"로 시작하세요.</p>}

      <div className="thread-lanes">
        {lanes.map(({ thread, beats }) => (
          <div className="thread-lane" key={thread.id}>
            <div className="thread-lane-head">
              <span className="thread-dot" style={{ backgroundColor: thread.color }} />
              <span className="thread-lane-name">{thread.name}</span>
              {thread.summary && <span className="thread-lane-summary">{thread.summary}</span>}
            </div>
            <div className="thread-lane-track">
              {beats.map((b) => {
                const left = `${((b.ordinal - 1) / Math.max(1, maxOrdinal - 1)) * 100}%`;
                const size = INTENSITY_PX[b.intensity] ?? 14;
                const isOrphan = !b.node_id;
                return (
                  <button
                    key={b.id}
                    type="button"
                    className={`beat-disc${isOrphan ? " orphan" : ""}`}
                    title={`#${b.ordinal} ${b.label}${isOrphan ? " (씬 삭제됨)" : ""}`}
                    style={{
                      left,
                      width: size,
                      height: size,
                      backgroundColor: isOrphan ? "#999" : thread.color,
                    }}
                    disabled={isOrphan}
                    onClick={() => jumpTo(b)}
                  />
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </main>
  );
}
