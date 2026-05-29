import { useCallback, useEffect, useRef, useState } from "react";
import { ai as aiApi } from "../rpc";
import type { AICancelled, AIDelta, AIDone, AIError, AIOptions, AIReset } from "../types";
import { useEngineEvent } from "../../hooks/useEngineEvent";

export interface GenVariation {
  text: string;
  done: boolean;
  error?: string;
}

export type GenStatus =
  | { kind: "idle" }
  | { kind: "running" }
  | { kind: "done" };

export interface GenRunArgs {
  nodeId: string;
  prompt: string;
  options: AIOptions;
  selectionText?: string;
}

/**
 * useAIGeneration runs one or N parallel ai.run calls and accumulates each
 * run's streamed text into React state (no editor decoration). The consumer
 * (AIModal) renders variations; the Workspace commits the chosen one.
 */
export function useAIGeneration() {
  const [variations, setVariations] = useState<GenVariation[]>([]);
  const [currentIdx, setCurrentIdx] = useState(0);
  const [status, setStatus] = useState<GenStatus>({ kind: "idle" });

  const activeRunIdsRef = useRef<string[]>([]);
  const runIdToVariationRef = useRef<Map<string, number>>(new Map());
  const variationsRef = useRef(variations);
  variationsRef.current = variations;

  const cancelAllInFlight = useCallback(() => {
    for (const id of activeRunIdsRef.current) {
      aiApi.cancel(id).catch(() => {});
    }
    activeRunIdsRef.current = [];
    runIdToVariationRef.current.clear();
  }, []);

  // Derive status from variations: running while any slot is incomplete, done
  // once every slot is done (or errored), idle when there are no variations.
  useEffect(() => {
    if (variations.length === 0) {
      setStatus({ kind: "idle" });
      return;
    }
    setStatus(variations.every((v) => v.done) ? { kind: "done" } : { kind: "running" });
  }, [variations]);

  const launch = useCallback(
    ({ nodeId, prompt, options, selectionText = "" }: GenRunArgs, n: number) => {
      cancelAllInFlight();
      const slots: GenVariation[] = Array.from({ length: n }, () => ({ text: "", done: false }));
      setVariations(slots);
      setCurrentIdx(0);
      setStatus({ kind: "running" });
      for (let i = 0; i < n; i++) {
        const idx = i;
        aiApi
          .run(nodeId, prompt, options, selectionText)
          .then(({ run_id }) => {
            activeRunIdsRef.current.push(run_id);
            runIdToVariationRef.current.set(run_id, idx);
          })
          .catch((e) => {
            setVariations((prev) => {
              const next = prev.slice();
              if (next[idx]) next[idx] = { ...next[idx], done: true, error: String(e) };
              return next;
            });
          });
      }
    },
    [cancelAllInFlight],
  );

  const start = useCallback((args: GenRunArgs) => launch(args, 1), [launch]);
  const startVariations = useCallback(
    (args: GenRunArgs, n: number) => launch(args, n),
    [launch],
  );

  const switchVariation = useCallback((direction: -1 | 1) => {
    const n = variationsRef.current.length;
    if (n <= 1) return;
    setCurrentIdx((idx) => ((idx + direction) % n + n) % n);
  }, []);

  // cancel: stop all in-flight runs and clear state. Used both for explicit
  // cancel (Esc/취소/백드롭) and after accept (to kill the losing variations).
  const cancel = useCallback(() => {
    cancelAllInFlight();
    setVariations([]);
    setCurrentIdx(0);
    setStatus({ kind: "idle" });
  }, [cancelAllInFlight]);

  useEngineEvent<AIDelta>("ai-delta", (p) => {
    const idx = runIdToVariationRef.current.get(p.run_id);
    if (idx === undefined) return;
    setVariations((prev) => {
      const next = prev.slice();
      if (next[idx]) next[idx] = { ...next[idx], text: next[idx].text + p.text };
      return next;
    });
  });

  useEngineEvent<AIReset>("ai-reset", (p) => {
    const idx = runIdToVariationRef.current.get(p.run_id);
    if (idx === undefined) return;
    setVariations((prev) => {
      const next = prev.slice();
      if (next[idx]) next[idx] = { ...next[idx], text: p.text };
      return next;
    });
  });

  useEngineEvent<AIDone>("ai-done", (p) => {
    const idx = runIdToVariationRef.current.get(p.run_id);
    if (idx === undefined) return;
    activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
    setVariations((prev) => {
      const next = prev.slice();
      // full_text is the authoritative backstop for any deltas dropped by the
      // early-delta race (delta arriving before run_id → idx mapping is set).
      if (next[idx]) next[idx] = { ...next[idx], text: p.full_text, done: true };
      return next;
    });
  });

  useEngineEvent<AIError>("ai-error", (p) => {
    const idx = runIdToVariationRef.current.get(p.run_id);
    if (idx === undefined) return;
    activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
    setVariations((prev) => {
      const next = prev.slice();
      if (next[idx]) next[idx] = { ...next[idx], done: true, error: p.message };
      return next;
    });
  });

  useEngineEvent<AICancelled>("ai-cancelled", (p) => {
    const idx = runIdToVariationRef.current.get(p.run_id);
    if (idx === undefined) return;
    runIdToVariationRef.current.delete(p.run_id);
    activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
  });

  // Cleanup on unmount.
  useEffect(() => {
    return () => {
      cancelAllInFlight();
    };
  }, [cancelAllInFlight]);

  return {
    variations,
    currentIdx,
    status,
    start,
    startVariations,
    switchVariation,
    cancel,
  };
}
