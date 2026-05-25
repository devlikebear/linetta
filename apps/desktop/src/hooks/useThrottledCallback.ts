import { useEffect, useMemo, useRef } from "react";

/** Calls `fn` at most once every `intervalMs`. The first call is immediate;
 *  subsequent calls within the window are coalesced and dispatched at window end. */
export function useThrottledCallback<T extends (...args: any[]) => void>(
  fn: T,
  intervalMs: number,
): T {
  const ref = useRef(fn);
  useEffect(() => { ref.current = fn; }, [fn]);
  const lastRun = useRef<number>(0);
  const queued = useRef<{ args: any[] } | null>(null);
  const timer = useRef<number | undefined>(undefined);

  return useMemo(
    () =>
      ((...args: any[]) => {
        const now = Date.now();
        const elapsed = now - lastRun.current;
        if (elapsed >= intervalMs) {
          lastRun.current = now;
          ref.current(...args);
          return;
        }
        queued.current = { args };
        if (timer.current !== undefined) return;
        timer.current = window.setTimeout(() => {
          timer.current = undefined;
          lastRun.current = Date.now();
          if (queued.current) {
            const q = queued.current;
            queued.current = null;
            ref.current(...q.args);
          }
        }, intervalMs - elapsed);
      }) as T,
    [intervalMs],
  );
}
