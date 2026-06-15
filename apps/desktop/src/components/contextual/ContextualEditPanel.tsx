import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode, RefObject } from "react";
import { ChevronLeft, ChevronRight, FileText, Replace, Search, X } from "lucide-react";
import { manuscript } from "../../lib/rpc";
import type { ManuscriptSearchHit } from "../../lib/types";
import { localeForLanguage, useI18n } from "../../lib/i18n";
import type { TiptapFindResult, TiptapHandle } from "../editor/Tiptap";
import "./ContextualEditPanel.css";

type Scope = "scene" | "project";

interface Props {
  open: boolean;
  projectId: string;
  currentNodeId: string;
  editorRef: RefObject<TiptapHandle>;
  onNavigateNode: (nodeId: string) => void;
  onClose: () => void;
}

function highlight(text: string, query: string) {
  const needle = query.trim();
  if (!needle) return text;
  const lower = text.toLocaleLowerCase();
  const normalized = needle.toLocaleLowerCase();
  const parts: ReactNode[] = [];
  let cursor = 0;
  let key = 0;

  while (cursor < text.length) {
    const at = lower.indexOf(normalized, cursor);
    if (at < 0) {
      parts.push(<Fragment key={key++}>{text.slice(cursor)}</Fragment>);
      break;
    }
    if (at > cursor) parts.push(<Fragment key={key++}>{text.slice(cursor, at)}</Fragment>);
    parts.push(<mark key={key++}>{text.slice(at, at + needle.length)}</mark>);
    cursor = at + needle.length;
  }

  return parts;
}

function formatUpdatedAt(language: ReturnType<typeof useI18n>["language"], value?: number): string {
  if (!value) return "";
  return new Intl.DateTimeFormat(localeForLanguage(language), {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value * 1000));
}

export function ContextualEditPanel({ open, projectId, currentNodeId, editorRef, onNavigateNode, onClose }: Props) {
  const { language, t } = useI18n();
  const [scope, setScope] = useState<Scope>("scene");
  const [sceneQuery, setSceneQuery] = useState("");
  const [replacement, setReplacement] = useState("");
  const [findResult, setFindResult] = useState<TiptapFindResult>({ count: 0, activeIndex: -1 });
  const [sceneFindCommitted, setSceneFindCommitted] = useState(false);
  const [notice, setNotice] = useState("");
  const [projectQuery, setProjectQuery] = useState("");
  const [projectResults, setProjectResults] = useState<ManuscriptSearchHit[]>([]);
  const [projectLoading, setProjectLoading] = useState(false);
  const [projectError, setProjectError] = useState("");
  const sceneInputRef = useRef<HTMLInputElement>(null);
  const projectInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    window.setTimeout(() => {
      if (scope === "scene") sceneInputRef.current?.focus();
      else projectInputRef.current?.focus();
    }, 0);
  }, [open, scope]);

  useEffect(() => {
    if (!open) return;
    setFindResult({ count: 0, activeIndex: -1 });
    setNotice("");
    setSceneQuery("");
    setReplacement("");
    setSceneFindCommitted(false);
    setProjectQuery("");
    setProjectResults([]);
    setProjectError("");
  }, [currentNodeId, open]);

  useEffect(() => {
    if (!open || scope !== "scene") return;
    const query = sceneQuery.trim();
    if (!query) {
      setFindResult({ count: 0, activeIndex: -1 });
      setSceneFindCommitted(false);
      setNotice("");
      return;
    }
    setFindResult(editorRef.current?.findText(query, { select: false }) ?? { count: 0, activeIndex: -1 });
    setSceneFindCommitted(false);
  }, [editorRef, open, sceneQuery, scope]);

  useEffect(() => {
    if (!open || scope !== "project") return;
    const query = projectQuery.trim();
    if (!query) {
      setProjectResults([]);
      setProjectLoading(false);
      setProjectError("");
      return;
    }

    let cancelled = false;
    const timer = window.setTimeout(() => {
      setProjectLoading(true);
      setProjectError("");
      manuscript.search(projectId, query, 20)
        .then((hits) => {
          if (cancelled) return;
          setProjectResults(hits);
        })
        .catch((err) => {
          if (cancelled) return;
          setProjectResults([]);
          setProjectError(String(err));
        })
        .finally(() => {
          if (!cancelled) setProjectLoading(false);
        });
    }, 220);

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [open, projectId, projectQuery, scope]);

  const matchLabel = useMemo(() => {
    if (findResult.count <= 0) return t("contextual.find.noMatches");
    return t("contextual.find.count", {
      current: String(findResult.activeIndex + 1),
      total: String(findResult.count),
    });
  }, [findResult, t]);

  if (!open) return null;

  const commitSceneFind = () => {
    const query = sceneQuery.trim();
    if (!query) {
      const empty = { count: 0, activeIndex: -1 };
      setFindResult(empty);
      setSceneFindCommitted(false);
      return empty;
    }
    const result = editorRef.current?.findText(query, { select: true }) ?? { count: 0, activeIndex: -1 };
    setFindResult(result);
    setSceneFindCommitted(result.count > 0);
    setNotice("");
    return result;
  };

  const runNext = () => {
    if (findResult.count <= 0) return;
    const base = sceneFindCommitted ? findResult : commitSceneFind();
    if (base.count <= 0) return;
    editorRef.current?.nextMatch();
    setFindResult({ count: base.count, activeIndex: (base.activeIndex + 1) % base.count });
    setSceneFindCommitted(true);
  };

  const runPrev = () => {
    if (findResult.count <= 0) return;
    const base = sceneFindCommitted ? findResult : commitSceneFind();
    if (base.count <= 0) return;
    editorRef.current?.prevMatch();
    setFindResult({ count: base.count, activeIndex: (base.activeIndex - 1 + base.count) % base.count });
    setSceneFindCommitted(true);
  };

  const replaceCurrent = () => {
    if (!sceneQuery.trim() || findResult.count <= 0) return;
    const base = sceneFindCommitted ? findResult : commitSceneFind();
    if (base.count <= 0) return;
    const updated = editorRef.current?.replaceActiveMatch(replacement);
    if (!updated) return;
    const result = editorRef.current?.findText(sceneQuery, { select: true }) ?? { count: 0, activeIndex: -1 };
    setFindResult(result);
    setSceneFindCommitted(result.count > 0);
    setNotice(t("contextual.replace.currentDone"));
  };

  const replaceSceneAll = () => {
    if (!sceneQuery.trim()) return;
    const updated = editorRef.current?.replaceAllMatches(sceneQuery, replacement);
    if (!updated) return;
    const result = editorRef.current?.findText(sceneQuery, { select: true }) ?? { count: 0, activeIndex: -1 };
    setFindResult(result);
    setSceneFindCommitted(result.count > 0);
    setNotice(t("contextual.replace.sceneAllDone"));
  };

  const showProjectEmpty = !projectLoading && !projectError && projectQuery.trim() && projectResults.length === 0;

  return (
    <aside className="panel contextual-panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl"><span className="ic"><Replace size={16} /></span> {t("contextual.title")}</span>
        <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}><X size={16} /></button>
      </div>

      <div className="contextual-scope" role="tablist" aria-label={t("contextual.scope.label")}>
        <button
          type="button"
          role="tab"
          aria-selected={scope === "scene"}
          className={scope === "scene" ? "on" : ""}
          onClick={() => setScope("scene")}
        >
          <FileText size={14} /> {t("contextual.scope.scene")}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={scope === "project"}
          className={scope === "project" ? "on" : ""}
          onClick={() => setScope("project")}
        >
          <Search size={14} /> {t("contextual.scope.project")}
        </button>
      </div>

      {scope === "scene" ? (
        <div className="panel-scroll contextual-scroll">
          <section className="contextual-section">
            <h4>{t("contextual.find.title")}</h4>
            <label className="contextual-input">
              <span>{t("contextual.find.inputLabel")}</span>
              <input
                ref={sceneInputRef}
                aria-label={t("contextual.find.inputLabel")}
                value={sceneQuery}
                placeholder={t("contextual.find.placeholder")}
                onChange={(event) => setSceneQuery(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key !== "Enter") return;
                  event.preventDefault();
                  commitSceneFind();
                }}
              />
            </label>
            <div className="contextual-match-row">
              <span className="contextual-count">{matchLabel}</span>
              <span className="contextual-nav">
                <button type="button" onClick={runPrev} disabled={findResult.count <= 0} aria-label={t("contextual.find.prev")}>
                  <ChevronLeft size={14} />
                </button>
                <button type="button" onClick={runNext} disabled={findResult.count <= 0} aria-label={t("contextual.find.next")}>
                  <ChevronRight size={14} />
                </button>
              </span>
            </div>
          </section>

          <section className="contextual-section">
            <h4>{t("contextual.replace.title")}</h4>
            <label className="contextual-input">
              <span>{t("contextual.replace.inputLabel")}</span>
              <input
                aria-label={t("contextual.replace.inputLabel")}
                value={replacement}
                placeholder={t("contextual.replace.placeholder")}
                onChange={(event) => setReplacement(event.target.value)}
              />
            </label>
            <div className="contextual-actions">
              <button type="button" onClick={replaceCurrent} disabled={!sceneQuery.trim() || findResult.count <= 0}>
                {t("contextual.replace.current")}
              </button>
              <button type="button" onClick={replaceSceneAll} disabled={!sceneQuery.trim() || findResult.count <= 0}>
                {t("contextual.replace.sceneAll")}
              </button>
            </div>
            {notice && <p className="contextual-notice">{notice}</p>}
          </section>
        </div>
      ) : (
        <div className="panel-scroll contextual-scroll">
          <section className="contextual-section">
            <h4>{t("contextual.project.title")}</h4>
            <label className="contextual-input">
              <span>{t("contextual.project.inputLabel")}</span>
              <input
                ref={projectInputRef}
                aria-label={t("contextual.project.inputLabel")}
                value={projectQuery}
                placeholder={t("contextual.find.placeholder")}
                onChange={(event) => setProjectQuery(event.target.value)}
              />
            </label>
            <button type="button" className="contextual-disabled-action" disabled>
              {t("contextual.projectReplace.disabled")}
            </button>
          </section>

          <div className="contextual-results">
            {projectLoading && <p className="contextual-empty">{t("contextual.project.loading")}</p>}
            {projectError && <p className="contextual-empty error">{t("contextual.project.failed", { error: projectError })}</p>}
            {!projectQuery.trim() && !projectLoading && <p className="contextual-empty">{t("contextual.project.emptyPrompt")}</p>}
            {showProjectEmpty && <p className="contextual-empty">{t("contextual.project.noResults")}</p>}
            {projectResults.map((hit) => (
              <button
                key={hit.node_id}
                type="button"
                className="contextual-result"
                onClick={() => onNavigateNode(hit.node_id)}
              >
                <span className="contextual-result-top">
                  <span className="contextual-breadcrumb">{hit.breadcrumb}</span>
                  {hit.updated_at && <span className="contextual-date">{formatUpdatedAt(language, hit.updated_at)}</span>}
                </span>
                <span className="contextual-snippet">{highlight(hit.snippet, projectQuery)}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </aside>
  );
}
