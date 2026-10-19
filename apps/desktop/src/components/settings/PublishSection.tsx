import { useCallback, useEffect, useRef, useState } from "react";

import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import { openExternalUrl, publishApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { PublishConnectStatus, PublishFailureReason } from "../../lib/types";

/** Settings → Publishing: connect this device to Linetta Works.
 *
 *  Publishing is optional, and the pane's first job is to say what it sends
 *  where before anything is sent: the connect button stays disabled until the
 *  writer ticks the disclosure, the same way a provider's consent works. The
 *  connection itself happens in the browser — the app opens the site's
 *  connect page and waits, showing the code the browser shows too.
 */

const POLL_MS = 1500;

const FAILURE_KEYS: Record<PublishFailureReason, MessageKey> = {
  denied: "settings.publish.failed.denied",
  expired: "settings.publish.failed.expired",
  network: "settings.publish.failed.network",
  claimed: "settings.publish.failed.claimed",
  store: "settings.publish.failed.store",
  failed: "settings.publish.failed.failed",
  revoked: "settings.publish.failed.revoked",
};

export type PublishSectionProps = {
  /** False on builds that cannot publish (iOS): the pane renders nothing. */
  available: boolean;
};

export function PublishSection({ available }: PublishSectionProps) {
  const { t } = useI18n();
  const [status, setStatus] = useState<PublishConnectStatus | null>(null);
  const [consented, setConsented] = useState(false);
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
    if (!available) return;
    let cancelled = false;
    publishApi
      .connectStatus()
      .then((s) => {
        if (cancelled) return;
        setStatus(s);
        // Settings was left and reopened mid-connect: keep waiting.
        if (s.state === "waiting") startPolling();
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e);
      });
    return () => {
      cancelled = true;
      stopPolling();
    };
  }, [available, startPolling, stopPolling]);

  if (!available) return null;

  const guard = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const connect = () =>
    guard(async () => {
      const s = await publishApi.connectStart(consented);
      setStatus(s);
      if (s.verify_url) await openExternalUrl(s.verify_url);
      startPolling();
    });

  const cancel = () =>
    guard(async () => {
      stopPolling();
      await publishApi.disconnect();
      setStatus(await publishApi.connectStatus());
    });

  const disconnect = () =>
    guard(async () => {
      stopPolling();
      await publishApi.disconnect();
      setConsented(false);
      setStatus(await publishApi.connectStatus());
    });

  const state = status?.state ?? "disconnected";
  const site = status?.site_url ?? "";

  return (
    <section className="settings-section" id="publish-settings" data-testid="publish-section">
      <h3>{t("settings.publish.title")}</h3>
      <p className="sd">{t("settings.publish.description")}</p>
      {site && (
        <div className="field-actions">
          <button
            type="button"
            className="btn ghost sm"
            onClick={() => void guard(() => openExternalUrl(site))}
            data-testid="publish-browse"
          >
            {t("settings.publish.browse")}
          </button>
        </div>
      )}

      {state === "connected" && status && (
        <div className="modal-field" data-testid="publish-connected">
          <p className="sk">
            {t("settings.publish.connected", { penName: status.pen_name ?? "", handle: status.handle ?? "" })}
          </p>
          <div className="field-actions">
            <button
              type="button"
              className="btn ghost sm"
              disabled={busy}
              onClick={() => void guard(() => openExternalUrl(`${site}/me`))}
              data-testid="publish-my-works"
            >
              {t("settings.publish.myWorks")}
            </button>
            <button
              type="button"
              className="btn ghost sm"
              disabled={busy}
              onClick={() => void disconnect()}
              data-testid="publish-disconnect"
            >
              {t("settings.publish.disconnect")}
            </button>
          </div>
          <p className="sd">{t("settings.publish.disconnectNote")}</p>
        </div>
      )}

      {state === "waiting" && status && (
        <div className="modal-field" data-testid="publish-waiting">
          <p className="sd">{t("settings.publish.waiting")}</p>
          <p className="sk publish-confirm-code" aria-label={t("settings.publish.confirmCode")} data-testid="publish-confirm-code">
            {status.confirm_code}
          </p>
          <div className="field-actions">
            {status.verify_url && (
              <button
                type="button"
                className="btn ghost sm"
                disabled={busy}
                onClick={() => void guard(() => openExternalUrl(status.verify_url ?? ""))}
                data-testid="publish-open-again"
              >
                {t("settings.publish.openAgain")}
              </button>
            )}
            <button
              type="button"
              className="btn ghost sm"
              disabled={busy}
              onClick={() => void cancel()}
              data-testid="publish-cancel"
            >
              {t("settings.publish.cancel")}
            </button>
          </div>
        </div>
      )}

      {(state === "disconnected" || state === "failed") && (
        <>
          {state === "failed" && status?.failure_reason && (
            <p className="sd" role="alert" data-testid="publish-failed">
              {t(FAILURE_KEYS[status.failure_reason] ?? "settings.publish.failed.failed")}
            </p>
          )}
          <p className="sd" data-testid="publish-disclosure">{t("settings.publish.disclosure")}</p>
          <label className="modal-field modal-check">
            <input
              type="checkbox"
              checked={consented}
              disabled={busy}
              onChange={(e) => setConsented(e.target.checked)}
              data-testid="publish-consent"
            />
            {t("settings.publish.consent")}
          </label>
          <div className="field-actions">
            <button
              type="button"
              className="btn sm"
              disabled={busy || !consented}
              onClick={() => void connect()}
              data-testid="publish-connect"
            >
              {state === "failed" ? t("settings.publish.retry") : t("settings.publish.connect")}
            </button>
          </div>
        </>
      )}

      <p className="sd">{t("settings.publish.storage")}</p>

      {error ? (
        <p className="sd" role="alert" data-testid="publish-error">
          {rpcErrorMessage(error, t)}
        </p>
      ) : null}
    </section>
  );
}
