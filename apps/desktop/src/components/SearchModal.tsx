import { Fragment, useEffect, useRef, useState } from "react";
import { Search as SearchIcon } from "lucide-react";
import { manuscript, search } from "../lib/rpc";
import type { ManuscriptSearchHit, PassageTarget, SearchResult } from "../lib/types";
import { megabytes, useSemanticStatus } from "../hooks/useSemanticStatus";
import { displayNodeLabel, useI18n } from "../lib/i18n";
import "./SearchModal.css";

interface Props {
  open: boolean;
  onClose: () => void;
  onSelect: (result: SearchResult) => void;
  /** The open work: with it, the box can also find by meaning (#384). */
  projectId?: string;
  /** Opens a scene at the paragraph meaning search found. */
  onReveal?: (target: PassageTarget) => void;
}

type Mode = "words" | "meaning";
const MODE_KEY = "linetta.search.mode";

function readMode(): Mode {
  try {
    return window.localStorage.getItem(MODE_KEY) === "meaning" ? "meaning" : "words";
  } catch {
    return "words";
  }
}

// A meaning search embeds the question, so it waits for a pause in typing.
const MEANING_DEBOUNCE_MS = 300;

// Highlight every case-insensitive occurrence of the query term in a snippet.
function highlight(text: string, query: string) {
  const q = query.trim();
  if (!q) return text;
  const lower = text.toLowerCase();
  const needle = q.toLowerCase();
  const parts: React.ReactNode[] = [];
  let i = 0;
  let key = 0;
  while (i < text.length) {
    const at = lower.indexOf(needle, i);
    if (at === -1) {
      parts.push(<Fragment key={key++}>{text.slice(i)}</Fragment>);
      break;
    }
    if (at > i) parts.push(<Fragment key={key++}>{text.slice(i, at)}</Fragment>);
    parts.push(<mark key={key++}>{text.slice(at, at + needle.length)}</mark>);
    i = at + needle.length;
  }
  return parts;
}

export function SearchModal({ open, onClose, onSelect, projectId, onReveal }: Props) {
  const { language, t } = useI18n();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult[]>([]);
  const [modeChoice, setModeChoice] = useState<Mode>(readMode);
  const mode: Mode = projectId && onReveal ? modeChoice : "words";
  const [meaningHits, setMeaningHits] = useState<ManuscriptSearchHit[]>([]);
  const semantic = useSemanticStatus(projectId, open && mode === "meaning");
  const sem = semantic.status;
  const meaningReady = !!sem?.enabled && !!sem.model.installed;
  const chooseMode = (next: Mode) => {
    setModeChoice(next);
    setActive(0);
    try {
      window.localStorage.setItem(MODE_KEY, next);
    } catch {
      // Remembered only as a convenience.
    }
  };
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setQuery("");
    setResults([]);
    setError(null);
    setActive(0);
    window.setTimeout(() => inputRef.current?.focus(), 0);
  }, [open]);

  useEffect(() => {
    if (!open || mode !== "meaning") return;
    const trimmed = query.trim();
    setMeaningHits([]);
    if (!trimmed || !projectId || !meaningReady) {
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    const timer = window.setTimeout(() => {
      manuscript.search(projectId, trimmed, 20, "meaning")
        .then((hits) => {
          if (cancelled) return;
          setMeaningHits(hits);
          setActive(0);
        })
        .catch((err) => {
          if (!cancelled) setError(String(err));
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    }, MEANING_DEBOUNCE_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [open, mode, query, projectId, meaningReady]);

  useEffect(() => {
    if (!open || mode !== "words") return;
    const trimmed = query.trim();
    if (!trimmed) {
      setResults([]);
      setLoading(false);
      setError(null);
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);
    search.query(trimmed, 20)
      .then((rows) => {
        if (cancelled) return;
        setResults(rows);
        setActive(0);
      })
      .catch((err) => {
        if (cancelled) return;
        setResults([]);
        setError(String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => { cancelled = true; };
  }, [open, mode, query]);

  if (!open) return null;

  const select = (result: SearchResult) => {
    onSelect(result);
    onClose();
  };
  const reveal = (hit: ManuscriptSearchHit) => {
    onReveal?.({ nodeId: hit.node_id, paragraphFrom: hit.paragraph_from ?? 0, paragraphTo: hit.paragraph_to });
    onClose();
  };
  const count = mode === "meaning" ? meaningHits.length : results.length;

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((i) => Math.min(count - 1, i + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((i) => Math.max(0, i - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (mode === "meaning") {
        const h = meaningHits[active];
        if (h) reveal(h);
        return;
      }
      const r = results[active];
      if (r) select(r);
    }
  };

  const showEmpty = !loading && !error && query.trim() && count === 0 && (mode === "words" || meaningReady);
  const notice = mode === "meaning" ? meaningNotice() : null;

  function meaningNotice() {
    if (!sem) return null;
    if (!sem.supported) return <p className="palette-empty">{t("search.meaning.unsupported")}</p>;
    if (!sem.enabled) {
      return (
        <div className="search-notice" data-testid="search-meaning-offer">
          <span>{t("search.meaning.offer", { size: megabytes(sem.model.total) })}</span>
          <button type="button" className="btn sm" onClick={() => void semantic.enable()}>{t("search.meaning.download")}</button>
        </div>
      );
    }
    if (sem.model.downloading) {
      const percent = sem.model.total ? Math.floor((100 * sem.model.received) / sem.model.total) : 0;
      return <p className="search-notice" role="status">{t("search.meaning.downloading", { percent: String(percent) })}</p>;
    }
    if (!sem.model.installed) {
      return (
        <div className="search-notice" role="status">
          <span>{sem.model.error ? t("search.meaning.downloadFailed", { error: sem.model.error }) : t("search.meaning.missing")}</span>
          <button type="button" className="btn sm" onClick={() => void semantic.enable()}>{t("search.meaning.download")}</button>
        </div>
      );
    }
    if (sem.index && sem.index.done < sem.index.total) {
      return (
        <p className="search-notice" role="status">
          {t("search.meaning.indexing", { done: sem.index.done.toLocaleString(), total: sem.index.total.toLocaleString() })}
        </p>
      );
    }
    return null;
  }

  return (
    <div className="backdrop top" role="dialog" aria-modal="true" onClick={onClose}>
      <div className="palette" onClick={(e) => e.stopPropagation()} onKeyDown={onKeyDown}>
        <div className="palette-input-wrap">
          <span className="ic"><SearchIcon size={17} /></span>
          <input
            ref={inputRef}
            className="palette-input"
            placeholder={t("search.placeholder")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        {projectId && onReveal && (
          <div className="search-modes" role="group" aria-label={t("search.mode")}>
            {(["words", "meaning"] as const).map((m) => (
              <button key={m} type="button" className={`chip${mode === m ? " on" : ""}`} aria-pressed={mode === m}
                data-testid={`search-mode-${m}`} onClick={() => chooseMode(m)}>
                {t(`search.mode.${m}`)}
              </button>
            ))}
          </div>
        )}
        {semantic.listener}
        {notice}

        <div className="palette-list">
          {loading && <p className="palette-empty">{t("search.loading")}</p>}
          {error && <p className="palette-empty">{t("search.failed", { error })}</p>}
          {showEmpty && <p className="palette-empty">{t("search.noResults")}</p>}
          {!query.trim() && !loading && <p className="palette-empty">{t("search.emptyPrompt")}</p>}
          {mode === "meaning" && meaningHits.map((hit, i) => (
            <button
              key={`${hit.node_id}:${hit.paragraph_from ?? 0}`}
              type="button"
              className={`search-result${i === active ? " active" : ""}`}
              onMouseMove={() => setActive(i)}
              onClick={() => reveal(hit)}
            >
              <div className="sr-top">
                <span className="sr-scene">{hit.breadcrumb}</span>
                {hit.paragraph_from ? (
                  <span className="sr-proj">
                    ¶{hit.paragraph_from}{hit.paragraph_to && hit.paragraph_to !== hit.paragraph_from ? `–${hit.paragraph_to}` : ""}
                  </span>
                ) : null}
              </div>
              <div className="sr-snip">{hit.snippet}</div>
            </button>
          ))}
          {mode === "words" && results.map((result, i) => (
            <button
              key={`${result.project_id}:${result.node_id}`}
              type="button"
              className={`search-result${i === active ? " active" : ""}`}
              onMouseMove={() => setActive(i)}
              onClick={() => select(result)}
            >
              <div className="sr-top">
                <span className="sr-proj">{result.project_title}</span>
                <span className="sr-scene">
                  {displayNodeLabel(language, result.node_label)}{result.node_title ? ` · ${result.node_title}` : ""}
                </span>
              </div>
              <div className="sr-snip">{highlight(result.preview, query)}</div>
            </button>
          ))}
        </div>

        <div className="palette-foot"><span>{t("search.footer")}</span></div>
      </div>
    </div>
  );
}
