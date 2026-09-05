import { useCallback, useEffect, useRef, useState } from "react";

import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import {
  codex as codexApi,
  openExternalUrl,
  providers as providersApi,
  settings as settingsApi,
} from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { CodexStatus, ProviderID, ProviderStatus } from "../../lib/types";

/** Connect an AI provider (BYOK).
 *
 *  The pane's job is to make one decision legible: from here on, the scenes
 *  the writer asks the built-in agent about leave this machine and reach a
 *  company the writer chose. So consent is a per-provider checkbox rather
 *  than something choosing a provider implies, and it is what unlocks the
 *  connection test — because the test itself sends a prompt.
 *
 *  The four ids are fixed by the engine's whitelist. There is no catalogue
 *  and no wizard: the 1.0 onboarding flow is not coming back.
 */

/** The order the engine's providers.list returns, kept explicit so the
 *  segmented control does not silently reorder when a response is cached.
 *  labelKey is written as a literal (not a template string) so a typo fails
 *  type-checking instead of silently falling through at render time. */
const PROVIDER_ORDER: { id: ProviderID; labelKey: MessageKey }[] = [
  { id: "openai-codex", labelKey: "settings.providers.name.openai-codex" },
  { id: "anthropic", labelKey: "settings.providers.name.anthropic" },
  { id: "gemini-native", labelKey: "settings.providers.name.gemini-native" },
  { id: "openai", labelKey: "settings.providers.name.openai" },
];

export function ProviderSection() {
  const { t } = useI18n();
  const [list, setList] = useState<ProviderStatus[]>([]);
  const [active, setActive] = useState<ProviderID>("openai-codex");
  // The raw failure is kept and translated at render time: a reason code has
  // no language of its own, and switching language should redraw the message
  // rather than leave a stale sentence on screen.
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [codexStatus, setCodexStatus] = useState<CodexStatus | null>(null);
  // The stored key never comes back from settings.get — only whether one is
  // set. So these drafts start empty and are the only source of what gets
  // saved; nothing pre-fills them with a secret.
  const [keyDraft, setKeyDraft] = useState("");
  const [baseUrlDraft, setBaseUrlDraft] = useState("");
  // The poll handle, so a second click cannot start a second loop and so the
  // interval dies with the component. A login the writer abandons must not
  // leave a timer calling the engine forever.
  const pollRef = useRef<number | null>(null);

  const stopPolling = useCallback(() => {
    if (pollRef.current !== null) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }, []);

  useEffect(() => stopPolling, [stopPolling]);

  const refresh = useCallback(async () => {
    const rows = await providersApi.list();
    setList(rows);
    const chosen = rows.find((r) => r.active);
    if (chosen) setActive(chosen.id);
  }, []);

  useEffect(() => {
    void refresh().catch(setError);
  }, [refresh]);

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

  const current = list.find((r) => r.id === active);

  // Choosing a provider is itself a saved setting, not a local tab. The agent
  // reads settings.provider at the start of every turn, so a writer who picks
  // Anthropic here and opens the panel gets Anthropic without a save button.
  const choose = (id: ProviderID) =>
    guard(async () => {
      await settingsApi.set({ provider: id });
      setActive(id);
      await refresh();
    });

  // Every draft is scoped to the provider it belongs to. Carrying an openai
  // base URL into an anthropic patch is not a cosmetic bug: the engine
  // rejects base_url on any id but openai, and settings.set is all-or-
  // nothing, so the whole patch — including the key the writer just typed —
  // would be refused.
  //
  // What retires a draft is the writer *moving to another provider* — not a
  // providers.list arriving. The two are easy to confuse and are not the
  // same: refresh() parses fresh JSON, so every background reload (a save's
  // own reload, an abandoned Codex login's poll tick) hands back a new array
  // identity. Keying this effect on that identity threw away whatever the
  // writer had typed and not yet saved, silently and with no error. So the
  // provider the drafts belong to is tracked explicitly.
  const draftProvider = useRef<ProviderID | null>(null);
  useEffect(() => {
    if (draftProvider.current === active) return;
    draftProvider.current = active;
    setKeyDraft("");
    setBaseUrlDraft(list.find((r) => r.id === active)?.base_url ?? "");
  }, [active, list]);

  // codex.login_status is the only thing that knows whether an account is
  // signed in — providers.list says "configured", not who. Asking for it only
  // from inside the poll meant opening settings with an account already
  // signed in showed "Sign in with ChatGPT", and the logout button could not
  // be reached without a whole fresh OAuth round trip.
  useEffect(() => {
    if (active !== "openai-codex") {
      // Leaving the provider drops the status, so a stale login_failed does
      // not greet the writer on the way back in.
      setCodexStatus(null);
      return;
    }
    let cancelled = false;
    void codexApi
      .loginStatus()
      .then((s) => {
        if (!cancelled) setCodexStatus(s);
      })
      // Nobody is waiting on this one, unlike the poll: the sign-in button is
      // already the right thing to show, and an error banner on every
      // settings open would say nothing the writer can act on.
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [active]);

  const startCodexLogin = () =>
    guard(async () => {
      // Cleared before the first await, not after: login_start plus opening
      // the browser is most of the window the stale banner would otherwise
      // stay up for, and a login_start that rejects would leave the old
      // "sign-in failed" line sitting next to the fresh RPC error. Nothing is
      // lost — the engine's next login_start clears login_failed its side.
      setCodexStatus(null);
      const { auth_url } = await codexApi.loginStart();
      await openExternalUrl(auth_url);
      stopPolling();
      pollRef.current = window.setInterval(() => {
        void codexApi
          .loginStatus()
          .then((s) => {
            setCodexStatus(s);
            // Both outcomes end the poll. login_failed is how the engine
            // reports a failed exchange — it is a status field, not an RPC
            // error, because the failure happens after login_start already
            // returned.
            if (s.logged_in || s.login_failed) {
              stopPolling();
              void refresh().catch(setError);
            }
          })
          // A poll that dies quietly leaves the writer watching a sign-in
          // button that will never change its mind. Stop, and say why.
          .catch((e: unknown) => {
            stopPolling();
            setError(e);
          });
      }, 1500);
    });

  const logoutCodex = () =>
    guard(async () => {
      stopPolling();
      await codexApi.logout();
      setCodexStatus(null);
      await refresh();
    });

  const saveKey = () =>
    guard(async () => {
      await settingsApi.set({ providers: { [active]: { api_key: keyDraft.trim() } } });
      setKeyDraft("");
      await refresh();
    });

  // An empty string is not a no-op: the engine deletes the stored secret.
  // That is the only way to clear a key, and it is why this is a separate
  // button rather than "save an empty field".
  const clearKey = () =>
    guard(async () => {
      await settingsApi.set({ providers: { [active]: { api_key: "" } } });
      setKeyDraft("");
      await refresh();
    });

  const saveBaseUrl = async () => {
    const next = baseUrlDraft.trim();
    // A blur that changed nothing is not a save. Without this, tabbing
    // through the field costs a full settings.set + providers.list round
    // trip — and that reload is what used to eat a half-typed API key.
    if (next === (current?.base_url ?? "")) return;
    await guard(async () => {
      await settingsApi.set({ providers: { [active]: { base_url: next } } });
      await refresh();
    });
  };

  return (
    <section className="settings-section" id="provider-settings" data-testid="provider-section">
      <h3>{t("settings.providers.title")}</h3>
      <p className="sd">{t("settings.providers.description")}</p>

      <div className="modal-field" data-testid="provider-choices">
        {PROVIDER_ORDER.map(({ id, labelKey }) => (
          <button
            key={id}
            type="button"
            disabled={busy}
            aria-pressed={id === active}
            onClick={() => void choose(id)}
            data-testid={`provider-choice-${id}`}
          >
            {t(labelKey)}
          </button>
        ))}
      </div>

      {current ? (
        <p className="sd" data-testid="provider-state">
          {current.configured
            ? t("settings.providers.state.configured")
            : t("settings.providers.state.notConfigured")}
          {" · "}
          {current.consented
            ? t("settings.providers.state.consented")
            : t("settings.providers.state.notConsented")}
        </p>
      ) : null}

      {active === "openai-codex" ? (
        <div className="modal-field" data-testid="provider-codex">
          {codexStatus?.logged_in ? (
            <>
              <span data-testid="provider-codex-email">
                {/* `||`, not `??`: an id_token can carry an empty email
                    claim, and an empty span next to a Logout button reads
                    as "not signed in". */}
                {codexStatus.email || t("settings.providers.codex.signedIn")}
              </span>
              <button
                type="button"
                disabled={busy}
                onClick={() => void logoutCodex()}
                data-testid="provider-codex-logout"
              >
                {t("settings.providers.codex.logout")}
              </button>
            </>
          ) : (
            <button
              type="button"
              disabled={busy}
              onClick={() => void startCodexLogin()}
              data-testid="provider-codex-login"
            >
              {t("settings.providers.codex.login")}
            </button>
          )}
          {codexStatus?.login_failed ? (
            <p className="sd" role="alert" data-testid="provider-codex-failed">
              {t("settings.providers.codex.failed")}
            </p>
          ) : null}
        </div>
      ) : null}

      {active !== "openai-codex" ? (
        <div className="modal-field" data-testid="provider-key">
          <label htmlFor="provider-api-key">{t("settings.providers.apiKey")}</label>
          <input
            id="provider-api-key"
            type="password"
            value={keyDraft}
            placeholder={
              current?.configured
                ? t("settings.providers.apiKey.stored")
                : t("settings.providers.apiKey.placeholder")
            }
            disabled={busy}
            onChange={(e) => setKeyDraft(e.target.value)}
            data-testid="provider-key-input"
          />
          <button
            type="button"
            disabled={busy || !keyDraft.trim()}
            onClick={() => void saveKey()}
            data-testid="provider-key-save"
          >
            {t("settings.providers.apiKey.save")}
          </button>
          {current?.configured ? (
            <button
              type="button"
              disabled={busy}
              onClick={() => void clearKey()}
              data-testid="provider-key-clear"
            >
              {t("settings.providers.apiKey.clear")}
            </button>
          ) : null}
        </div>
      ) : null}

      {active === "openai" ? (
        <div className="modal-field" data-testid="provider-base-url">
          <label htmlFor="provider-base-url-input">{t("settings.providers.baseUrl")}</label>
          <input
            id="provider-base-url-input"
            type="url"
            value={baseUrlDraft}
            placeholder="https://openrouter.ai/api/v1"
            disabled={busy}
            onChange={(e) => setBaseUrlDraft(e.target.value)}
            onBlur={() => void saveBaseUrl()}
            data-testid="provider-base-url-input"
          />
          <p className="sd">{t("settings.providers.baseUrl.hint")}</p>
        </div>
      ) : null}

      {error ? (
        <p className="sd" role="alert" data-testid="provider-error">
          {rpcErrorMessage(error, t)}
        </p>
      ) : null}
    </section>
  );
}
