import { act, render, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { buildMentionExtension } from "./editor/MentionExtension";
import { sceneEditorExtensions } from "./editor/sceneExtensions";
import type { TiptapHandle } from "./editor/Tiptap";
import { ZenMode } from "./ZenMode";

// ZenMode.test.tsx mocks the editor; this file mounts the real one, because
// what broke here was the schema the Zen editor was built with.

const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
}));

beforeEach(() => {
  vi.clearAllMocks();
  mocks.settingsGet.mockResolvedValue({ language: "ko" });
});

const sceneDoc = {
  type: "doc",
  content: [
    {
      type: "paragraph",
      content: [
        { type: "text", text: "그날 " },
        { type: "mention", attrs: { id: "ent-1", label: "민호" } },
        { type: "text", text: "는 웃었다." },
        { type: "noteMarker", attrs: { noteId: "note-1" } },
      ],
    },
  ],
};

function nodeTypes(doc: unknown): string[] {
  const out: string[] = [];
  const walk = (n: unknown) => {
    if (!n || typeof n !== "object") return;
    const { type, content } = n as { type?: unknown; content?: unknown };
    if (typeof type === "string") out.push(type);
    if (Array.isArray(content)) content.forEach(walk);
  };
  walk(doc);
  return out;
}

describe("ZenMode editor", () => {
  it("keeps mentions and note markers when a scene is opened and edited in Zen", async () => {
    const onChange = vi.fn();
    let handle: TiptapHandle | null = null;
    render(
      <I18nProvider>
        <ZenMode
          initialDoc={sceneDoc}
          charCount={0}
          sceneLabel="씬 1"
          extensions={sceneEditorExtensions(
            buildMentionExtension({ search: async () => [], onStateChange: () => {} }),
          )}
          onChange={onChange}
          onCharCount={vi.fn()}
          onManualSave={vi.fn()}
          onMountEditor={(h) => { handle = h; }}
          onExit={vi.fn()}
        />
      </I18nProvider>,
    );
    await waitFor(() => expect(handle?.editor).toBeTruthy());

    expect(nodeTypes(handle!.getDoc())).toEqual(
      expect.arrayContaining(["mention", "noteMarker"]),
    );

    act(() => {
      handle!.editor!.commands.insertContent("!");
    });

    expect(onChange).toHaveBeenCalled();
    const saved = onChange.mock.lastCall![0];
    expect(nodeTypes(saved)).toEqual(expect.arrayContaining(["mention", "noteMarker"]));
    expect(JSON.stringify(saved)).toContain("는 웃었다.");
  });
});
