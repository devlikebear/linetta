import { useEffect, useState } from "react";
import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { nodes as nodesApi, projects as projectsApi, style as styleApi } from "../lib/rpc";
import type { NodeRow, StyleApproveMode, StyleCheckResult, StyleDraftState, StyleViolation, WriterStyleRules } from "../lib/types";
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
  /** Opens Settings at the writer's default style. */
  onOpenWriterStyle?: () => void;
}

type Scope = "scene" | "work";

/** One phrase per line in the box; the engine trims and de-duplicates. */
function toPhrases(text: string): string[] {
  return text.split("\n").map((line) => line.trim()).filter((line) => line !== "");
}

export function StyleSection({ projectId, nodeId, onOpenNode, onBeforeCheck, onOpenWriterStyle }: Readonly<Props>) {
  const { t } = useI18n();
  const [notes, setNotes] = useState<string | null>(null);
  const [phrases, setPhrases] = useState<string | null>(null);
  const [maxSentence, setMaxSentence] = useState("");
  const [anchor, setAnchor] = useState("");
  // The writer's default style (#181): rules this work inherits unless it
  // opts out, shown read-only here and changed only in Settings.
  const [writer, setWriter] = useState<WriterStyleRules | null>(null);
  const [followWriter, setFollowWriter] = useState(true);
  const [ownLimit, setOwnLimit] = useState(false);
  const [scenes, setScenes] = useState<NodeRow[]>([]);
  const [draft, setDraft] = useState<StyleDraftState | null>(null);
  const [draftBody, setDraftBody] = useState("");
  const [draftRevision, setDraftRevision] = useState(0);
  const [busy, setBusy] = useState<"notes" | "rules" | "check" | "draft" | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [report, setReport] = useState<StyleCheckResult | null>(null);

  useEffect(() => {
    let cancelled = false;
    Promise.all([projectsApi.get(projectId), styleApi.getRules(projectId), nodesApi.listTree(projectId)]).then(([project, rules, tree]) => {
      if (cancelled) return;
      setScenes(tree.filter((n) => n.kind === "leaf" && n.word_count > 0));
      setNotes(project.style_notes ?? "");
      setPhrases(rules.avoid_phrases.join("\n"));
      setMaxSentence(rules.max_sentence_chars > 0 ? String(rules.max_sentence_chars) : "");
      setAnchor(rules.anchor_node_id ?? "");
      setWriter(rules.writer ?? null);
      setFollowWriter(rules.ignore_writer_style !== true);
      setOwnLimit(rules.own_sentence_limit === true);
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

  // Only a writer limit makes "this work's own" a choice. Without one, the
  // box works as it always did: a number is the work's limit, empty is none.
  const limitChoice = (writer?.max_sentence_chars ?? 0) > 0;
  const usesOwnLimit = !limitChoice || ownLimit;

  const saveRules = (overrides: { followWriter?: boolean } = {}) => run("rules", async () => {
    const limit = maxSentence.trim() === "" ? 0 : Number(maxSentence);
    const follow = overrides.followWriter ?? followWriter;
    // A work that does not follow the writer's style has only its own limit.
    const own = usesOwnLimit || !follow ? (Number.isFinite(limit) ? Math.trunc(limit) : 0) : 0;
    const saved = await styleApi.setRules({
      project_id: projectId,
      avoid_phrases: toPhrases(phrases ?? ""),
      max_sentence_chars: own,
      own_sentence_limit: limitChoice && follow ? ownLimit : own > 0,
      ignore_writer_style: !follow,
      anchor_node_id: anchor,
    });
    setPhrases(saved.avoid_phrases.join("\n"));
    setMaxSentence(saved.max_sentence_chars > 0 ? String(saved.max_sentence_chars) : "");
    setFollowWriter(saved.ignore_writer_style !== true);
    setOwnLimit(saved.own_sentence_limit === true);
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
        {writer && (!writerEmpty(writer) || !followWriter) && (
          <div className="style-inherited" data-testid="style-writer-rules">
            <label className="style-toggle">
              <input
                type="checkbox"
                checked={followWriter}
                disabled={working}
                data-testid="style-follow-writer"
                onChange={(e) => {
                  setFollowWriter(e.target.checked);
                  void saveRules({ followWriter: e.target.checked });
                }}
              />
              {t("style.followWriter")}
            </label>
            <p className="style-hint">{t("style.followWriterHint")}</p>
            {followWriter && (
              <>
                <p className="style-draft-title">{t("style.writerRules")}</p>
                {writerEmpty(writer) && <p className="style-hint">{t("style.writerEmpty")}</p>}
                {writer.avoid_phrases.length > 0 && (
                  <p className="style-hint">{t("style.writerPhrases", { phrases: writer.avoid_phrases.join(", ") })}</p>
                )}
                {writer.max_sentence_chars > 0 && !ownLimit && (
                  <p className="style-hint">{t("style.writerLimit", { limit: writer.max_sentence_chars })}</p>
                )}
              </>
            )}
            {onOpenWriterStyle && (
              <button type="button" className="btn ghost sm" onClick={onOpenWriterStyle}>
                {t("style.openWriterStyle")}
              </button>
            )}
          </div>
        )}
        <label>{t("style.avoidPhrases")}
          <textarea
            rows={4}
            value={phrases}
            disabled={working}
            placeholder={t("style.avoidPhrasesPlaceholder")}
            onChange={(e) => setPhrases(e.target.value)}
          />
        </label>
        {limitChoice && followWriter && (
          <>
            <label className="style-toggle">
              <input
                type="checkbox"
                checked={ownLimit}
                disabled={working}
                data-testid="style-own-limit"
                onChange={(e) => setOwnLimit(e.target.checked)}
              />
              {t("style.ownLimit")}
            </label>
            <p className="style-hint">{t("style.ownLimitHint")}</p>
          </>
        )}
        {(usesOwnLimit || !followWriter) && (
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
        )}
        <label>{t("style.anchor")}
          <select value={anchor} disabled={working} onChange={(e) => setAnchor(e.target.value)}>
            <option value="">{t("style.anchorNone")}</option>
            {scenes.map((scene) => <option key={scene.id} value={scene.id}>{scene.label}</option>)}
          </select>
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
  // Whose rule a row broke matters only once there are two lists (#181);
  // with the writer's default style empty the report reads as it always did.
  const layered = (report.rules.writer_phrases?.length ?? 0) > 0 || report.rules.sentence_limit_source === "writer";
  return (
    <div className="style-result" role="status">
      <p>{t("style.found", { total: report.total, scenes: report.scenes_checked })}</p>
      {report.truncated && <p className="style-hint">{t("style.truncated", { shown: report.violations.length })}</p>}
      <ul className="style-violations">
        {report.violations.map((v, i) => (
          // A report is immutable and its rows have no ids, so position is the key.
          <li key={`${v.node_id}-${v.paragraph}-${i}`}>
            <ViolationRow v={v} onOpenNode={onOpenNode} showSource={layered} />
          </li>
        ))}
      </ul>
    </div>
  );
}

function writerEmpty(w: WriterStyleRules): boolean {
  return w.avoid_phrases.length === 0 && w.max_sentence_chars === 0;
}

function violationText(v: StyleViolation, t: ReturnType<typeof useI18n>["t"]): string {
  switch (v.rule) {
    case "sentence_too_long":
      return t("style.ruleSentence", { length: v.length ?? 0, limit: v.limit ?? 0 });
    case "parallel_contrast":
      return t("style.ruleParallel");
    case "narrator_explains":
      return t("style.ruleExplainer");
    case "repeated_line":
      return t("style.ruleRepeated", { label: v.phrase ?? "" });
    case "ending_formula":
      return t("style.ruleEnding", { label: v.phrase ?? "" });
    default:
      return t("style.rulePhrase", { phrase: v.phrase ?? "" });
  }
}

function ViolationRow({ v, onOpenNode, showSource }: Readonly<{ v: StyleViolation; onOpenNode?: (nodeId: string) => void; showSource?: boolean }>) {
  const { t } = useI18n();
  const where = t("style.where", { label: v.label, paragraph: v.paragraph });
  const what = violationText(v, t);
  return (
    <>
      {showSource && v.source && (
        <span className={`style-source style-source-${v.source}`} data-testid="style-violation-source">
          {t(v.source === "writer" ? "style.sourceWriter" : "style.sourceWork")}
        </span>
      )}
      {onOpenNode
        ? <button type="button" className="style-where" onClick={() => onOpenNode(v.node_id)}>{where}</button>
        : <span className="style-where">{where}</span>}
      <span className="style-rule">{what}</span>
      <span className="style-excerpt">{v.excerpt}</span>
    </>
  );
}
