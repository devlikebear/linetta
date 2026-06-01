import { FileText, Folder } from "lucide-react";
import type { ImportPreviewNode, ImportPreviewResult } from "../lib/types";
import "./ImportPreviewModal.css";

interface Props {
  preview: ImportPreviewResult;
  fileName: string;
  busy: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ImportPreviewModal({ preview, fileName, busy, onConfirm, onCancel }: Props) {
  const total = preview.container_count + preview.leaf_count;
  return (
    <div className="backdrop center" onMouseDown={busy ? undefined : onCancel}>
      <div className="modal import-modal" onMouseDown={(e) => e.stopPropagation()}>
        <h2>가져오기 미리보기</h2>
        <p className="modal-sub">
          {fileName} → <strong>{preview.title}</strong> · 컨테이너 {preview.container_count}개 · 씬 {preview.leaf_count}개
        </p>

        {preview.warnings.length > 0 && (
          <div className="import-warnings" role="alert">
            <strong>경고</strong>
            <ul>
              {preview.warnings.map((w, i) => (
                <li key={i}>{w}</li>
              ))}
            </ul>
          </div>
        )}

        <div className="import-tree">
          {total === 0 ? (
            <p className="import-empty">가져올 노드가 없습니다.</p>
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
            취소
          </button>
          <button type="button" className="btn accent" onClick={onConfirm} disabled={busy || total === 0}>
            {busy ? "가져오는 중…" : "확인 후 가져오기"}
          </button>
        </div>
      </div>
    </div>
  );
}

function PreviewItem({ node }: { node: ImportPreviewNode }) {
  return (
    <li>
      <span className={`import-node kind-${node.kind}`}>
        {node.kind === "container" ? <Folder size={13} /> : <FileText size={13} />}
        {node.label || "(이름 없음)"}
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
