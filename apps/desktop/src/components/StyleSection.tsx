import { useEffect, useState } from "react";
import { style as styleApi } from "../lib/rpc";
import type { StyleCheckResult, StyleViolation } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";
import "./StyleSection.css";

/** The part of a writer's style a machine can hold a draft against (#162).
 *
 *  Style notes tell an agent how the prose should read; nothing told the
 *  writer whether the prose an agent handed back did. This section keeps the
 *  rules that have one answer — phrases to keep out, how long a sentence may
 *  run — and runs the check. It reports and never edits: what to change is
 *  the writer's call.
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
  const [phrases, setPhrases] = useState<string | null>(null);
  const [maxSentence, setMaxSentence] = useState("");
  const [saving, setSaving] = useState(false);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [report, setReport] = useState<StyleCheckResult | null>(null);

  useEffect(() => {
    let cancelled = false;
    styleApi.getRules(projectId).then((rules) => {
      if (cancelled) return;
      setPhrases(rules.avoid_phrases.join("\n"));
      setMaxSentence(rules.max_sentence_chars > 0 ? String(rules.max_sentence_chars) : "");
    }).catch((e) => { if (!cancelled) setError(e); });
    return () => { cancelled = true; };
  }, [projectId]);

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
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
    } catch (e) {
      setError(e);
    } finally {
      setSaving(false);
    }
  };

  const check = async (scope: Scope) => {
    setChecking(true);
    setError(null);
    try {
      await onBeforeCheck?.();
      setReport(await styleApi.check(projectId, scope === "scene" ? nodeId : undefined));
    } catch (e) {
      setError(e);
    } finally {
      setChecking(false);
    }
  };

  const busy = saving || checking;
  return (
    <details className="sec es-field style-section">
      <summary>{t("style.title")}</summary>
      <p className="style-hint">{t("style.hint")}</p>
      {error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}
      {phrases != null && <>
        <label>{t("style.avoidPhrases")}
          <textarea
            rows={4}
            value={phrases}
            disabled={busy}
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
            disabled={busy}
            placeholder={t("style.maxSentencePlaceholder")}
            onChange={(e) => setMaxSentence(e.target.value)}
          />
        </label>
        <div className="style-actions">
          <button type="button" className="btn accent sm" disabled={busy} onClick={() => void save()}>
            {saving ? t("common.saving") : t("style.save")}
          </button>
          {nodeId && (
            <button type="button" className="btn sm" disabled={busy} onClick={() => void check("scene")}>
              {t("style.checkScene")}
            </button>
          )}
          <button type="button" className="btn sm" disabled={busy} onClick={() => void check("work")}>
            {t("style.checkWork")}
          </button>
        </div>
      </>}
      {checking && <p className="style-hint">{t("style.checking")}</p>}
      {report && !checking && <StyleReport report={report} onOpenNode={onOpenNode} />}
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
