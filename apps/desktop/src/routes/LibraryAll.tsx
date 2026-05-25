import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { projects as projectsApi } from "../lib/rpc";
import type { Project } from "../lib/types";

type Tab = "active" | "archived";

export function LibraryAll() {
  const [tab, setTab] = useState<Tab>("active");
  const [items, setItems] = useState<Project[]>([]);
  const [error, setError] = useState<string | null>(null);

  const load = async (which: Tab) => {
    setError(null);
    try {
      const all = await projectsApi.list({ include_archived: true, limit: 200 });
      const filtered = which === "active"
        ? all.filter((p) => !p.archived_at)
        : all.filter((p) => !!p.archived_at);
      setItems(filtered);
    } catch (e) {
      setError(String(e));
    }
  };

  useEffect(() => { load(tab); }, [tab]);

  const archive = async (id: string) => {
    await projectsApi.archive(id);
    await load(tab);
  };

  return (
    <main className="shell library-all">
      <p>
        <Link to="/">← Library</Link>
      </p>
      <h2>전체 라이브러리</h2>

      <div className="tabs">
        <button className={`chip${tab === "active" ? " on" : ""}`} onClick={() => setTab("active")}>
          진행 중
        </button>
        <button className={`chip${tab === "archived" ? " on" : ""}`} onClick={() => setTab("archived")}>
          보관됨
        </button>
      </div>

      {error && <p className="error">{error}</p>}

      <ul className="all-list">
        {items.map((p) => (
          <li key={p.id} className="all-row">
            <Link to={`/workspace/${p.id}`} className="all-row-link">
              <span className="all-row-title">{p.title}</span>
              <span className="all-row-meta">{p.length_target} · {p.word_count}자</span>
            </Link>
            {tab === "active" && (
              <button className="chip" onClick={() => archive(p.id)}>아카이브</button>
            )}
          </li>
        ))}
        {items.length === 0 && <li className="hint">없음</li>}
      </ul>
    </main>
  );
}
