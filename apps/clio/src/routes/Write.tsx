import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { FindingList } from "../components/FindingList";
import {
  HighlightedTextarea,
  type HighlightedTextareaHandle,
  type HighlightRange,
} from "../components/HighlightedTextarea";
import { Toast } from "../components/Toast";
import { CODE_CONTENT_CONFLICT, check, drafts } from "../lib/drafts";
import { t, tDynamic } from "../lib/i18n";
import { findingRange } from "../lib/offsets";
import { RpcError } from "../lib/rpc";
import type { Draft, Finding, Platform, PostKind } from "../lib/types";
import { X_LIMIT, xLength } from "../lib/xLength";
import "./Write.css";

export const CHECK_DELAY_MS = 300;
export const SAVE_DELAY_MS = 1000;

const CONTEXTS = ["x/post", "x/reply", "threads/post", "blog/post"] as const;

function splitContext(key: string): { platform: Platform; kind: PostKind } {
  const [platform, kind] = key.split("/");
  return { platform: platform as Platform, kind: kind as PostKind };
}

const contextOf = (d: Draft) => `${d.platform}/${d.kind}`;

function preview(body: string): string {
  const line = body.split("\n").find((l) => l.trim() !== "");
  return line ? line.trim() : t("write.emptyDraft");
}

function formatTime(ms: number): string {
  return new Date(ms).toLocaleString("ko-KR", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

/** Drafts on the left, the editor and its findings on the right. */
export function Write() {
  const [list, setList] = useState<Draft[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [currentId, setCurrentId] = useState<number | null>(null);
  const [body, setBody] = useState("");
  const [ctx, setCtx] = useState<string>("x/post");
  const [findings, setFindings] = useState<Finding[]>([]);
  const [contextUsed, setContextUsed] = useState("");
  const [toast, setToast] = useState("");
  const [error, setError] = useState("");

  const editorRef = useRef<HighlightedTextareaHandle>(null);
  // What the timers and the save queue read, so they never see stale state.
  const latest = useRef<{ id: number | null; body: string; ctx: string }>({ id: null, body: "", ctx: "x/post" });
  const versionRef = useRef(1);
  const dirtyRef = useRef(false);
  const queueRef = useRef<Promise<void>>(Promise.resolve());
  const checkTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const saveTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const checkSeq = useRef(0);

  const patchList = useCallback((d: Draft) => {
    setList((prev) => [d, ...prev.filter((x) => x.id !== d.id)].sort((a, b) => b.updated_at - a.updated_at));
  }, []);

  const runCheck = useCallback(async () => {
    const seq = ++checkSeq.current;
    const cur = latest.current;
    if (cur.id == null || cur.body.trim() === "") {
      setFindings([]);
      setContextUsed("");
      return;
    }
    try {
      const res = await check.run({ body: cur.body, ...splitContext(cur.ctx) });
      if (seq !== checkSeq.current) return;
      setFindings(res.findings);
      setContextUsed(res.context_used);
    } catch (e) {
      if (seq === checkSeq.current) setError(t("write.checkFailed", { error: errText(e) }));
    }
  }, []);

  const applyDraft = useCallback((d: Draft) => {
    latest.current = { id: d.id, body: d.body, ctx: contextOf(d) };
    versionRef.current = d.content_version;
    dirtyRef.current = false;
    setCurrentId(d.id);
    setBody(d.body);
    setCtx(contextOf(d));
    setError("");
  }, []);

  /** Writes the current draft if it changed. Saves run one at a time so the version never races itself. */
  const save = useCallback(() => {
    queueRef.current = queueRef.current.then(async () => {
      const cur = latest.current;
      if (cur.id == null || !dirtyRef.current) return;
      dirtyRef.current = false;
      try {
        const d = await drafts.update({
          id: cur.id,
          content_version: versionRef.current,
          body: cur.body,
          ...splitContext(cur.ctx),
        });
        if (latest.current.id === d.id) versionRef.current = d.content_version;
        patchList(d);
      } catch (e) {
        if (e instanceof RpcError && e.code === CODE_CONTENT_CONFLICT) {
          try {
            const fresh = await drafts.get(cur.id);
            patchList(fresh);
            if (latest.current.id === fresh.id) {
              applyDraft(fresh);
              setToast(t("write.conflict"));
              void runCheck();
            }
          } catch (e2) {
            setError(t("write.saveFailed", { error: errText(e2) }));
          }
        } else {
          dirtyRef.current = true;
          setError(t("write.saveFailed", { error: errText(e) }));
        }
      }
    });
    return queueRef.current;
  }, [applyDraft, patchList, runCheck]);

  const clearTimers = () => {
    clearTimeout(checkTimer.current);
    clearTimeout(saveTimer.current);
  };

  // Initial load.
  useEffect(() => {
    let cancelled = false;
    drafts
      .list()
      .then((items) => {
        if (cancelled) return;
        setList(items);
        if (items.length > 0) {
          applyDraft(items[0]);
          void runCheck();
        }
      })
      .catch((e) => {
        if (!cancelled) setError(t("common.error", { error: errText(e) }));
      })
      .finally(() => {
        if (!cancelled) setLoaded(true);
      });
    return () => {
      cancelled = true;
    };
  }, [applyDraft, runCheck]);

  // Leaving the screen: stop the timers but still write what is pending.
  useEffect(
    () => () => {
      clearTimeout(checkTimer.current);
      clearTimeout(saveTimer.current);
      if (dirtyRef.current) void save();
    },
    [save],
  );

  const edit = (nextBody: string, nextCtx: string) => {
    latest.current = { ...latest.current, body: nextBody, ctx: nextCtx };
    dirtyRef.current = true;
    setBody(nextBody);
    setCtx(nextCtx);
    clearTimers();
    checkTimer.current = setTimeout(() => void runCheck(), CHECK_DELAY_MS);
    saveTimer.current = setTimeout(() => void save(), SAVE_DELAY_MS);
  };

  const selectDraft = async (d: Draft) => {
    if (d.id === currentId) return;
    clearTimers();
    await save();
    applyDraft(d);
    setFindings([]);
    void runCheck();
  };

  const newDraft = async () => {
    clearTimers();
    await save();
    try {
      const d = await drafts.create({ ...splitContext(ctx), body: "" });
      patchList(d);
      applyDraft(d);
      setFindings([]);
      setContextUsed("");
    } catch (e) {
      setError(t("common.error", { error: errText(e) }));
    }
  };

  const removeDraft = async (d: Draft) => {
    if (!window.confirm(t("write.deleteConfirm"))) return;
    if (d.id === currentId) {
      clearTimers();
      dirtyRef.current = false;
    }
    try {
      await drafts.delete(d.id);
    } catch (e) {
      setError(t("common.error", { error: errText(e) }));
      return;
    }
    const rest = list.filter((x) => x.id !== d.id);
    setList(rest);
    if (d.id === currentId) {
      if (rest.length > 0) {
        applyDraft(rest[0]);
        void runCheck();
      } else {
        latest.current = { id: null, body: "", ctx };
        setCurrentId(null);
        setBody("");
        setFindings([]);
        setContextUsed("");
      }
    }
  };

  const ranges = useMemo<HighlightRange[]>(
    () =>
      findings.flatMap((f) => {
        const r = findingRange(body, f);
        return r ? [{ ...r, severity: f.severity }] : [];
      }),
    [findings, body],
  );

  const selectFinding = (f: Finding) => {
    const r = findingRange(body, f);
    if (r) editorRef.current?.selectRange(r.start, r.end);
  };

  const isX = ctx.startsWith("x/");
  const length = xLength(body);

  return (
    <section className="write">
      <aside className="write-list" aria-label={t("write.draftsLabel")}>
        <button type="button" className="primary" onClick={newDraft}>
          {t("write.newDraft")}
        </button>
        <ul>
          {list.map((d) => (
            <li key={d.id} className={d.id === currentId ? "draft active" : "draft"}>
              <button type="button" className="draft-open" onClick={() => selectDraft(d)} aria-current={d.id === currentId}>
                <span className="draft-preview">{preview(d.id === currentId ? body : d.body)}</span>
                <span className="draft-time muted">{formatTime(d.updated_at)}</span>
              </button>
              <button
                type="button"
                className="draft-delete"
                aria-label={`${t("write.delete")}: ${preview(d.body)}`}
                onClick={() => removeDraft(d)}
              >
                {t("write.delete")}
              </button>
            </li>
          ))}
        </ul>
      </aside>

      <div className="write-main">
        {!loaded ? (
          <p className="muted">{t("common.loading")}</p>
        ) : currentId == null ? (
          <p className="muted">{t("write.noDrafts")}</p>
        ) : (
          <>
            <div className="write-bar">
              <label htmlFor="write-context" className="muted">
                {t("write.context")}
              </label>
              <select id="write-context" value={ctx} onChange={(e) => edit(body, e.target.value)}>
                {CONTEXTS.map((c) => (
                  <option key={c} value={c}>
                    {tDynamic(`write.context.${c}`, c)}
                  </option>
                ))}
              </select>
              {isX && (
                <span className={length > X_LIMIT ? "counter num over" : "counter num"} data-testid="x-counter">
                  {t("write.counter", { n: length })}
                </span>
              )}
            </div>

            <HighlightedTextarea
              ref={editorRef}
              value={body}
              ranges={ranges}
              onChange={(v) => edit(v, ctx)}
              aria-label={t("write.bodyLabel")}
              placeholder={t("write.placeholder")}
            />

            {error && (
              <p className="error" role="alert">
                {error}
              </p>
            )}

            <h2>{t("write.findingsTitle")}</h2>
            {contextUsed && <p className="muted note">{t("write.contextUsed", { context: contextUsed })}</p>}
            <FindingList findings={findings} onSelect={selectFinding} />
          </>
        )}
      </div>
      {toast && <Toast message={toast} onDone={() => setToast("")} />}
    </section>
  );
}
