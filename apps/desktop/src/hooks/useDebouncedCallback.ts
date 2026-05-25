import { useEffect, useMemo, useRef } from "react";

/** Returns a stable callback that delays running `fn` until `delayMs` have
 *  elapsed without a new call. The latest `fn` is always used. */
export function useDebouncedCallback<T extends (...args: any[]) => void>(
  fn: T,
  delayMs: number,
): T {
  const ref = useRef(fn);
  useEffect(() => { ref.current = fn; }, [fn]);
  const timer = useRef<number | undefined>(undefined);
  return useMemo(
    () =>
      ((...args: any[]) => {
        if (timer.current !== undefined) window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => ref.current(...args), delayMs);
      }) as T,
    [delayMs],
  );
}
