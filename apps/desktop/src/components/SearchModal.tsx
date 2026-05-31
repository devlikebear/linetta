import { useEffect, useRef, useState } from "react";
import { search } from "../lib/rpc";
import type { SearchResult } from "../lib/types";
import "./SearchModal.css";

interface Props {
  open: boolean;
  onClose: () => void;
  onSelect: (result: SearchResult) => void;
}

export function SearchModal({ open, onClose, onSelect }: Props) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setQuery("");
    setResults([]);
    setError(null);
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

  return (
    <div className="search-backdrop" role="dialog" aria-modal="true" onClick={onClose}>
      <div className="search-modal" onClick={(e) => e.stopPropagation()}>
        <input
          ref={inputRef}
          className="search-input"
          placeholder="작품, 씬, 본문 검색"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              onClose();
            }
          }}
        />

        <div className="search-results">
          {loading && <p className="search-state">검색 중...</p>}
          {error && <p className="search-state error">검색 실패: {error}</p>}
          {!loading && !error && query.trim() && results.length === 0 && (
            <p className="search-state">결과 없음</p>
          )}
          {!query.trim() && (
            <p className="search-state">찾을 단어를 입력하세요</p>
          )}
          {results.map((result) => (
            <button
              key={`${result.project_id}:${result.node_id}`}
              type="button"
              className="search-result"
              onClick={() => select(result)}
            >
              <span className="search-result-title">
                {result.project_title}
              </span>
              <span className="search-result-node">
                {result.node_label}{result.node_title ? ` · ${result.node_title}` : ""}
              </span>
              <span className="search-result-preview">{result.preview}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
