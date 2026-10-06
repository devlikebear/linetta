import { useEffect, useState } from "react";
import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { projects as projectsApi, style as styleApi } from "../lib/rpc";
import type { StyleApproveMode, StyleCheckResult, StyleDraftState, StyleViolation } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";
import "./StyleSection.css";

/** A work's style, in the three forms Linetta keeps it.
 *
 *  Style notes are prose: how the writing should read. An agent gets them in
 *  every brief. The rules are the part a machine can hold a draft against —
 *  phrases to keep out, how long a sentence may run — and the check reports
 *  where a scene breaks them without editing anything (#162).
 *
 *  The third form is a draft: a profile an agent wrote from the writer's own
 *  scenes (#163). It is the agent's reading, not the writer's wish, so it
 *  waits here until the writer approves it, edits it, or throws it away.
 *  Nothing reads a draft when a brief is built.
 */

interface Props {
  projectId: string;
  /** The scene open in the editor, for the "this scene" scope. */
  nodeId?: string;
  /** Opens a scene from a violation row. */
  onOpenNode?: (nodeId: string) => void;
  /** Flushes the editor's pending autosave, so the check reads what is on screen. */
  onBeforeCheck?: () => Promise<void>;
}

type Scope = "scene" | "work";

/** One phrase per line in the box; the engine trims and de-duplicates. */
function toPhrases(text: string): string[] {
  return text.split("\n").map((line) => line.trim()).filter((line) => line !== "");
}

export function StyleSection({ projectId, nodeId, onOpenNode, onBeforeCheck }: Readonly<Props>) {
  const { t } = useI18n();
  const [notes, setNotes] = useState<string | null>(null);
  const [phrases, setPhrases] = useState<string | null>(null);
  const [maxSentence, setMaxSentence] = useState("");
  const [draft, setDraft] = useState<StyleDraftState | null>(null);
  const [draftBody, setDraftBody] = useState("");
  const [draftRevision, setDraftRevision] = useState(0);
  const [busy, setBusy] = useState<"notes" | "rules" | "check" | "draft" | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [report, setReport] = useState<StyleCheckResult | null>(null);

  useEffect(() => {
    let cancelled = false;
    Promise.all([projectsApi.get(projectId), styleApi.getRules(projectId)]).then(([project, rules]) => {
      if (cancelled) return;
      setNotes(project.style_notes ?? "");
      setPhrases(rules.avoid_phrases.join("\n"));
      setMaxSentence(rules.max_sentence_chars > 0 ? String(rules.max_sentence_chars) : "");
    }).catch((e) => { if (!cancelled) setError(e); });
    return () => { cancelled = true; };
  }, [projectId]);

  // The draft arrives from outside this component — an agent's tool call — so
  // it reloads on the engine's change event rather than only on mount.
  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.project_id && event.project_id !== projectId) return;
    if (event.tool === "linetta_propose_style_profile") setDraftRevision((n) => n + 1);
  });
  useEffect(() => {
    let cancelled = false;
    styleApi.getDraft(projectId).then((state) => {
      if (cancelled) return;
      setDraft(state);
      setDraftBody(state.pending ? state.draft.body : "");
    }).catch((e) => { if (!cancelled) setError(e); });
    return () => { cancelled = true; };
  }, [projectId, draftRevision]);

  const run = async (kind: NonNullable<typeof busy>, work: () => Promise<void>) => {
    setBusy(kind);
    setError(null);
    try {
      await work();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  const saveNotes = () => run("notes", async () => {
    const saved = await projectsApi.update({ id: projectId, style_notes: notes ?? "" });
    setNotes(saved.style_notes ?? "");
  });

  const saveRules = () => run("rules", async () => {
    const limit = maxSentence.trim() === "" ? 0 : Number(maxSentence);
    const saved = await styleApi.setRules({
      project_id: projectId,
      avoid_phrases: toPhrases(phrases ?? ""),
      max_sentence_chars: Number.isFinite(limit) ? Math.trunc(limit) : 0,
    });
    setPhrases(saved.avoid_phrases.join("\n"));
    setMaxSentence(saved.max_sentence_chars > 0 ? String(saved.max_sentence_chars) : "");
    // A report from the old rules would now be describing rules that are gone.
    setReport(null);
  });

  const check = (scope: Scope) => run("check", async () => {
    await onBeforeCheck?.();
    setReport(await styleApi.check(projectId, scope === "scene" ? nodeId : undefined));
  });

  const approve = (mode: StyleApproveMode) => run("draft", async () => {
    const result = await styleApi.approveDraft(projectId, draftBody, mode);
    setNotes(result.style_notes);
    setDraft((prev) => (prev ? { ...prev, pending: false } : prev));
  });

  const discard = () => run("draft", async () => {
    await styleApi.discardDraft(projectId);
    setDraft((prev) => (prev ? { ...prev, pending: false } : prev));
  });

  const working = busy != null;
  const limit = draft?.limit ?? 0;
  return (
    <details className="sec es-field style-section">
      <summary>
        {t("style.title")}
        {draft?.pending && <span className="style-badge">{t("style.draftBadge")}</span>}
      </summary>
      {error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}

      {draft?.pending && (
        <div className="style-draft" role="group" aria-label={t("style.draftTitle")}>
          <p className="style-draft-title">{t("style.draftTitle")}</p>
          <p className="style-hint">{t("style.draftHint")}</p>
          <textarea
            rows={6}
            aria-label={t("style.draftBody")}
            value={draftBody}
            disabled={working}
            onChange={(e) => setDraftBody(e.target.value)}
          />
          <p className="style-hint">{t("style.draftCount", { count: [...draftBody].length, limit })}</p>
          <div className="style-actions">
            <button type="button" className="btn accent sm" disabled={working || draftBody.trim() === ""} onClick={() => void approve("append")}>
              {t("style.draftAppend")}
            </button>
            <button type="button" className="btn sm" disabled={working || draftBody.trim() === ""} onClick={() => void approve("replace")}>
              {t("style.draftReplace")}
            </button>
            <button type="button" className="btn sm" disabled={working} onClick={() => void discard()}>
              {t("style.draftDiscard")}
            </button>
          </div>
        </div>
      )}

      {notes != null && phrases != null && <>
        <label>{t("style.notes")}
          <textarea
            rows={4}
            value={notes}
            disabled={working}
            placeholder={t("style.notesPlaceholder")}
            onChange={(e) => setNotes(e.target.value)}
          />
        </label>
        <div className="style-actions">
          <button type="button" className="btn accent sm" disabled={working} onClick={() => void saveNotes()}>
            {busy === "notes" ? t("common.saving") : t("style.saveNotes")}
          </button>
        </div>

        <p className="style-hint style-rules-hint">{t("style.hint")}</p>
        <label>{t("style.avoidPhrases")}
          <textarea
            rows={4}
            value={phrases}
            disabled={working}
            placeholder={t("style.avoidPhrasesPlaceholder")}
            onChange={(e) => setPhrases(e.target.value)}
          />
        </label>
        <label>{t("style.maxSentence")}
          <input
            type="number"
            min={0}
            max={2000}
            inputMode="numeric"
            value={maxSentence}
            disabled={working}
            placeholder={t("style.maxSentencePlaceholder")}
            onChange={(e) => setMaxSentence(e.target.value)}
          />
        </label>
        <div className="style-actions">
          <button type="button" className="btn accent sm" disabled={working} onClick={() => void saveRules()}>
            {busy === "rules" ? t("common.saving") : t("style.save")}
          </button>
          {nodeId && (
            <button type="button" className="btn sm" disabled={working} onClick={() => void check("scene")}>
              {t("style.checkScene")}
            </button>
          )}
          <button type="button" className="btn sm" disabled={working} onClick={() => void check("work")}>
            {t("style.checkWork")}
          </button>
        </div>
      </>}
      {busy === "check" && <p className="style-hint">{t("style.checking")}</p>}
      {report && busy !== "check" && <StyleReport report={report} onOpenNode={onOpenNode} />}
    </details>
  );
}

function StyleReport({ report, onOpenNode }: Readonly<{ report: StyleCheckResult; onOpenNode?: (nodeId: string) => void }>) {
  const { t } = useI18n();
  const noRules = report.rules.avoid_phrases.length === 0 && report.rules.max_sentence_chars === 0;
  if (noRules) {
    // Not "no violations": nothing was checked, and saying otherwise would be
    // a clean bill of health nobody earned.
    return <p className="style-result" role="status">{t("style.noRules")}</p>;
  }
  if (report.total === 0) {
    return <p className="style-result" role="status">{t("style.clean", { scenes: report.scenes_checked })}</p>;
  }
  return (
    <div className="style-result" role="status">
      <p>{t("style.found", { total: report.total, scenes: report.scenes_checked })}</p>
      {report.truncated && <p className="style-hint">{t("style.truncated", { shown: report.violations.length })}</p>}
      <ul className="style-violations">
        {report.violations.map((v, i) => (
          // A report is immutable and its rows have no ids, so position is the key.
          <li key={`${v.node_id}-${v.paragraph}-${i}`}>
            <ViolationRow v={v} onOpenNode={onOpenNode} />
          </li>
        ))}
      </ul>
    </div>
  );
}

function ViolationRow({ v, onOpenNode }: Readonly<{ v: StyleViolation; onOpenNode?: (nodeId: string) => void }>) {
  const { t } = useI18n();
  const where = t("style.where", { label: v.label, paragraph: v.paragraph });
  const what = v.rule === "sentence_too_long"
    ? t("style.ruleSentence", { length: v.length ?? 0, limit: v.limit ?? 0 })
    : t("style.rulePhrase", { phrase: v.phrase ?? "" });
  return (
    <>
      {onOpenNode
        ? <button type="button" className="style-where" onClick={() => onOpenNode(v.node_id)}>{where}</button>
        : <span className="style-where">{where}</span>}
      <span className="style-rule">{what}</span>
      <span className="style-excerpt">{v.excerpt}</span>
    </>
  );
}
