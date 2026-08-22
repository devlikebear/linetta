import type { ProposalOp } from "./types";
import { companion as companionApi } from "./rpc";

export interface ApplyFailure {
  index: number;
  op?: ProposalOp;
  error: string;
}

export interface ApplyResult {
  applied: number;
  failures: ApplyFailure[];
  /** The batch failed partway and the outline was put back as it was. */
  rolledBack: boolean;
  /** Identifies the pre-change outline, for a one-step undo. */
  undoBatchId?: string;
}

// applyProposal keeps the proposal-card UI thin. The engine owns op validation,
// ref resolution, partial-apply behavior, and persistence mutations.
export async function applyProposal(
  ops: ProposalOp[],
  projectId: string,
  currentNodeId: string | null,
): Promise<ApplyResult> {
  const result = await companionApi.applyOps(projectId, currentNodeId, "", ops);
  return {
    applied: result.applied,
    failures: (result.failures ?? []).map((failure) => ({
      index: failure.index,
      op: resolveFailureOp(ops, failure.index, failure.op),
      error: failure.error,
    })),
    rolledBack: result.rolled_back === true,
    ...(result.undo_batch_id ? { undoBatchId: result.undo_batch_id } : {}),
  };
}

function resolveFailureOp(ops: ProposalOp[], index: number, opType?: string): ProposalOp | undefined {
  if (index >= 0 && index < ops.length) {
    return ops[index];
  }
  if (!opType) {
    return undefined;
  }
  return { op: (opType || "set_outline") as ProposalOp["op"] };
}
