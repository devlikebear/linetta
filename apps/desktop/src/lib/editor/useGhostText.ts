import { useCallback, useEffect, useRef, useState } from "react";
import type { Editor } from "@tiptap/react";
import type { Transaction } from "@tiptap/pm/state";
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
 * notifications to a Tiptap editor's GhostExtension commands.
 *
 * Single-mode: start() — one run, auto-commit on done.
 * Variation-mode: startVariations(args, n) — N parallel runs, user picks via ◀▶+Tab.
 */
export function useGhostText(editor: Editor | null) {
  const [status, setStatus] = useState<GhostStatus>({ kind: "idle" });
  // Single-mode active run.
  const runIdRef = useRef<string | null>(null);
  const accumulatedRef = useRef<string>("");
  // Variation-mode active runs.
  const activeRunIdsRef = useRef<string[]>([]);
  const runIdToVariationRef = useRef<Map<string, number>>(new Map());

  // Helper: cancel every in-flight run (single + variations).
  const cancelAllInFlight = useCallback(() => {
    for (const id of activeRunIdsRef.current) {
      aiApi.cancel(id).catch(() => {});
    }
    if (runIdRef.current) {
      aiApi.cancel(runIdRef.current).catch(() => {});
    }
    activeRunIdsRef.current = [];
    runIdToVariationRef.current.clear();
    runIdRef.current = null;
  }, []);

  const start = useCallback(
    async ({ nodeId, prompt, options, selectionText = "", replaceRange }: RunArgs) => {
      if (!editor) return;
      cancelAllInFlight();
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
    [editor, cancelAllInFlight],
  );

  const startVariations = useCallback(
    async (
      { nodeId, prompt, options, selectionText = "", replaceRange }: RunArgs,
      n: number,
    ) => {
      if (!editor) return;
      cancelAllInFlight();
      editor.commands.dropGhostText();

      const mode: GhostMode = replaceRange
        ? { kind: "replace", from: replaceRange.from, to: replaceRange.to }
        : { kind: "insert", pos: editor.state.selection.head };
      editor.commands.setGhostVariations(n, mode);
      setStatus({ kind: "running", runId: "(variations)", text: "" });

      for (let i = 0; i < n; i++) {
        const idx = i;
        aiApi
          .run(nodeId, prompt, options, selectionText)
          .then(({ run_id }) => {
            activeRunIdsRef.current.push(run_id);
            runIdToVariationRef.current.set(run_id, idx);
          })
          .catch((e) => {
            editor.commands.setGhostVariationDone(idx, String(e));
          });
      }
    },
    [editor, cancelAllInFlight],
  );

  const cancel = useCallback(async () => {
    cancelAllInFlight();
    if (editor) editor.commands.dropGhostText();
    setStatus({ kind: "idle" });
  }, [editor, cancelAllInFlight]);

  const accept = useCallback(() => {
    if (!editor) return;
    // Cancel any remaining in-flight runs (token saving) before commit.
    cancelAllInFlight();
    editor.commands.acceptGhostText();
    accumulatedRef.current = "";
    setStatus({ kind: "idle" });
  }, [editor, cancelAllInFlight]);

  const drop = useCallback(() => {
    if (!editor) return;
    cancelAllInFlight();
    editor.commands.dropGhostText();
    accumulatedRef.current = "";
    setStatus({ kind: "idle" });
  }, [editor, cancelAllInFlight]);

  useEngineEvent<AIDelta>("ai-delta", (p) => {
    if (!editor) return;
    // Variation-mode first. NOTE: an early delta can race ahead of the
    // aiApi.run().then() that registers this run_id → idx mapping; such a
    // delta finds no mapping and is dropped here. That's benign — the ai-done
    // handler overwrites the variation with full_text, so the final text is
    // always complete.
    const vIdx = runIdToVariationRef.current.get(p.run_id);
    if (vIdx !== undefined) {
      const existing = ghostPluginKey.getState(editor.state);
      const current = existing?.variations[vIdx]?.text ?? "";
      editor.commands.setGhostVariationText(vIdx, current + p.text);
      return;
    }
    // Single-mode fallback.
    if (p.run_id !== runIdRef.current) return;
    accumulatedRef.current += p.text;
    const existing = ghostPluginKey.getState(editor.state);
    editor.commands.setGhostText(accumulatedRef.current, existing?.mode);
    setStatus({ kind: "running", runId: p.run_id, text: accumulatedRef.current });
  });

  useEngineEvent<AIReset>("ai-reset", (p) => {
    if (!editor) return;
    const vIdx = runIdToVariationRef.current.get(p.run_id);
    if (vIdx !== undefined) {
      editor.commands.setGhostVariationText(vIdx, p.text);
      return;
    }
    if (p.run_id !== runIdRef.current) return;
    accumulatedRef.current = p.text;
    const existing = ghostPluginKey.getState(editor.state);
    editor.commands.setGhostText(p.text, existing?.mode);
    setStatus({ kind: "running", runId: p.run_id, text: p.text });
  });

  useEngineEvent<AIDone>("ai-done", (p) => {
    if (!editor) return;
    const vIdx = runIdToVariationRef.current.get(p.run_id);
    if (vIdx !== undefined) {
      // Variation-mode: mark this variation done, do NOT auto-commit.
      editor.commands.setGhostVariationText(vIdx, p.full_text);
      editor.commands.setGhostVariationDone(vIdx);
      // Remove from active runs (already done).
      activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
      return;
    }
    // Single-mode: auto-commit (Plan 18 fixup).
    if (p.run_id !== runIdRef.current) return;
    const existing = ghostPluginKey.getState(editor.state);
    editor.commands.setGhostText(p.full_text, existing?.mode);
    editor.commands.acceptGhostText();
    runIdRef.current = null;
    accumulatedRef.current = "";
    setStatus({ kind: "done", text: p.full_text });
  });

  useEngineEvent<AIError>("ai-error", (p) => {
    if (!editor) return;
    const vIdx = runIdToVariationRef.current.get(p.run_id);
    if (vIdx !== undefined) {
      editor.commands.setGhostVariationDone(vIdx, p.message);
      activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
      return;
    }
    if (p.run_id !== runIdRef.current) return;
    editor.commands.dropGhostText();
    runIdRef.current = null;
    setStatus({ kind: "error", message: p.message });
  });

  useEngineEvent<AICancelled>("ai-cancelled", (p) => {
    if (!editor) return;
    const vIdx = runIdToVariationRef.current.get(p.run_id);
    if (vIdx !== undefined) {
      // Just clean up the mapping; do NOT touch ghost decoration (might be other variations alive or already accepted).
      runIdToVariationRef.current.delete(p.run_id);
      activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
      return;
    }
    if (p.run_id !== runIdRef.current) return;
    runIdRef.current = null;
    editor.commands.dropGhostText();
    setStatus({ kind: "idle" });
  });

  // Plan 20: the GhostExtension Tab/Esc shortcuts (editor-focused, variation mode)
  // dispatch a "drop" plugin meta directly — bypassing this hook's accept()/drop().
  // Observe those transactions so we still cancel the losing variation runs and
  // reset status. Single-mode is unaffected: activeRunIdsRef is empty, and the
  // ai-done handler's explicit setStatus("done") runs after this listener (same
  // tick) so auto-close still fires.
  useEffect(() => {
    if (!editor) return;
    const handler = ({
      transaction,
      appendedTransactions,
    }: {
      transaction: Transaction;
      appendedTransactions: Transaction[];
    }) => {
      for (const tr of [transaction, ...appendedTransactions]) {
        const meta = tr.getMeta(ghostPluginKey) as { kind?: string } | undefined;
        if (meta?.kind === "drop") {
          if (activeRunIdsRef.current.length > 0) {
            for (const id of activeRunIdsRef.current) {
              aiApi.cancel(id).catch(() => {});
            }
            activeRunIdsRef.current = [];
            runIdToVariationRef.current.clear();
            setStatus({ kind: "idle" });
          }
        }
      }
    };
    editor.on("transaction", handler);
    return () => {
      editor.off("transaction", handler);
    };
  }, [editor]);

  // Cleanup on editor change/unmount — cancel any in-flight runs.
  useEffect(() => {
    return () => {
      cancelAllInFlight();
    };
  }, [editor, cancelAllInFlight]);

  return { status, start, startVariations, cancel, accept, drop };
}
