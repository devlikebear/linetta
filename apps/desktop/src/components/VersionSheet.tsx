import { useEffect, useState } from "react";
import { snapshots } from "../lib/rpc";
import type { NodeRow, SnapshotCompareResult, SnapshotEntry } from "../lib/types";
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

type ViewMode = "preview" | "compare";
type DiffKind = "same" | "removed" | "added";

interface DiffLine {
  kind: DiffKind;
  text: string;
}

function splitLines(text: string): string[] {
  const lines = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n");
  if (lines.length > 1 && lines[lines.length - 1] === "") lines.pop();
  return lines;
}

function diffLines(leftText: string, rightText: string): DiffLine[] {
  const left = splitLines(leftText);
  const right = splitLines(rightText);
  const dp = Array.from({ length: left.length + 1 }, () => Array(right.length + 1).fill(0));
  for (let i = left.length - 1; i >= 0; i--) {
    for (let j = right.length - 1; j >= 0; j--) {
      dp[i][j] = left[i] === right[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }

  const out: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < left.length && j < right.length) {
    if (left[i] === right[j]) {
      out.push({ kind: "same", text: left[i] });
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      out.push({ kind: "removed", text: left[i] });
      i++;
    } else {
      out.push({ kind: "added", text: right[j] });
      j++;
    }
  }
  while (i < left.length) {
    out.push({ kind: "removed", text: left[i] });
    i++;
  }
  while (j < right.length) {
    out.push({ kind: "added", text: right[j] });
    j++;
  }
  return out;
}

export function VersionSheet({ nodeId, onClose, onRestored }: Props) {
  const { t } = useI18n();
  const [entries, setEntries] = useState<SnapshotEntry[] | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>("preview");
  const [compareLeftId, setCompareLeftId] = useState<string | null>(null);
  const [compareRightId, setCompareRightId] = useState<string | null>(null);
  const [comparison, setComparison] = useState<SnapshotCompareResult | null>(null);
  const [comparing, setComparing] = useState(false);
  const [compareError, setCompareError] = useState<string | null>(null);
  const [restoring, setRestoring] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!nodeId) return;
    setEntries(null);
    setError(null);
    setCompareError(null);
    setComparison(null);
    setComparing(false);
    setViewMode("preview");
    snapshots.listForNode(nodeId)
      .then((list) => {
        setEntries(list);
        setSelectedId(list[0]?.id ?? null);
        setCompareLeftId(list[1]?.id ?? list[0]?.id ?? null);
        setCompareRightId(list[0]?.id ?? null);
      })
      .catch((e) => setError(String(e)));
  }, [nodeId]);

  const selected = entries?.find((e) => e.id === selectedId) ?? null;
  const compareLeft = entries?.find((e) => e.id === compareLeftId) ?? null;
  const compareRight = entries?.find((e) => e.id === compareRightId) ?? null;
  const canCompare = Boolean(compareLeftId && compareRightId && compareLeftId !== compareRightId);
  const diff = comparison && comparison.left.id === compareLeftId && comparison.right.id === compareRightId
    ? diffLines(comparison.left.plaintext, comparison.right.plaintext)
    : [];
  const hasChanges = diff.some((line) => line.kind !== "same");
  const reasonLabel = (reason: SnapshotEntry["reason"]) => {
    if (reason === "companion-before") return t("version.reason.companionBefore");
    return t(`version.reason.${reason}`);
  };

  useEffect(() => {
    if (!nodeId || viewMode !== "compare" || !canCompare || !compareLeftId || !compareRightId) return;
    let cancelled = false;
    setComparing(true);
    setCompareError(null);
    snapshots.compare(compareLeftId, compareRightId)
      .then((got) => {
        if (!cancelled) setComparison(got);
      })
      .catch((e) => {
        if (!cancelled) setCompareError(String(e));
      })
      .finally(() => {
        if (!cancelled) setComparing(false);
      });
    return () => { cancelled = true; };
  }, [canCompare, compareLeftId, compareRightId, nodeId, viewMode]);

  if (!nodeId) return null;

  const setCompareSlot = (slot: "left" | "right", id: string) => {
    if (slot === "left") setCompareLeftId(id);
    else setCompareRightId(id);
    setComparison(null);
    setCompareError(null);
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

  // Group: "주요" (manual + companion-before) on top, then autosaves by YYYY-MM-DD.
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

  const renderRow = (e: SnapshotEntry) => {
    const time = formatTime(e.created_at);
    return (
      <div key={e.id} className={"vs-row" + (e.id === selectedId ? " sel" : "")}>
        <button type="button" className="vs-row-main" onClick={() => setSelectedId(e.id)}>
          <span className="vs-reason">{reasonLabel(e.reason)}</span>
          <span className="vs-time">{time}</span>
        </button>
        <div className="vs-compare-picks" aria-label={t("version.compare.pickGroup")}>
          <button
            type="button"
            className={"vs-slot" + (compareLeftId === e.id ? " on" : "")}
            onClick={() => setCompareSlot("left", e.id)}
            aria-label={t("version.compare.pickA", { time })}
          >
            A
          </button>
          <button
            type="button"
            className={"vs-slot" + (compareRightId === e.id ? " on" : "")}
            onClick={() => setCompareSlot("right", e.id)}
            aria-label={t("version.compare.pickB", { time })}
          >
            B
          </button>
        </div>
      </div>
    );
  };

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
                    {major.map(renderRow)}
                  </div>
                )}
                {autoByDay.map((g) => (
                  <div className="vs-group" key={g.day}>
                    <p className="vs-group-head">{t("version.autoGroup", { day: g.day })}</p>
                    {g.rows.map(renderRow)}
                  </div>
                ))}
              </div>
            </div>

            <div className="sec">
              <div className="vs-view-tabs" role="tablist" aria-label={t("version.viewMode")}>
                <button
                  type="button"
                  role="tab"
                  aria-selected={viewMode === "preview"}
                  className={viewMode === "preview" ? "on" : ""}
                  onClick={() => setViewMode("preview")}
                >
                  {t("version.preview")}
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={viewMode === "compare"}
                  className={viewMode === "compare" ? "on" : ""}
                  onClick={() => setViewMode("compare")}
                >
                  {t("version.compare")}
                </button>
              </div>

              {viewMode === "preview" && (
                <pre className="vs-preview">{selected?.doc_preview || t("version.emptyBody")}</pre>
              )}

              {viewMode === "compare" && (
                <div className="vs-compare">
                  <div className="vs-compare-head">
                    <span>{t("version.compare.a", { time: compareLeft ? formatTime(compareLeft.created_at) : "-" })}</span>
                    <span>{t("version.compare.b", { time: compareRight ? formatTime(compareRight.created_at) : "-" })}</span>
                  </div>
                  {!canCompare && <p className="vs-hint">{t("version.compare.needTwo")}</p>}
                  {compareError && <p className="vs-error inline">{compareError}</p>}
                  {canCompare && comparing && <p className="vs-loading inline">{t("version.compare.loading")}</p>}
                  {canCompare && !comparing && comparison && !hasChanges && (
                    <p className="vs-hint">{t("version.compare.same")}</p>
                  )}
                  {canCompare && !comparing && comparison && hasChanges && (
                    <pre className="vs-diff" aria-label={t("version.compare.diffLabel")}>
                      {diff.map((line, idx) => {
                        const prefix = line.kind === "removed" ? "- " : line.kind === "added" ? "+ " : "  ";
                        return (
                          <span key={`${idx}-${line.kind}`} className={`vs-diff-line ${line.kind}`}>
                            {prefix}{line.text || " "}
                          </span>
                        );
                      })}
                    </pre>
                  )}
                </div>
              )}
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
