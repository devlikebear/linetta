import { describe, expect, it } from "vitest";

import { parseToolEvent } from "../components/agent/AgentPanel";
import { readAppFile } from "../test/readSource";
import { READ_TOOL_NAMES, WRITE_TOOL_NAMES, toolKind, toolLabelKey } from "./agentTools";
import { messageCatalogs } from "./i18n";
import type { AppLanguage } from "./types";

/**
 * The engine decides which tools the agent has, and which of them write. The
 * panel decides what verb to put beside a tool call — 읽음 or 씀 — and it
 * decides it from a copy of those two Go lists.
 *
 * A copy nothing checks is how the panel comes to say the agent READ a scene
 * it deleted: add a delete tool to `WriteToolNames`, ship it, and every
 * desktop test still passes while the panel renders 읽음 for a deletion. This
 * is the same hazard `coreRoleParity.test.ts` exists for, and the same fix —
 * read the Go source and compare.
 */
const LANGUAGES: AppLanguage[] = ["ko", "en", "ja"];

/** Pull a `var XNames = []string{…}` block's literals out of a Go file. */
async function goToolNames(file: string, varName: string): Promise<string[]> {
  const src = await readAppFile(`../../engine/internal/mcphost/${file}`);
  const start = src.indexOf(`var ${varName} = []string{`);
  if (start === -1) throw new Error(`${varName} not found in ${file}`);
  const end = src.indexOf("}", start);
  return [...src.slice(start, end).matchAll(/"([a-z0-9_]+)"/g)].map((m) => m[1]);
}

const goReadNames = () => goToolNames("tools_read.go", "ReadToolNames");
const goWriteNames = () => goToolNames("tools_write.go", "WriteToolNames");

describe("agent tool catalogue parity with the engine", () => {
  it("finds both Go lists (guards against the extraction silently breaking)", async () => {
    // If the extraction ever returns nothing, every comparison below passes
    // vacuously — which is exactly the failure this file exists to prevent.
    expect(await goReadNames()).toContain("linetta_get_story_context");
    expect(await goWriteNames()).toContain("linetta_apply_story_ops");
  });

  it("lists exactly the engine's read tools", async () => {
    expect([...READ_TOOL_NAMES].sort()).toEqual((await goReadNames()).sort());
  });

  it("lists exactly the engine's write tools", async () => {
    // The direction that matters: a write tool missing here is classified
    // "other" at best and "read" at worst — the panel telling a writer their
    // manuscript was only read.
    expect([...WRITE_TOOL_NAMES].sort()).toEqual((await goWriteNames()).sort());
  });

  it("never classifies an engine write tool as a read", async () => {
    for (const name of await goWriteNames()) {
      expect(toolKind(name), `${name} must not read as a read tool`).toBe("write");
    }
  });

  it("has a short label for every engine tool, in every language", async () => {
    const names = [...(await goReadNames()), ...(await goWriteNames())];
    expect(names.length).toBeGreaterThan(10);
    for (const name of names) {
      const key = toolLabelKey(name);
      expect(key, `${name} has no label key`).not.toBeNull();
      for (const language of LANGUAGES) {
        const label = messageCatalogs[language][key as keyof (typeof messageCatalogs)["ko"]];
        expect(label, `${language} label for ${name}`).toBeTruthy();
        // A label that still contains the tool's wire name is not a label.
        expect(label).not.toContain("linetta_");
      }
    }
  });

  it("carries no label for a tool the engine does not have", async () => {
    const names = new Set([...(await goReadNames()), ...(await goWriteNames())]);
    const stale = Object.keys(messageCatalogs.ko)
      .filter((key) => key.startsWith("agentPanel.toolName."))
      .map((key) => key.slice("agentPanel.toolName.".length))
      .filter((name) => !names.has(name));
    expect(stale, "these agentPanel.toolName.* keys name tools the engine no longer has").toEqual([]);
  });
});

/**
 * The same hazard one struct over, and the one the branch had left unguarded.
 *
 * A restored tool line is drawn from a `role: "tool"` history row whose
 * `content` is the JSON of agent/transcript.go's `toolEvent`. AgentPanel's
 * `RestoredToolEvent` / `parseToolEvent` hand-mirror that struct's JSON tags,
 * and a mismatch is silent by construction: `parseToolEvent` returns null for
 * a row it cannot read, `linesFromHistory` skips a null, and every restore
 * test in AgentPanel.test.tsx builds its fixtures by hand with the field names
 * the panel expects — so renaming `ok` to `success` in the Go struct makes
 * every restored tool line in the app vanish with the whole desktop suite
 * green.
 *
 * So the fixture here is built from the tags the Go file actually carries, not
 * from the names the panel expects.
 */

/** The JSON tag names on a `type <name> struct { … }` block in a Go file,
 *  in declaration order, with any `,omitempty` stripped. */
async function goJSONTags(file: string, typeName: string): Promise<string[]> {
  const src = await readAppFile(`../../engine/internal/agent/${file}`);
  const start = src.indexOf(`type ${typeName} struct {`);
  if (start === -1) throw new Error(`${typeName} not found in ${file}`);
  const end = src.indexOf("\n}", start);
  if (end === -1) throw new Error(`${typeName} has no closing brace in ${file}`);
  return [...src.slice(start, end).matchAll(/`json:"([^",]+)[^"]*"`/g)].map((m) => m[1]);
}

const toolEventTags = () => goJSONTags("transcript.go", "toolEvent");

describe("restored tool row parity with the engine's toolEvent", () => {
  it("finds the Go struct's tags (guards against the extraction silently breaking)", async () => {
    // Without this, a broken extraction makes every assertion below pass
    // vacuously — the exact failure mode this file exists to prevent.
    expect(await toolEventTags()).toHaveLength(5);
  });

  it("reads a row written with the engine's own field names", async () => {
    const tags = await toolEventTags();
    // Built from the Go tags, so the row is what the engine would marshal
    // rather than what the panel hopes for. Values are per-tag: only `ok` is
    // a bool, and only the two the line cannot be drawn without are asserted
    // back out.
    const row: Record<string, unknown> = {};
    for (const tag of tags) row[tag] = tag === "ok" ? true : tag === "node_ids" ? ["n1"] : `${tag}-value`;

    const parsed = parseToolEvent(JSON.stringify(row));
    expect(parsed, "parseToolEvent could not read a row in the engine's own shape").not.toBeNull();
    expect(parsed?.name).toBe("name-value");
    expect(parsed?.ok).toBe(true);
    expect(parsed?.batch_id).toBe("batch_id-value");
  });

  it("carries every required field the engine's struct declares", async () => {
    const tags = await toolEventTags();
    // The panel needs `name` and `ok` to draw the line and `batch_id` to offer
    // undo. `summary` and `node_ids` are deliberately not mirrored — see the
    // notes on `Line` and on AgentToolPayload — so this asserts the direction
    // that matters: everything the panel DOES read must still exist in Go.
    for (const needed of ["name", "ok", "batch_id"]) {
      expect(tags, `the panel reads ${needed} off a restored tool row`).toContain(needed);
    }
  });

  it("cannot read a row whose fields the engine renamed", async () => {
    // The mutation this file is here for, written out: a row in a shape the Go
    // struct no longer produces must fail to parse, so the failure above is a
    // real signal rather than a permissive parser shrugging.
    expect(parseToolEvent(JSON.stringify({ name: "linetta_write_scene", success: true }))).toBeNull();
  });
});
