import { useState } from "react";
import type { Project } from "../lib/types";
import { projects as projectsApi } from "../lib/rpc";
import { useI18n } from "../lib/i18n";
import { BUILTIN_CHAR_COUNT_PRESETS, MINIMUM_PRESET_ID, lengthRuleOf } from "../lib/charCountPresets";

interface Props {
  project: Project;
  onProjectChanged?: (project: Project) => void;
}

const DISMISS_PREFIX = "linetta.episodeRuleOffer.dismissed.";

const minimumRule = BUILTIN_CHAR_COUNT_PRESETS.find((p) => p.id === MINIMUM_PRESET_ID)!;

/** A web novel made before episodes had a minimum, counted with spaces: the
 *  kind of work the offer is for. Works counted without spaces are left
 *  alone: 5,000 without spaces is a different, longer episode. */
export function needsEpisodeRule(p: Project): boolean {
  const rule = lengthRuleOf(p);
  return (
    p.outline_preset === "webnovel" &&
    !p.archived_at &&
    rule.char_count_basis === "with_spaces" &&
    rule.episode_char_min === 0 &&
    rule.episode_char_max === 0
  );
}

/** What the offer sets: the minimum, and a target raised to the preset's if
 *  it was lower. A longer target the writer chose stays. */
function ruleUpdate(p: Project) {
  return {
    id: p.id,
    episode_char_min: minimumRule.episode_char_min,
    episode_char_target: Math.max(lengthRuleOf(p).episode_char_target, minimumRule.episode_char_target),
  };
}

function wasDismissed(id: string): boolean {
  try {
    return localStorage.getItem(DISMISS_PREFIX + id) === "1";
  } catch {
    return false;
  }
}

interface Outcome {
  applied: number;
  skipped: string[];
  failed: number;
}

/** Offers an existing web novel the minimum new ones start with, once, with
 *  a way to give every such work the same rule in one step (episode-length
 *  Phase 1, 1.7). No migration does this: other writers' works must not wake
 *  up to warnings they never asked for. */
export function EpisodeRuleOffer({ project, onProjectChanged }: Readonly<Props>) {
  const { t } = useI18n();
  const [dismissed, setDismissed] = useState(() => wasDismissed(project.id));
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [failed, setFailed] = useState(false);

  if (outcome) {
    return (
      <div className="episode-rule-offer" role="status" data-testid="episode-rule-outcome">
        <p>
          {t("episodeRuleOffer.applied", { count: String(outcome.applied) })}
          {outcome.skipped.length > 0 &&
            " " + t("episodeRuleOffer.skipped", { count: String(outcome.skipped.length), names: outcome.skipped.join(", ") })}
          {outcome.failed > 0 && " " + t("episodeRuleOffer.failedSome", { count: String(outcome.failed) })}
        </p>
        <div className="episode-rule-offer-actions">
          <button type="button" className="chip" onClick={() => setOutcome(null)}>
            {t("episodeRuleOffer.close")}
          </button>
        </div>
      </div>
    );
  }
  if (dismissed || !needsEpisodeRule(project)) return null;

  const dismiss = () => {
    try {
      localStorage.setItem(DISMISS_PREFIX + project.id, "1");
    } catch {
      /* asked again next session; nothing else depends on it */
    }
    setDismissed(true);
  };

  const applyThis = async () => {
    setBusy(true);
    setFailed(false);
    try {
      onProjectChanged?.(await projectsApi.update(ruleUpdate(project)));
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
    }
  };

  const applyAll = async () => {
    setBusy(true);
    setFailed(false);
    try {
      const all = await projectsApi.list({ limit: 200 });
      const webnovels = all.filter((p) => p.outline_preset === "webnovel" && !p.archived_at);
      const result: Outcome = { applied: 0, skipped: [], failed: 0 };
      for (const p of webnovels) {
        const rule = lengthRuleOf(p);
        if (rule.episode_char_min > 0 || rule.episode_char_max > 0) continue;
        if (!needsEpisodeRule(p)) {
          result.skipped.push(p.title);
          continue;
        }
        try {
          const updated = await projectsApi.update(ruleUpdate(p));
          result.applied += 1;
          if (updated.id === project.id) onProjectChanged?.(updated);
        } catch {
          result.failed += 1;
        }
      }
      setOutcome(result);
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="episode-rule-offer" data-testid="episode-rule-offer">
      <p>{t("episodeRuleOffer.body")}</p>
      <div className="episode-rule-offer-actions">
        <button type="button" className="chip" disabled={busy} onClick={() => void applyThis()} data-testid="episode-rule-apply">
          {t("episodeRuleOffer.applyThis")}
        </button>
        <button type="button" className="chip" disabled={busy} onClick={() => void applyAll()} data-testid="episode-rule-apply-all">
          {t("episodeRuleOffer.applyAll")}
        </button>
        <button type="button" className="chip" disabled={busy} onClick={dismiss} data-testid="episode-rule-dismiss">
          {t("episodeRuleOffer.dismiss")}
        </button>
      </div>
      {failed && <p className="char-preset-error" role="alert">{t("episodeRuleOffer.failed")}</p>}
    </div>
  );
}
