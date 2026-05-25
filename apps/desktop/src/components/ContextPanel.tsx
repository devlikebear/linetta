import type { NodeRow, Project } from "../lib/types";

interface Props {
  project: Project;
  node: NodeRow;
  charCount: number;
  typewriter: boolean;
  onToggleTypewriter: () => void;
}

const STATUS_LABEL: Record<NodeRow["status"], string> = {
  draft: "초고",
  revision: "퇴고",
  final: "완성",
};

export function ContextPanel({ node, charCount, typewriter, onToggleTypewriter }: Props) {
  return (
    <aside className="ctx-panel">
      <section className="ctx-section">
        <h4>인물 · 장소</h4>
        <p className="ctx-empty">(곧 추가됨 — Plan 4)</p>
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
