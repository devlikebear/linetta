import { createElement, useCallback, useEffect, useState, type ReactElement } from "react";
import { semanticSearch } from "../lib/rpc";
import type { SemanticModelStatus, SemanticStatus } from "../lib/types";
import { useEngineEvent } from "./useEngineEvent";

/** What search.semantic_progress carries: the model downloading, or a work's
 *  paragraphs being embedded. */
type ProgressEvent =
  | { phase: "download"; model: SemanticModelStatus }
  | { phase: "index"; project_id: string; done: number; total: number };

/** Meaning search's state (#384), kept current from the engine's progress
 *  events while active. projectId scopes the index progress. */
export function useSemanticStatus(projectId: string | undefined, active: boolean) {
  const [status, setStatus] = useState<SemanticStatus | null>(null);
  const [error, setError] = useState<unknown>(null);

  const refresh = useCallback(() => {
    if (!active) return;
    semanticSearch
      .status(projectId)
      .then((s) => { setStatus(s); setError(null); })
      .catch(setError);
  }, [projectId, active]);
  useEffect(refresh, [refresh]);

  const onProgress = useCallback((event: ProgressEvent) => {
    setStatus((cur) => {
      if (!cur) return cur;
      if (event.phase === "download") {
        // The model just finished: the index is about to start.
        if (event.model.installed && !cur.model.installed) window.setTimeout(refresh, 0);
        return { ...cur, model: event.model };
      }
      if (event.project_id !== projectId) return cur;
      return { ...cur, index: { project_id: event.project_id, done: event.done, total: event.total } };
    });
  }, [projectId, refresh]);

  const enable = useCallback(async () => {
    const s = await semanticSearch.enable();
    setStatus((cur) => ({ ...s, index: cur?.index }));
    refresh();
  }, [refresh]);

  const disable = useCallback(async (deleteModel: boolean) => {
    const s = await semanticSearch.disable(deleteModel);
    setStatus((cur) => ({ ...s, index: cur?.index }));
    refresh();
  }, [refresh]);

  // Rendered only while active: a closed search box must not hold an engine
  // subscription (and a page that never opens it never makes one).
  const listener: ReactElement | null = active ? createElement(ProgressListener, { onProgress }) : null;

  return { status, error, enable, disable, refresh, listener };
}

function ProgressListener({ onProgress }: { onProgress: (event: ProgressEvent) => void }) {
  useEngineEvent<ProgressEvent>("search-semantic-progress", onProgress);
  return null;
}

/** A byte count as the settings and the search box show it: "135MB". */
export function megabytes(bytes: number): string {
  return `${Math.round(bytes / (1024 * 1024))}MB`;
}
