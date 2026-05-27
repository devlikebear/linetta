import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { projects as projectsApi, imports as importsApi } from "../lib/rpc";
import type { Project, NewProjectInput } from "../lib/types";
import { ProjectCard } from "../components/ProjectCard";
import { NewProjectModal } from "../components/NewProjectModal";
import { pickAndReadMarkdown } from "../lib/importLoad";
import { MoreHorizontal, Settings, Plus, Upload } from "../lib/icons";
import { useToast } from "../components/ToastProvider";

const RECENT_LIMIT = 5;

export function Library() {
  const [recent, setRecent] = useState<Project[]>([]);
  const [totalRecent, setTotalRecent] = useState<number>(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);
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
      const res = await importsApi.markdown(picked.fileName, picked.content);
      navigate(`/workspace/${res.project_id}`);
    } catch (err) {
      showToast(`가져오기 실패: ${err}`);
    } finally {
      setImporting(false);
    }
  };

  return (
    <main className="library">
      <header className="library-top">
        <button className="icon-btn" aria-label="라이브러리 옵션" disabled>
          <MoreHorizontal size={16} />
        </button>
        <Link to="/settings" className="icon-btn" aria-label="설정">
          <Settings size={16} />
        </Link>
      </header>

      <section className="library-center">
        <h1 className="library-heading">Linetta</h1>

        <button className="new-button" onClick={() => setModalOpen(true)}>
          <Plus size={16} />
          <span>새 작품</span>
        </button>
        <button
          className="new-button"
          onClick={handleImport}
          disabled={importing}
          style={{ marginTop: "0.4rem" }}
        >
          <Upload size={16} />
          <span>{importing ? "가져오는 중…" : "가져오기 (.md)"}</span>
        </button>

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
    </main>
  );
}
