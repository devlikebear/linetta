import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { projects as projectsApi } from "../lib/rpc";
import type { Project } from "../lib/types";

export function Workspace() {
  const { projectId } = useParams();
  const [project, setProject] = useState<Project | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!projectId) return;
    projectsApi.get(projectId).then(setProject).catch((e) => setError(String(e)));
  }, [projectId]);

  return (
    <main className="shell">
      <p>
        <Link to="/">← Library</Link>
      </p>
      {error && <p className="error">{error}</p>}
      {!error && !project && <p className="hint">불러오는 중…</p>}
      {project && (
        <>
          <h2>{project.title}</h2>
          <p className="hint">
            장르: {project.genres.join(", ") || "—"} · {project.length_target} · {project.default_pov}
          </p>
          <p className="hint">
            <code>씬 1</code>의 본문 편집기는 Plan 2에서 추가됩니다.
          </p>
        </>
      )}
    </main>
  );
}
