import type { CompanionProposal, ProposalOp, ProposalOpType } from "./types";

// Removes model-visible control payloads that can leak into assistant prose.
export function stripProposalBlock(text: string): string {
  return stripToolArgumentEchoes(text)
    .replace(/```linetta-(?:proposal|query|choices)[\s\S]*?```/g, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

export function extractApplyOpsProposal(text: string, runId = "inline-apply-ops"): CompanionProposal | null {
  const payload = findApplyOpsPayload(text);
  if (!payload) return null;
  const ops = parseProposalOps(payload.ops_json ?? payload.ops);
  if (ops.length === 0) return null;
  return {
    run_id: runId,
    valid: true,
    summary: typeof payload.summary === "string" ? payload.summary : undefined,
    ops,
  };
}

function stripToolArgumentEchoes(text: string): string {
  return text
    .split(/\r?\n/)
    .map(stripToolArgumentEchoFromLine)
    .filter((line): line is string => line !== null)
    .join("\n");
}

function stripToolArgumentEchoFromLine(line: string): string | null {
  const leading = line.length - line.trimStart().length;
  const body = line.slice(leading);
  if (!body.startsWith("{")) return line;
  const end = jsonObjectPrefixEnd(body);
  if (end <= 0) return line;
  if (!isToolArgumentJSON(body.slice(0, end))) return line;
  const rest = body.slice(end).trimStart();
  return rest ? line.slice(0, leading) + rest : null;
}

function isToolArgumentJSON(json: string): boolean {
  const value = parseJSON(json);
  if (!value || Array.isArray(value) || typeof value !== "object") return false;
  const obj = value as Record<string, unknown>;
  const keys = Object.keys(obj);
  if (keys.length === 0) return false;
  const everyKeyIn = (allowed: string[]) => keys.every((key) => allowed.includes(key));
  if (typeof obj.query === "string" && ("provider" in obj || "count" in obj) && everyKeyIn(["query", "provider", "count"])) {
    return true;
  }
  if (typeof obj.url === "string" && "max_chars" in obj && everyKeyIn(["url", "max_chars"])) {
    return true;
  }
  if ((typeof obj.ops_json === "string" || Array.isArray(obj.ops)) && everyKeyIn(["summary", "ops_json", "ops"])) {
    return true;
  }
  return false;
}

function findApplyOpsPayload(text: string): Record<string, unknown> | null {
  for (const line of text.split(/\r?\n/)) {
    const body = line.trimStart();
    if (!body.startsWith("{")) continue;
    const end = jsonObjectPrefixEnd(body);
    if (end <= 0) continue;
    const value = parseJSON(body.slice(0, end));
    if (!value || Array.isArray(value) || typeof value !== "object") continue;
    const obj = value as Record<string, unknown>;
    if (typeof obj.ops_json === "string" || Array.isArray(obj.ops)) return obj;
  }
  return null;
}

function parseProposalOps(value: unknown): ProposalOp[] {
  let raw = value;
  if (typeof raw === "string") {
    raw = parseJSON(raw);
  }
  if (!Array.isArray(raw)) return [];
  return raw.filter(isProposalOp);
}

const PROPOSAL_OP_TYPES = new Set<ProposalOpType>([
  "create_thread",
  "update_thread",
  "add_beat",
  "update_beat",
  "delete_beat",
  "set_outline",
  "set_scene_text",
  "remember",
  "create_entity",
  "update_entity",
  "create_relationship",
  "create_scene",
  "create_outline_node",
  "create_fact_card",
]);

function isProposalOp(value: unknown): value is ProposalOp {
  if (!value || Array.isArray(value) || typeof value !== "object") return false;
  const op = (value as { op?: unknown }).op;
  return typeof op === "string" && PROPOSAL_OP_TYPES.has(op as ProposalOpType);
}

function parseJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

function jsonObjectPrefixEnd(text: string): number {
  let depth = 0;
  let inString = false;
  let escaped = false;
  for (let i = 0; i < text.length; i += 1) {
    const ch = text[i];
    if (inString) {
      if (escaped) {
        escaped = false;
      } else if (ch === "\\") {
        escaped = true;
      } else if (ch === "\"") {
        inString = false;
      }
      continue;
    }
    if (ch === "\"") {
      inString = true;
      continue;
    }
    if (ch === "{") {
      depth += 1;
    } else if (ch === "}") {
      depth -= 1;
      if (depth === 0) return i + 1;
      if (depth < 0) return -1;
    }
  }
  return -1;
}
