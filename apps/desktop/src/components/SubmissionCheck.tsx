import { useEffect, useState } from "react";
import type { Project, SubmissionEpisode, SubmissionReport } from "../lib/types";
import { projects as projectsApi } from "../lib/rpc";
import { useI18n, type MessageKey } from "../lib/i18n";
import { charCountBasisLabelKey, charCountBasisOf } from "../lib/charCount";
import { InlineEditableText } from "./InlineEditableText";

interface Props {
  project: Project;
  /** Changes when a scene is saved, so the check follows the prose. */
  refreshKey?: number | null;
  onProjectChanged?: (project: Project) => void;
  /** Opens a scene; a short episode's label leads to its first scene. */
  onOpenScene?: (nodeId: string) => void;
}

type NumberField = "submission_min_episodes" | "submission_total_min" | "submission_total_max";

/** How many short or long episodes are named before "외 N화". */
const NAMED_EPISODES = 3;

/** A work's submission rule and what it still lacks (episode-length Phase
 *  3). The engine measures; this draws. Nothing here stops the writer: a
 *  work in progress is short by nature. */
export function SubmissionCheck({ project, refreshKey = null, onProjectChanged, onOpenScene }: Readonly<Props>) {
  const { t, language } = useI18n();
  const locale = language;
  const minEpisodes = project.submission_min_episodes ?? 0;
  const totalMin = project.submission_total_min ?? 0;
  const totalMax = project.submission_total_max ?? 0;
  const needsSynopsis = project.submission_needs_synopsis ?? false;
  const hasRule = minEpisodes > 0 || totalMin > 0 || totalMax > 0 || needsSynopsis;

  const [report, setReport] = useState<SubmissionReport | null>(null);
  const [rangeError, setRangeError] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!hasRule) {
      setReport(null);
      return;
    }
    let cancelled = false;
    projectsApi
      .submissionCheck(project.id)
      .then((r) => {
        if (!cancelled) setReport(r);
      })
      .catch(() => {
        if (!cancelled) setReport(null);
      });
    return () => {
      cancelled = true;
    };
    // updated_at moves with every change to the rule and the synopsis.
  }, [project.id, project.updated_at, refreshKey, hasRule]);

  const saveNumber = async (field: NumberField, next: string) => {
    const compact = next.replace(/,/g, "").trim();
    if (!/^\d*$/.test(compact)) throw new Error("invalid submission number");
    const parsed = compact === "" ? 0 : Number.parseInt(compact, 10);
    const lo = field === "submission_total_min" ? parsed : totalMin;
    const hi = field === "submission_total_max" ? parsed : totalMax;
    if (hi > 0 && lo > hi) {
      setRangeError(true);
      throw new Error("submission total out of order");
    }
    setRangeError(false);
    const updated = await projectsApi.update({ id: project.id, [field]: parsed });
    onProjectChanged?.(updated);
  };

  const saveSynopsisNeeded = async (next: boolean) => {
    setBusy(true);
    try {
      const updated = await projectsApi.update({ id: project.id, submission_needs_synopsis: next });
      onProjectChanged?.(updated);
    } catch {
      /* the box stays as the work still has it */
    } finally {
      setBusy(false);
    }
  };

  const num = (n: number) => n.toLocaleString(locale);
  const basis = t(charCountBasisLabelKey(charCountBasisOf(project)));

  const field = (key: NumberField, labelKey: MessageKey, value: number, unitKey: MessageKey) => (
    <span className="episode-target-meta">
      <span>{t(labelKey)}</span>
      <InlineEditableText
        value={value > 0 ? String(value) : ""}
        ariaLabel={t(labelKey)}
        className="episode-target-input"
        allowEmpty
        placeholder={t("workspace.episodeBoundNone")}
        onCommit={(v) => saveNumber(key, v)}
      />
      <span>{t(unitKey)}</span>
    </span>
  );

  const episodeNames = (list: SubmissionEpisode[], testId: string) => (
    <>
      {list.slice(0, NAMED_EPISODES).map((ep) => (
        <button
          key={ep.node_id}
          type="button"
          className="submission-episode"
          onClick={() => onOpenScene?.(ep.scene_id)}
          data-testid={testId}
        >
          {t("submission.episodeDelta", {
            label: ep.label,
            delta: `${ep.delta > 0 ? "+" : "−"}${num(Math.abs(ep.delta))}`,
          })}
        </button>
      ))}
      {list.length > NAMED_EPISODES && (
        <span>{t("submission.moreEpisodes", { count: String(list.length - NAMED_EPISODES) })}</span>
      )}
    </>
  );

  const totalRule = () => {
    if (totalMin > 0 && totalMax > 0) return t("submission.totalRange", { min: num(totalMin), max: num(totalMax) });
    if (totalMin > 0) return t("submission.totalAtLeast", { min: num(totalMin) });
    return t("submission.totalAtMost", { max: num(totalMax) });
  };

  return (
    <details className="submission-check" open={hasRule} data-testid="submission-check">
      <summary>{t("submission.title")}</summary>
      <div className="progress-meta episode-bounds-meta">
        {field("submission_min_episodes", "submission.minEpisodes", minEpisodes, "submission.episodeUnit")}
        {field("submission_total_min", "submission.totalMin", totalMin, "workspace.charUnit")}
        {field("submission_total_max", "submission.totalMax", totalMax, "workspace.charUnit")}
      </div>
      <label className="char-basis-row">
        <input
          type="checkbox"
          checked={needsSynopsis}
          disabled={busy}
          onChange={(e) => void saveSynopsisNeeded(e.target.checked)}
          data-testid="submission-needs-synopsis"
        />
        <span>{t("submission.needsSynopsis")}</span>
      </label>
      {rangeError && <p className="char-preset-error" role="alert">{t("submission.rangeError")}</p>}

      {report?.has_rule && (
        <ul className="submission-list" data-testid="submission-list">
          {report.min_episodes > 0 && (
            <li className={report.episodes >= report.min_episodes ? "is-met" : "is-short"} data-testid="submission-episodes">
              {t("submission.episodes", { count: num(report.episodes), min: num(report.min_episodes) })}
            </li>
          )}
          {report.structured && report.episode_rule && (
            <li className={report.short.length > 0 ? "is-short" : "is-met"} data-testid="submission-episode-length">
              <span>{t("submission.meeting", { count: num(report.meeting) })}</span>
              {report.short.length > 0 && (
                <span>
                  {" · "}
                  {t("submission.shortCount", { count: num(report.short.length) })} {episodeNames(report.short, "submission-short-episode")}
                </span>
              )}
              {report.long.length > 0 && (
                <span className="is-long">
                  {" · "}
                  {t("submission.longCount", { count: num(report.long.length) })} {episodeNames(report.long, "submission-long-episode")}
                </span>
              )}
            </li>
          )}
          {(report.total_min > 0 || report.total_max > 0) && (
            <li
              className={report.total_state === "short" ? "is-short" : report.total_state === "long" ? "is-long" : "is-met"}
              data-testid="submission-total"
            >
              {t("submission.total", { count: num(report.total_chars), rule: totalRule(), basis })}
            </li>
          )}
          {report.needs_synopsis && (
            <li className={report.has_synopsis ? "is-met" : "is-short"} data-testid="submission-synopsis">
              {report.has_synopsis ? t("submission.synopsisPresent") : t("submission.synopsisMissing")}
            </li>
          )}
          {report.ready && (
            <li className="is-met" data-testid="submission-ready">{t("submission.ready")}</li>
          )}
        </ul>
      )}
    </details>
  );
}
