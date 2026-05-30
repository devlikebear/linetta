import { useState } from "react";
import type { CompanionProposal, ProposalOp } from "../../lib/types";
import { applyProposal, type ApplyResult } from "../../lib/applyProposal";

function opLabel(op: ProposalOp): string {
  switch (op.op) {
    case "create_thread": return `스토리라인 생성: ${op.name ?? ""}`;
    case "update_thread": return `스토리라인 수정`;
    case "add_beat": return `비트 추가: ${op.label ?? ""}`;
    case "update_beat": return `비트 수정: ${op.label ?? ""}`;
    case "delete_beat": return `비트 삭제`;
    case "set_outline": return `작품 개요 설정`;
    case "remember": return `기억하기: ${op.text ?? ""}`;
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
      <div className="companion-proposal invalid">
        <div className="companion-proposal-head">AI 제안 형식 오류</div>
        {proposal.error && <div className="companion-proposal-error">{proposal.error}</div>}
      </div>
    );
  }
  if (rejected) {
    return <div className="companion-proposal done">제안 거절됨</div>;
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
    <div className="companion-proposal">
      {proposal.summary && <div className="companion-proposal-head">{proposal.summary}</div>}
      <ul className="companion-proposal-ops">
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
        <div className="companion-proposal-result">
          <div>적용됨 {result.applied}건{result.failures.length > 0 ? ` · 실패 ${result.failures.length}건` : ""}</div>
          {result.failures.length > 0 && (
            <ul className="companion-proposal-failures">
              {result.failures.map((f, i) => (
                <li key={i}>{opLabel(f.op)} — {f.error}</li>
              ))}
            </ul>
          )}
        </div>
      ) : (
        <div className="companion-proposal-actions">
          <button type="button" onClick={() => setRejected(true)} disabled={busy}>거절</button>
          <button type="button" className="primary" onClick={apply} disabled={busy || sel.every((v) => !v)}>
            {busy ? "적용 중…" : "적용"}
          </button>
        </div>
      )}
    </div>
  );
}
