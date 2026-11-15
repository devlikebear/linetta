import { useCallback, useEffect, useRef, useState } from "react";

import { trial as trialApi } from "../lib/rpc";
import type { TrialStatus } from "../lib/types";

const UNREACHABLE: TrialStatus = { state: "unreachable", granted_tokens: 0, used_tokens: 0, remaining_tokens: 0 };

/** Where this device's Works account stands in the Linetta trial. Read when
 *  enabled, and again whenever the caller asks: the agent panel does after
 *  each trial turn ends, from the agent-done/agent-error handlers it already
 *  has (a second subscriber to those would only duplicate them). */
export function useTrialStatus(enabled: boolean) {
  const [status, setStatus] = useState<TrialStatus | null>(null);
  const mountedRef = useRef(true);
  // A later read wins even if an earlier one resolves after it.
  const genRef = useRef(0);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const refresh = useCallback(() => {
    const gen = ++genRef.current;
    void trialApi
      .status()
      .then((s) => {
        if (mountedRef.current && gen === genRef.current) setStatus(s);
      })
      .catch(() => {
        if (mountedRef.current && gen === genRef.current) setStatus(UNREACHABLE);
      });
  }, []);

  useEffect(() => {
    if (enabled) refresh();
  }, [enabled, refresh]);

  return { status, refresh };
}

/** Remaining tokens in millions with one decimal, for the panel's header. */
export function formatMillions(tokens: number): string {
  return (Math.max(0, tokens) / 1_000_000).toFixed(1);
}

/** Below this the header shows the remaining tokens as a warning. */
export const TRIAL_LOW_TOKENS = 500_000;
