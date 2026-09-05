import { useCallback, useEffect, useReducer, useRef, useState } from "react";

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

/* ---------------------------------------------------------------------------
 * The server-derived state, and the one reducer that owns it.
 *
 * Everything below this line answers a single question that used to be
 * answered per-field, differently each time: *which snapshot of server state
 * is this piece of local state valid against?* A ref guarded the drafts, a
 * signature compared at render time guarded the test badge, and `list` itself
 * was written from five call sites with no ordering discipline at all. Each
 * round of fixes closed one of those and opened another, because the three
 * were never the same mechanism.
 *
 * They are now. `list`, `active`, the drafts, the credential epoch and the
 * test outcome move together, under one reducer, and every transition ends in
 * settle() — which reseeds the drafts when the provider they belong to is no
 * longer the active one, and *discards* a test result whose configuration no
 * longer matches. Two whole classes of bug stop being representable:
 *
 *   - A providers.list response that is older than one already applied cannot
 *     land, because responses carry the sequence number of the refresh that
 *     asked for them and the reducer keeps the newest. Two overlapping
 *     settings.set + refresh pairs can no longer un-tick a box.
 *   - A test result cannot be orphaned by a refresh landing between the click
 *     and the render, and cannot be resurrected by leaving a provider and
 *     coming back, because staleness is decided once — at the moment state
 *     changes — and a stale result is thrown away rather than hidden.
 * ------------------------------------------------------------------------ */

/** The three input drafts, tagged with the provider they were seeded from.
 *
 *  The tag is what retires them: the writer *moving to another provider*, not
 *  a providers.list arriving. The two are easy to confuse and are not the
 *  same — every background reload parses fresh JSON and hands back a new array
 *  identity, so keying the reseed on the list threw away whatever the writer
 *  had typed and not yet saved, silently and with no error.
 *
 *  `provider` starts null rather than at the active id. The first render
 *  happens before providers.list has resolved, with `active` still its initial
 *  "openai-codex" — the engine's default and ActiveProvider()'s fallback, so
 *  also the value it most often reports back. Claiming the drafts for that
 *  provider on the empty first pass meant the pass that finally had rows found
 *  the tag already matching and skipped the seed: the pane told a writer with a
 *  saved model that no model was configured, and the next blur wrote that
 *  emptiness back. Rows are what open the gate — see settle(). */
interface Drafts {
  provider: ProviderID | null;
  /** The stored key never comes back from settings.get — only whether one is
   *  set. So this starts empty and is the only source of what gets saved;
   *  nothing pre-fills it with a secret. */
  key: string;
  baseUrl: string;
  model: string;
}

/** providers.test's own result, kept apart from the section-level `error` for
 *  the same reason as the model list: a failed connection test is information
 *  about the provider, not a broken pane. `signature` is the configuration the
 *  writer saw when they clicked — see signatureOf. */
interface TestOutcome {
  signature: string;
  ok: boolean;
  error: unknown;
}

interface ServerState {
  /** The sequence number of the providers.list response currently in `list`.
   *  A response numbered no higher than this one is a straggler and is
   *  dropped: `list` only ever moves forward. */
  applied: number;
  list: ProviderStatus[];
  active: ProviderID;
  /** Bumped only by this pane's own saveKey/clearKey. `configured` only flips
   *  on a bare set/clear transition — rotating to a different key while a key
   *  is already stored leaves it true on both sides, and no field on
   *  ProviderStatus says otherwise, because settings.get returns only
   *  `api_key_set` and never the key itself. This counter is the honest
   *  substitute: it moves at the one place a key change is known to have
   *  happened. It still cannot see a key changed some other way (hand-editing
   *  the settings file, a second window) while this pane stays mounted; there
   *  is no observable on ProviderStatus that would close that gap either. */
  credentialEpoch: number;
  drafts: Drafts;
  test: TestOutcome | null;
}

type ServerAction =
  | { type: "listed"; seq: number; rows: ProviderStatus[] }
  | { type: "chose"; id: ProviderID }
  | { type: "credentialChanged" }
  | { type: "typed"; field: "key" | "baseUrl" | "model"; value: string }
  | { type: "testStarted" }
  | { type: "testSettled"; signature: string; ok: boolean; error: unknown };

const INITIAL_STATE: ServerState = {
  applied: 0,
  list: [],
  active: "openai-codex",
  credentialEpoch: 0,
  drafts: { provider: null, key: "", baseUrl: "", model: "" },
  test: null,
};

const activeRow = (s: ServerState): ProviderStatus | undefined =>
  s.list.find((r) => r.id === s.active);

/** Everything a passing test result was earned under, as one string.
 *
 *  A green tick left over from Anthropic sitting on the Gemini screen is a
 *  lie; so is one still reading "Connected" after the writer has unticked
 *  consent, cleared or rotated the key, pointed the model at one the provider
 *  does not serve, or — for `openai` — repointed base_url at a different
 *  server, because the connection it describes can no longer be made (or was
 *  never made against what the screen now names).
 *
 *  This used to be an effect that cleared the result, keyed on the same
 *  values; the difference matters because that dependency array had to be
 *  complete and nothing said when it was not. It was extended twice and missed
 *  `model` both times. A field forgotten here instead shows up in one place,
 *  next to the values it is supposed to stand beside.
 *
 *  base_url and model are read from the *drafts*, not from the row. The drafts
 *  are what the writer can see, and they are also what the writer's most
 *  recent blur-save is on its way to store — so a save still in flight when
 *  the test is clicked does not change this signature when its reload lands,
 *  and the result the writer asked for is still theirs when it arrives.
 *  Reading the row instead made the pane's whole first-run gesture — type a
 *  model, click Test — send a real request to the provider and render nothing,
 *  swallowing a genuine auth failure along with it.
 *
 *  NUL-joined so no value can spell out another one's boundary: a base_url and
 *  a model cannot collide into the same signature by containing the
 *  separator. */
function signatureOf(s: ServerState): string {
  const row = activeRow(s);
  return [
    s.active,
    String(Boolean(row?.consented)),
    String(Boolean(row?.configured)),
    s.drafts.baseUrl.trim(),
    s.drafts.model.trim(),
    String(s.credentialEpoch),
  ].join("\u0000");
}

/** The end of every transition: reseed drafts that no longer belong to the
 *  active provider, then throw away a test result the new state has invalidated.
 *
 *  Discarded, not hidden. A result merely hidden while the signature differs
 *  comes back the moment the signature does — leaving a provider and returning,
 *  or unticking consent and reticking it, resurrected a "Connected" badge that
 *  no test run stands behind. */
function settle(s: ServerState): ServerState {
  let next = s;
  const row = activeRow(next);
  if (row && next.drafts.provider !== next.active) {
    next = {
      ...next,
      drafts: {
        provider: next.active,
        key: "",
        baseUrl: row.base_url ?? "",
        model: row.model ?? "",
      },
    };
  }
  if (next.test && next.test.signature !== signatureOf(next)) next = { ...next, test: null };
  return next;
}

function reduceServerState(s: ServerState, a: ServerAction): ServerState {
  switch (a.type) {
    case "listed": {
      // The whole of NEW-2's fix. Two settings.set + refresh pairs overlap
      // whenever a background blur-save is still travelling as the writer
      // clicks something else; without this, whichever providers.list happened
      // to resolve last won, and a pre-write snapshot could un-tick consent,
      // flip `configured` false and disable the test button, or revert
      // base_url and hide a badge that had just been earned.
      if (a.seq <= s.applied) return s;
      const chosen = a.rows.find((r) => r.active);
      return settle({ ...s, applied: a.seq, list: a.rows, active: chosen ? chosen.id : s.active });
    }
    case "chose":
      return settle({ ...s, active: a.id });
    case "credentialChanged":
      return settle({ ...s, credentialEpoch: s.credentialEpoch + 1 });
    case "typed":
      return settle({ ...s, drafts: { ...s.drafts, [a.field]: a.value } });
    // A stale result must not sit next to the one replacing it, in either
    // direction, so the run clears before it starts.
    case "testStarted":
      return { ...s, test: null };
    case "testSettled":
      return settle({ ...s, test: { signature: a.signature, ok: a.ok, error: a.error } });
  }
}

export function ProviderSection() {
  const { t } = useI18n();
  const [state, dispatch] = useReducer(reduceServerState, INITIAL_STATE);
  const { list, active, drafts, test: shownTest } = state;
  // The raw failure is kept and translated at render time: a reason code has
  // no language of its own, and switching language should redraw the message
  // rather than leave a stale sentence on screen.
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [codexStatus, setCodexStatus] = useState<CodexStatus | null>(null);
  // The list is a convenience layered on top of the input, never the input's
  // source of truth: it only exists once a key is stored, and a new model
  // announced today is usable before this list ever hears about it. Its own
  // failure is kept separate from `error` — see loadModels below. It is not
  // server state in the sense above: nothing in providers.list describes it,
  // and it is fetched only when the writer asks.
  const [models, setModels] = useState<string[]>([]);
  const [modelsError, setModelsError] = useState<unknown>(null);
  // The number of the newest providers.list issued. A ref rather than reducer
  // state because it is not rendered and because refresh() has to read the
  // number it just claimed, synchronously, before it awaits.
  const listSeqRef = useRef(0);
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

  // The one way `list` ever moves. Every caller goes through here, and the
  // number is claimed before the await, so responses are ordered by when they
  // were *asked for* rather than by when the socket happened to answer.
  const refresh = useCallback(async () => {
    listSeqRef.current += 1;
    const seq = listSeqRef.current;
    const rows = await providersApi.list();
    dispatch({ type: "listed", seq, rows });
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

  // A blur-save is not a click on a button: nobody is waiting on it, and it
  // must not disable the control the writer is on their way to. Blur and
  // click are separate event batches, so React re-renders in between — a
  // blur handler that raised the shared `busy` flag made every other control
  // `disabled` before the click that caused the blur landed, and the click
  // was simply dropped. Typing a model and clicking "Test connection", or an
  // endpoint and then the consent box, are the two most natural gestures in
  // this pane, and both went nowhere: nothing incorrect was recorded, the
  // control just looked dead. So these saves report failure the same way but
  // leave the pane usable while they travel.
  const saveInBackground = async (fn: () => Promise<void>) => {
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e);
    }
  };

  const current = list.find((r) => r.id === active);

  // Choosing a provider is itself a saved setting, not a local tab. The agent
  // reads settings.provider at the start of every turn, so a writer who picks
  // Anthropic here and opens the panel gets Anthropic without a save button.
  const choose = (id: ProviderID) =>
    guard(async () => {
      await settingsApi.set({ provider: id });
      // The drafts follow inside the reducer: `chose` ends in settle(), which
      // reseeds them from the new provider's row. Carrying an openai base URL
      // into an anthropic patch is not a cosmetic bug — the engine rejects
      // base_url on any id but openai, and settings.set is all-or-nothing, so
      // the whole patch, including the key the writer just typed, is refused.
      dispatch({ type: "chose", id });
      await refresh();
    });

  // A model list belongs to the provider it was fetched for. Carrying
  // Anthropic's list into an OpenAI-compatible screen would offer models that
  // provider does not serve, so a provider change drops it — the writer asks
  // for it again with the refresh button. Keyed on `active` alone: a
  // background providers.list must not empty a datalist the writer just
  // fetched.
  useEffect(() => {
    setModels([]);
    setModelsError(null);
  }, [active]);

  // What the rest of the pane reads off the active row. `baseUrl` and `model`
  // are the *stored* values, used for the blur no-op guards below and for the
  // consent sentence; what the writer sees is drafts.baseUrl / drafts.model.
  const consented = Boolean(current?.consented);
  const configured = Boolean(current?.configured);
  const baseUrl = current?.base_url ?? "";
  const model = current?.model ?? "";
  // The configuration on screen right now, which runTest stamps onto the
  // result it stores. The same function the reducer settles against, so the
  // badge is shown exactly while the two agree — there is no second definition
  // to fall out of step with the first.
  const testSignature = signatureOf(state);

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
      //
      // stopPolling() itself matters too, not just the generation bump:
      // without it the interval keeps calling codex.login_status every 1.5s
      // for as long as this component stays mounted, even while the writer
      // works on an unrelated provider. The gen check already keeps that
      // call from writing anything, so this was waste rather than
      // incorrectness — but a live call to the engine forever is still worth
      // retiring outright.
      stopPolling();
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
  }, [active, stopPolling]);

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

  // A model list was fetched with a credential. Once that credential is gone
  // or replaced, the list on screen is a leftover from an account that may
  // not be the one the writer is now talking to — and after a clear there is
  // no credential behind it at all. Dropping it costs one click of the
  // refresh button to get back, and only when there is a key to get it with.
  const forgetModels = () => {
    setModels([]);
    setModelsError(null);
  };

  const saveKey = () =>
    guard(async () => {
      await settingsApi.set({ providers: { [active]: { api_key: drafts.key.trim() } } });
      dispatch({ type: "typed", field: "key", value: "" });
      forgetModels();
      // A rotated key can leave `configured` true on both sides of the
      // write, so it is this call itself — not a value read back off
      // providers.list — that tells the test signature a credential changed.
      dispatch({ type: "credentialChanged" });
      await refresh();
    });

  // An empty string is not a no-op: the engine deletes the stored secret.
  // That is the only way to clear a key, and it is why this is a separate
  // button rather than "save an empty field".
  const clearKey = () =>
    guard(async () => {
      await settingsApi.set({ providers: { [active]: { api_key: "" } } });
      dispatch({ type: "typed", field: "key", value: "" });
      forgetModels();
      dispatch({ type: "credentialChanged" });
      await refresh();
    });

  const saveBaseUrl = async () => {
    const next = drafts.baseUrl.trim();
    // A blur that changed nothing is not a save. Without this, tabbing
    // through the field costs a full settings.set + providers.list round
    // trip — and that reload is what used to eat a half-typed API key.
    if (next === baseUrl) return;
    await saveInBackground(async () => {
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
  // a provider ships a new one. Clearing a field that held a model is
  // therefore a real write, which is why the no-op check below compares
  // against what is *stored* rather than testing `next` for emptiness.
  const saveModel = async () => {
    const next = drafts.model.trim();
    // Same as the base URL: a blur that changed nothing is not a save. Tab
    // order runs through this field on the way to the consent box, so
    // without this every keyboard user reaching consent spent a settings.set
    // and a providers.list on saving the value that was already there.
    if (next === model) return;
    await saveInBackground(async () => {
      await settingsApi.set({ providers: { [active]: { model: next } } });
      await refresh();
    });
  };

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
      // The configuration the writer saw when they clicked. It is stored with
      // the result and re-checked by settle() on every later transition: if
      // anything it covers has moved by the time the result lands, the result
      // describes a configuration that is no longer on screen and is thrown
      // away rather than rendered.
      //
      // What it covers is deliberately the *drafts*, not the row — so the
      // blur-save the writer's own click interrupted, whose reload lands
      // somewhere in here, does not orphan the result they asked for.
      const signature = testSignature;
      dispatch({ type: "testStarted" });
      try {
        await providersApi.test(active);
        dispatch({ type: "testSettled", signature, ok: true, error: null });
      } catch (e) {
        // Same reasoning as the model list: a failed test is information
        // about the provider, not a broken pane, so it lands on its own line
        // rather than in `error`.
        dispatch({ type: "testSettled", signature, ok: false, error: e });
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
            // A password field in a webview is an invitation for the OS or an
            // embedded password manager to offer to save it — for a provider
            // key that is at best noise and at worst a second copy of a
            // secret the writer meant to keep in one place. The name is
            // deliberately not "password": autofill heuristics read it.
            name="provider-api-key"
            autoComplete="off"
            value={drafts.key}
            placeholder={
              current?.configured
                ? t("settings.providers.apiKey.stored")
                : t("settings.providers.apiKey.placeholder")
            }
            disabled={busy}
            onChange={(e) => dispatch({ type: "typed", field: "key", value: e.target.value })}
            data-testid="provider-key-input"
          />
          <button
            type="button"
            disabled={busy || !drafts.key.trim()}
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
            value={drafts.baseUrl}
            placeholder="https://openrouter.ai/api/v1"
            disabled={busy}
            onChange={(e) =>
              dispatch({ type: "typed", field: "baseUrl", value: e.target.value })
            }
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
          value={drafts.model}
          placeholder={t("settings.providers.model.default")}
          disabled={busy}
          onChange={(e) => dispatch({ type: "typed", field: "model", value: e.target.value })}
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
        {/* role="status", not a bare span: a screen reader announced the
            failure and said nothing at all about the success, so the one
            outcome that means "you can start writing" was the one nobody
            heard. `status` is polite — it waits for a pause rather than
            interrupting, which suits a result the writer just asked for. */}
        {shownTest?.ok ? (
          <span role="status" data-testid="provider-test-ok">
            {t("settings.providers.test.ok")}
          </span>
        ) : null}
        {shownTest?.error ? (
          <span role="alert" data-testid="provider-test-error">
            {rpcErrorMessage(shownTest.error, t)}
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
