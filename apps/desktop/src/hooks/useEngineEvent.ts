import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { useEffect } from "react";

/** Subscribe to a Tauri event for the lifetime of the component. The handler
 *  is captured fresh each render — pass an inline arrow or memoize upstream
 *  as needed. */
export function useEngineEvent<T>(event: string, handler: (payload: T) => void) {
  useEffect(() => {
    let unlisten: UnlistenFn | null = null;
    let cancelled = false;
    listen<T>(event, (e) => handler(e.payload)).then((fn) => {
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
  }, [event, handler]);
}
