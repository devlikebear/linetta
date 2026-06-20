import { useCallback, useEffect, useRef } from "react";

export function useIdleTimer(idleMs: number, onIdle: () => void) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const onIdleRef = useRef(onIdle);
  onIdleRef.current = onIdle;

  const cancel = useCallback(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const markActivity = useCallback(() => {
    if (timerRef.current !== null) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      timerRef.current = null;
      onIdleRef.current();
    }, idleMs);
  }, [idleMs]);

  useEffect(() => cancel, [cancel]);

  return { markActivity, cancel };
}
