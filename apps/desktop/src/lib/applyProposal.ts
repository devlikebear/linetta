import type { ProposalOp } from "./types";
import { threads as threadsApi, beats as beatsApi, projects as projectsApi, companion as companionApi } from "./rpc";

export interface ApplyFailure {
  index: number;
  op: ProposalOp;
  error: string;
}

export interface ApplyResult {
  applied: number;
  failures: ApplyFailure[];
}

// applyProposal executes the selected ops in order against existing RPCs,
// resolving create_thread.ref -> created thread id for later add_beat.thread_ref.
// A failing op is recorded and execution continues (partial apply; no rollback).
export async function applyProposal(ops: ProposalOp[], projectId: string, currentNodeId: string | null): Promise<ApplyResult> {
  const refMap = new Map<string, string>();
  const failures: ApplyFailure[] = [];
  let applied = 0;

  for (let i = 0; i < ops.length; i++) {
    const op = ops[i];
    try {
      switch (op.op) {
        case "create_thread": {
          const th = await threadsApi.create({
            project_id: projectId,
            name: op.name ?? "",
            color: op.color,
          });
          if (op.ref) refMap.set(op.ref, th.id);
          if (op.summary) {
            await threadsApi.update({ id: th.id, summary: op.summary });
          }
          break;
        }
        case "update_thread": {
          if (!op.thread_id) throw new Error("thread_id 없음");
          await threadsApi.update({
            id: op.thread_id,
            name: op.name,
            color: op.color,
            summary: op.summary,
          });
          break;
        }
        case "add_beat": {
          const tid = op.thread_id ?? (op.thread_ref ? refMap.get(op.thread_ref) : undefined);
          if (!tid) throw new Error("스토리라인 참조를 해소할 수 없음");
          const nodeId = op.node_id ?? currentNodeId ?? undefined;
          await beatsApi.create({
            thread_id: tid,
            node_id: nodeId,
            label: op.label,
            description: op.description,
            intensity: op.intensity,
          });
          break;
        }
        case "update_beat": {
          if (!op.beat_id) throw new Error("beat_id 없음");
          await beatsApi.update({
            id: op.beat_id,
            label: op.label,
            description: op.description,
            intensity: op.intensity,
          });
          break;
        }
        case "delete_beat": {
          if (!op.beat_id) throw new Error("beat_id 없음");
          await beatsApi.delete(op.beat_id);
          break;
        }
        case "set_outline": {
          await projectsApi.update({ id: projectId, outline: op.outline ?? "" });
          break;
        }
        case "remember": {
          if (!op.text || !op.text.trim()) throw new Error("기억할 내용 없음");
          await companionApi.remember(projectId, op.text, op.category);
          break;
        }
        default: {
          throw new Error(`알 수 없는 op: ${(op as ProposalOp).op}`);
        }
      }
      applied++;
    } catch (e) {
      failures.push({ index: i, op, error: e instanceof Error ? e.message : String(e) });
    }
  }

  return { applied, failures };
}
