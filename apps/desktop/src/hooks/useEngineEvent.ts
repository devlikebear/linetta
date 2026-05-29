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
 *  the AI stream as visibly duplicated text ("오늘오늘은은…").
 *
 *  The ref alone does NOT close the StrictMode double-mount window — the
 *  first mount's listener can still fire between its registration and the
 *  cleanup-triggered unlisten resolving. The `cancelled` guard INSIDE the
 *  listener callback below suppresses those late deliveries. */
export function useEngineEvent<T>(event: string, handler: (payload: T) => void) {
  const handlerRef = useRef(handler);
  // Keep the ref current without forcing a re-subscribe.
  handlerRef.current = handler;

  useEffect(() => {
    let unlisten: UnlistenFn | null = null;
    let cancelled = false;
    listen<T>(event, (e) => {
      // StrictMode mounts twice; even if a previous-mount listener was
      // registered, suppress its callbacks until its unlisten resolves and
      // actually fires. Without this guard, both subscriptions briefly run
      // concurrently and each event delivers twice.
      if (cancelled) return;
      handlerRef.current(e.payload);
    }).then((fn) => {
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
