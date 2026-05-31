import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { projects as projectsApi, imports as importsApi, settings as settingsApi, openPath } from "../lib/rpc";
import type { ImportPreviewResult, NewProjectInput, Project } from "../lib/types";
import { ProjectCard } from "../components/ProjectCard";
import { NewProjectModal } from "../components/NewProjectModal";
import { ImportPreviewModal } from "../components/ImportPreviewModal";
import { pickAndReadMarkdown } from "../lib/importLoad";
import { MoreHorizontal, Settings, Plus, Upload } from "../lib/icons";
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
      const current = await settingsApi.get();
      await openPath(current.backup_dir);
    } catch (err) {
      showToast(`백업 폴더 열기 실패: ${err}`);
    }
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
              <button type="button" role="menuitem" onClick={openBackupFolder}>
                백업 폴더 열기
              </button>
              <button type="button" role="menuitem" onClick={() => navigate("/settings")}>
                설정
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

      {pending && (
        <ImportPreviewModal
          preview={pending.preview}
          fileName={pending.fileName}
          busy={importing}
          onConfirm={confirmImport}
          onCancel={() => setPending(null)}
        />
      )}
    </main>
  );
}
