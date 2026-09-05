import { describe, expect, it } from "vitest";

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
