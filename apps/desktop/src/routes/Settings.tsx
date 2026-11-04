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
import { BackgroundSection } from "../components/settings/BackgroundSection";
import { McpSection } from "../components/settings/McpSection";
import { MemorySection } from "../components/settings/MemorySection";
import { ProviderSection } from "../components/settings/ProviderSection";
import { ImageGenSection } from "../components/settings/ImageGenSection";
import { PublishSection } from "../components/settings/PublishSection";
import { SyncOverview } from "../components/settings/SyncOverview";
import { VaultSection } from "../components/settings/VaultSection";
import { RestoreSection } from "../components/settings/RestoreSection";
import { SkillsSection } from "../components/settings/SkillsSection";
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
  SettingsPatch,
  PalettePreference,
  ThemePreference,
} from "../lib/types";

const JOB_BACKUP = "backup.daily";
const JOB_GIT_SYNC = "git_sync";
const JOB_FOLDER_SYNC = "folder_sync";
const JOB_SUMMARIZER = "summarizer";
// Mirrors engine/internal/settings: settings.set refuses anything outside.
const DEFAULT_AGENT_MAX_TOOL_CALLS = 48;
const MIN_AGENT_MAX_TOOL_CALLS = 8;
const MAX_AGENT_MAX_TOOL_CALLS = 200;
// settings.MinAgentHistoryBudget and MaxAgentHistoryBudget. Blank is automatic.
const MIN_AGENT_HISTORY_BUDGET = 20000;
const MAX_AGENT_HISTORY_BUDGET = 200000;

/** Top tabs, one set of related panes each. Only the active tab is mounted. */
const SETTINGS_TABS = ["general", "writing", "ai", "agent", "mcp", "sync", "publish", "backup"] as const;
type SettingsTab = (typeof SETTINGS_TABS)[number];

/** What a stored category means now. These are the sidebar's old names: a tab
 *  open across the update still holds one. */
const TAB_OF_STORED: Record<string, SettingsTab> = {
  background: "general",
  editor: "writing",
  providers: "ai",
  images: "ai",
  tools: "agent",
  memory: "agent",
  skills: "agent",
  vault: "sync",
};

const CATEGORY_STORAGE_KEY = "linetta.settings.category";

/** Tool-schema bytes as something a writer can weigh (#99). kB, one decimal,
 *  and the same unit for every figure on the pane so the group costs visibly
 *  add up to the total — a group shown in bytes beside a total in kilobytes
 *  would make the arithmetic the writer is doing harder, not easier. The
 *  engine measures the number; this only chooses how to say it. */
function formatToolBytes(bytes: number): string {
  return `${(bytes / 1024).toFixed(1)} kB`;
}

/** Open Settings on a given tab next time it mounts — for a screen elsewhere
 *  that sends the writer to one place ("connect in Settings → Publishing").
 *  Image generation also scrolls: it sits under the providers on its tab. */
export function rememberSettingsCategory(category: SettingsTab | "images"): void {
  try {
    sessionStorage.setItem(CATEGORY_STORAGE_KEY, category);
  } catch {
    /* storage unavailable — Settings opens on its first tab */
  }
}

function storedCategory(): string | null {
  try {
    return sessionStorage.getItem(CATEGORY_STORAGE_KEY);
  } catch {
    /* storage unavailable — start at the first tab */
    return null;
  }
}

function initialTab(): SettingsTab {
  const stored = storedCategory();
  if (stored && stored in TAB_OF_STORED) return TAB_OF_STORED[stored];
  if (stored && (SETTINGS_TABS as readonly string[]).includes(stored)) {
    return stored as SettingsTab;
  }
  return "general";
}

/** Bring a pane to the top. The panes above it are still loading and will push
 *  it down, so this holds on for a moment rather than scrolling once. */
function scrollToPane(id: string): () => void {
  const pane = document.getElementById(id);
  const scroller = pane?.closest(".settings-content");
  if (!pane || !scroller) return () => {};
  const scroll = () => pane.scrollIntoView?.({ block: "start" });
  scroll();
  if (typeof ResizeObserver === "undefined") return () => {};
  const observer = new ResizeObserver(scroll);
  observer.observe(scroller);
  const timer = setTimeout(() => observer.disconnect(), 1000);
  return () => {
    clearTimeout(timer);
    observer.disconnect();
  };
}

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
  // Hidden on a build that never links internal/agent (mobile) — offering a
  // screen to configure providers nothing can use would be a false promise.
  // #93 added this field to DiagnosticsSnapshot and set it from real
  // capability detection; nothing read it until now.
  const [agentAvailable, setAgentAvailable] = useState(false);
  // Hidden on builds that cannot publish to Linetta Works (iOS).
  const [publishAvailable, setPublishAvailable] = useState(false);
  const [vaultAvailable, setVaultAvailable] = useState(false);
  // Off on iOS, where the folder picker has nothing to pick. With git and the
  // vault also off there, the whole sync tab goes.
  const [foldersAvailable, setFoldersAvailable] = useState(false);
  const [current, setCurrent] = useState<SettingsRow | null>(null);
  const [tab, setTabState] = useState<SettingsTab>(initialTab);
  const [openOnImages] = useState(() => storedCategory() === "images");
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
  const [maxToolCallsDraft, setMaxToolCallsDraft] = useState(String(DEFAULT_AGENT_MAX_TOOL_CALLS));
  const [historyBudgetDraft, setHistoryBudgetDraft] = useState("");
  const [editorLineHeightDraft, setEditorLineHeightDraft] = useState("1.92");

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
        setMaxToolCallsDraft(String(s.agent_max_tool_calls ?? DEFAULT_AGENT_MAX_TOOL_CALLS));
        setHistoryBudgetDraft(s.agent_history_budget ? String(s.agent_history_budget) : "");
        setEditorLineHeightDraft(String(s.editor_line_height ?? 1.92));
        setOpsRows(rows);
        setGitSyncAvailable(diag.git_sync_available ?? true);
        setMcpAvailable(diag.mcp_available ?? false);
        setAgentAvailable(diag.agent_available ?? false);
        setPublishAvailable(diag.publish_available ?? false);
        setVaultAvailable(diag.vault_available ?? false);
        setFoldersAvailable(diag.folders_available ?? true);
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

  // SettingsPatch, not Partial<SettingsRow>: what may be *written* is
  // narrower than what settings.get may *return*. `provider` is the case —
  // a patch must name a live provider id, while a read can hand back a
  // retired one still sitting in someone's settings.json.
  const apply = async (patch: SettingsPatch) => {
    if (!current) return;
    setSaving(true);
    setError(null);
    try {
      const next = await settingsApi.set(patch);
      setCurrent(next);
      setLanguage(next.language);
      setEditorFontSizeDraft(String(next.editor_font_size ?? 20));
      setMaxToolCallsDraft(String(next.agent_max_tool_calls ?? DEFAULT_AGENT_MAX_TOOL_CALLS));
      setHistoryBudgetDraft(next.agent_history_budget ? String(next.agent_history_budget) : "");
      setEditorLineHeightDraft(String(next.editor_line_height ?? 1.92));
      dispatchAppEvent("linetta:settings-updated", next);
      setSavedAt(Date.now());
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  // An out-of-range or unparsable draft is pulled to the nearest bound
  // rather than refused: the field has no error state of its own.
  const commitMaxToolCalls = () => {
    if (!current) return;
    const saved = current.agent_max_tool_calls ?? DEFAULT_AGENT_MAX_TOOL_CALLS;
    const parsed = Math.round(Number(maxToolCallsDraft));
    if (maxToolCallsDraft.trim() === "" || !Number.isFinite(parsed)) {
      setMaxToolCallsDraft(String(saved));
      return;
    }
    const value = Math.min(MAX_AGENT_MAX_TOOL_CALLS, Math.max(MIN_AGENT_MAX_TOOL_CALLS, parsed));
    setMaxToolCallsDraft(String(value));
    if (value !== saved) void apply({ agent_max_tool_calls: value });
  };

  // Blank means automatic, which is why this cannot share the cap's rule of
  // putting a blank draft back: clearing the box is how the writer asks for it.
  const commitHistoryBudget = () => {
    if (!current) return;
    const saved = current.agent_history_budget ?? 0;
    const parsed = Math.round(Number(historyBudgetDraft));
    if (historyBudgetDraft.trim() !== "" && !Number.isFinite(parsed)) {
      setHistoryBudgetDraft(saved ? String(saved) : "");
      return;
    }
    const value =
      historyBudgetDraft.trim() === "" || parsed === 0
        ? 0
        : Math.min(MAX_AGENT_HISTORY_BUDGET, Math.max(MIN_AGENT_HISTORY_BUDGET, parsed));
    setHistoryBudgetDraft(value ? String(value) : "");
    if (value !== saved) void apply({ agent_history_budget: value });
  };

















  const setTab = (next: SettingsTab) => {
    setTabState(next);
    try {
      sessionStorage.setItem(CATEGORY_STORAGE_KEY, next);
    } catch {
      /* per-tab convenience only */
    }
  };

  const loaded = current !== null;
  useEffect(() => {
    if (loaded && openOnImages) return scrollToPane("image-settings");
  }, [loaded, openOnImages]);

  // On a narrow window the strip scrolls; the open tab stays in it.
  useEffect(() => {
    if (loaded) document.querySelector(".settings-tab.on")?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }, [loaded, tab]);

  // Hidden on builds that have no agent and no external client: nothing there
  // could read a memory or load a skill.
  const agentPanes = mcpAvailable || agentAvailable;
  const syncAvailable = gitSyncAvailable || vaultAvailable || foldersAvailable;
  const tabs: Array<{ id: SettingsTab; label: string }> = [
    { id: "general", label: t("settings.tab.general") },
    { id: "writing", label: t("settings.tab.writing") },
    // Connecting a provider is what a writer does first; MCP is only for
    // people using outside tools.
    ...(agentAvailable ? [{ id: "ai" as const, label: t("settings.tab.ai") }] : []),
    ...(agentPanes ? [{ id: "agent" as const, label: t("settings.tab.agent") }] : []),
    ...(mcpAvailable ? [{ id: "mcp" as const, label: t("settings.tab.mcp") }] : []),
    ...(syncAvailable ? [{ id: "sync" as const, label: t("settings.tab.sync") }] : []),
    // Publishing sends text off the device, like sync does, so it sits
    // beside it rather than with the AI connections.
    ...(publishAvailable ? [{ id: "publish" as const, label: t("settings.tab.publish") }] : []),
    { id: "backup", label: t("settings.tab.backup") },
  ];

  // A remembered tab this build does not show (sync on iOS) would render a
  // pane with no tab to say where the writer is.
  const tabShown = tabs.some((item) => item.id === tab);
  useEffect(() => {
    if (loaded && !tabShown) setTabState("general");
  }, [loaded, tabShown]);

  /* The tool budget, but only when it is a real measurement (#99).
     Truthiness is not the test: an engine whose measurement failed answers
     with a well-formed `{tools: 0, bytes: 0, …}` — nothing in that payload
     says "unknown" — and gating on the object alone rendered "the agent
     currently gets 0 tools — about 0.0 kB" over a tool set of nineteen.
     A budget of zero tools is not a state this pane can be in: the agent
     always keeps the manuscript tools, which no switch can remove. So zero
     means the number is not known, and an unknown number is drawn the same
     way an absent one is — not at all. The engine omits the key in that case
     too; this is the second lock on the same door, because the pane is what
     the writer reads and inventing a number here is the drift the whole
     feature exists to avoid. */
  const toolBudget = current?.tool_budget && current.tool_budget.tools > 0 ? current.tool_budget : null;

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
            <div className="settings-tabbar">
              <nav className="settings-tabs" aria-label={t("settings.title")}>
                {tabs.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    className={`settings-tab${tab === item.id ? " on" : ""}`}
                    aria-current={tab === item.id ? "page" : undefined}
                    onClick={() => setTab(item.id)}
                    data-testid={`settings-tab-${item.id}`}
                  >
                    {item.label}
                  </button>
                ))}
              </nav>
              {savedAt && <span className="settings-saved">{t("settings.saved")}</span>}
            </div>
            <div className="settings-content">
            {tab === "general" && (
            <section className="settings-section">
              <h3>{t("settings.language.title")}</h3>
              <div className="set-row">
                <span className="sk-wrap">
                  <label className="sk" htmlFor="app-language">{t("settings.language.label")}</label>
                  <span className="sd">{t("settings.language.description")}</span>
                </span>
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
            )}

            {tab === "writing" && (
            <section className="settings-section">
              <h3>{t("settings.editor.title")}</h3>
              <div className="set-row">
                <span className="sk">{t("settings.editor.theme")}</span>
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
              <div className="set-row is-stacked">
                <span className="sk">{t("settings.editor.palette")}</span>
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
                <span className="sd">{t("settings.editor.paletteHint")}</span>
              </div>
              <div className="set-row">
                <label className="sk" htmlFor="editor-font-size">{t("settings.editor.fontSize")}</label>
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
              <div className="set-row">
                <label className="sk" htmlFor="editor-line-height">{t("settings.editor.lineHeight")}</label>
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
              <div className="set-row">
                <label className="sk" htmlFor="copy-profile">{t("settings.editor.copyProfile")}</label>
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
            )}

            {tab === "writing" && (
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
            )}

            {tab === "general" && (
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
            )}

            {tab === "general" && <BackgroundSection />}

            {tab === "ai" && agentAvailable && <ProviderSection />}

            {tab === "ai" && agentAvailable && (
              <ImageGenSection onOpenProviders={() => scrollToPane("provider-settings")} />
            )}

            {tab === "mcp" && mcpAvailable && <McpSection />}

            {tab === "publish" && <PublishSection available={publishAvailable} />}

            {/* The tool budget (#99). Both switches are ON by default, so they
                read `!== false`: a settings payload from an engine that
                predates the keys has no opinion, and truthiness would read
                that as off and draw a switch the writer never threw.

                The numbers come from the engine — measured from the tools it
                would actually register — and the block is simply skipped when
                they are absent, which is what a build that cannot measure them
                returns. A count invented here would be the drift this feature
                exists to avoid. */}
            {tab === "agent" && agentPanes && (
            <section className="settings-section">
              <h3>{t("settings.tools.title")}</h3>
              <p className="sd">{t("settings.tools.description")}</p>
              {toolBudget && (
                <p className="sd" data-testid="tool-budget-current">
                  {t("settings.tools.current", {
                    tools: String(toolBudget.tools),
                    size: formatToolBytes(toolBudget.bytes),
                  })}
                </p>
              )}
              <button type="button" className="set-row set-row-btn" disabled={saving}
                onClick={() => !saving && apply({ visual_write_tools_disabled: !current.visual_write_tools_disabled })}>
                <span className="sk-wrap">
                  <span className="sk">{t("settings.tools.visuals.title")}</span>
                  <span className="sd">{t("settings.tools.visuals.description")}</span>
                  {toolBudget?.visual_writes && <span className="sd">{t("settings.tools.groupCost", {
                    tools: String(toolBudget.visual_writes.tools), size: formatToolBytes(toolBudget.visual_writes.bytes),
                  })}</span>}
                </span>
                <span className={`switch${!current.visual_write_tools_disabled ? " on" : ""}`} />
              </button>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() =>
                  !saving && apply({ memory_tools_enabled: current.memory_tools_enabled === false })
                }
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">{t("settings.tools.memory.title")}</span>
                  <span className="sd">{t("settings.tools.memory.description")}</span>
                  {toolBudget && (
                    <span className="sd" data-testid="tool-budget-memory">
                      {t("settings.tools.groupCost", {
                        tools: String(toolBudget.memory.tools),
                        size: formatToolBytes(toolBudget.memory.bytes),
                      })}
                    </span>
                  )}
                </span>
                <span className={`switch${current.memory_tools_enabled !== false ? " on" : ""}`} />
              </button>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() =>
                  !saving && apply({ skill_tools_enabled: current.skill_tools_enabled === false })
                }
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">{t("settings.tools.skills.title")}</span>
                  <span className="sd">{t("settings.tools.skills.description")}</span>
                  {toolBudget && (
                    <span className="sd" data-testid="tool-budget-skills">
                      {t("settings.tools.groupCost", {
                        tools: String(toolBudget.skills.tools),
                        size: formatToolBytes(toolBudget.skills.bytes),
                      })}
                    </span>
                  )}
                </span>
                <span className={`switch${current.skill_tools_enabled !== false ? " on" : ""}`} />
              </button>
              {/* story-assist Phase 2: linetta_report_continuity, and
                  whatever later phases add beside it. Same default-on,
                  `!== false` read as the two switches above. */}
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() =>
                  !saving && apply({ review_tools_enabled: current.review_tools_enabled === false })
                }
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">{t("settings.tools.review.title")}</span>
                  <span className="sd">{t("settings.tools.review.description")}</span>
                  {toolBudget?.review && (
                    <span className="sd" data-testid="tool-budget-review">
                      {t("settings.tools.groupCost", {
                        tools: String(toolBudget.review.tools),
                        size: formatToolBytes(toolBudget.review.bytes),
                      })}
                    </span>
                  )}
                </span>
                <span className={`switch${current.review_tools_enabled !== false ? " on" : ""}`} />
              </button>
              {/* The built-in agent's cap only: an external MCP client runs
                  its own loop and never reads it. Saved on blur or Enter, not
                  per keystroke — "120" passes through "12" on the way. */}
              {agentAvailable && (
                <div className="set-row">
                  <span className="sk-wrap">
                    <label className="sk" htmlFor="agent-max-tool-calls">{t("settings.tools.maxCalls.title")}</label>
                    <span className="sd">
                      {t("settings.tools.maxCalls.description", {
                        min: String(MIN_AGENT_MAX_TOOL_CALLS),
                        max: String(MAX_AGENT_MAX_TOOL_CALLS),
                        default: String(DEFAULT_AGENT_MAX_TOOL_CALLS),
                      })}
                    </span>
                  </span>
                  <input
                    id="agent-max-tool-calls"
                    data-testid="agent-max-tool-calls"
                    type="number"
                    min={MIN_AGENT_MAX_TOOL_CALLS}
                    max={MAX_AGENT_MAX_TOOL_CALLS}
                    step={1}
                    value={maxToolCallsDraft}
                    onChange={(e) => setMaxToolCallsDraft(e.currentTarget.value)}
                    onBlur={commitMaxToolCalls}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") e.currentTarget.blur();
                    }}
                    disabled={saving}
                  />
                </div>
              )}
              {agentAvailable && (
                <div className="set-row">
                  <span className="sk-wrap">
                    <label className="sk" htmlFor="agent-history-budget">{t("settings.tools.historyBudget.title")}</label>
                    <span className="sd">
                      {t("settings.tools.historyBudget.description", {
                        min: String(MIN_AGENT_HISTORY_BUDGET),
                        max: String(MAX_AGENT_HISTORY_BUDGET),
                      })}
                    </span>
                  </span>
                  <input
                    id="agent-history-budget"
                    data-testid="agent-history-budget"
                    type="number"
                    min={MIN_AGENT_HISTORY_BUDGET}
                    max={MAX_AGENT_HISTORY_BUDGET}
                    step={1000}
                    placeholder={t("settings.tools.historyBudget.auto")}
                    value={historyBudgetDraft}
                    onChange={(e) => setHistoryBudgetDraft(e.currentTarget.value)}
                    onBlur={commitHistoryBudget}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") e.currentTarget.blur();
                    }}
                    disabled={saving}
                  />
                </div>
              )}
            </section>
            )}

            {tab === "agent" && agentPanes && <MemorySection />}

            {tab === "agent" && agentPanes && (
            <>
            {/* With the skills tools switched off (#99) this whole pane is
                still fully usable and none of it reaches the agent: the list
                below is not put in its prompt, linetta_read_skill is not
                registered, and the self-improvement switch under it governs a
                pass whose first act would be to call a tool the agent does
                not have — so it can never fire. Hiding the pane would be
                worse: the skills are the writer's documents and they are
                still theirs to write. What was missing is the sentence saying
                nobody is reading them, and where to change that. */}
            {current.skill_tools_enabled === false && (
              <p className="hint" data-testid="skills-tools-off">
                {t("settings.skills.toolsOff", { pane: t("settings.tools.title") })}
              </p>
            )}
            {/* The self-improvement loop (#98). It sits above the skill list
                rather than in the providers pane because what it produces is
                skills: the writer who wants to know why a skill appeared they
                did not write is already looking at this page. It is on by
                default, so the switch reads `!== false` — a settings.json from
                a build before the key simply has no opinion. */}
            <section className="settings-section">
              <h3>{t("settings.skills.selfReview.title")}</h3>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() =>
                  !saving &&
                  apply({ agent_self_review_enabled: current.agent_self_review_enabled === false })
                }
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">{t("settings.skills.selfReview.enabled")}</span>
                  <span className="sd">{t("settings.skills.selfReview.description")}</span>
                </span>
                <span
                  className={`switch${current.agent_self_review_enabled !== false ? " on" : ""}`}
                />
              </button>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() =>
                  !saving &&
                  apply({ agent_episode_review_enabled: current.agent_episode_review_enabled !== true })
                }
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">{t("settings.skills.episodeReview.enabled")}</span>
                  <span className="sd">{t("settings.skills.episodeReview.description")}</span>
                </span>
                <span className={`switch${current.agent_episode_review_enabled === true ? " on" : ""}`} />
              </button>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() =>
                  !saving &&
                  apply({ agent_settlement_enabled: current.agent_settlement_enabled !== true })
                }
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">{t("settings.skills.settlement.enabled")}</span>
                  <span className="sd">{t("settings.skills.settlement.description")}</span>
                </span>
                <span className={`switch${current.agent_settlement_enabled === true ? " on" : ""}`} />
              </button>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() =>
                  !saving &&
                  apply({ agent_impact_review_enabled: current.agent_impact_review_enabled !== true })
                }
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">{t("settings.skills.impactReview.enabled")}</span>
                  <span className="sd">{t("settings.skills.impactReview.description")}</span>
                </span>
                <span className={`switch${current.agent_impact_review_enabled === true ? " on" : ""}`} />
              </button>
              <label className="set-row is-stacked">
                <span className="sk-wrap">
                  <span className="sk">{t("settings.style.globalPhrases")}</span>
                  <span className="sd">{t("settings.style.globalPhrasesDescription")}</span>
                </span>
                <textarea
                  rows={3}
                  defaultValue={(current.style_avoid_phrases ?? []).join("\n")}
                  disabled={saving}
                  onBlur={(e) => {
                    const phrases = e.target.value.split("\n").map((line) => line.trim()).filter((line) => line !== "");
                    if (phrases.join("\n") !== (current.style_avoid_phrases ?? []).join("\n")) {
                      apply({ style_avoid_phrases: phrases });
                    }
                  }}
                />
              </label>
            </section>
            <SkillsSection />
            </>
            )}

            {tab === "sync" && vaultAvailable && (
              <>
                <SyncOverview />
                <VaultSection available={vaultAvailable} onOpenPublish={() => setTab("publish")} />
              </>
            )}

            {tab === "sync" && !gitSyncAvailable && (
            <section className="settings-section">
              <h3>{t("settings.git.title")}</h3>
              <p className="sd">{t("settings.git.unavailableNote")}</p>
            </section>
            )}

            {tab === "sync" && gitSyncAvailable && (
            <section className="settings-section">
              <h3>{t("settings.git.title")}</h3>
              <p className="sd">{t("settings.sync.exportOnly")}</p>
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

            {tab === "sync" && foldersAvailable && (
            <section className="settings-section">
              <h3>{t("settings.folder.title")}</h3>
              <p className="sd">{t("settings.sync.exportOnly")}</p>
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
            )}

            {tab === "backup" && (
            <section className="settings-section">
              <h3>{t("settings.backup.title")}</h3>
              <p className="sd">{t("settings.backup.description")}</p>
              <div className="set-row is-stacked">
                <span className="sk">{t("settings.backup.folder")}</span>
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
            )}

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

            </div>
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
