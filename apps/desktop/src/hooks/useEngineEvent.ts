import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { useEffect, useRef } from "react";

/** Subscribe to a Tauri event for the lifetime of the component.
 *
 *  The handler is read through a ref so the underlying Tauri subscription is
 *  registered ONCE per event-name. If `handler` were in the effect's deps,
 *  every parent re-render (e.g., setState triggered from inside the handler
 *  itself) would unsubscribe + resubscribe — and during the brief window
 *  between the two async `listen()` promises resolving, both subscriptions
 *  can be live, causing each event to fire twice. That race manifested in
 *  the AI stream as visibly duplicated text ("오늘오늘은은…"). */
export function useEngineEvent<T>(event: string, handler: (payload: T) => void) {
  const handlerRef = useRef(handler);
  // Keep the ref current without forcing a re-subscribe.
  handlerRef.current = handler;

  useEffect(() => {
    let unlisten: UnlistenFn | null = null;
    let cancelled = false;
    listen<T>(event, (e) => handlerRef.current(e.payload)).then((fn) => {
      if (cancelled) {
        fn();
      } else {
        unlisten = fn;
      }
    });
    return () => {
      cancelled = true;
      if (unlisten) unlisten();
    };
  }, [event]);
}
