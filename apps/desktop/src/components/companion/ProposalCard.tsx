import { useState } from "react";
import { Sparkles } from "lucide-react";
import type { CompanionProposal, ProposalOp } from "../../lib/types";
import { applyProposal, type ApplyResult } from "../../lib/applyProposal";

function entityKindLabel(kind?: string): string {
  switch (kind) {
    case "place": return "장소";
    case "item": return "물건";
    case "concept": return "개념";
    default: return "캐릭터";
  }
}

function opLabel(op: ProposalOp): string {
  switch (op.op) {
    case "create_thread": return `스토리라인 생성: ${op.name ?? ""}`;
    case "update_thread": return `스토리라인 수정`;
    case "add_beat": return `비트 추가: ${op.label ?? ""}`;
    case "update_beat": return `비트 수정: ${op.label ?? ""}`;
    case "delete_beat": return `비트 삭제`;
    case "set_outline": return `작품 개요 설정`;
    case "remember": return `기억하기: ${op.text ?? ""}`;
    case "create_entity": return `${entityKindLabel(op.kind)} 생성: ${op.name ?? ""}`;
    case "update_entity": return `엔티티 수정`;
    case "create_relationship": return `관계 생성: ${op.label ?? ""}`;
    case "create_scene": return `씬 생성: ${op.label ?? ""}`;
    case "create_outline_node": return `${op.kind === "container" ? "아웃라인 묶음" : "아웃라인 씬"} 생성: ${op.label ?? ""}`;
    default: return op.op;
  }
}

interface Props {
  proposal: CompanionProposal;
  projectId: string;
  nodeIdRef: { current: string | null };
  onApplied: () => void;
}

export function ProposalCard({ proposal, projectId, nodeIdRef, onApplied }: Props) {
  const ops = proposal.ops ?? [];
  const [sel, setSel] = useState<boolean[]>(ops.map(() => true));
  const [result, setResult] = useState<ApplyResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [rejected, setRejected] = useState(false);

  if (!proposal.valid) {
    return (
      <div className="apply-card invalid">
        <div className="ttl">AI 제안 형식 오류</div>
        {proposal.error && <div className="apply-error">{proposal.error}</div>}
      </div>
    );
  }
  if (rejected) {
    return <div className="apply-card done">제안 거절됨</div>;
  }

  const apply = async () => {
    setBusy(true);
    const chosen = ops.filter((_, i) => sel[i]);
    try {
      const res = await applyProposal(chosen, projectId, nodeIdRef.current);
      setResult(res);
      onApplied();
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="apply-card">
      <div className="ttl"><Sparkles size={14} /> {proposal.summary || "AI 제안"}</div>
      <ul className="apply-ops">
        {ops.map((op, i) => (
          <li key={i}>
            <label>
              <input
                type="checkbox"
                checked={sel[i]}
                disabled={!!result || busy}
                onChange={(e) => setSel((prev) => prev.map((v, j) => (j === i ? e.target.checked : v)))}
              />
              <span>{opLabel(op)}</span>
            </label>
          </li>
        ))}
      </ul>
      {result ? (
        <div className="apply-result">
          <div>적용됨 {result.applied}건{result.failures.length > 0 ? ` · 실패 ${result.failures.length}건` : ""}</div>
          {result.failures.length > 0 && (
            <ul className="apply-failures">
              {result.failures.map((f, i) => (
                <li key={i}>{f.op ? opLabel(f.op) : "제안 적용"} — {f.error}</li>
              ))}
            </ul>
          )}
        </div>
      ) : (
        <div className="apply-actions">
          <button type="button" className="btn accent sm" onClick={apply} disabled={busy || sel.every((v) => !v)}>
            {busy ? "적용 중…" : "적용"}
          </button>
          <button type="button" className="btn ghost sm" onClick={() => setRejected(true)} disabled={busy}>건너뛰기</button>
        </div>
      )}
    </div>
  );
}
