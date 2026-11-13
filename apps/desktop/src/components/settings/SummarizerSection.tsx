import { useEffect, useState } from "react";

import { useI18n } from "../../lib/i18n";
import { settings as settingsApi, summarizer as summarizerApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { SummarizerUsage } from "../../lib/types";

/** AI summaries (#135): the writer's own provider summarizes long scenes after
 *  they are saved and can rewrite the synopsis. Off until switched on here,
 *  and the provider's consent still decides whether anything goes. The usage
 *  line reads like the agent panel's token line, summed over every call. */
export function SummarizerSection() {
  const { t } = useI18n();
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [usage, setUsage] = useState<SummarizerUsage | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    settingsApi
      .get()
      .then((s) => {
        if (!cancelled) setEnabled(s.summarizer_provider_enabled === true);
      })
      .catch(() => {});
    summarizerApi
      .usage()
      .then((u) => {
        if (!cancelled) setUsage(u);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  if (enabled === null) return null;

  const toggle = async () => {
    setBusy(true);
    setError(null);
    try {
      const next = await settingsApi.set({ summarizer_provider_enabled: !enabled });
      setEnabled(next.summarizer_provider_enabled === true);
    } catch (err) {
      setError(rpcErrorMessage(err, t));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-section" data-testid="summarizer-section">
      <h3>{t("settings.summarizer.title")}</h3>
      <button
        type="button"
        className="set-row set-row-btn"
        role="switch"
        aria-checked={enabled}
        data-testid="summarizer-toggle"
        onClick={() => void toggle()}
        disabled={busy}
      >
        <span className="sk-wrap">
          <span className="sk">{t("settings.summarizer.enabled")}</span>
          <span className="sd">{t("settings.summarizer.description")}</span>
        </span>
        <span className={`switch${enabled ? " on" : ""}`} />
      </button>
      {usage && usage.calls > 0 && (
        <p className="sd" data-testid="summarizer-usage">
          {t("settings.summarizer.usage", { calls: usage.calls, input: usage.input, output: usage.output })}
        </p>
      )}
      {error && (
        <p className="sd" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
