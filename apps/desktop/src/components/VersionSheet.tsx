import { useEffect, useState } from "react";
import { snapshots } from "../lib/rpc";
import type { NodeRow, SnapshotEntry } from "../lib/types";
import { X } from "../lib/icons";
import { useI18n } from "../lib/i18n";
import "./VersionSheet.css";

interface Props {
  nodeId: string | null;
  onClose: () => void;
  onRestored: (node: NodeRow) => void;
}

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
  const { t } = useI18n();
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
  const reasonLabel = (reason: SnapshotEntry["reason"]) => {
    if (reason === "ai-replace") return t("version.reason.aiReplace");
    return t(`version.reason.${reason}`);
  };

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
        <span className="ttl">{t("version.title")}</span>
        <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}>
          <X size={16} />
        </button>
      </div>

      {error && <p className="vs-error">{error}</p>}
      {!entries && !error && <p className="vs-loading">{t("common.loading")}</p>}
      {entries && entries.length === 0 && (
        <div className="panel-scroll">
          <div className="sec"><p className="sec-empty">{t("version.empty")}</p></div>
        </div>
      )}

      {entries && entries.length > 0 && (
        <>
          <div className="panel-scroll">
            <div className="sec">
              <h4>{t("version.timeline")}</h4>
              <div className="vs-timeline">
                {major.length > 0 && (
                  <div className="vs-group">
                    <p className="vs-group-head">{t("version.major")}</p>
                    {major.map((e) => (
                      <button
                        key={e.id}
                        type="button"
                        className={"vs-row" + (e.id === selectedId ? " sel" : "")}
                        onClick={() => setSelectedId(e.id)}
                      >
                        <span className="vs-reason">{reasonLabel(e.reason)}</span>
                        <span className="vs-time">{formatTime(e.created_at)}</span>
                      </button>
                    ))}
                  </div>
                )}
                {autoByDay.map((g) => (
                  <div className="vs-group" key={g.day}>
                    <p className="vs-group-head">{t("version.autoGroup", { day: g.day })}</p>
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
              <h4>{t("version.preview")}</h4>
              <pre className="vs-preview">{selected?.doc_preview || t("version.emptyBody")}</pre>
            </div>
          </div>

          <div className="panel-foot">
            <span className="spacer" />
            <button type="button" className="btn ghost sm" onClick={onClose} disabled={restoring}>{t("common.cancel")}</button>
            <button
              type="button"
              className="btn accent sm"
              onClick={onRestore}
              disabled={restoring || !selected}
            >
              {restoring ? t("version.restoring") : t("version.restore")}
            </button>
          </div>
        </>
      )}
    </aside>
  );
}
