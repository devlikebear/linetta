import { useEffect, useState } from "react";

import { useI18n } from "../../lib/i18n";
import { style as styleApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";

/** The writer's default style (#178, #181): check rules that sit under every
 *  work's own. Changing them changes every work at once — they are not a
 *  template — and a work can turn them off in its own style section. Only the
 *  writer sets them; no agent tool does. */
export function WriterStyleSection() {
  const { t } = useI18n();
  const [phrases, setPhrases] = useState<string | null>(null);
  const [maxSentence, setMaxSentence] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    let cancelled = false;
    styleApi
      .getWriterRules()
      .then((rules) => {
        if (cancelled) return;
        setPhrases(rules.avoid_phrases.join("\n"));
        setMaxSentence(rules.max_sentence_chars > 0 ? String(rules.max_sentence_chars) : "");
      })
      .catch((e) => {
        if (!cancelled) setError(e);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const save = async () => {
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      const limit = maxSentence.trim() === "" ? 0 : Math.trunc(Number(maxSentence));
      const next = await styleApi.setWriterRules({
        avoid_phrases: (phrases ?? "").split("\n").map((line) => line.trim()).filter((line) => line !== ""),
        max_sentence_chars: Number.isFinite(limit) ? limit : 0,
      });
      setPhrases(next.avoid_phrases.join("\n"));
      setMaxSentence(next.max_sentence_chars > 0 ? String(next.max_sentence_chars) : "");
      setSaved(true);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-section" data-testid="writer-style-section">
      <h3>{t("settings.writerStyle.title")}</h3>
      <p className="sd">{t("settings.writerStyle.description")}</p>
      {phrases != null && (
        <>
          <label className="set-row is-stacked">
            <span className="sk">{t("style.avoidPhrases")}</span>
            <textarea
              rows={4}
              value={phrases}
              disabled={busy}
              placeholder={t("style.avoidPhrasesPlaceholder")}
              onChange={(e) => {
                setPhrases(e.target.value);
                setSaved(false);
              }}
            />
          </label>
          <label className="set-row is-stacked">
            <span className="sk">{t("style.maxSentence")}</span>
            <input
              type="number"
              min={0}
              max={2000}
              inputMode="numeric"
              value={maxSentence}
              disabled={busy}
              placeholder={t("style.maxSentencePlaceholder")}
              onChange={(e) => {
                setMaxSentence(e.target.value);
                setSaved(false);
              }}
            />
          </label>
          <div className="set-row">
            <button type="button" className="btn accent sm" disabled={busy} onClick={() => void save()}>
              {busy ? t("common.saving") : t("style.save")}
            </button>
            {saved && (
              <span className="sd" role="status">
                {t("settings.writerStyle.saved")}
              </span>
            )}
          </div>
        </>
      )}
      {error != null && (
        <p className="sd" role="alert">
          {rpcErrorMessage(error, t)}
        </p>
      )}
    </section>
  );
}
