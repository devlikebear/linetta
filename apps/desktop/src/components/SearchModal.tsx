import { Fragment, useEffect, useRef, useState } from "react";
import { Search as SearchIcon } from "lucide-react";
import { search } from "../lib/rpc";
import type { SearchResult } from "../lib/types";
import "./SearchModal.css";

interface Props {
  open: boolean;
  onClose: () => void;
  onSelect: (result: SearchResult) => void;
}

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

export function SearchModal({ open, onClose, onSelect }: Props) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult[]>([]);
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
    if (!open) return;
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
  }, [open, query]);

  if (!open) return null;

  const select = (result: SearchResult) => {
    onSelect(result);
    onClose();
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((i) => Math.min(results.length - 1, i + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((i) => Math.max(0, i - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const r = results[active];
      if (r) select(r);
    }
  };

  const showEmpty = !loading && !error && query.trim() && results.length === 0;

  return (
    <div className="backdrop top" role="dialog" aria-modal="true" onClick={onClose}>
      <div className="palette" onClick={(e) => e.stopPropagation()} onKeyDown={onKeyDown}>
        <div className="palette-input-wrap">
          <span className="ic"><SearchIcon size={17} /></span>
          <input
            ref={inputRef}
            className="palette-input"
            placeholder="작품 전체 검색…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>

        <div className="palette-list">
          {loading && <p className="palette-empty">검색 중…</p>}
          {error && <p className="palette-empty">검색 실패: {error}</p>}
          {showEmpty && <p className="palette-empty">결과 없음</p>}
          {!query.trim() && !loading && <p className="palette-empty">찾을 단어를 입력하세요</p>}
          {results.map((result, i) => (
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
                  {result.node_label}{result.node_title ? ` · ${result.node_title}` : ""}
                </span>
              </div>
              <div className="sr-snip">{highlight(result.preview, query)}</div>
            </button>
          ))}
        </div>

        <div className="palette-foot"><span>제목 · 씬 · 본문에서 검색</span></div>
      </div>
    </div>
  );
}
