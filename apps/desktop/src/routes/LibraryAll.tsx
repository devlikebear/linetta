import { useEffect, useState, type CSSProperties } from "react";
import { Link, useNavigate } from "react-router-dom";
import { projects as projectsApi } from "../lib/rpc";
import type { Project } from "../lib/types";
import { formatWordCount, lengthLabel, useI18n } from "../lib/i18n";

type Tab = "active" | "archived";

const SPINE_COLORS = [
  "var(--t-teal)",
  "var(--t-sienna)",
  "var(--t-blue)",
  "var(--t-plum)",
  "var(--t-olive)",
];

export function LibraryAll() {
  const { language, t } = useI18n();
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
        <Link to="/" className="lib-shelf-all">← {t("settings.backToLibrary")}</Link>
        <div className="lib-brandmark">{t("library.allProjects").replace(" →", "")}</div>
        <div className="lib-top-actions" />
      </header>

      <div className="lib-body">
        <div className="lib-actions">
          <button
            className={`btn ${tab === "active" ? "accent" : "ghost"}`}
            onClick={() => setTab("active")}
          >
            {t("library.active")}
          </button>
          <button
            className={`btn ${tab === "archived" ? "accent" : "ghost"}`}
            onClick={() => setTab("archived")}
          >
            {t("library.archived")}
          </button>
        </div>

        {error && <p className="error">{error}</p>}

        <div className="lib-shelf-head">
          <span className="lib-shelf-title">
            {t("library.itemCount", { label: tab === "active" ? t("library.active") : t("library.archived"), count: items.length })}
          </span>
        </div>

        {items.length === 0 ? (
          <p className="hint">{t("library.empty")}</p>
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
                <div className="book-scenes">{lengthLabel(language, p.length_target)}</div>
                <div className="book-meta">{formatWordCount(language, p.word_count)}</div>
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
                    {t("library.archive")}
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
