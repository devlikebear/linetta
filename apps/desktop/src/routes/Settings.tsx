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
  diagnostics as diagnosticsApi,
  exportApi,
} from "../lib/rpc";
import { rpcErrorMessage } from "../lib/rpcMessage";
import { saveExportedMarkdown } from "../lib/exportSave";
import { McpSection } from "../components/settings/McpSection";
import { RestoreSection } from "../components/settings/RestoreSection";
import { APP_LANGUAGES, localeForLanguage, useI18n } from "../lib/i18n";
import { dispatchAppEvent } from "../lib/appEvents";

type Translate = ReturnType<typeof useI18n>["t"];
import {
  MANUAL_PHASE_STORAGE_KEY,
  WORKSPACE_PENDING_STORAGE_KEY,
  clearStoredPhase,
  storePhase,
} from "../components/onboarding/onboardingState";
import "./Settings.css";
import type {
  AppLanguage,
  OpsStatus,
  PlatformProfileId,
  Settings as SettingsRow,
  PalettePreference,
  ThemePreference,
} from "../lib/types";

const JOB_BACKUP = "backup.daily";
const JOB_GIT_SYNC = "git_sync";
const JOB_FOLDER_SYNC = "folder_sync";
const JOB_SUMMARIZER = "summarizer";

export function Settings() {
  const { language, setLanguage, t } = useI18n();
  const navigate = useNavigate();
  // Whether this install ever used the built-in companion. The engine answers
  // the durable half (any companion message ever written); the consent
  // timestamp below covers someone who set AI up but never sent anything.
  // null while diagnostics is in flight — guessing either way for that moment
  // would flash the legacy block open or shut before the answer lands.
  const [companionHistoryExists, setCompanionHistoryExists] = useState<boolean | null>(null);
  const [companionExport, setCompanionExport] = useState<"idle" | "busy" | "saved">("idle");
  const [gitSyncAvailable, setGitSyncAvailable] = useState(true);
  // Hidden entirely on builds without MCP (mobile), the same way git sync is.
  const [mcpAvailable, setMcpAvailable] = useState(false);
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
  const [editorFontSizeDraft, setEditorFontSizeDraft] = useState("20");
  const [editorLineHeightDraft, setEditorLineHeightDraft] = useState("1.92");

  // Per-provider config drafts (re-synced when the active provider changes).


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
        setEditorFontSizeDraft(String(s.editor_font_size ?? 20));
        setEditorLineHeightDraft(String(s.editor_line_height ?? 1.92));
        setOpsRows(rows);
        setGitSyncAvailable(diag.git_sync_available ?? true);
        setMcpAvailable(diag.mcp_available ?? false);
        setCompanionHistoryExists(diag.companion_history_exists ?? true);
      })
      // Inside an effect: keeping `t` out of the deps avoids re-running the
      // fetch on every render. A mount failure is rare and stays raw.
      .catch((e) => { if (!cancelled) setError(String(e)); });
    return () => { cancelled = true; };
  }, [setLanguage]);

  const exportCompanionHistory = async () => {
    setCompanionExport("busy");
    setError(null);
    try {
      const payload = await exportApi.companionHistory();
      const path = await saveExportedMarkdown(payload);
      setCompanionExport(path ? "saved" : "idle");
    } catch (e) {
      setError(rpcErrorMessage(e, t));
      setCompanionExport("idle");
    }
  };

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
      dispatchAppEvent("linetta:settings-updated", next);
      setSavedAt(Date.now());
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

















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
              <div className="modal-field">
                <label>{t("settings.editor.palette")}</label>
                <div className="settings-palette" role="radiogroup" aria-label={t("settings.editor.palette")}>
                  {([
                    ["hanji", t("settings.editor.paletteHanji"), t("settings.editor.paletteHanjiDesc")],
                    ["paper", t("settings.editor.palettePaper"), t("settings.editor.palettePaperDesc")],
                    ["bone", t("settings.editor.paletteBone"), t("settings.editor.paletteBoneDesc")],
                    ["press", t("settings.editor.palettePress"), t("settings.editor.palettePressDesc")],
                  ] as Array<[PalettePreference, string, string]>).map(([palette, label, description]) => (
                    <button
                      key={palette}
                      type="button"
                      role="radio"
                      aria-checked={current.palette === palette}
                      data-palette={palette}
                      className={`palette-card${current.palette === palette ? " is-selected" : ""}`}
                      onClick={() => !saving && apply({ palette })}
                      disabled={saving}
                    >
                      <span className="palette-chips" aria-hidden="true">
                        <span className="palette-chip paper" />
                        <span className="palette-chip ink" />
                        <span className="palette-chip accent" />
                      </span>
                      <span className="palette-name">{label}</span>
                      <span className="palette-desc">{description}</span>
                    </button>
                  ))}
                </div>
                <p className="palette-hint">{t("settings.editor.paletteHint")}</p>
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


            {mcpAvailable && <McpSection />}

            {!gitSyncAvailable && (
            <section className="settings-section">
              <h3>{t("settings.git.title")}</h3>
              <p className="sd">{t("settings.git.unavailableNote")}</p>
            </section>
            )}

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
              <h4>{t("settings.restore.title")}</h4>
              <RestoreSection />
              {/* The companion is gone; its conversations are not. This is the
                  only way left to read them, so it outlives the settings block
                  it used to live in. Shown only to a library that holds a
                  transcript — a newcomer has nothing to export. */}
              {companionHistoryExists && (
                <>
                  <p className="sd">{t("settings.legacyAI.exportDescription")}</p>
                  <button
                    type="button"
                    className="btn ghost sm"
                    onClick={() => { void exportCompanionHistory(); }}
                    disabled={companionExport === "busy"}
                    data-testid="legacy-ai-export"
                  >
                    {companionExport === "busy"
                      ? t("settings.legacyAI.exporting")
                      : t("settings.legacyAI.export")}
                  </button>
                  {companionExport === "saved" && (
                    <p className="sd" data-testid="legacy-ai-exported">{t("settings.legacyAI.exported")}</p>
                  )}
                </>
              )}
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
