import { useEffect, useRef, useState } from "react";

import { useTrialStatus } from "../../hooks/useTrialStatus";
import { useWorksConnect } from "../../hooks/useWorksConnect";
import { useI18n } from "../../lib/i18n";
import { openExternalUrl, settings as settingsApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { TrialStatus } from "../../lib/types";
import { dispatchAppEvent } from "../../lib/appEvents";
import { FAILURE_KEYS } from "../settings/PublishSection";

/** The Linetta trial's setup, in the settings pane and in the agent panel.
 *
 *  Two steps, in order: connect this device to Linetta Works (the same
 *  connection publishing uses, with the same disclosure), then agree that the
 *  manuscript goes through Linetta Works to the model it serves. The trial has
 *  no key, no model and no connection test: the site picks the model, and a
 *  test request would spend the writer's tokens.
 *
 *  In the settings pane ("settings") the trial is already the selected
 *  provider and the consent box saves as it is ticked, like any provider's.
 *  In the agent panel ("panel") nothing changes until "Start the trial", which
 *  saves consent and makes the trial the active provider in one write; Cancel
 *  before that leaves the active provider as it was. */
export function TrialSetup({
  mode,
  consented = false,
  onConsentChange,
  onConnectionChange,
  onStarted,
  onCancel,
}: {
  mode: "settings" | "panel";
  /** settings: the trial row's stored consent. */
  consented?: boolean;
  /** settings: saves the consent box. */
  onConsentChange?: (consented: boolean) => Promise<void> | void;
  /** The Works connection came or went: the provider rows changed. */
  onConnectionChange?: () => void;
  /** panel: the trial is now the active provider, consented to. */
  onStarted?: () => void;
  /** panel: the writer stepped back. */
  onCancel?: () => void;
}) {
  const { t } = useI18n();
  const works = useWorksConnect();
  const connected = works.status?.state === "connected";
  const { status: trial } = useTrialStatus(connected);
  const [publishConsent, setPublishConsent] = useState(false);
  const [trialConsent, setTrialConsent] = useState(false);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const state = works.status?.state ?? "disconnected";
  // The first answer is where things stood when this opened, not a change.
  const seenRef = useRef<string | null>(null);
  const known = works.status?.state ?? null;
  useEffect(() => {
    if (known === null) return;
    const before = seenRef.current;
    seenRef.current = known;
    if (before !== null && before !== known && (known === "connected" || before === "connected")) {
      onConnectionChange?.();
    }
    // onConnectionChange is the caller's; only the state matters here.
  }, [known]); // eslint-disable-line react-hooks/exhaustive-deps

  const busy = works.busy || starting;

  const start = async () => {
    setStarting(true);
    setError(null);
    try {
      const saved = await settingsApi.set({
        provider: "linetta-trial",
        providers: { "linetta-trial": { consented_at: Date.now() } },
      });
      dispatchAppEvent("linetta:settings-updated", saved);
      onStarted?.();
    } catch (e) {
      setError(e);
    } finally {
      setStarting(false);
    }
  };

  const cancel = async () => {
    // A connection still waiting for the browser is abandoned; one already
    // made stays, as it would in Settings → Publishing.
    if (state === "waiting") await works.disconnect();
    onCancel?.();
  };

  const usable = trial !== null && (trial.state === "available" || trial.state === "not_started");

  return (
    <div className="trial-setup" data-testid={`trial-setup-${mode}`}>
      {state !== "connected" ? (
        <>
          <p className="sd" data-testid="trial-intro">{t("trial.intro")}</p>
          {state === "waiting" && works.status ? (
            <div className="modal-field" data-testid="trial-waiting">
              <p className="sd">{t("settings.publish.waiting")}</p>
              <p className="sk publish-confirm-code" aria-label={t("settings.publish.confirmCode")}>
                {works.status.confirm_code}
              </p>
              {works.status.verify_url ? (
                <div className="field-actions">
                  <button
                    type="button"
                    className="btn ghost sm"
                    disabled={busy}
                    onClick={() => void works.guard(() => openExternalUrl(works.status?.verify_url ?? ""))}
                  >
                    {t("settings.publish.openAgain")}
                  </button>
                </div>
              ) : null}
            </div>
          ) : (
            <>
              {state === "failed" && works.status?.failure_reason ? (
                <p className="sd" role="alert" data-testid="trial-connect-failed">
                  {t(FAILURE_KEYS[works.status.failure_reason] ?? "settings.publish.failed.failed")}
                </p>
              ) : null}
              <p className="sd">{t("trial.connectNote")}</p>
              <p className="sd">{t("settings.publish.disclosure")}</p>
              <label className="modal-field modal-check">
                <input
                  type="checkbox"
                  checked={publishConsent}
                  disabled={busy}
                  onChange={(e) => setPublishConsent(e.target.checked)}
                  data-testid="trial-publish-consent"
                />
                {t("settings.publish.consent")}
              </label>
              <div className="field-actions">
                <button
                  type="button"
                  className="btn sm"
                  disabled={busy || !publishConsent}
                  onClick={() => void works.start(publishConsent)}
                  data-testid="trial-connect"
                >
                  {t("trial.connect")}
                </button>
              </div>
            </>
          )}
        </>
      ) : (
        <>
          <TrialStanding status={trial} />
          {mode === "settings" ? (
            <label className="modal-field modal-check">
              <input
                type="checkbox"
                checked={consented}
                disabled={busy}
                onChange={(e) => void onConsentChange?.(e.target.checked)}
                data-testid="trial-consent"
              />
              {t("trial.consent")}
            </label>
          ) : usable ? (
            <label className="modal-field modal-check">
              <input
                type="checkbox"
                checked={trialConsent}
                disabled={busy}
                onChange={(e) => setTrialConsent(e.target.checked)}
                data-testid="trial-consent"
              />
              {t("trial.consent")}
            </label>
          ) : null}
        </>
      )}

      {mode === "panel" ? (
        <div className="field-actions">
          {connected && usable ? (
            <button
              type="button"
              className="btn sm"
              disabled={busy || !trialConsent}
              onClick={() => void start()}
              data-testid="trial-start"
            >
              {t("trial.start")}
            </button>
          ) : null}
          <button
            type="button"
            className="btn ghost sm"
            disabled={starting}
            onClick={() => void cancel()}
            data-testid="trial-cancel"
          >
            {t("trial.cancel")}
          </button>
        </div>
      ) : null}

      {works.error || error ? (
        <p className="sd" role="alert" data-testid="trial-error">
          {rpcErrorMessage(error ?? works.error, t)}
        </p>
      ) : null}
    </div>
  );
}

/** One line on how much of the trial is left, or why there is none. */
export function TrialStanding({ status }: { status: TrialStatus | null }) {
  const { t } = useI18n();
  if (!status) return null;
  const n = (v: number) => v.toLocaleString();
  switch (status.state) {
    case "available":
      return (
        <p className="sd" data-testid="trial-standing" data-state="available">
          {t("trial.remaining", { remaining: n(status.remaining_tokens), granted: n(status.granted_tokens) })}
        </p>
      );
    case "not_started":
      return (
        <p className="sd" data-testid="trial-standing" data-state="not_started">
          {t("trial.notStarted", { granted: n(status.granted_tokens) })}
        </p>
      );
    case "exhausted":
      return (
        <p className="sd" role="status" data-testid="trial-standing" data-state="exhausted">
          {t("trial.exhausted")}
        </p>
      );
    case "not_connected":
      return null;
    default:
      return (
        <p className="sd" role="status" data-testid="trial-standing" data-state={status.state}>
          {t("trial.unavailable")}
        </p>
      );
  }
}
