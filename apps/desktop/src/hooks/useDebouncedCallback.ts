import { useEffect, useMemo, useRef } from "react";

export type KeyedDebouncedCallback<K, Args extends unknown[]> =
  ((key: K, ...args: Args) => void) & {
    cancel: (key?: K) => void;
    flush: (key?: K) => void;
  };

/** Returns a stable callback that delays running `fn` until `delayMs` have
 *  elapsed without a new call. The latest `fn` is always used. */
export function useDebouncedCallback<Args extends unknown[]>(
  fn: (...args: Args) => void,
  delayMs: number,
): (...args: Args) => void {
  const ref = useRef(fn);
  useEffect(() => { ref.current = fn; }, [fn]);
  const timer = useRef<number | undefined>(undefined);
  return useMemo(
    () =>
      (...args: Args) => {
        if (timer.current !== undefined) window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => ref.current(...args), delayMs);
      },
    [delayMs],
  );
}

/**
 * Debounces work independently per key. Arguments are captured when the work
 * is scheduled, so a later render cannot redirect an old scene's save to the
 * currently active scene.
 */
export function useKeyedDebouncedCallback<K, Args extends unknown[]>(
  fn: (key: K, ...args: Args) => void,
  delayMs: number,
): KeyedDebouncedCallback<K, Args> {
  const fnRef = useRef(fn);
  useEffect(() => { fnRef.current = fn; }, [fn]);

  const pendingRef = useRef(new Map<K, { timer: number; args: Args }>());
  const callback = useMemo(() => {
    const invoke = ((key: K, ...args: Args) => {
      const previous = pendingRef.current.get(key);
      if (previous) window.clearTimeout(previous.timer);
      const timer = window.setTimeout(() => {
        pendingRef.current.delete(key);
        fnRef.current(key, ...args);
      }, delayMs);
      pendingRef.current.set(key, { timer, args });
    }) as KeyedDebouncedCallback<K, Args>;

    invoke.cancel = (key?: K) => {
      if (key !== undefined) {
        const pending = pendingRef.current.get(key);
        if (pending) window.clearTimeout(pending.timer);
        pendingRef.current.delete(key);
        return;
      }
      for (const pending of pendingRef.current.values()) {
        window.clearTimeout(pending.timer);
      }
      pendingRef.current.clear();
    };

    invoke.flush = (key?: K) => {
      const flushOne = (pendingKey: K) => {
        const pending = pendingRef.current.get(pendingKey);
        if (!pending) return;
        window.clearTimeout(pending.timer);
        pendingRef.current.delete(pendingKey);
        fnRef.current(pendingKey, ...pending.args);
      };
      if (key !== undefined) {
        flushOne(key);
        return;
      }
      for (const pendingKey of [...pendingRef.current.keys()]) flushOne(pendingKey);
    };

    return invoke;
  }, [delayMs]);

  useEffect(() => () => callback.cancel(), [callback]);
  return callback;
}
