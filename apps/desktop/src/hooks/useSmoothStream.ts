import { useEffect, useRef, useState } from "react";

/**
 * nextReveal returns the next displayed text on the way from `shown` toward
 * `target`. It reveals a fraction of the remaining gap each step (min 2 chars)
 * so a large chunk arriving at once unrolls smoothly instead of in one jump.
 *
 * - When `target` diverges (e.g. a stream reset shrinks/replaces it), snap to
 *   `target` so we re-sync instead of revealing stale characters.
 * - When already caught up, return `target` unchanged.
 */
export function nextReveal(shown: string, target: string): string {
  if (!target.startsWith(shown)) return target;
  if (shown.length >= target.length) return target;
  const remaining = target.length - shown.length;
  const step = Math.max(2, Math.ceil(remaining * 0.18));
  return target.slice(0, shown.length + step);
}

/**
 * useSmoothStream decouples the render rate from the (chunky, bursty) delta
 * arrival rate: it reveals `target` toward completion one animation frame at a
 * time. While `active`, the returned text catches up to `target` smoothly;
 * when inactive (e.g. the run finished) it returns the full `target` at once.
 */
export function useSmoothStream(target: string, active: boolean): string {
  const [shown, setShown] = useState(target);
  const targetRef = useRef(target);
  targetRef.current = target;

  useEffect(() => {
    if (!active) {
      setShown(targetRef.current);
      return;
    }
    let raf = 0;
    const tick = () => {
      setShown((cur) => nextReveal(cur, targetRef.current));
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [active]);

  return active ? shown : target;
}
