import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ChevronLeft } from "lucide-react";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import {
  settings as settingsApi,
  gitSync,
  setFolderSyncDir,
  folderSyncNow,
  opsStatus as opsStatusApi,
  providers as providersApi,
  openRouter as openRouterApi,
  webSearch as webSearchApi,
  diagnostics as diagnosticsApi,
} from "../lib/rpc";
import { APP_LANGUAGES, localeForLanguage, useI18n } from "../lib/i18n";
import {
  MANUAL_PHASE_STORAGE_KEY,
  WORKSPACE_PENDING_STORAGE_KEY,
  clearStoredPhase,
  storePhase,
} from "../components/onboarding/onboardingState";
import { AISetupStart, guideForProvider, type GuideID } from "../components/ai/AISetupStart";
import "./Settings.css";
import type {
  AppLanguage,
  OpsStatus,
  OpenRouterKeyInfo,
  PlatformProfileId,
  ProviderConfig,
  ProviderID,
  Settings as SettingsRow,
  ThemePreference,
  WebSearchProvider,
} from "../lib/types";

const JOB_BACKUP = "backup.daily";
const JOB_GIT_SYNC = "git_sync";
const JOB_FOLDER_SYNC = "folder_sync";
const JOB_SUMMARIZER = "summarizer";
const JOB_COMPANION = "companion.persistence";

interface ProviderMeta {
  id: ProviderID;
  label: string;
  desc: string;
  /** "key" => API key field, "cli" => CLI path field, "oauth" => no field (uses OAuth login). */
  credential: "key" | "cli" | "oauth";
  /** Whether a custom base URL may be set (OpenAI/Anthropic-compatible endpoints). */
  endpoint?: boolean;
  legacy?: boolean;
}

type Translate = ReturnType<typeof useI18n>["t"];

function buildProviders(t: Translate): ProviderMeta[] {
  return [
    { id: "openai-codex", label: t("settings.provider.openaiCodex.label"), desc: t("settings.provider.openaiCodex.desc"), credential: "oauth" },
    { id: "openrouter", label: t("settings.provider.openrouter.label"), desc: t("settings.provider.openrouter.desc"), credential: "key" },
    { id: "openai", label: t("settings.provider.openai.label"), desc: t("settings.provider.openai.desc"), credential: "key", endpoint: true },
    { id: "anthropic", label: t("settings.provider.anthropic.label"), desc: t("settings.provider.anthropic.desc"), credential: "key", endpoint: true },
    { id: "gemini-native", label: t("settings.provider.gemini.label"), desc: t("settings.provider.gemini.desc"), credential: "key", endpoint: true },
    { id: "claude-code-cli", label: t("settings.provider.claudeCli.label"), desc: t("settings.provider.claudeCli.desc"), credential: "cli", legacy: true },
  ];
}

export function Settings() {
  const { language, setLanguage, t } = useI18n();
  const navigate = useNavigate();
  const [unavailableProviders, setUnavailableProviders] = useState<string[]>([]);
  const [gitSyncAvailable, setGitSyncAvailable] = useState(true);
  const providers = useMemo(
    () => buildProviders(t).filter((p) => !unavailableProviders.includes(p.id)),
    [t, unavailableProviders],
  );
  const [current, setCurrent] = useState<SettingsRow | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<number | null>(null);
  const [opsRows, setOpsRows] = useState<OpsStatus[]>([]);

  // Local draft for text-input fields so the cursor doesn't bounce while
  // typing. We commit to the engine on blur (or when the folder picker
  // returns), not on every keystroke.
  const [gitDirDraft, setGitDirDraft] = useState("");
  const [folderDirDraft, setFolderDirDraft] = useState("");
  const [gitTmplDraft, setGitTmplDraft] = useState("");
  const [webSearchKeyDraft, setWebSearchKeyDraft] = useState("");
  const [editorFontSizeDraft, setEditorFontSizeDraft] = useState("20");
  const [editorLineHeightDraft, setEditorLineHeightDraft] = useState("1.92");

  // Per-provider config drafts (re-synced when the active provider changes).
  const [modelDraft, setModelDraft] = useState("");
  const [apiKeyDraft, setApiKeyDraft] = useState("");
  const [baseUrlDraft, setBaseUrlDraft] = useState("");
  const [cliPathDraft, setCliPathDraft] = useState("");
  const [modelOptions, setModelOptions] = useState<string[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [modelsError, setModelsError] = useState<string | null>(null);
  const [cliDetecting, setCliDetecting] = useState(false);
  const [cliDetectMsg, setCliDetectMsg] = useState<string | null>(null);
  const [guideId, setGuideId] = useState<GuideID>("chatgpt-subscription");
  const [providerTesting, setProviderTesting] = useState(false);
  const [providerTestMsg, setProviderTestMsg] = useState<{ kind: "ok" | "error"; text: string } | null>(null);
  const [openRouterKeyInfo, setOpenRouterKeyInfo] = useState<OpenRouterKeyInfo | null>(null);
  const [openRouterKeyInfoLoading, setOpenRouterKeyInfoLoading] = useState(false);
  const [openRouterKeyInfoError, setOpenRouterKeyInfoError] = useState("");
  const [openRouterOAuthBusy, setOpenRouterOAuthBusy] = useState(false);
  const [openRouterOAuthURL, setOpenRouterOAuthURL] = useState("");
  const [openRouterOAuthError, setOpenRouterOAuthError] = useState("");
  const [webSearchTesting, setWebSearchTesting] = useState(false);
  const [webSearchTestMsg, setWebSearchTestMsg] = useState<{ kind: "ok" | "error"; text: string } | null>(null);

  useEffect(() => {
    let cancelled = false;
    Promise.all([settingsApi.get(), opsStatusApi.get(), diagnosticsApi.get()])
      .then(([s, rows, diag]) => {
        if (cancelled) return;
        setCurrent(s);
        setLanguage(s.language);
        setGitDirDraft(s.git_sync_dir);
        setFolderDirDraft(s.folder_sync_dir);
        setGitTmplDraft(s.git_sync_commit_template);
        setWebSearchKeyDraft(s.web_search_api_key ?? "");
        setEditorFontSizeDraft(String(s.editor_font_size ?? 20));
        setEditorLineHeightDraft(String(s.editor_line_height ?? 1.92));
        setOpsRows(rows);
        setUnavailableProviders(diag.unavailable_providers ?? []);
        setGitSyncAvailable(diag.git_sync_available ?? true);
      })
      .catch((e) => { if (!cancelled) setError(String(e)); });
    return () => { cancelled = true; };
  }, []);

  // Reset the per-provider drafts whenever the active provider changes (or on
  // first load) so each provider's stored config shows in the fields.
  const activeProvider = current?.provider;
  useEffect(() => {
    if (!current) return;
    const pc = current.providers?.[current.provider] ?? {};
    setGuideId(guideForProvider(current.provider));
    setModelDraft(pc.model ?? "");
    setApiKeyDraft(pc.api_key ?? "");
    setBaseUrlDraft(pc.base_url ?? "");
    setCliPathDraft(pc.cli_path ?? "");
    setModelOptions([]);
    setModelsError(null);
    setCliDetectMsg(null);
    setProviderTestMsg(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeProvider]);

  const opsByJob = useMemo(() => {
    return new Map(opsRows.map((row) => [row.job_name, row]));
  }, [opsRows]);

  const refreshOps = async () => {
    const rows = await opsStatusApi.get();
    setOpsRows(rows);
  };

  const clearOpsError = async (jobName: string) => {
    setSaving(true);
    setError(null);
    try {
      await opsStatusApi.clearError(jobName);
      await refreshOps();
      setSavedAt(Date.now());
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const apply = async (patch: Partial<SettingsRow>) => {
    if (!current) return;
    setSaving(true);
    setError(null);
    try {
      const next = await settingsApi.set(patch);
      setCurrent(next);
      setLanguage(next.language);
      setEditorFontSizeDraft(String(next.editor_font_size ?? 20));
      setEditorLineHeightDraft(String(next.editor_line_height ?? 1.92));
      window.dispatchEvent(new CustomEvent("linetta:settings-updated", { detail: next }));
      setSavedAt(Date.now());
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const applyProviderConfig = async (id: ProviderID, partial: ProviderConfig) => {
    if (!current) return;
    const existing = current.providers?.[id] ?? {};
    await apply({ providers: { [id]: { ...existing, ...partial } } } as Partial<SettingsRow>);
  };

  const detectCliPath = async (id: ProviderID) => {
    setCliDetecting(true);
    setCliDetectMsg(null);
    try {
      const { path } = await providersApi.detectCli();
      if (path) {
        setCliPathDraft(path);
        await applyProviderConfig(id, { cli_path: path });
        setCliDetectMsg(t("settings.provider.detectFound", { path }));
      } else {
        setCliDetectMsg(t("settings.provider.detectMissing"));
      }
    } catch (e) {
      setCliDetectMsg(String(e));
    } finally {
      setCliDetecting(false);
    }
  };

  const fetchModels = async (id: ProviderID) => {
    setModelsLoading(true);
    setModelsError(null);
    try {
      const res = await providersApi.listModels(id);
      setModelOptions(res.models);
    } catch (e) {
      setModelsError(String(e));
    } finally {
      setModelsLoading(false);
    }
  };

  const persistActiveProviderDrafts = async (meta: ProviderMeta) => {
    if (!current) return;
    const stored = current.providers?.[meta.id] ?? {};
    const patch: ProviderConfig = {};
    if (modelDraft !== (stored.model ?? "")) {
      patch.model = modelDraft;
    }
    if (meta.credential === "key" && apiKeyDraft !== (stored.api_key ?? "")) {
      patch.api_key = apiKeyDraft;
    }
    if (meta.endpoint && baseUrlDraft !== (stored.base_url ?? "")) {
      patch.base_url = baseUrlDraft;
    }
    if (meta.credential === "cli" && cliPathDraft !== (stored.cli_path ?? "")) {
      patch.cli_path = cliPathDraft;
    }
    if (Object.keys(patch).length > 0) {
      await applyProviderConfig(meta.id, patch);
    }
  };

  const testActiveProvider = async (meta: ProviderMeta) => {
    setProviderTesting(true);
    setProviderTestMsg(null);
    try {
      await persistActiveProviderDrafts(meta);
      const res = await providersApi.test(meta.id);
      setProviderTestMsg({ kind: "ok", text: t("settings.provider.testOk", { message: res.message }) });
    } catch (e) {
      setProviderTestMsg({ kind: "error", text: t("settings.provider.testError", { message: String(e) }) });
    } finally {
      setProviderTesting(false);
    }
  };

  const persistWebSearchKeyDraft = async () => {
    const draft = (webSearchKeyDraft ?? "").trim();
    if (!current || draft === "") return;
    const next = await settingsApi.set({ web_search_api_key: draft });
    setCurrent(next);
    setLanguage(next.language);
    setWebSearchKeyDraft(next.web_search_api_key ?? "");
    setSavedAt(Date.now());
  };

  const saveWebSearchKeyDraft = async () => {
    if (!current || (webSearchKeyDraft ?? "").trim() === "") return;
    setSaving(true);
    setError(null);
    try {
      await persistWebSearchKeyDraft();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const testWebSearchConnection = async () => {
    setWebSearchTesting(true);
    setSaving(true);
    setError(null);
    setWebSearchTestMsg(null);
    try {
      await persistWebSearchKeyDraft();
      const res = await webSearchApi.test();
      setWebSearchTestMsg({ kind: "ok", text: t("settings.tools.webSearchTestOk", { message: res.message }) });
    } catch (e) {
      setWebSearchTestMsg({ kind: "error", text: t("settings.tools.webSearchTestError", { message: String(e) }) });
    } finally {
      setSaving(false);
      setWebSearchTesting(false);
    }
  };

  const activeMeta = current ? providers.find((m) => m.id === current.provider) : undefined;
  const activeConfig = current?.providers?.[current.provider] ?? {};
  const credentialState = getCredentialState(t, activeMeta, activeConfig);
  const webSearchKeyPlaceholder = current ? getWebSearchKeyPlaceholder(t, current) : t("settings.tools.keyPlaceholder");

  const refreshOpenRouterKeyInfo = async () => {
    const cfg = current?.providers?.openrouter;
    if (!cfg?.api_key_set) {
      setOpenRouterKeyInfo(null);
      setOpenRouterKeyInfoError("");
      return;
    }
    setOpenRouterKeyInfoLoading(true);
    setOpenRouterKeyInfoError("");
    try {
      setOpenRouterKeyInfo(await openRouterApi.keyInfo());
    } catch (e) {
      setOpenRouterKeyInfo(null);
      setOpenRouterKeyInfoError(String(e));
    } finally {
      setOpenRouterKeyInfoLoading(false);
    }
  };

  const connectOpenRouterOAuth = async () => {
    setOpenRouterOAuthBusy(true);
    setOpenRouterOAuthError("");
    setOpenRouterOAuthURL("");
    setProviderTestMsg(null);
    try {
      const started = await openRouterApi.oauthStart();
      setOpenRouterOAuthURL(started.auth_url);
      window.open(started.auth_url, "_blank", "noopener,noreferrer");
      setProviderTestMsg({ kind: "ok", text: t("settings.setup.openrouter.oauthStarted") });
      const finished = await openRouterApi.oauthFinish(started.request_id);
      const next = await settingsApi.get();
      setCurrent(next);
      setLanguage(next.language);
      setGuideId("openrouter-safe");
      setProviderTestMsg({ kind: "ok", text: t("settings.provider.testOk", { message: finished.message }) });
      window.dispatchEvent(new CustomEvent("linetta:settings-updated", { detail: next }));
      setSavedAt(Date.now());
      try {
        setOpenRouterKeyInfo(await openRouterApi.keyInfo());
      } catch {
        setOpenRouterKeyInfo(null);
      }
    } catch (e) {
      const message = String(e);
      setOpenRouterOAuthError(t("settings.setup.openrouter.oauthError", { message }));
      setProviderTestMsg({ kind: "error", text: t("settings.setup.openrouter.oauthError", { message }) });
    } finally {
      setOpenRouterOAuthBusy(false);
    }
  };

  useEffect(() => {
    if (!current || (current.provider !== "openrouter" && guideId !== "openrouter-safe")) return;
    if (!current.providers?.openrouter?.api_key_set) {
      setOpenRouterKeyInfo(null);
      setOpenRouterKeyInfoError("");
      return;
    }
    void refreshOpenRouterKeyInfo();
  }, [current?.provider, current?.providers?.openrouter?.api_key_set, guideId]);

  const replayOnboardingTour = () => {
    clearStoredPhase(WORKSPACE_PENDING_STORAGE_KEY);
    storePhase(MANUAL_PHASE_STORAGE_KEY, "library");
    navigate("/");
  };

  return (
    <div className="settings">
      <div className="lib-top">
        <Link to="/" className="btn ghost sm">
          <ChevronLeft size={15} /> {t("settings.backToLibrary")}
        </Link>
        <div className="lib-brandmark">{t("settings.brand")}</div>
        <span style={{ width: 90 }} />
      </div>
      <div className="settings-inner">
        <h1>{t("settings.title")}</h1>
        {error && <p className="error">{error}</p>}
        {!current ? (
          <p className="hint">{t("common.loading")}</p>
        ) : (
          <>
            <section className="settings-section">
              <h3>{t("settings.language.title")}</h3>
              <p className="sd">{t("settings.language.description")}</p>
              <div className="modal-field">
                <label htmlFor="app-language">{t("settings.language.label")}</label>
                <select
                  id="app-language"
                  value={current.language ?? language}
                  onChange={(e) => apply({ language: e.target.value as AppLanguage })}
                  disabled={saving}
                >
                  {APP_LANGUAGES.map((lang) => (
                    <option key={lang.value} value={lang.value}>
                      {lang.nativeLabel}
                    </option>
                  ))}
                </select>
              </div>
            </section>

            <AISetupStart
              currentProvider={current.provider}
              currentProviderLabel={activeMeta?.label ?? current.provider}
              credentialState={credentialState}
              unavailableProviders={unavailableProviders}
              selectedGuideId={guideId}
              onGuideIdChange={setGuideId}
              onSelectProvider={(provider) => { void apply({ provider }); }}
              openRouterKeyInfo={openRouterKeyInfo}
              openRouterKeyInfoLoading={openRouterKeyInfoLoading}
              openRouterKeyInfoError={openRouterKeyInfoError}
              onRefreshOpenRouterKeyInfo={() => { void refreshOpenRouterKeyInfo(); }}
              onConnectOpenRouterOAuth={() => { void connectOpenRouterOAuth(); }}
              openRouterOAuthBusy={openRouterOAuthBusy}
              openRouterOAuthURL={openRouterOAuthURL}
              openRouterOAuthError={openRouterOAuthError}
              saving={saving}
            />

            <section className="settings-section">
              <h3>{t("settings.aiAdvanced.title")}</h3>
              <p className="sd">{t("settings.aiAdvanced.description")}</p>
              {providers.map((meta) => (
                <button
                  key={meta.id}
                  type="button"
                  className={`set-row set-row-btn${meta.legacy ? " is-legacy" : ""}`}
                  onClick={() => !saving && apply({ provider: meta.id })}
                  disabled={saving}
                >
                  <span className="sk-wrap">
                    <span className="sk">{meta.label}</span>
                    <span className="sd">{meta.desc}</span>
                  </span>
                  <span className={`switch${current.provider === meta.id ? " on" : ""}`} />
                </button>
              ))}
              <p className="sd">{t("settings.aiAdvanced.changeNote")}</p>

              {(() => {
                const meta = providers.find((m) => m.id === current.provider);
                if (!meta) return null;
                return (
                  <div className="provider-config">
                    {meta.credential === "key" && (
                      <div className="modal-field">
                        <label htmlFor="provider-key">{t("settings.provider.apiKey")}</label>
                        <div className="set-field-row">
                          <input
                            id="provider-key"
                            type="password"
                            value={apiKeyDraft}
                            onChange={(e) => setApiKeyDraft(e.target.value)}
                            onBlur={() => {
                              if (apiKeyDraft !== "") {
                                applyProviderConfig(meta.id, { api_key: apiKeyDraft });
                              }
                            }}
                            placeholder={activeConfig.api_key_set ? t("settings.provider.apiKeySavedPlaceholder") : t("settings.provider.apiKeyPlaceholder")}
                            autoComplete="off"
                          />
                          {activeConfig.api_key_set && (
                            <button
                              type="button"
                              className="btn ghost sm"
                              onClick={() => {
                                setApiKeyDraft("");
                                applyProviderConfig(meta.id, { clear_api_key: true });
                              }}
                              disabled={saving}
                            >
                              {t("common.deleteKey")}
                            </button>
                          )}
                        </div>
                        <p className="sd">{t("settings.provider.apiKeyHelp")}</p>
                      </div>
                    )}
                    {meta.endpoint && (
                      <div className="modal-field">
                        <label htmlFor="provider-base-url">{t("settings.provider.baseUrl")}</label>
                        <input
                          id="provider-base-url"
                          type="text"
                          value={baseUrlDraft}
                          onChange={(e) => setBaseUrlDraft(e.target.value)}
                          onBlur={() => {
                            const stored = current.providers?.[meta.id]?.base_url ?? "";
                            if (baseUrlDraft !== stored) {
                              applyProviderConfig(meta.id, { base_url: baseUrlDraft });
                            }
                          }}
                          placeholder={t("settings.provider.baseUrlPlaceholder")}
                        />
                      </div>
                    )}
                    {meta.credential === "cli" && (
                      <div className="modal-field">
                        <label htmlFor="provider-cli">{t("settings.provider.cliPath")}</label>
                        <div className="set-field-row">
                          <input
                            id="provider-cli"
                            type="text"
                            value={cliPathDraft}
                            onChange={(e) => setCliPathDraft(e.target.value)}
                            onBlur={() => {
                              const stored = current.providers?.[meta.id]?.cli_path ?? "";
                              if (cliPathDraft !== stored) {
                                applyProviderConfig(meta.id, { cli_path: cliPathDraft });
                              }
                            }}
                            placeholder={t("settings.provider.cliPathPlaceholder")}
                          />
                          <button
                            type="button"
                            className="btn ghost sm"
                            onClick={() => detectCliPath(meta.id)}
                            disabled={saving || cliDetecting}
                          >
                            {cliDetecting ? t("settings.provider.cliDetecting") : t("settings.provider.cliDetect")}
                          </button>
                        </div>
                        <p className="sd">{t("settings.provider.cliHelp")}</p>
                        {cliDetectMsg && <p className="sd">{cliDetectMsg}</p>}
                      </div>
                    )}
                    <div className="modal-field">
                      <label htmlFor="provider-model">{t("settings.provider.model")}</label>
                      <div className="set-field-row">
                        <input
                          id="provider-model"
                          type="text"
                          list="provider-model-options"
                          value={modelDraft}
                          onChange={(e) => setModelDraft(e.target.value)}
                          onBlur={() => {
                            const stored = current.providers?.[meta.id]?.model ?? "";
                            if (modelDraft !== stored) {
                              applyProviderConfig(meta.id, { model: modelDraft });
                            }
                          }}
                          placeholder={t("settings.provider.modelPlaceholder")}
                        />
                        <datalist id="provider-model-options">
                          {modelOptions.map((m) => (
                            <option key={m} value={m} />
                          ))}
                        </datalist>
                        <button
                          type="button"
                          className="btn ghost sm"
                          onClick={() => fetchModels(meta.id)}
                          disabled={saving || modelsLoading || meta.id === "claude-code-cli" || meta.credential === "oauth"}
                        >
                          {modelsLoading ? t("settings.provider.refreshingModels") : t("settings.provider.refreshModels")}
                        </button>
                      </div>
                      {meta.id === "claude-code-cli" ? (
                        <p className="sd">{t("settings.provider.cliNoModels")}</p>
                      ) : meta.credential === "oauth" ? (
                        <p className="sd">{t("settings.provider.oauthNoModels")}</p>
                      ) : (
                        <p className="sd">{t("settings.provider.modelHelp")}</p>
                      )}
                      {modelsError && <p className="error">{modelsError}</p>}
                    </div>
                    <p className="sd">{t("settings.provider.storageHelp")}</p>
                    <div className="provider-test">
                      <button
                        type="button"
                        className="btn ghost sm"
                        onClick={() => testActiveProvider(meta)}
                        disabled={providerTesting}
                      >
                        {providerTesting ? t("settings.provider.testing") : t("settings.provider.test")}
                      </button>
                      <p className="sd">{t("settings.provider.testHelp")}</p>
                      {providerTestMsg && (
                        <p className={providerTestMsg.kind === "ok" ? "provider-test-ok" : "provider-test-error"}>
                          {providerTestMsg.text}
                        </p>
                      )}
                    </div>
                  </div>
                );
              })()}
            </section>

            <section className="settings-section">
              <h3>{t("settings.writing.title")}</h3>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() => !saving && apply({ typewriter_default: !current.typewriter_default })}
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">{t("settings.writing.typewriter")}</span>
                  <span className="sd">{t("settings.writing.typewriterDescription")}</span>
                </span>
                <span className={`switch${current.typewriter_default ? " on" : ""}`} />
              </button>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() => !saving && apply({ focus_default: !current.focus_default })}
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">{t("settings.writing.focus")}</span>
                  <span className="sd">{t("settings.writing.focusDescription")}</span>
                </span>
                <span className={`switch${current.focus_default ? " on" : ""}`} />
              </button>
            </section>

            <section className="settings-section">
              <h3>{t("settings.editor.title")}</h3>
              <div className="modal-field">
                <label>{t("settings.editor.theme")}</label>
                <div className="settings-segmented" role="group" aria-label={t("settings.editor.theme")}>
                  {([
                    ["system", t("settings.editor.themeSystem")],
                    ["light", t("settings.editor.themeLight")],
                    ["dark", t("settings.editor.themeDark")],
                  ] as Array<[ThemePreference, string]>).map(([theme, label]) => (
                    <button
                      key={theme}
                      type="button"
                      className={current.theme === theme ? "is-selected" : ""}
                      onClick={() => !saving && apply({ theme })}
                      disabled={saving}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>
              <div className="settings-number-grid">
                <div className="modal-field">
                  <label htmlFor="editor-font-size">{t("settings.editor.fontSize")}</label>
                  <input
                    id="editor-font-size"
                    type="number"
                    min={15}
                    max={22}
                    step={1}
                    value={editorFontSizeDraft}
                    onChange={(e) => {
                      const raw = e.currentTarget.value;
                      setEditorFontSizeDraft(raw);
                      const value = Number(raw);
                      if (Number.isFinite(value) && value >= 15 && value <= 22) {
                        apply({ editor_font_size: value });
                      }
                    }}
                    disabled={saving}
                  />
                </div>
                <div className="modal-field">
                  <label htmlFor="editor-line-height">{t("settings.editor.lineHeight")}</label>
                  <input
                    id="editor-line-height"
                    type="number"
                    min={1.6}
                    max={2.2}
                    step={0.1}
                    value={editorLineHeightDraft}
                    onChange={(e) => {
                      const raw = e.currentTarget.value;
                      setEditorLineHeightDraft(raw);
                      const value = Number(raw);
                      if (Number.isFinite(value) && value >= 1.6 && value <= 2.2) {
                        apply({ editor_line_height: value });
                      }
                    }}
                    disabled={saving}
                  />
                </div>
              </div>
              <div className="modal-field">
                <label htmlFor="copy-profile">{t("settings.editor.copyProfile")}</label>
                <select
                  id="copy-profile"
                  value={current.copy_profile ?? "plain"}
                  onChange={(e) => apply({ copy_profile: e.target.value as PlatformProfileId })}
                  disabled={saving}
                >
                  {(["plain", "munpia", "series", "joara"] as PlatformProfileId[]).map((profile) => (
                    <option key={profile} value={profile}>
                      {t(`platformProfile.${profile}`)}
                    </option>
                  ))}
                </select>
              </div>
            </section>

            <section className="settings-section">
              <h3>{t("settings.onboarding.title")}</h3>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() => !saving && apply({ onboarding_tour_enabled: current.onboarding_tour_enabled === false })}
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">{t("settings.onboarding.enabled")}</span>
                  <span className="sd">{t("settings.onboarding.enabledDescription")}</span>
                </span>
                <span className={`switch${current.onboarding_tour_enabled !== false ? " on" : ""}`} />
              </button>
              <div className="set-row">
                <span className="sk-wrap">
                  <span className="sk">{t("settings.onboarding.replay")}</span>
                  <span className="sd">{t("settings.onboarding.replayDescription")}</span>
                </span>
                <button type="button" className="btn ghost sm" onClick={replayOnboardingTour}>
                  {t("settings.onboarding.replay")}
                </button>
              </div>
            </section>

            <section className="settings-section">
              <h3>{t("settings.tools.title")}</h3>
              <p className="sd">{t("settings.tools.description")}</p>
              <div className="modal-field">
                <label htmlFor="ws-provider">{t("settings.tools.webSearchProvider")}</label>
                <select
                  id="ws-provider"
                  value={current.web_search_provider}
                  onChange={(e) => {
                    setWebSearchTestMsg(null);
                    apply({ web_search_provider: e.target.value as WebSearchProvider });
                  }}
                  disabled={saving}
                >
                  <option value="brave">Brave Search</option>
                  <option value="perplexity">Perplexity Sonar</option>
                </select>
              </div>
              <div className="modal-field">
                <label htmlFor="ws-key">{t("settings.tools.webSearchApiKey")}</label>
                <div className="set-field-row">
                  <input
                    id="ws-key"
                    type="password"
                    value={webSearchKeyDraft ?? ""}
                    onChange={(e) => {
                      setWebSearchKeyDraft(e.target.value);
                      setWebSearchTestMsg(null);
                    }}
                    onBlur={() => {
                      if ((webSearchKeyDraft ?? "").trim() !== "") {
                        saveWebSearchKeyDraft();
                      }
                    }}
                    placeholder={webSearchKeyPlaceholder}
                    autoComplete="off"
                  />
                  {current.web_search_api_key_set && (
                    <button
                      type="button"
                      className="btn ghost sm"
                      onClick={() => {
                        setWebSearchKeyDraft("");
                        apply({ web_search_api_key: "" });
                      }}
                      disabled={saving}
                    >
                      {t("common.deleteKey")}
                    </button>
                  )}
                </div>
              </div>
              <p className="sd">{t("settings.tools.keyHelp")}</p>
              <div className="provider-test">
                <button
                  type="button"
                  className="btn ghost sm"
                  onClick={testWebSearchConnection}
                  disabled={webSearchTesting}
                >
                  {webSearchTesting ? t("settings.tools.webSearchTesting") : t("settings.tools.webSearchTest")}
                </button>
                <p className="sd">{t("settings.tools.webSearchTestHelp")}</p>
                {webSearchTestMsg && (
                  <p className={webSearchTestMsg.kind === "ok" ? "provider-test-ok" : "provider-test-error"}>
                    {webSearchTestMsg.text}
                  </p>
                )}
              </div>
            </section>

            {gitSyncAvailable && (
            <section className="settings-section">
              <h3>{t("settings.git.title")}</h3>
              <p className="sd">{t("settings.git.description")}</p>
              <div className="modal-field">
                <label htmlFor="git-dir">{t("settings.git.folder")}</label>
                <div className="set-field-row">
                  <input
                    id="git-dir"
                    type="text"
                    value={gitDirDraft}
                    onChange={(e) => setGitDirDraft(e.target.value)}
                    onBlur={() => {
                      if (gitDirDraft !== current.git_sync_dir) {
                        apply({ git_sync_dir: gitDirDraft });
                      }
                    }}
                    placeholder={t("settings.git.folderPlaceholder")}
                  />
                  <button
                    type="button"
                    className="btn ghost sm"
                    onClick={async () => {
                      const picked = await openDialog({ directory: true, multiple: false });
                      if (typeof picked === "string") {
                        setGitDirDraft(picked);
                        await apply({ git_sync_dir: picked });
                      }
                    }}
                    disabled={saving}
                  >
                    {t("settings.git.pickFolder")}
                  </button>
                </div>
              </div>
              <div className="modal-field">
                <label htmlFor="git-tmpl">{t("settings.git.commitTemplate")}</label>
                <input
                  id="git-tmpl"
                  type="text"
                  value={gitTmplDraft}
                  onChange={(e) => setGitTmplDraft(e.target.value)}
                  onBlur={() => {
                    if (gitTmplDraft !== current.git_sync_commit_template) {
                      apply({ git_sync_commit_template: gitTmplDraft });
                    }
                  }}
                  placeholder={t("settings.git.commitTemplatePlaceholder")}
                />
              </div>
              <p className="sd">{t("settings.git.commitTemplateHelp")}</p>
              <p className="sd">
                {t("settings.git.initHelp")} <code>git remote add origin &lt;URL&gt;</code>
              </p>
              <button
                type="button"
                className="btn ghost sm"
                onClick={async () => {
                  try {
                    const res = await gitSync.init();
                    if (res.skipped) {
                      setError(t("settings.git.errorNoFolder"));
                      return;
                    }
                    if (res.error) {
                      setError(t("settings.git.errorInitFailed", { error: res.error }));
                      return;
                    }
                    if (res.already_repo) {
                      setSavedAt(Date.now());
                      setError(t("settings.git.alreadyRepo"));
                      return;
                    }
                    if (res.created) {
                      setError(null);
                      setSavedAt(Date.now());
                    }
                  } catch (e) {
                    setError(String(e));
                  }
                }}
                disabled={saving}
              >
                {t("settings.git.init")}
              </button>
              <OpsStatusCard
                title={t("settings.ops.gitStatus")}
                status={opsByJob.get(JOB_GIT_SYNC)}
                okText={t("settings.ops.gitOk")}
                idleText={t("settings.ops.noRuns")}
                onClearError={() => clearOpsError(JOB_GIT_SYNC)}
                disabled={saving}
                t={t}
                language={language}
              />
            </section>
            )}

            <section className="settings-section">
              <h3>{t("settings.folder.title")}</h3>
              <p className="sd">{t("settings.folder.description")}</p>
              <div className="modal-field">
                <label htmlFor="folder-dir">{t("settings.folder.folder")}</label>
                <div className="set-field-row">
                  <input
                    id="folder-dir"
                    type="text"
                    value={folderDirDraft}
                    readOnly
                    placeholder={t("settings.folder.folderPlaceholder")}
                  />
                  <button
                    type="button"
                    className="btn ghost sm"
                    onClick={async () => {
                      const picked = await openDialog({ directory: true, multiple: false });
                      if (typeof picked === "string") {
                        try {
                          await setFolderSyncDir(picked);
                          setFolderDirDraft(picked);
                          await apply({ folder_sync_dir: picked });
                        } catch (e) {
                          setError(String(e));
                        }
                      }
                    }}
                    disabled={saving}
                  >
                    {t("settings.folder.pickFolder")}
                  </button>
                </div>
              </div>
              <label className="set-toggle">
                <input
                  type="checkbox"
                  checked={current?.folder_sync_enabled ?? false}
                  onChange={(e) => apply({ folder_sync_enabled: e.target.checked })}
                  disabled={saving}
                />
                {t("settings.folder.enable")}
              </label>
              <p className="sd">{t("settings.folder.help")}</p>
              <button
                type="button"
                className="btn ghost sm"
                onClick={async () => {
                  try {
                    const res = await folderSyncNow();
                    if (res.error) {
                      setError(res.error);
                      return;
                    }
                    setError(null);
                    setSavedAt(Date.now());
                    await refreshOps();
                  } catch (e) {
                    setError(String(e));
                  }
                }}
                disabled={saving}
              >
                {t("settings.folder.runNow")}
              </button>
              <OpsStatusCard
                title={t("settings.ops.folderStatus")}
                status={opsByJob.get(JOB_FOLDER_SYNC)}
                okText={t("settings.ops.folderOk")}
                idleText={t("settings.ops.noRuns")}
                onClearError={() => clearOpsError(JOB_FOLDER_SYNC)}
                disabled={saving}
                t={t}
                language={language}
              />
            </section>

            <section className="settings-section">
              <h3>{t("settings.backup.title")}</h3>
              <p className="sd">{t("settings.backup.description")}</p>
              <div className="set-row">
                <span className="sk-wrap"><span className="sk">{t("settings.backup.folder")}</span></span>
                <span className="mono">{current.backup_dir}</span>
              </div>
              <OpsStatusCard
                title={t("settings.ops.backupStatus")}
                status={opsByJob.get(JOB_BACKUP)}
                okText={t("settings.ops.backupOk")}
                idleText={t("settings.ops.noRuns")}
                onClearError={() => clearOpsError(JOB_BACKUP)}
                disabled={saving}
                t={t}
                language={language}
              />
            </section>

            {isDegraded(opsByJob.get(JOB_SUMMARIZER)) && (
              <section className="settings-section">
                <h3>{t("settings.ops.summarizerStatus")}</h3>
                <OpsStatusCard
                  title={t("settings.ops.summarizerRecentFailure")}
                  status={opsByJob.get(JOB_SUMMARIZER)}
                  okText={t("settings.ops.summarizerOk")}
                  idleText={t("settings.ops.noRuns")}
                  onClearError={() => clearOpsError(JOB_SUMMARIZER)}
                  disabled={saving}
                  t={t}
                  language={language}
                />
              </section>
            )}

            {isDegraded(opsByJob.get(JOB_COMPANION)) && (
              <section className="settings-section">
                <h3>{t("settings.ops.companionStatus")}</h3>
                <OpsStatusCard
                  title={t("settings.ops.companionRecentFailure")}
                  status={opsByJob.get(JOB_COMPANION)}
                  okText={t("settings.ops.companionOk")}
                  idleText={t("settings.ops.noRuns")}
                  onClearError={() => clearOpsError(JOB_COMPANION)}
                  disabled={saving}
                  t={t}
                  language={language}
                />
              </section>
            )}

            {savedAt && <p className="settings-saved">{t("settings.saved")}</p>}
          </>
        )}
      </div>
    </div>
  );
}

function isDegraded(status?: OpsStatus): boolean {
  return Boolean(status?.last_error);
}

function getCredentialState(t: Translate, meta?: ProviderMeta, cfg: ProviderConfig = {}): string {
  if (!meta) return t("settings.provider.stateNeedsConnection");
  if (meta.credential === "oauth") {
    return t("settings.provider.stateCodexLogin");
  }
  if (meta.credential === "key") {
    return cfg.api_key_set || cfg.api_key ? t("settings.provider.stateApiSaved") : t("settings.provider.stateApiNeeded");
  }
  if (meta.credential === "cli") {
    return cfg.cli_path ? t("settings.provider.stateCliSaved") : t("settings.provider.stateCliLegacy");
  }
  return t("settings.provider.stateNeedsSettings");
}

function getWebSearchKeyPlaceholder(t: Translate, current: SettingsRow): string {
  if (current.web_search_api_key_set) {
    return t("settings.tools.keySavedPlaceholder");
  }
  if (current.web_search_provider === "perplexity") {
    return "pplx-...";
  }
  return t("settings.tools.keyPlaceholder");
}

function OpsStatusCard({
  title,
  status,
  okText,
  idleText,
  onClearError,
  disabled,
  t,
  language,
}: {
  title: string;
  status?: OpsStatus;
  okText: string;
  idleText: string;
  onClearError: () => void;
  disabled: boolean;
  t: Translate;
  language: AppLanguage;
}) {
  const metadata = parseMetadata(status?.metadata_json);
  const failed = Boolean(status?.last_error);
  const body = failed ? status?.last_error : status?.last_ok ? okText : idleText;
  const finished = formatMillis(language, status?.last_finished_at);
  const metadataLabel = formatMetadata(t, metadata);

  return (
    <div className={`ops-status ${failed ? "is-error" : status?.last_ok ? "is-ok" : ""}`}>
      <div className="ops-status-head">
        <h4>{title}</h4>
        {failed && (
          <button type="button" onClick={onClearError} disabled={disabled}>
            {t("settings.ops.clearError")}
          </button>
        )}
      </div>
      <p className="ops-status-line">{body}</p>
      {finished && <p className="hint">{t("settings.ops.lastRun")}: {finished}</p>}
      {metadataLabel && <p className="hint">{metadataLabel}</p>}
    </div>
  );
}

function parseMetadata(raw?: string): Record<string, unknown> {
  if (!raw) return {};
  try {
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return {};
  }
  return {};
}

function formatMillis(language: AppLanguage, value?: number): string {
  if (!value) return "";
  return new Date(value).toLocaleString(localeForLanguage(language));
}

function formatMetadata(t: Translate, metadata: Record<string, unknown>): string {
  const parts: string[] = [];
  if (typeof metadata.files_written === "number") {
    parts.push(t(metadata.files_written === 1 ? "settings.ops.metadata.file" : "settings.ops.metadata.files", { count: metadata.files_written }));
  }
  if (metadata.committed === true) parts.push(t("settings.ops.metadata.committed"));
  if (metadata.pushed === true) parts.push(t("settings.ops.metadata.pushed"));
  if (metadata.backup_ran === true) parts.push(t("settings.ops.metadata.backupRan"));
  if (typeof metadata.failure_count === "number" && metadata.failure_count > 0) {
    parts.push(t("settings.ops.metadata.failures", { count: metadata.failure_count }));
  }
  if (typeof metadata.path === "string" && metadata.path !== "") {
    parts.push(metadata.path);
  }
  return parts.join(" · ");
}
