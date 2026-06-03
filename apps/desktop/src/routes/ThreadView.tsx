import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { ChevronLeft } from "lucide-react";
import { beats as beatsApi, threads as threadsApi } from "../lib/rpc";
import type { Beat, Thread } from "../lib/types";
import { useI18n } from "../lib/i18n";
import "./ThreadView.css";

const INTENSITY_PX: Record<number, number> = { 1: 14, 2: 22, 3: 30 };

interface Lane {
  thread: Thread;
  beats: Beat[];
}

export function ThreadView() {
  const { t } = useI18n();
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

  const jumpTo = (b: Beat) => {
    if (!b.node_id) return;
    navigate(`/workspace/${projectId}`, { state: { jumpToNodeId: b.node_id } });
  };

  return (
    <div className="thread-view">
      <div className="lib-top">
        <Link to={`/workspace/${projectId}`} className="btn ghost sm">
          <ChevronLeft size={15} /> {t("threadView.workspace")}
        </Link>
        <div className="lib-brandmark">{t("threadView.title")}</div>
        <span style={{ width: 90 }} />
      </div>

      <div className="thread-inner">
        {error && <p className="thread-error">{error}</p>}
        {!error && !lanes && <p className="thread-hint">{t("common.loading")}</p>}
        {!error && lanes && lanes.length === 0 && (
          <p className="thread-hint">{t("threadView.empty", { command: t("workspace.markSceneAsThread") })}</p>
        )}

        {!error && lanes && lanes.length > 0 && (
          <div className="thread-lanes">
            {lanes.map(({ thread, beats }) => (
              <div className="panel thread-lane" key={thread.id}>
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
                        title={`#${b.ordinal} ${b.label}${isOrphan ? ` (${t("threadView.deletedScene")})` : ""}`}
                        style={{
                          left,
                          width: size,
                          height: size,
                          backgroundColor: isOrphan ? "var(--muted-2)" : thread.color,
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
        )}
      </div>
    </div>
  );
}
