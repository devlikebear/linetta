import { useEffect, useState } from "react";

import { useI18n } from "../../lib/i18n";
import { backgroundPrefsGet, backgroundPrefsSet, type BackgroundPrefs } from "../../lib/rpc";

/** Background residence (#81): keep the app — and the MCP server inside its
 *  engine — alive after the window closes, and start it hidden at login.
 *
 *  Both prefs live in the shell (tray behaviour and OS autostart are shell
 *  concerns), so this pane talks to the Tauri commands, not the engine. On a
 *  build without those commands (mobile) the first call rejects and the whole
 *  section stays hidden.
 */
export function BackgroundSection() {
  const { language, t } = useI18n();
  const [prefs, setPrefs] = useState<BackgroundPrefs | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    backgroundPrefsGet()
      .then((p) => {
        if (!cancelled) setPrefs(p);
      })
      .catch(() => {
        /* not a desktop shell — leave the section hidden */
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Keep the tray menu labels in the app language.
  useEffect(() => {
    if (prefs) void backgroundPrefsSet({ language }).catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [language, prefs !== null]);

  if (!prefs) return null;

  const toggle = async (patch: { closeToTray?: boolean; autostart?: boolean }) => {
    setBusy(true);
    setError(null);
    try {
      setPrefs(await backgroundPrefsSet(patch));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-section" data-testid="background-section">
      <h3>{t("settings.background.title")}</h3>
      <p className="sd">{t("settings.background.description")}</p>
      <button
        type="button"
        className="set-row set-row-btn"
        onClick={() => !busy && void toggle({ closeToTray: !prefs.close_to_tray })}
        disabled={busy}
        data-testid="background-close-to-tray"
      >
        <span className="sk-wrap">
          <span className="sk">{t("settings.background.closeToTray")}</span>
          <span className="sd">{t("settings.background.closeToTrayDescription")}</span>
        </span>
        <span className={`switch${prefs.close_to_tray ? " on" : ""}`} />
      </button>
      <button
        type="button"
        className="set-row set-row-btn"
        onClick={() => !busy && void toggle({ autostart: !prefs.autostart })}
        disabled={busy}
        data-testid="background-autostart"
      >
        <span className="sk-wrap">
          <span className="sk">{t("settings.background.autostart")}</span>
          <span className="sd">{t("settings.background.autostartDescription")}</span>
        </span>
        <span className={`switch${prefs.autostart ? " on" : ""}`} />
      </button>
      {error && <p className="settings-error">{error}</p>}
    </section>
  );
}
