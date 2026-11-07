import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";

import { useI18n } from "../lib/i18n";
import type { MessageKey } from "../lib/i18n";
import { ExternalLink, Search } from "../lib/icons";
import { openExternalUrl, worksApi } from "../lib/rpc";
import { rpcErrorMessage } from "../lib/rpcMessage";
import type { WorksGalleryPage, WorksGalleryWork } from "../lib/types";
import "./WorksGallery.css";

/** The site's genre keys (linetta-artworks lib/genres.js). They are in the
 *  site's addresses, so they do not change. */
export const WORKS_GENRES = [
  "fantasy",
  "modern-fantasy",
  "romance",
  "romance-fantasy",
  "martial",
  "sf",
  "mystery",
  "horror",
  "literary",
  "etc",
] as const;

const MAX_QUERY = 50;

function genreKey(genre: string): MessageKey {
  return `works.genre.${(WORKS_GENRES as readonly string[]).includes(genre) ? genre : "etc"}` as MessageKey;
}

// Covers by work and hash, for as long as the app runs: going back and forth
// between pages must not fetch them again. A new hash is a new cover.
const covers = new Map<string, Promise<string | null>>();

function loadCover(work: WorksGalleryWork): Promise<string | null> {
  const key = `${work.id}:${work.cover}`;
  let cached = covers.get(key);
  if (!cached) {
    cached = worksApi
      .cover(work.id)
      .then((r) => r.data_url)
      .catch(() => {
        covers.delete(key);
        return null;
      });
    covers.set(key, cached);
  }
  return cached;
}

function Cover({ work }: { work: WorksGalleryWork }) {
  const [src, setSrc] = useState<string | null>(null);
  useEffect(() => {
    if (!work.cover) return undefined;
    let live = true;
    void loadCover(work).then((url) => {
      if (live) setSrc(url);
    });
    return () => {
      live = false;
    };
  }, [work]);
  if (src) return <img className="works-cover" src={src} alt="" loading="lazy" />;
  return (
    <div className="works-cover works-cover-blank" aria-hidden="true">
      <span>{work.title.slice(0, 1)}</span>
    </div>
  );
}

/** What readers see on Linetta Works, inside the app. Opening a work goes to
 *  its page on the site, where it is read. */
export function WorksGallery() {
  const { t } = useI18n();
  const [query, setQuery] = useState("");
  const [submitted, setSubmitted] = useState("");
  const [genre, setGenre] = useState("");
  const [items, setItems] = useState<WorksGalleryWork[]>([]);
  const [page, setPage] = useState<WorksGalleryPage | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  // Only the newest request may land: typing fast or switching genres must not
  // let an older page overwrite the one asked for.
  const request = useRef(0);

  const load = useCallback(async (q: string, g: string, n: number) => {
    const id = ++request.current;
    setLoading(true);
    setError(null);
    try {
      const next = await worksApi.gallery({ q, genre: g, page: n });
      if (id !== request.current) return;
      setPage(next);
      setItems((prev) => (n === 1 ? next.items : [...prev, ...next.items]));
    } catch (e) {
      if (id === request.current) setError(e);
    } finally {
      if (id === request.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load(submitted, genre, 1);
  }, [load, submitted, genre]);

  const search = (e: FormEvent) => {
    e.preventDefault();
    setSubmitted(query.trim().replace(/\s+/g, " ").slice(0, MAX_QUERY));
  };

  const open = (url: string) => {
    void openExternalUrl(url).catch((e: unknown) => setError(e));
  };

  const more = page !== null && page.page < page.total_pages;

  return (
    <main className="library works-gallery fade-in" data-testid="works-gallery">
      <header className="lib-top">
        <Link to="/" className="lib-shelf-all">
          ← {t("settings.backToLibrary")}
        </Link>
        <div className="lib-brandmark">Linetta Works</div>
        <div className="lib-top-actions">
          {page?.site && (
            <button
              type="button"
              className="lib-icon-btn"
              aria-label={t("works.openSite")}
              title={t("works.openSite")}
              onClick={() => open(page.site)}
              data-testid="works-open-site"
            >
              <ExternalLink size={17} />
            </button>
          )}
        </div>
      </header>

      <div className="lib-body">
        <p className="works-intro">{t("works.intro")}</p>
        <form className="works-search" role="search" onSubmit={search}>
          <Search size={15} aria-hidden="true" />
          <input
            type="search"
            value={query}
            maxLength={MAX_QUERY}
            placeholder={t("works.searchPlaceholder")}
            aria-label={t("works.searchPlaceholder")}
            onChange={(e) => setQuery(e.target.value)}
            data-testid="works-search"
          />
          <button type="submit" className="btn sm" data-testid="works-search-go">
            {t("works.search")}
          </button>
        </form>
        <div className="works-genres" role="tablist" aria-label={t("works.genres")}>
          {["", ...WORKS_GENRES].map((key) => (
            <button
              key={key || "all"}
              type="button"
              role="tab"
              aria-selected={genre === key}
              className={`works-genre${genre === key ? " on" : ""}`}
              onClick={() => setGenre(key)}
              data-testid={`works-genre-${key || "all"}`}
            >
              {key ? t(genreKey(key)) : t("works.genre.all")}
            </button>
          ))}
        </div>

        {page && (
          <div className="lib-shelf-head">
            <span className="lib-shelf-title" data-testid="works-total">
              {submitted ? t("works.foundFor", { q: submitted, count: page.total }) : t("works.count", { count: page.total })}
            </span>
          </div>
        )}

        {error ? (
          <div className="works-error" role="alert" data-testid="works-error">
            <p className="sd">{t("works.loadFailed")}</p>
            <p className="sd">{rpcErrorMessage(error, t)}</p>
            <button type="button" className="btn ghost sm" onClick={() => void load(submitted, genre, 1)}>
              {t("works.retry")}
            </button>
          </div>
        ) : null}

        {!error && page && items.length === 0 && !loading && (
          <p className="hint" data-testid="works-empty">
            {submitted ? t("works.noMatch") : t("works.empty")}
          </p>
        )}

        <ul className="works-grid" data-testid="works-grid">
          {items.map((work) => (
            <li key={work.id}>
              <button
                type="button"
                className="works-card"
                onClick={() => open(work.url)}
                title={t("works.openWork", { title: work.title })}
                data-testid={`works-card-${work.id}`}
              >
                <Cover work={work} />
                <span className="works-card-body">
                  <span className="works-card-title">{work.title}</span>
                  <span className="works-card-author">{work.author.pen_name || `@${work.author.handle}`}</span>
                  <span className="works-card-meta">
                    {t(genreKey(work.category))} · {t("works.episodes", { count: work.episode_count })} ·{" "}
                    {work.serial_status === "completed" ? t("works.completed") : t("works.ongoing")}
                  </span>
                  {work.blurb && <span className="works-card-blurb">{work.blurb}</span>}
                </span>
              </button>
            </li>
          ))}
        </ul>

        {loading && (
          <p className="hint" role="status" data-testid="works-loading">
            {t("works.loading")}
          </p>
        )}
        {more && !loading && !error && (
          <div className="works-more">
            <button
              type="button"
              className="btn ghost"
              onClick={() => void load(submitted, genre, page.page + 1)}
              data-testid="works-more"
            >
              {t("works.more")}
            </button>
          </div>
        )}
      </div>
    </main>
  );
}
