import { useState } from "react";
import { GitBranch, Undo2 } from "lucide-react";
import type { OutlineChangePreview, OutlinePreviewNode } from "../../lib/types";
import { applyProposal } from "../../lib/applyProposal";
import { companion as companionApi } from "../../lib/rpc";
import { useI18n } from "../../lib/i18n";

type Translate = ReturnType<typeof useI18n>["t"];

// Rows shown before the list collapses. A full-book outline runs to dozens of
// rows; the first screenful is enough to judge the shape of the change.
const COLLAPSED_ROWS = 12;

function countsSummary(t: Translate, preview: OutlineChangePreview): string {
  const { counts } = preview;
  const parts: string[] = [];
  if (counts.created > 0) parts.push(t("companion.preview.created", { count: String(counts.created) }));
  if (counts.renamed > 0) parts.push(t("companion.preview.renamed", { count: String(counts.renamed) }));
  if (counts.deleted > 0) parts.push(t("companion.preview.deleted", { count: String(counts.deleted) }));
  if (counts.moved > 0) parts.push(t("companion.preview.moved", { count: String(counts.moved) }));
  if (counts.other > 0) parts.push(t("companion.preview.other", { count: String(counts.other) }));
  return parts.join(" · ");
}

function actionLabel(t: Translate, action: OutlinePreviewNode["action"]): string {
  switch (action) {
    case "rename": return t("companion.preview.action.rename");
    case "delete": return t("companion.preview.action.delete");
    case "move": return t("companion.preview.action.move");
    default: return t("companion.preview.action.create");
  }
}

interface Props {
  preview: OutlineChangePreview;
  projectId: string;
  nodeIdRef: { current: string | null };
  onApplied: () => void;
}

export function OutlinePreviewCard({ preview, projectId, nodeIdRef, onApplied }: Props) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<"pending" | "applied" | "dismissed">("pending");
  const [undoBatchId, setUndoBatchId] = useState<string | null>(null);
  const [notice, setNotice] = useState("");
  const [expanded, setExpanded] = useState(false);

  const tree = preview.tree ?? [];
  const hidden = expanded ? 0 : Math.max(0, tree.length - COLLAPSED_ROWS);
  const rows = expanded ? tree : tree.slice(0, COLLAPSED_ROWS);

  const apply = async () => {
    setBusy(true);
    setNotice("");
    try {
      const res = await applyProposal(preview.ops, projectId, nodeIdRef.current);
      if (res.rolledBack) {
        setNotice(t("companion.preview.rolledBack"));
        return;
      }
      if (res.applied === 0) {
        setNotice(res.failures[0]?.error ?? t("companion.preview.applyFailed"));
        return;
      }
      setOutcome("applied");
      setUndoBatchId(res.undoBatchId ?? null);
      setNotice(t("companion.preview.appliedCount", { count: String(res.applied) }));
      onApplied();
    } catch (e) {
      setNotice(String(e));
    } finally {
      setBusy(false);
    }
  };

  const undo = async () => {
    if (!undoBatchId) return;
    setBusy(true);
    try {
      await companionApi.undoApply(undoBatchId);
      setUndoBatchId(null);
      setNotice(t("companion.preview.undone"));
      onApplied();
    } catch (e) {
      setNotice(String(e));
    } finally {
      setBusy(false);
    }
  };

  if (outcome === "dismissed") {
    return <div className="apply-card done">{t("companion.preview.dismissed")}</div>;
  }

  return (
    <div className="apply-card outline-preview-card">
      <div className="ttl">
        <GitBranch size={14} /> {preview.summary || t("companion.preview.defaultTitle")}
      </div>
      <div className="outline-preview-counts">{countsSummary(t, preview)}</div>
      {tree.length > 0 && (
        <ul className="outline-preview-tree" aria-label={t("companion.preview.treeLabel")}>
          {rows.map((row, i) => (
            <li key={`${row.ref || row.node_id || row.label}-${i}`} style={{ paddingLeft: `${row.depth * 14}px` }}>
              <span className={`outline-preview-action is-${row.action}`}>{actionLabel(t, row.action)}</span>
              <span className="outline-preview-label">{row.label || row.node_id}</span>
              {row.title && <span className="outline-preview-title">{row.title}</span>}
            </li>
          ))}
        </ul>
      )}
      {hidden > 0 && (
        <button type="button" className="btn ghost sm" onClick={() => setExpanded(true)}>
          {t("companion.preview.showAll", { count: String(hidden) })}
        </button>
      )}
      {preview.truncated ? (
        <div className="outline-preview-truncated">{t("companion.preview.truncated", { count: String(preview.truncated) })}</div>
      ) : null}
      {notice && <div className="outline-preview-notice">{notice}</div>}
      {outcome === "applied" ? (
        undoBatchId && (
          <div className="apply-actions">
            <button type="button" className="btn ghost sm" onClick={undo} disabled={busy}>
              <Undo2 size={13} /> {t("companion.preview.undo")}
            </button>
          </div>
        )
      ) : (
        <div className="apply-actions">
          <button type="button" className="btn accent sm" onClick={apply} disabled={busy}>
            {busy ? t("companion.preview.applying") : t("companion.preview.apply")}
          </button>
          <button type="button" className="btn ghost sm" onClick={() => setOutcome("dismissed")} disabled={busy}>
            {t("companion.preview.discard")}
          </button>
        </div>
      )}
    </div>
  );
}
