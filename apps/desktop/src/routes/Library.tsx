import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  projects as projectsApi,
  imports as importsApi,
  settings as settingsApi,
  diagnostics as diagnosticsApi,
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
import { ProjectCard } from "../components/ProjectCard";
import { NewProjectModal } from "../components/NewProjectModal";
import { ImportPreviewModal } from "../components/ImportPreviewModal";
import { SearchModal } from "../components/SearchModal";
import { pickAndReadMarkdown } from "../lib/importLoad";
import { MoreHorizontal, Settings, Plus, Search, Upload } from "../lib/icons";
import { useToast } from "../components/ToastProvider";

const RECENT_LIMIT = 5;

interface PendingImport {
  fileName: string;
  content: string;
  preview: ImportPreviewResult;
}

export function Library() {
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
        if (!cancelled) showToast(`앱 상태 불러오기 실패: ${err}`);
      });
    return () => { cancelled = true; };
  }, [showToast]);

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
      showToast(`가져오기 실패: ${err}`);
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
      let msg = `가져오기 완료 · 컨테이너 ${res.container_count}개 · 씬 ${res.leaf_count}개`;
      if (total === 0) msg = "가져오기 완료 · 빈 작품 (헤딩 없음)";
      if (res.warnings.length > 0) msg += ` · 경고 ${res.warnings.length}개`;
      showToast(msg);
      setPending(null);
      navigate(`/workspace/${res.project_id}`);
    } catch (err) {
      showToast(`가져오기 실패: ${err}`);
    } finally {
      setImporting(false);
    }
  };

  const openBackupFolder = async () => {
    setMenuOpen(false);
    try {
      const current = settingsRow ?? await settingsApi.get();
      await openPath(current.backup_dir);
    } catch (err) {
      showToast(`백업 폴더 열기 실패: ${err}`);
    }
  };

  const openDataFolder = async () => {
    setMenuOpen(false);
    try {
      const current = diagnostics ?? await diagnosticsApi.get();
      setDiagnostics(current);
      await openPath(current.home);
    } catch (err) {
      showToast(`데이터 폴더 열기 실패: ${err}`);
    }
  };

  const openDiagnostics = async () => {
    setMenuOpen(false);
    try {
      const current = diagnostics ?? await diagnosticsApi.get();
      setDiagnostics(current);
      setDiagnosticsOpen(true);
    } catch (err) {
      showToast(`진단 정보 불러오기 실패: ${err}`);
    }
  };

  const dismissSafety = async () => {
    try {
      const next = await settingsApi.set({ safety_checklist_dismissed: true });
      setSettingsRow(next);
      setSafetyOpen(false);
    } catch (err) {
      showToast(`체크리스트 저장 실패: ${err}`);
    }
  };

  const handleSearchSelect = (result: SearchResult) => {
    navigate(`/workspace/${result.project_id}`, { state: { jumpToNodeId: result.node_id } });
  };

  return (
    <main className="library">
      <header className="library-top">
        <div className="library-menu-wrap">
          <button
            className="icon-btn"
            aria-label="라이브러리 옵션"
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen((open) => !open)}
          >
            <MoreHorizontal size={16} />
          </button>
          {menuOpen && (
            <div className="library-menu" role="menu">
              <button type="button" role="menuitem" onClick={openDataFolder}>
                데이터 폴더 열기
              </button>
              <button type="button" role="menuitem" onClick={openBackupFolder}>
                백업 폴더 열기
              </button>
              <button type="button" role="menuitem" onClick={handleMenuImport}>
                가져오기 (.md)
              </button>
              <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); setSearchOpen(true); }}>
                검색
              </button>
              <button type="button" role="menuitem" onClick={() => navigate("/settings")}>
                설정
              </button>
              <button type="button" role="menuitem" onClick={openDiagnostics}>
                진단 정보
              </button>
            </div>
          )}
        </div>
        <Link to="/settings" className="icon-btn" aria-label="설정">
          <Settings size={16} />
        </Link>
      </header>

      <section className="library-center">
        <h1 className="library-heading">Linetta</h1>

        <div className="library-actions">
          <button className="new-button" onClick={() => setModalOpen(true)}>
            <Plus size={16} />
            <span>새 작품</span>
          </button>
          <button
            className="new-button"
            onClick={handleImport}
            disabled={importing || pending !== null}
          >
            <Upload size={16} />
            <span>{importing ? "가져오는 중…" : "가져오기 (.md)"}</span>
          </button>
          <button className="new-button" onClick={() => setSearchOpen(true)}>
            <Search size={16} />
            <span>검색</span>
          </button>
        </div>

        {loading ? (
          <p className="hint">불러오는 중…</p>
        ) : error ? (
          <p className="error">{error}</p>
        ) : recent.length === 0 ? null : (
          <>
            <p className="library-label">최근 작품 · {recent.length}개</p>
            <div className="card-grid">
              {recent.map((p) => (
                <ProjectCard key={p.id} project={p} />
              ))}
            </div>
            {totalRecent > RECENT_LIMIT && (
              <Link to="/library/all" className="library-all-link">
                전체 라이브러리 →
              </Link>
            )}
          </>
        )}
      </section>

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
            <h2>쓰기 안전 체크리스트</h2>
            <ul className="safety-list">
              <li>
                <span>데이터</span>
                <code>{diagnostics?.home ?? "확인 중"}</code>
              </li>
              <li>
                <span>백업</span>
                <code>{settingsRow?.backup_dir ?? "확인 중"}</code>
              </li>
              <li>
                <span>Git sync</span>
                <strong>{settingsRow?.git_sync_dir ? "설정됨" : "비활성"}</strong>
              </li>
            </ul>
            <div className="modal-actions">
              <button type="button" onClick={() => navigate("/settings")}>Git sync 설정</button>
              <button type="button" onClick={dismissSafety}>다시 보지 않기</button>
            </div>
          </div>
        </div>
      )}

      {diagnosticsOpen && diagnostics && (
        <div className="modal-backdrop" role="dialog" aria-modal="true">
          <div className="modal diagnostics-modal">
            <h2>진단 정보</h2>
            <pre>{JSON.stringify(diagnostics, null, 2)}</pre>
            <div className="modal-actions">
              <button type="button" onClick={() => setDiagnosticsOpen(false)}>닫기</button>
            </div>
          </div>
        </div>
      )}
    </main>
  );
}
