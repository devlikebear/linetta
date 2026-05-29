import { useCallback, useEffect, useRef, useState } from "react";
import type { Editor } from "@tiptap/react";
import { ai as aiApi } from "../rpc";
import type { AICancelled, AIDelta, AIDone, AIError, AIOptions, AIReset } from "../types";
import { useEngineEvent } from "../../hooks/useEngineEvent";
import { ghostPluginKey, type GhostMode } from "../../components/editor/GhostExtension";

export type GhostStatus =
  | { kind: "idle" }
  | { kind: "running"; runId: string; text: string }
  | { kind: "done"; text: string }
  | { kind: "error"; message: string };

interface RunArgs {
  nodeId: string;
  prompt: string;
  options: AIOptions;
  selectionText?: string;
  /** When provided, ghost is committed by replacing this range instead of inserting at the head. */
  replaceRange?: { from: number; to: number };
}

/**
 * useGhostText wires ai.run RPC + ai-delta/done/error/reset/cancelled
 * notifications to a Tiptap editor's GhostExtension commands. The hook keeps
 * a single active run; starting a new one cancels the previous.
 */
export function useGhostText(editor: Editor | null) {
  const [status, setStatus] = useState<GhostStatus>({ kind: "idle" });
  const runIdRef = useRef<string | null>(null);
  const accumulatedRef = useRef<string>("");

  const start = useCallback(
    async ({ nodeId, prompt, options, selectionText = "", replaceRange }: RunArgs) => {
      if (!editor) return;
      if (runIdRef.current) {
        try {
          await aiApi.cancel(runIdRef.current);
        } catch {
          /* benign */
        }
      }
      editor.commands.dropGhostText();
      accumulatedRef.current = "";
      try {
        const { run_id } = await aiApi.run(nodeId, prompt, options, selectionText);
        runIdRef.current = run_id;
        setStatus({ kind: "running", runId: run_id, text: "" });
        const mode: GhostMode = replaceRange
          ? { kind: "replace", from: replaceRange.from, to: replaceRange.to }
          : { kind: "insert", pos: editor.state.selection.head };
        editor.commands.setGhostText("", mode);
      } catch (e) {
        setStatus({ kind: "error", message: String(e) });
      }
    },
    [editor],
  );

  const cancel = useCallback(async () => {
    if (!runIdRef.current) return;
    try {
      await aiApi.cancel(runIdRef.current);
    } catch {
      /* benign */
    }
    runIdRef.current = null;
    if (editor) editor.commands.dropGhostText();
    setStatus({ kind: "idle" });
  }, [editor]);

  const accept = useCallback(() => {
    if (!editor) return;
    editor.commands.acceptGhostText();
    runIdRef.current = null;
    accumulatedRef.current = "";
    setStatus({ kind: "idle" });
  }, [editor]);

  const drop = useCallback(() => {
    if (!editor) return;
    if (runIdRef.current) {
      aiApi.cancel(runIdRef.current).catch(() => {});
    }
    editor.commands.dropGhostText();
    runIdRef.current = null;
    accumulatedRef.current = "";
    setStatus({ kind: "idle" });
  }, [editor]);

  useEngineEvent<AIDelta>("ai-delta", (p) => {
    if (p.run_id !== runIdRef.current || !editor) return;
    accumulatedRef.current += p.text;
    const existing = ghostPluginKey.getState(editor.state);
    editor.commands.setGhostText(accumulatedRef.current, existing?.mode);
    setStatus({ kind: "running", runId: p.run_id, text: accumulatedRef.current });
  });

  useEngineEvent<AIReset>("ai-reset", (p) => {
    if (p.run_id !== runIdRef.current || !editor) return;
    accumulatedRef.current = p.text;
    const existing = ghostPluginKey.getState(editor.state);
    editor.commands.setGhostText(p.text, existing?.mode);
    setStatus({ kind: "running", runId: p.run_id, text: p.text });
  });

  useEngineEvent<AIDone>("ai-done", (p) => {
    if (p.run_id !== runIdRef.current || !editor) return;
    accumulatedRef.current = p.full_text;
    const existing = ghostPluginKey.getState(editor.state);
    editor.commands.setGhostText(p.full_text, existing?.mode);
    // Mark ghost as done (stops blinking) via the GhostExtension's "done" meta.
    const tr = editor.state.tr.setMeta(ghostPluginKey, { kind: "done" });
    editor.view.dispatch(tr);
    setStatus({ kind: "done", text: p.full_text });
  });

  useEngineEvent<AIError>("ai-error", (p) => {
    if (p.run_id !== runIdRef.current || !editor) return;
    editor.commands.dropGhostText();
    runIdRef.current = null;
    setStatus({ kind: "error", message: p.message });
  });

  useEngineEvent<AICancelled>("ai-cancelled", (p) => {
    if (p.run_id !== runIdRef.current || !editor) return;
    runIdRef.current = null;
    editor.commands.dropGhostText();
    setStatus({ kind: "idle" });
  });

  // Cleanup on editor change/unmount — cancel any in-flight run.
  useEffect(() => {
    return () => {
      if (runIdRef.current) {
        aiApi.cancel(runIdRef.current).catch(() => {});
      }
    };
  }, [editor]);

  return { status, start, cancel, accept, drop };
}
