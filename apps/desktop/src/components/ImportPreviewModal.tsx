import { FileText, Folder } from "lucide-react";
import type { ImportPreviewNode, ImportPreviewResult } from "../lib/types";
import { displayNodeLabel, useI18n } from "../lib/i18n";
import { importWarningMessage } from "../lib/importWarnings";
import "./ImportPreviewModal.css";

interface Props {
  preview: ImportPreviewResult;
  fileName: string;
  busy: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ImportPreviewModal({ preview, fileName, busy, onConfirm, onCancel }: Props) {
  const { t } = useI18n();
  const total = preview.container_count + preview.leaf_count;
  return (
    <div className="backdrop center" onMouseDown={busy ? undefined : onCancel}>
      <div className="modal import-modal" onMouseDown={(e) => e.stopPropagation()}>
        <h2>{t("importPreview.title")}</h2>
        <p className="modal-sub">
          {t("importPreview.summary", {
            fileName,
            title: preview.title,
            containers: preview.container_count,
            scenes: preview.leaf_count,
          })}
        </p>

        {preview.warnings.length > 0 && (
          <div className="import-warnings" role="alert">
            <strong>{t("importPreview.warning")}</strong>
            <ul>
              {preview.warnings.map((w, i) => (
                <li key={i}>{importWarningMessage(t, w)}</li>
              ))}
            </ul>
          </div>
        )}

        <div className="import-tree">
          {total === 0 ? (
            <p className="import-empty">{t("importPreview.empty")}</p>
          ) : (
            <ul>
              {preview.roots.map((n, i) => (
                <PreviewItem key={i} node={n} />
              ))}
            </ul>
          )}
        </div>

        <div className="modal-actions">
          <button type="button" className="btn ghost" onClick={onCancel} disabled={busy}>
            {t("importPreview.cancel")}
          </button>
          <button type="button" className="btn accent" onClick={onConfirm} disabled={busy || total === 0}>
            {busy ? t("importPreview.importing") : t("importPreview.confirm")}
          </button>
        </div>
      </div>
    </div>
  );
}

function PreviewItem({ node }: { node: ImportPreviewNode }) {
  const { language, t } = useI18n();
  return (
    <li>
      <span className={`import-node kind-${node.kind}`}>
        {node.kind === "container" ? <Folder size={13} /> : <FileText size={13} />}
        {node.label ? displayNodeLabel(language, node.label) : t("importPreview.unnamed")}
      </span>
      {node.children && node.children.length > 0 && (
        <ul>
          {node.children.map((c, i) => (
            <PreviewItem key={i} node={c} />
          ))}
        </ul>
      )}
    </li>
  );
}
