import { useEffect, useState, type CSSProperties, type MouseEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  projects as projectsApi,
  imports as importsApi,
  settings as settingsApi,
  diagnostics as diagnosticsApi,
  exportApi,
  openPath,
} from "../lib/rpc";
import type {
  DiagnosticsSnapshot,
  ImportPreviewResult,
  NewProjectInput,
  Project,
  SearchResult,
  Settings as SettingsRow,
} from "../lib/types";
import { NewProjectModal } from "../components/NewProjectModal";
import { ImportPreviewModal } from "../components/ImportPreviewModal";
import { SearchModal } from "../components/SearchModal";
import { pickAndReadMarkdown } from "../lib/importLoad";
import { saveExportedMarkdown } from "../lib/exportSave";
import { Archive, Download, FolderOpen, MoreHorizontal, Settings, Plus, Search, Trash2, Upload } from "../lib/icons";
import { useToast } from "../components/ToastProvider";
import { formatWordCount, lengthLabel, useI18n } from "../lib/i18n";
import { OnboardingTour, type OnboardingTourStep } from "../components/onboarding/OnboardingTour";
import {
  CURRENT_ONBOARDING_TOUR_VERSION,
  MANUAL_PHASE_STORAGE_KEY,
  WORKSPACE_PENDING_STORAGE_KEY,
  clearStoredPhase,
  readStoredPhase,
  shouldAutoStartOnboarding,
  storePhase,
} from "../components/onboarding/onboardingState";

const RECENT_LIMIT = 5;

const SPINE_COLORS = [
  "var(--t-teal)",
  "var(--t-sienna)",
  "var(--t-blue)",
  "var(--t-plum)",
  "var(--t-olive)",
];

interface PendingImport {
  fileName: string;
  content: string;
  preview: ImportPreviewResult;
}

export function Library() {
  const { language, t } = useI18n();
  const [recent, setRecent] = useState<Project[]>([]);
  const [totalRecent, setTotalRecent] = useState<number>(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);
  const [pending, setPending] = useState<PendingImport | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [settingsRow, setSettingsRow] = useState<SettingsRow | null>(null);
  const [diagnostics, setDiagnostics] = useState<DiagnosticsSnapshot | null>(null);
  const [safetyOpen, setSafetyOpen] = useState(false);
  const [diagnosticsOpen, setDiagnosticsOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [tourOpen, setTourOpen] = useState(false);
  const [projectMenuOpenId, setProjectMenuOpenId] = useState<string | null>(null);
  const navigate = useNavigate();
  const { showToast } = useToast();

  const refresh = async () => {
    setLoading(true);
    setError(null);
    try {
      const all = await projectsApi.list({ limit: RECENT_LIMIT + 1 });
      setRecent(all.slice(0, RECENT_LIMIT));
      setTotalRecent(all.length);
      if (all.length === 0) setModalOpen(true);
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  useEffect(() => {
    let cancelled = false;
    Promise.all([settingsApi.get(), diagnosticsApi.get()])
      .then(([s, d]) => {
        if (cancelled) return;
        setSettingsRow(s);
        setDiagnostics(d);
        setSafetyOpen(!s.safety_checklist_dismissed);
      })
      .catch((err) => {
        if (!cancelled) showToast(t("library.toast.appStateFailed", { error: String(err) }));
      });
    return () => { cancelled = true; };
  }, [showToast, t]);

  const handleCreate = async (input: NewProjectInput) => {
    const created = await projectsApi.create(input);
    setModalOpen(false);
    navigate(`/workspace/${created.id}`);
  };

  const handleImport = async () => {
    setImporting(true);
    try {
      const picked = await pickAndReadMarkdown();
      if (!picked) return;
      const preview = await importsApi.preview(picked.fileName, picked.content);
      setPending({ fileName: picked.fileName, content: picked.content, preview });
    } catch (err) {
      showToast(t("library.toast.importFailed", { error: String(err) }));
    } finally {
      setImporting(false);
    }
  };

  const handleMenuImport = async () => {
    setMenuOpen(false);
    await handleImport();
  };

  const confirmImport = async () => {
    if (!pending) return;
    setImporting(true);
    try {
      const res = await importsApi.markdown(pending.fileName, pending.content);
      const total = res.container_count + res.leaf_count;
      let msg = t("library.toast.importComplete", {
        containers: res.container_count,
        scenes: res.leaf_count,
      });
      if (total === 0) msg = t("library.toast.importEmpty");
      if (res.warnings.length > 0) {
        msg += ` · ${t("library.toast.importWarnings", { count: res.warnings.length })}`;
      }
      showToast(msg);
      setPending(null);
      navigate(`/workspace/${res.project_id}`);
    } catch (err) {
      showToast(t("library.toast.importFailed", { error: String(err) }));
    } finally {
      setImporting(false);
    }
  };

  const openBackupFolder = async () => {
    setMenuOpen(false);
    setProjectMenuOpenId(null);
    try {
      const current = settingsRow ?? await settingsApi.get();
      await openPath(current.backup_dir);
    } catch (err) {
      showToast(t("library.toast.openBackupFailed", { error: String(err) }));
    }
  };

  const openDataFolder = async () => {
    setMenuOpen(false);
    setProjectMenuOpenId(null);
    try {
      const current = diagnostics ?? await diagnosticsApi.get();
      setDiagnostics(current);
      await openPath(current.home);
    } catch (err) {
      showToast(t("library.toast.openDataFailed", { error: String(err) }));
    }
  };

  const openDiagnostics = async () => {
    setMenuOpen(false);
    setProjectMenuOpenId(null);
    try {
      const current = diagnostics ?? await diagnosticsApi.get();
      setDiagnostics(current);
      setDiagnosticsOpen(true);
    } catch (err) {
      showToast(t("library.toast.diagnosticsFailed", { error: String(err) }));
    }
  };

  const dismissSafety = async () => {
    try {
      const next = await settingsApi.set({ safety_checklist_dismissed: true });
      setSettingsRow(next);
      setSafetyOpen(false);
    } catch (err) {
      showToast(t("library.toast.safetySaveFailed", { error: String(err) }));
    }
  };

  const handleSearchSelect = (result: SearchResult) => {
    navigate(`/workspace/${result.project_id}`, { state: { jumpToNodeId: result.node_id } });
  };

  const toggleProjectMenu = (event: MouseEvent, projectId: string) => {
    event.preventDefault();
    event.stopPropagation();
    setMenuOpen(false);
    setProjectMenuOpenId((current) => current === projectId ? null : projectId);
  };

  const openProjectContextMenu = (event: MouseEvent, projectId: string) => {
    event.preventDefault();
    event.stopPropagation();
    setMenuOpen(false);
    setProjectMenuOpenId(projectId);
  };

  const handleProjectBackup = async (p: Project) => {
    setProjectMenuOpenId(null);
    try {
      const payload = await exportApi.project(p.id);
      const path = await saveExportedMarkdown(payload);
      if (path) showToast(t("library.toast.projectBackupComplete"));
    } catch (err) {
      showToast(t("library.toast.projectBackupFailed", { error: String(err) }));
    }
  };

  const handleProjectArchive = async (p: Project) => {
    setProjectMenuOpenId(null);
    try {
      await projectsApi.archive(p.id);
      showToast(t("library.toast.projectArchiveSuccess"));
      await refresh();
    } catch (err) {
      showToast(t("library.toast.projectArchiveFailed", { error: String(err) }));
    }
  };

  const handleProjectDelete = async (p: Project) => {
    setProjectMenuOpenId(null);
    const ok = window.confirm(t("library.confirm.deleteProject", { title: p.title }));
    if (!ok) return;
    try {
      await projectsApi.delete(p.id);
      showToast(t("library.toast.projectDeleteSuccess"));
      await refresh();
    } catch (err) {
      showToast(t("library.toast.projectDeleteFailed", { error: String(err) }));
    }
  };

  const tourSteps: OnboardingTourStep[] = [
    {
      target: "library-brand",
      title: t("onboarding.library.brand.title"),
      body: t("onboarding.library.brand.body"),
    },
    {
      target: "library-new",
      title: t("onboarding.library.new.title"),
      body: t("onboarding.library.new.body"),
    },
    {
      target: "library-import",
      title: t("onboarding.library.import.title"),
      body: t("onboarding.library.import.body"),
    },
    {
      target: "library-search",
      title: t("onboarding.library.search.title"),
      body: t("onboarding.library.search.body"),
    },
    {
      target: "library-settings",
      title: t("onboarding.library.settings.title"),
      body: t("onboarding.library.settings.body"),
    },
  ];

  const finishLibraryTour = () => {
    storePhase(WORKSPACE_PENDING_STORAGE_KEY, "workspace");
    clearStoredPhase(MANUAL_PHASE_STORAGE_KEY);
    setTourOpen(false);
  };

  const skipLibraryTour = async () => {
    clearStoredPhase(WORKSPACE_PENDING_STORAGE_KEY);
    clearStoredPhase(MANUAL_PHASE_STORAGE_KEY);
    setTourOpen(false);
    try {
      const next = await settingsApi.set({ onboarding_tour_seen_version: CURRENT_ONBOARDING_TOUR_VERSION });
      setSettingsRow(next);
    } catch (err) {
      showToast(t("library.toast.appStateFailed", { error: String(err) }));
    }
  };

  useEffect(() => {
    const blocked = loading || modalOpen || safetyOpen || pending !== null || diagnosticsOpen || searchOpen || tourOpen;
    if (blocked || !settingsRow) return;
    if (readStoredPhase(WORKSPACE_PENDING_STORAGE_KEY) === "workspace") return;
    if (readStoredPhase(MANUAL_PHASE_STORAGE_KEY) === "library") {
      setTourOpen(true);
      return;
    }
    if (shouldAutoStartOnboarding(settingsRow)) {
      setTourOpen(true);
    }
  }, [diagnosticsOpen, loading, modalOpen, pending, safetyOpen, searchOpen, settingsRow, tourOpen]);

  return (
    <main className="library fade-in">
      <header className="lib-top">
        <div style={{ position: "relative" }}>
          <button
            className="lib-icon-btn"
            aria-label={t("library.menu.label")}
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen((open) => !open)}
          >
            <MoreHorizontal size={18} />
          </button>
          {menuOpen && (
            <div className="lib-menu" role="menu" onMouseLeave={() => setMenuOpen(false)}>
              <button type="button" role="menuitem" onClick={openDataFolder}>
                {t("library.menu.dataFolder")}
              </button>
              <button type="button" role="menuitem" onClick={openBackupFolder}>
                {t("library.menu.backupFolder")}
              </button>
              <button type="button" role="menuitem" onClick={handleMenuImport}>
                {t("library.menu.importMarkdown")}
              </button>
              <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); setSearchOpen(true); }}>
                {t("library.menu.search")}
              </button>
              <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); navigate("/settings"); }}>
                {t("library.menu.settings")}
              </button>
              <button type="button" role="menuitem" onClick={openDiagnostics}>
                {t("library.menu.diagnostics")}
              </button>
            </div>
          )}
        </div>
        <div className="lib-brandmark">{t("library.brand")}</div>
        <div className="lib-top-actions">
          <button className="lib-icon-btn" aria-label={t("library.menu.search")} onClick={() => setSearchOpen(true)}>
            <Search size={17} />
          </button>
          <Link to="/settings" className="lib-icon-btn" aria-label={t("library.menu.settings")} data-tour="library-settings">
            <Settings size={17} />
          </Link>
        </div>
      </header>

      <div className="lib-body">
        <div className="lib-hero" data-tour="library-brand">
          <div>
            <h1 className="lib-wordmark">Linetta<span className="dot">.</span></h1>
            <p className="lib-tagline">{t("library.tagline")}</p>
          </div>
          <div className="lib-meta-col">
            <div><b>{totalRecent}</b> {t("library.projectCount")}</div>
            <div><b>{t("library.noExternalTransfer")}</b></div>
            <div>{t("library.dataLocal", { local: t("library.local") })}</div>
          </div>
        </div>

        <div className="lib-actions">
          <button className="btn accent" onClick={() => setModalOpen(true)} data-tour="library-new">
            <Plus size={16} /> {t("library.newProject")}
          </button>
          <button
            className="btn ghost"
            onClick={handleImport}
            disabled={importing || pending !== null}
            data-tour="library-import"
          >
            <Upload size={15} /> {importing ? t("library.importing") : t("library.menu.importMarkdown")}
          </button>
          <button className="btn ghost" onClick={() => setSearchOpen(true)} data-tour="library-search">
            <Search size={15} /> {t("library.menu.search")} <span className="kbd" style={{ marginLeft: 4 }}>⌘F</span>
          </button>
        </div>

        <div className="lib-shelf-head">
          <span className="lib-shelf-title">{t("library.recentProjects")}</span>
          <div className="lib-shelf-links">
            <Link to="/library/all?tab=archived" className="lib-shelf-all">{t("library.archiveBox")}</Link>
            <Link to="/library/all" className="lib-shelf-all">{t("library.allProjects")}</Link>
          </div>
        </div>

        {loading ? (
          <p className="hint">{t("common.loading")}</p>
        ) : error ? (
          <p className="error">{error}</p>
        ) : (
          <div className="lib-grid">
            {recent.map((p, i) => (
              <div
                key={p.id}
                className="book-wrap"
                style={{ "--spine": SPINE_COLORS[i % SPINE_COLORS.length] } as CSSProperties}
                onContextMenu={(event) => openProjectContextMenu(event, p.id)}
              >
                <button
                  className="book"
                  onClick={() => navigate(`/workspace/${p.id}`)}
                >
                  <h3 className="book-title">{p.title}</h3>
                  <div className="book-spacer" />
                  <div className="book-scenes">{lengthLabel(language, p.length_target)}</div>
                  <div className="book-meta">{formatWordCount(language, p.word_count)}</div>
                </button>
                <button
                  type="button"
                  className="book-action"
                  aria-label={t("library.projectActionsLabel", { title: p.title })}
                  aria-expanded={projectMenuOpenId === p.id}
                  aria-haspopup="menu"
                  onClick={(event) => toggleProjectMenu(event, p.id)}
                >
                  <MoreHorizontal size={16} />
                </button>
                {projectMenuOpenId === p.id && (
                  <div className="lib-menu book-menu" role="menu">
                    <button type="button" role="menuitem" onClick={() => handleProjectBackup(p)}>
                      <Download size={15} /> <span>{t("library.projectBackup")}</span>
                    </button>
                    <button type="button" role="menuitem" onClick={openBackupFolder}>
                      <FolderOpen size={15} /> <span>{t("library.menu.backupFolder")}</span>
                    </button>
                    <button type="button" role="menuitem" onClick={() => handleProjectArchive(p)}>
                      <Archive size={15} /> <span>{t("library.archive")}</span>
                    </button>
                    <button type="button" role="menuitem" className="danger" onClick={() => handleProjectDelete(p)}>
                      <Trash2 size={15} /> <span>{t("library.deleteProject")}</span>
                    </button>
                  </div>
                )}
              </div>
            ))}
            <button className="book-new" onClick={() => setModalOpen(true)}>
              <span className="plus-ring"><Plus size={20} /></span>
              <span>{t("library.startNewProject")}</span>
            </button>
          </div>
        )}
      </div>

      <NewProjectModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        onSubmit={handleCreate}
      />

      <SearchModal
        open={searchOpen}
        onClose={() => setSearchOpen(false)}
        onSelect={handleSearchSelect}
      />

      {pending && (
        <ImportPreviewModal
          preview={pending.preview}
          fileName={pending.fileName}
          busy={importing}
          onConfirm={confirmImport}
          onCancel={() => setPending(null)}
        />
      )}

      {safetyOpen && !modalOpen && (
        <div className="modal-backdrop" role="dialog" aria-modal="true">
          <div className="modal safety-modal">
            <h2>{t("library.safety.title")}</h2>
            <ul className="safety-list">
              <li>
                <span>{t("library.safety.data")}</span>
                <code>{diagnostics?.home ?? t("library.safety.checking")}</code>
              </li>
              <li>
                <span>{t("library.safety.backup")}</span>
                <code>{settingsRow?.backup_dir ?? t("library.safety.checking")}</code>
              </li>
              <li>
                <span>Git sync</span>
                <strong>{settingsRow?.git_sync_dir ? t("library.safety.configured") : t("library.safety.disabled")}</strong>
              </li>
            </ul>
            <div className="modal-actions">
              <button type="button" onClick={() => navigate("/settings")}>{t("library.safety.gitSettings")}</button>
              <button type="button" onClick={dismissSafety}>{t("library.safety.dismiss")}</button>
            </div>
          </div>
        </div>
      )}

      {diagnosticsOpen && diagnostics && (
        <div className="modal-backdrop" role="dialog" aria-modal="true">
          <div className="modal diagnostics-modal">
            <h2>{t("library.diagnostics.title")}</h2>
            <pre>{JSON.stringify(diagnostics, null, 2)}</pre>
            <div className="modal-actions">
              <button type="button" onClick={() => setDiagnosticsOpen(false)}>{t("common.close")}</button>
            </div>
          </div>
        </div>
      )}

      <OnboardingTour
        open={tourOpen}
        steps={tourSteps}
        onFinish={finishLibraryTour}
        onSkip={skipLibraryTour}
      />
    </main>
  );
}
