import { useCallback, useEffect, useRef, useState } from "react";

import { openExternalUrl, publishApi } from "../lib/rpc";
import type { PublishConnectStatus } from "../lib/types";

const POLL_MS = 1500;

/** Connecting this device to Linetta Works: the connection publishing and the
 *  Linetta trial both use. The app opens the site's connect page and polls
 *  until the writer answers in the browser.
 *
 *  `start` takes the writer's agreement to the publishing disclosure, which
 *  the engine requires before it asks the site for anything. */
export function useWorksConnect(enabled = true) {
  const [status, setStatus] = useState<PublishConnectStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const pollRef = useRef<number | null>(null);
  // Every poll tick checks it still belongs to the newest attempt: a tick
  // resolving after a cancel, or after a second connect, must not paint its
  // stale answer over the current one (the ProviderSection lesson).
  const pollGenRef = useRef(0);

  const stopPolling = useCallback(() => {
    if (pollRef.current !== null) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
    pollGenRef.current += 1;
  }, []);

  const startPolling = useCallback(() => {
    stopPolling();
    const gen = pollGenRef.current;
    pollRef.current = window.setInterval(() => {
      void publishApi
        .connectStatus()
        .then((s) => {
          if (gen !== pollGenRef.current) return;
          setStatus(s);
          if (s.state !== "waiting") stopPolling();
        })
        .catch((e: unknown) => {
          if (gen !== pollGenRef.current) return;
          stopPolling();
          setError(e);
        });
    }, POLL_MS);
  }, [stopPolling]);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    publishApi
      .connectStatus()
      .then((s) => {
        if (cancelled) return;
        setStatus(s);
        // Left and reopened mid-connect: keep waiting.
        if (s.state === "waiting") startPolling();
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e);
      });
    return () => {
      cancelled = true;
      stopPolling();
    };
  }, [enabled, startPolling, stopPolling]);

  const guard = useCallback(async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }, []);

  const start = useCallback(
    (consented: boolean) =>
      guard(async () => {
        const s = await publishApi.connectStart(consented);
        setStatus(s);
        if (s.verify_url) await openExternalUrl(s.verify_url);
        startPolling();
      }),
    [guard, startPolling],
  );

  /** Ends the connection, or abandons an attempt still waiting. */
  const disconnect = useCallback(
    () =>
      guard(async () => {
        stopPolling();
        await publishApi.disconnect();
        setStatus(await publishApi.connectStatus());
      }),
    [guard, stopPolling],
  );

  return { status, busy, error, guard, start, disconnect };
}
