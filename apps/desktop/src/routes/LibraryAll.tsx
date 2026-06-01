import { useEffect, useState, type CSSProperties } from "react";
import { Link, useNavigate } from "react-router-dom";
import { projects as projectsApi } from "../lib/rpc";
import type { LengthTarget, Project } from "../lib/types";

type Tab = "active" | "archived";

const SPINE_COLORS = [
  "var(--t-teal)",
  "var(--t-sienna)",
  "var(--t-blue)",
  "var(--t-plum)",
  "var(--t-olive)",
];

const LENGTH_LABEL: Record<LengthTarget, string> = {
  flash: "플래시",
  short: "단편",
  novella: "중편",
  novel: "장편",
  series: "시리즈",
};

function humanCount(words: number): string {
  if (words === 0) return "초안 시작 전";
  if (words < 10_000) return `${words.toLocaleString("ko-KR")}자`;
  return `${(words / 1000).toFixed(0)}k자`;
}

export function LibraryAll() {
  const [tab, setTab] = useState<Tab>("active");
  const [items, setItems] = useState<Project[]>([]);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();

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
    <main className="library fade-in">
      <header className="lib-top">
        <Link to="/" className="lib-shelf-all">← Library</Link>
        <div className="lib-brandmark">전체 라이브러리</div>
        <div className="lib-top-actions" />
      </header>

      <div className="lib-body">
        <div className="lib-actions">
          <button
            className={`btn ${tab === "active" ? "accent" : "ghost"}`}
            onClick={() => setTab("active")}
          >
            진행 중
          </button>
          <button
            className={`btn ${tab === "archived" ? "accent" : "ghost"}`}
            onClick={() => setTab("archived")}
          >
            보관됨
          </button>
        </div>

        {error && <p className="error">{error}</p>}

        <div className="lib-shelf-head">
          <span className="lib-shelf-title">{tab === "active" ? "진행 중" : "보관됨"} · {items.length}개</span>
        </div>

        {items.length === 0 ? (
          <p className="hint">없음</p>
        ) : (
          <div className="lib-grid">
            {items.map((p, i) => (
              <button
                key={p.id}
                className="book"
                style={{ "--spine": SPINE_COLORS[i % SPINE_COLORS.length] } as CSSProperties}
                onClick={() => navigate(`/workspace/${p.id}`)}
              >
                <h3 className="book-title">{p.title}</h3>
                <div className="book-spacer" />
                <div className="book-scenes">{LENGTH_LABEL[p.length_target]}</div>
                <div className="book-meta">{humanCount(p.word_count)}</div>
                {tab === "active" && (
                  <div
                    className="lib-shelf-all"
                    style={{ marginTop: 8 }}
                    role="button"
                    tabIndex={0}
                    onClick={(e) => { e.stopPropagation(); archive(p.id); }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.stopPropagation();
                        archive(p.id);
                      }
                    }}
                  >
                    아카이브
                  </div>
                )}
              </button>
            ))}
          </div>
        )}
      </div>
    </main>
  );
}
