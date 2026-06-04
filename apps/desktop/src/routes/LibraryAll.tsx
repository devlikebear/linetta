import { useEffect, useState, type CSSProperties, type MouseEvent } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { projects as projectsApi } from "../lib/rpc";
import type { Project } from "../lib/types";
import { formatWordCount, lengthLabel, useI18n } from "../lib/i18n";
import { Archive, MoreHorizontal, RotateCcw } from "../lib/icons";

type Tab = "active" | "archived";

function tabFromSearch(search: string): Tab {
  return new URLSearchParams(search).get("tab") === "archived" ? "archived" : "active";
}

const SPINE_COLORS = [
  "var(--t-teal)",
  "var(--t-sienna)",
  "var(--t-blue)",
  "var(--t-plum)",
  "var(--t-olive)",
];

export function LibraryAll() {
  const { language, t } = useI18n();
  const location = useLocation();
  const [tab, setTab] = useState<Tab>(() => tabFromSearch(location.search));
  const [items, setItems] = useState<Project[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [menuOpenId, setMenuOpenId] = useState<string | null>(null);
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
  useEffect(() => {
    const next = tabFromSearch(location.search);
    setTab((current) => current === next ? current : next);
  }, [location.search]);

  const selectTab = (next: Tab) => {
    setTab(next);
    navigate(next === "archived" ? "/library/all?tab=archived" : "/library/all", { replace: true });
  };

  const archive = async (id: string) => {
    setMenuOpenId(null);
    await projectsApi.archive(id);
    await load(tab);
  };

  const restore = async (id: string) => {
    setMenuOpenId(null);
    await projectsApi.restore(id);
    await load(tab);
  };

  const toggleProjectMenu = (event: MouseEvent, id: string) => {
    event.preventDefault();
    event.stopPropagation();
    setMenuOpenId((current) => current === id ? null : id);
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
            onClick={() => selectTab("active")}
          >
            {t("library.active")}
          </button>
          <button
            className={`btn ${tab === "archived" ? "accent" : "ghost"}`}
            onClick={() => selectTab("archived")}
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
              <div
                key={p.id}
                className="book-wrap"
                style={{ "--spine": SPINE_COLORS[i % SPINE_COLORS.length] } as CSSProperties}
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
                  aria-expanded={menuOpenId === p.id}
                  aria-haspopup="menu"
                  onClick={(event) => toggleProjectMenu(event, p.id)}
                >
                  <MoreHorizontal size={16} />
                </button>
                {menuOpenId === p.id && (
                  <div className="lib-menu book-menu" role="menu">
                    {tab === "active" ? (
                      <button type="button" role="menuitem" onClick={() => archive(p.id)}>
                        <Archive size={15} /> <span>{t("library.archive")}</span>
                      </button>
                    ) : (
                      <button type="button" role="menuitem" onClick={() => restore(p.id)}>
                        <RotateCcw size={15} /> <span>{t("library.restore")}</span>
                      </button>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </main>
  );
}
