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

/** The i18n key naming one provider, for interpolation into a sentence that
 *  has to say which company it means — see the consent checkbox below. */
const nameKeyFor = (id: ProviderID): MessageKey =>
  PROVIDER_ORDER.find((p) => p.id === id)!.labelKey;

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
  const [modelDraft, setModelDraft] = useState("");
  // The list is a convenience layered on top of the input, never the input's
  // source of truth: it only exists once a key is stored, and a new model
  // announced today is usable before this list ever hears about it. Its own
  // failure is kept separate from `error` — see loadModels below.
  const [models, setModels] = useState<string[]>([]);
  const [modelsError, setModelsError] = useState<unknown>(null);
  // providers.test's own result, kept apart from `error` for the same reason
  // as the model list: a failed connection test is information about the
  // provider, not a broken pane. Reset below whenever the provider changes,
  // since a passing result belongs to the provider it was run against.
  const [testResult, setTestResult] = useState<"ok" | null>(null);
  const [testError, setTestError] = useState<unknown>(null);
  // The poll handle, so a second click cannot start a second loop and so the
  // interval dies with the component. A login the writer abandons must not
  // leave a timer calling the engine forever.
  const pollRef = useRef<number | null>(null);
  // Which login attempt a poll tick belongs to. Every startCodexLogin() bumps
  // this and captures the new value for the interval it creates; leaving
  // "openai-codex" bumps it too, so every poll alive at that moment is
  // invalidated for good. A tick writes only if the generation it captured is
  // still current.
  //
  // A single shared boolean cannot express this. clearInterval stops future
  // ticks but cannot cancel a login_status() call already in flight, so an
  // abandoned attempt's tick can still resolve after the writer has come back
  // and started a retry — and by then a shared latch reads true again,
  // because the retry set it. The stale payload would go through, and a
  // terminal login_failed would then stop the retry's own poll. A captured
  // generation can never match a later attempt's, so it is simply dropped.
  const pollGenRef = useRef(0);

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
    setModelDraft(list.find((r) => r.id === active)?.model ?? "");
    // A model list belongs to the provider it was fetched for. Carrying
    // Anthropic's list into an OpenAI-compatible screen would offer models
    // that provider does not serve, so a provider change drops it — the
    // writer asks for it again with the refresh button.
    setModels([]);
    setModelsError(null);
  }, [active, list]);

  // A passing test result belongs to the provider it was run against *and*
  // to the credentials and consent it was run under. A green tick left over
  // from Anthropic sitting on the Gemini screen is a lie; so is one still
  // reading "Connected" after the writer has unticked consent or cleared the
  // key, because the connection it describes can no longer be made.
  //
  // The dependencies are the three booleans-and-an-id themselves, never
  // `list`: refresh() hands back a fresh array on every background reload
  // (a model save, an abandoned Codex login's poll tick), and keying on that
  // identity would make a passing tick vanish for no reason the writer can
  // see. Primitives only change when the fact they encode changes.
  const consented = Boolean(current?.consented);
  const configured = Boolean(current?.configured);
  useEffect(() => {
    setTestResult(null);
    setTestError(null);
  }, [active, consented, configured]);

  // codex.login_status is the only thing that knows whether an account is
  // signed in — providers.list says "configured", not who. Asking for it only
  // from inside the poll meant opening settings with an account already
  // signed in showed "Sign in with ChatGPT", and the logout button could not
  // be reached without a whole fresh OAuth round trip.
  useEffect(() => {
    if (active !== "openai-codex") {
      // Leaving the provider drops the status, so a stale login_failed does
      // not greet the writer on the way back in. Bumping the generation also
      // permanently invalidates whatever poll is still running: that poll
      // belongs to a login attempt the writer just walked away from, and a
      // fresh fetch below is what the next visit to Codex will trust instead.
      pollGenRef.current += 1;
      setCodexStatus(null);
      return;
    }
    let cancelled = false;
    void codexApi
      .loginStatus()
      .then((s) => {
        if (!cancelled) setCodexStatus(s);
      })
      .catch((e: unknown) => {
        // An unreachable engine is a signal, not silence: without this a
        // writer opening Settings on Codex with the engine down sees a
        // sign-in button and nothing else explaining why it never responds.
        if (!cancelled) setError(e);
      });
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
      // This poll's writes are valid only while it is still the newest
      // attempt — see pollGenRef's declaration.
      pollGenRef.current += 1;
      const gen = pollGenRef.current;
      pollRef.current = window.setInterval(() => {
        void codexApi
          .loginStatus()
          .then((s) => {
            // The writer may have left, come back, and even started a fresh
            // attempt before this tick resolves; `active` would read
            // "openai-codex" again by then, and a shared flag would read
            // valid again. Only the generation this interval captured tells
            // this tick's attempt apart from the one now running. Skipping
            // leaves the newer truth — a retry's poll, or the mount fetch's
            // result — standing instead of clobbering it with a stale one.
            if (gen !== pollGenRef.current) return;
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
            // The same gate on the failure path: a rejection belonging to an
            // abandoned attempt must not blame the attempt the writer is
            // actually waiting on, nor kill its poll.
            if (gen !== pollGenRef.current) return;
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

  // list_models requires Configured() but not Consented() — a model name
  // carries no manuscript text, so a writer can browse it before ticking the
  // consent box. The refresh button gates on the same credential check.
  const loadModels = () =>
    guard(async () => {
      setModelsError(null);
      try {
        const { models: rows } = await providersApi.listModels(active);
        setModels(rows);
      } catch (e) {
        // A model list that will not load is an inconvenience, not a failure
        // of the pane: the writer can still type a model name they already
        // know. So this lands on its own line and never in `error`, which
        // drives the section-level alert that would otherwise lock the pane.
        setModelsError(e);
        setModels([]);
      }
    });

  // An empty string is meaningful — "the provider's own default" — not a
  // skip. #91 chose that over shipping a model catalogue that ages the day
  // a provider ships a new one.
  const saveModel = () =>
    guard(async () => {
      await settingsApi.set({ providers: { [active]: { model: modelDraft.trim() } } });
      await refresh();
    });

  // The one decision this whole pane exists to make legible: from here on,
  // scenes the writer asks the built-in agent about leave this machine and
  // reach the company behind `active`. Consent lives in
  // providers[id].consented_at — a plain epoch-ms int64 the engine overwrites
  // wholesale, where 0 means none. It is NOT the ai_data_sharing_consent_*
  // pair elsewhere in settings: that field is dead, read by nothing, and
  // writing it would leave this checkbox looking like it works while the
  // engine never sees consent.
  const saveConsent = (consented: boolean) =>
    guard(async () => {
      await settingsApi.set({
        providers: { [active]: { consented_at: consented ? Date.now() : 0 } },
      });
      await refresh();
    });

  // Who the consent sentence names. For the three fixed providers that is the
  // company behind the id. For `openai` the id names a *protocol*, not a
  // destination: "sent to OpenAI-compatible" is both broken prose and silent
  // about where the scenes actually go, which is wherever base_url points —
  // OpenRouter, or a local Ollama that never leaves this machine. So name the
  // endpoint the writer configured, and fall back to describing it in prose
  // while none is set. The saved value is used, not the draft: a half-typed
  // URL must not appear in the sentence the writer is consenting to.
  const consentDestination =
    active === "openai"
      ? current?.base_url?.trim() || t("settings.providers.consent.customEndpoint")
      : t(nameKeyFor(active));

  // providers.test goes through Source.Client(), which requires Configured()
  // AND Consented() on the engine side — this is a server contract, not a UX
  // preference. So the button below is disabled until both are true; there
  // is no way to "just check the key works" ahead of consent.
  const runTest = () =>
    guard(async () => {
      setTestResult(null);
      setTestError(null);
      try {
        await providersApi.test(active);
        setTestResult("ok");
      } catch (e) {
        // Same reasoning as the model list: a failed test is information
        // about the provider, not a broken pane, so it lands on its own line
        // rather than in `error`.
        setTestError(e);
      }
    });

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

      <div className="modal-field" data-testid="provider-model">
        <label htmlFor="provider-model-input">{t("settings.providers.model")}</label>
        <input
          id="provider-model-input"
          list="provider-model-list"
          value={modelDraft}
          placeholder={t("settings.providers.model.default")}
          disabled={busy}
          onChange={(e) => setModelDraft(e.target.value)}
          onBlur={() => void saveModel()}
          data-testid="provider-model-input"
        />
        <datalist id="provider-model-list">
          {models.map((m) => (
            <option key={m} value={m} />
          ))}
        </datalist>
        <button
          type="button"
          disabled={busy || !current?.configured}
          onClick={() => void loadModels()}
          data-testid="provider-model-refresh"
        >
          {t("settings.providers.model.refresh")}
        </button>
        {modelsError ? (
          <p className="sd" data-testid="provider-model-error">
            {rpcErrorMessage(modelsError, t)}
          </p>
        ) : null}
      </div>

      <label className="modal-field">
        <input
          type="checkbox"
          checked={consented}
          disabled={busy}
          onChange={(e) => void saveConsent(e.target.checked)}
          data-testid="provider-consent"
        />
        {t("settings.providers.consent", { provider: consentDestination })}
      </label>

      <div className="modal-field">
        <button
          type="button"
          disabled={busy || !consented || !configured}
          onClick={() => void runTest()}
          data-testid="provider-test"
        >
          {t("settings.providers.test")}
        </button>
        {testResult === "ok" ? (
          <span data-testid="provider-test-ok">{t("settings.providers.test.ok")}</span>
        ) : null}
        {testError ? (
          <span role="alert" data-testid="provider-test-error">
            {rpcErrorMessage(testError, t)}
          </span>
        ) : null}
      </div>

      {error ? (
        <p className="sd" role="alert" data-testid="provider-error">
          {rpcErrorMessage(error, t)}
        </p>
      ) : null}
    </section>
  );
}
