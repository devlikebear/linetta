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
    <div className="import-preview-backdrop" onMouseDown={busy ? undefined : onCancel}>
      <div className="import-preview-modal" onMouseDown={(e) => e.stopPropagation()}>
        <h3 className="import-preview-title">가져오기 미리보기</h3>
        <p className="import-preview-meta">
          {fileName} → <strong>{preview.title}</strong> · 컨테이너 {preview.container_count}개 · 씬 {preview.leaf_count}개
        </p>

        {preview.warnings.length > 0 && (
          <div className="import-preview-warnings" role="alert">
            <strong>경고</strong>
            <ul>
              {preview.warnings.map((w, i) => (
                <li key={i}>{w}</li>
              ))}
            </ul>
          </div>
        )}

        <div className="import-preview-tree">
          {total === 0 ? (
            <p className="import-preview-empty">가져올 노드가 없습니다.</p>
          ) : (
            <ul>
              {preview.roots.map((n, i) => (
                <PreviewItem key={i} node={n} />
              ))}
            </ul>
          )}
        </div>

        <div className="import-preview-actions">
          <button type="button" onClick={onCancel} disabled={busy}>
            취소
          </button>
          <button type="button" className="primary" onClick={onConfirm} disabled={busy || total === 0}>
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
      <span className={`kind-${node.kind}`}>
        {node.kind === "container" ? "📁 " : "📄 "}
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
