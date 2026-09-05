import type { MessageKey } from "./i18n";

/**
 * The agent's tool surface, as the panel needs to describe it to a writer.
 *
 * `agent.tool` events carry a tool NAME and a `summary`. The summary is not
 * usable copy: `agent/loop.go`'s `runTool` sets it to `summarize(result.Text)`,
 * and `result.Text` is the go-sdk's JSON serialisation of the tool's typed
 * output (every Linetta tool returns a nil `*CallToolResult`, so the SDK
 * marshals the output struct). So a resolved call's summary is always raw
 * JSON — a project id, a node id and the opening prose of whatever was read —
 * and an errored call's is the engine's English sentence. Neither belongs on
 * a Korean writer's screen.
 *
 * The name is the one field the panel can translate, so the line is built
 * from it alone: a verb (read/wrote/…) plus a short label for the tool.
 *
 * Both name lists mirror the engine, which is where they are authoritative:
 *   engine/internal/mcphost/tools_read.go   ReadToolNames
 *   engine/internal/mcphost/tools_write.go  WriteToolNames
 * Nothing in TypeScript can check that, so `agentToolParity.test.ts` reads
 * those Go files and compares. Without it, an engine that gained a delete
 * tool would render "읽음 · …" for a deletion — the panel telling the writer
 * the agent READ a scene it destroyed — and every desktop test would pass.
 */
export const READ_TOOL_NAMES = [
  "linetta_list_works",
  "linetta_get_outline",
  "linetta_get_story_context",
  "linetta_read_scene",
  "linetta_search_manuscript",
  "linetta_list_characters",
  "linetta_where_does_appear",
  "linetta_get_plot",
  "linetta_get_fact_cards",
] as const;

export const WRITE_TOOL_NAMES = [
  "linetta_create_work",
  "linetta_write_scene",
  "linetta_write_summary",
  "linetta_revise_scene",
  "linetta_apply_story_ops",
  "linetta_create_checkpoint",
  "linetta_undo_last_change",
] as const;

const READS: ReadonlySet<string> = new Set(READ_TOOL_NAMES);
const WRITES: ReadonlySet<string> = new Set(WRITE_TOOL_NAMES);

/** read / write / neither. */
export type ToolKind = "read" | "write" | "other";

/** What a tool name means for the writer.
 *
 *  A name in neither list is `"other"`, NOT `"read"`. Falling back to "read"
 *  is the one answer that can be actively false in the dangerous direction:
 *  an unrecognised name is most likely a tool the engine gained and this file
 *  did not, and calling a brand-new mutation "읽음" tells the writer their
 *  manuscript was untouched when it may have been rewritten or deleted.
 *  Falling back to "write" is safe in that direction but lies the other way,
 *  reporting a change for a plain lookup and inviting the writer to hunt for
 *  an edit that never happened.
 *
 *  So the fallback claims neither: a neutral verb ("도구 실행") and no label.
 *  It is honest about the one thing the panel actually knows — that the agent
 *  called a tool — and the parity test above keeps this branch residual
 *  rather than routine. */
export function toolKind(name: string): ToolKind {
  if (WRITES.has(name)) return "write";
  if (READS.has(name)) return "read";
  return "other";
}

/** The line's verb: kind × resolved state.
 *
 *  `"running"` reads as still-in-progress rather than reusing the past-tense
 *  copy, and `"error"` gets its own wording so a failure is legible without
 *  relying on colour. */
export function toolVerbKey(kind: ToolKind, state: "running" | "ok" | "error"): MessageKey {
  if (kind === "write") {
    if (state === "running") return "agentPanel.tool.writing";
    return state === "error" ? "agentPanel.tool.writeFailed" : "agentPanel.tool.wrote";
  }
  if (kind === "read") {
    if (state === "running") return "agentPanel.tool.reading";
    return state === "error" ? "agentPanel.tool.readFailed" : "agentPanel.tool.read";
  }
  if (state === "running") return "agentPanel.tool.using";
  return state === "error" ? "agentPanel.tool.failed" : "agentPanel.tool.used";
}

/** A short human label for the tool, or null when the name is unrecognised.
 *
 *  The cast is unavoidable — the key is built from the tool name — so it is
 *  the parity test, not the compiler, that guarantees every name in the two
 *  lists has an entry in all three catalogues. */
export function toolLabelKey(name: string): MessageKey | null {
  if (toolKind(name) === "other") return null;
  return `agentPanel.toolName.${name}` as MessageKey;
}
