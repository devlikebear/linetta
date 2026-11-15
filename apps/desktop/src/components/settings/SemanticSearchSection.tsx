import { useState } from "react";
import { megabytes, useSemanticStatus } from "../../hooks/useSemanticStatus";
import { useI18n } from "../../lib/i18n";
import { rpcErrorMessage } from "../../lib/rpcMessage";

/** Settings → Writing: meaning search (#384). Switching it on downloads the
 *  model once; the index is filled in the background. Deleting the model
 *  asks first, in place. */
export function SemanticSearchSection() {
  const { t } = useI18n();
  const { status, error, enable, disable, listener } = useSemanticStatus(undefined, true);
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [failed, setFailed] = useState<unknown>(null);
  if (!status || !status.supported) return null;
  const model = status.model;
  const size = megabytes(model.total);

  const run = async (f: () => Promise<void>) => {
    setBusy(true);
    setFailed(null);
    try {
      await f();
    } catch (e) {
      setFailed(e);
    } finally {
      setBusy(false);
    }
  };

  let state: string | null = null;
  if (model.downloading) {
    const percent = model.total ? Math.floor((100 * model.received) / model.total) : 0;
    state = t("search.meaning.downloading", { percent: String(percent) });
  } else if (model.error) {
    state = t("search.meaning.downloadFailed", { error: model.error });
  } else if (model.installed) {
    state = t("settings.semantic.installed", { size });
  }

  return (
    <section className="settings-section" data-testid="semantic-search-section">
      {listener}
      <h3>{t("settings.semantic.title")}</h3>
      <button
        type="button"
        className="set-row set-row-btn"
        data-testid="semantic-enabled"
        disabled={busy}
        onClick={() => void run(() => (status.enabled ? disable(false) : enable()))}
      >
        <span className="sk-wrap">
          <span className="sk">{t("settings.semantic.title")}</span>
          <span className="sd">{t("settings.semantic.description", { size })}</span>
        </span>
        <span className={`switch${status.enabled ? " on" : ""}`} />
      </button>
      {(state || model.installed) && (
        <div className="set-row">
          <span className="sk-wrap">
            {state && <span className="sd" role="status" data-testid="semantic-state">{state}</span>}
          </span>
          {model.installed && !confirming && (
            <button type="button" className="btn ghost sm" disabled={busy} onClick={() => setConfirming(true)}>
              {t("settings.semantic.deleteModel")}
            </button>
          )}
        </div>
      )}
      {confirming && (
        <div className="set-row" role="alertdialog" aria-label={t("settings.semantic.deleteConfirm")}>
          <span className="sd">{t("settings.semantic.deleteConfirm")}</span>
          <button type="button" className="btn ghost sm" onClick={() => setConfirming(false)}>{t("common.cancel")}</button>
          <button
            type="button"
            className="btn sm"
            data-testid="semantic-delete-confirm"
            disabled={busy}
            onClick={() => void run(async () => { await disable(true); setConfirming(false); })}
          >
            {t("settings.semantic.deleteModel")}
          </button>
        </div>
      )}
      {(failed != null || error != null) && <p role="alert" className="sd">{rpcErrorMessage(failed ?? error, t)}</p>}
    </section>
  );
}
