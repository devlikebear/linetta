import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { getSchema } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import { describe, expect, it } from "vitest";
import { buildMentionExtension } from "./MentionExtension";
import { NoteMarkerExtension } from "./NoteMarkerExtension";

// The engine's markdown exporter keeps its own list of the nodes and marks it
// handles, and a Go test holds that list against this same file. If you add or
// remove an editor extension this test fails: update the JSON, and the Go test
// then fails until the exporter decides what the new node becomes (#160).
const fixture = resolve(
  __dirname,
  "../../../../../engine/internal/export/testdata/editor_schema.json",
);

describe("editor schema", () => {
  it("matches the node and mark list the markdown exporter is tested against", () => {
    const schema = getSchema([
      // The same set Tiptap.tsx and Workspace.tsx mount.
      StarterKit.configure({}),
      buildMentionExtension({ search: async () => [], onStateChange: () => {} }),
      NoteMarkerExtension,
    ]);
    const expected = JSON.parse(readFileSync(fixture, "utf8")) as { nodes: string[]; marks: string[] };

    expect(Object.keys(schema.nodes).sort()).toEqual([...expected.nodes].sort());
    expect(Object.keys(schema.marks).sort()).toEqual([...expected.marks].sort());
  });
});
