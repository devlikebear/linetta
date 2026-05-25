import { useEffect, useState } from "react";
import type { NodeRow, Project, Entity } from "../lib/types";

export type SaveStatus =
  | { kind: "idle" }
  | { kind: "saving" }
  | { kind: "saved"; at: number }
  | { kind: "error"; message: string };

interface Props {
  project: Project;
  node: NodeRow;
  charCount: number;
  typewriter: boolean;
  onToggleTypewriter: () => void;
  saveStatus: SaveStatus;
  mentionedEntities: Entity[];
  onMentionClick: (entityId: string) => void;
}

const STATUS_LABEL: Record<NodeRow["status"], string> = {
  draft: "초고",
  revision: "퇴고",
  final: "완성",
};

export function ContextPanel({ node, charCount, typewriter, onToggleTypewriter, saveStatus, mentionedEntities, onMentionClick }: Props) {
  return (
    <aside className="ctx-panel">
      <section className="ctx-section">
        <h4>인물 · 장소</h4>
        {mentionedEntities.length === 0 && (
          <p className="ctx-empty">아직 @멘션 없음</p>
        )}
        {mentionedEntities.map((e) => (
          <button
            key={e.id}
            type="button"
            className="ctx-entity"
            onClick={() => onMentionClick(e.id)}
          >
            <span className="ctx-entity-avatar">{e.name.slice(0, 1)}</span>
            <span className="ctx-entity-name">{e.name}</span>
            {e.role && <span className="ctx-entity-role">{e.role}</span>}
          </button>
        ))}
      </section>

      <section className="ctx-section">
        <h4>활성 Thread</h4>
        <p className="ctx-empty">(곧 추가됨 — post-MVP)</p>
      </section>

      <section className="ctx-section">
        <h4>씬 상태</h4>
        <p className="ctx-line">
          ● {STATUS_LABEL[node.status]} · {charCount.toLocaleString("ko-KR")}자
        </p>
        <SaveStatusLine status={saveStatus} />
      </section>

      <section className="ctx-section">
        <h4>옵션</h4>
        <label className="ctx-check">
          <input type="checkbox" checked={typewriter} onChange={onToggleTypewriter} />
          타자기 모드
        </label>
      </section>
    </aside>
  );
}

function SaveStatusLine({ status }: { status: SaveStatus }) {
  // Re-render every second when in "saved" state so "X초 전" updates.
  const [, tick] = useState(0);
  useEffect(() => {
    if (status.kind !== "saved") return;
    const id = window.setInterval(() => tick((n) => n + 1), 1000);
    return () => window.clearInterval(id);
  }, [status]);

  switch (status.kind) {
    case "idle":
      return null;
    case "saving":
      return <p className="ctx-save saving">저장 중…</p>;
    case "saved": {
      const seconds = Math.max(0, Math.floor((Date.now() - status.at) / 1000));
      const label = seconds < 1 ? "방금 저장됨" : `${seconds}초 전 저장됨`;
      return <p className="ctx-save saved">{label}</p>;
    }
    case "error":
      return <p className="ctx-save error">저장 실패: {status.message}</p>;
  }
}
