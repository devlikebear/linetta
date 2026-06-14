import { useState } from "react";
import { Sparkles } from "lucide-react";
import type { CompanionProposal, ProposalOp } from "../../lib/types";
import { applyProposal, type ApplyResult } from "../../lib/applyProposal";
import { useI18n } from "../../lib/i18n";

type Translate = ReturnType<typeof useI18n>["t"];

function entityKindLabel(t: Translate, kind?: string): string {
  switch (kind) {
    case "place": return t("companion.entity.place");
    case "item": return t("companion.entity.item");
    case "concept": return t("companion.entity.concept");
    default: return t("companion.entity.character");
  }
}

function opLabel(t: Translate, op: ProposalOp): string {
  switch (op.op) {
    case "create_thread": return t("companion.op.createThread", { name: op.name ?? "" });
    case "update_thread": return t("companion.op.updateThread");
    case "add_beat": return t("companion.op.addBeat", { label: op.label ?? "" });
    case "update_beat": return t("companion.op.updateBeat", { label: op.label ?? "" });
    case "delete_beat": return t("companion.op.deleteBeat");
    case "set_outline": return t("companion.op.setOutline");
    case "set_scene_text": return t("companion.op.setSceneText");
    case "remember": return t("companion.op.remember", { text: op.text ?? "" });
    case "create_entity": return t("companion.op.createEntity", { kind: entityKindLabel(t, op.kind), name: op.name ?? "" });
    case "update_entity": return t("companion.op.updateEntity");
    case "create_relationship": return t("companion.op.createRelationship", { label: op.label ?? "" });
    case "create_scene": return t("companion.op.createScene", { label: op.label ?? "" });
    case "create_outline_node": return t(op.kind === "container" ? "companion.op.createOutlineContainer" : "companion.op.createOutlineScene", { label: op.label ?? "" });
    case "create_fact_card": return t("companion.op.createFactCard", { claim: op.claim ?? "" });
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
  const { t } = useI18n();
  const ops = proposal.ops ?? [];
  const [sel, setSel] = useState<boolean[]>(ops.map(() => true));
  const [result, setResult] = useState<ApplyResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [rejected, setRejected] = useState(false);

  if (!proposal.valid) {
    return (
        <div className="apply-card invalid">
        <div className="ttl">{t("companion.proposal.invalid")}</div>
        {proposal.error && <div className="apply-error">{proposal.error}</div>}
      </div>
    );
  }
  if (rejected) {
    return <div className="apply-card done">{t("companion.proposal.rejected")}</div>;
  }

  const apply = async () => {
    setBusy(true);
    const chosen = ops.filter((_, i) => sel[i]);
    try {
      const res = await applyProposal(chosen, projectId, nodeIdRef.current);
      setResult(res);
      if (res.applied > 0) {
        onApplied();
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="apply-card">
      <div className="ttl"><Sparkles size={14} /> {proposal.summary || t("companion.proposal.defaultTitle")}</div>
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
              <span>{opLabel(t, op)}</span>
            </label>
          </li>
        ))}
      </ul>
      {result ? (
        <div className="apply-result">
          <div>
            {t("companion.proposal.applied", { count: result.applied })}
            {result.failures.length > 0 ? ` · ${t("companion.proposal.failed", { count: result.failures.length })}` : ""}
          </div>
          {result.failures.length > 0 && (
            <ul className="apply-failures">
              {result.failures.map((f, i) => (
                <li key={i}>{f.op ? opLabel(t, f.op) : t("companion.proposal.applyFallback")} - {f.error}</li>
              ))}
            </ul>
          )}
        </div>
      ) : (
        <div className="apply-actions">
          <button type="button" className="btn accent sm" onClick={apply} disabled={busy || sel.every((v) => !v)}>
            {busy ? t("companion.proposal.applying") : t("companion.proposal.apply")}
          </button>
          <button type="button" className="btn ghost sm" onClick={() => setRejected(true)} disabled={busy}>{t("companion.proposal.skip")}</button>
        </div>
      )}
    </div>
  );
}
