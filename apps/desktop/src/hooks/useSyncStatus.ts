import { useEffect, useState } from "react";

import { syncApi } from "../lib/rpc";
import type { SyncStatus } from "../lib/types";
import { useEngineEvent } from "./useEngineEvent";

/** Automatic sync as the engine last reported it. A build without it, or a
 *  failure to ask, leaves null: every screen then shows nothing about sync. */
export function useSyncStatus(available: boolean): [SyncStatus | null, (next: SyncStatus) => void] {
  const [status, setStatus] = useState<SyncStatus | null>(null);

  useEffect(() => {
    if (!available) return undefined;
    let cancelled = false;
    syncApi
      .status()
      .then((next) => {
        if (!cancelled) setStatus(next);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [available]);

  useEngineEvent<SyncStatus>("sync-status", (next) => {
    if (available) setStatus(next);
  });

  return [status, setStatus];
}
