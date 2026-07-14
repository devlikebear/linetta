import { useEffect, useMemo, useState } from "react";
import type { Project, WritingStatsDay, WritingStatsSummary } from "../lib/types";
import { stats as statsApi } from "../lib/rpc";
import { localeForLanguage, useI18n } from "../lib/i18n";
import "./StatsSection.css";

const HEATMAP_DAYS = 84;

interface Props {
  project: Project;
  refreshKey?: number | null;
  episodeCharTarget?: number;
}

interface StatsSnapshot {
  days: WritingStatsDay[];
  summary: WritingStatsSummary;
}

export function formatStatsDay(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function shiftStatsDay(date: Date, days: number): Date {
  const next = new Date(date.getFullYear(), date.getMonth(), date.getDate(), 12);
  next.setDate(next.getDate() + days);
  return next;
}

export function buildStatsRange(today = new Date(), days = HEATMAP_DAYS) {
  const end = shiftStatsDay(today, 0);
  const start = shiftStatsDay(end, -(days - 1));
  return {
    fromDay: formatStatsDay(start),
    toDay: formatStatsDay(end),
  };
}

export function statLevel(chars: number, maxChars: number): number {
  if (chars <= 0 || maxChars <= 0) return 0;
  const ratio = chars / maxChars;
  if (ratio <= 0.25) return 1;
  if (ratio <= 0.5) return 2;
  if (ratio <= 0.75) return 3;
  return 4;
}

function emptySummary(): WritingStatsSummary {
  return { today: 0, week_avg: 0, total_days: 0 };
}

export function StatsSection({ project, refreshKey = null, episodeCharTarget = 5000 }: Readonly<Props>) {
  const { language, t } = useI18n();
  const locale = localeForLanguage(language);
  const [snapshot, setSnapshot] = useState<StatsSnapshot | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    const { fromDay, toDay } = buildStatsRange();
    setLoading(true);
    (async () => {
      try {
        const [days, summary] = await Promise.all([
          statsApi.range(project.id, fromDay, toDay),
          statsApi.summary(project.id),
        ]);
        if (!cancelled) setSnapshot({ days, summary });
      } catch {
        if (!cancelled) setSnapshot({ days: [], summary: emptySummary() });
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [project.id, refreshKey]);

  const days = useMemo(() => snapshot?.days ?? [], [snapshot?.days]);
  const summary = snapshot?.summary ?? emptySummary();
  const maxChars = useMemo(
    () => days.reduce((max, day) => Math.max(max, day.chars_added), 0),
    [days],
  );
  const hasWritingDays = summary.total_days > 0 || days.some((day) => day.chars_added > 0);
  const weeklyEpisodeCount =
    project.length_target === "series" && project.outline_preset === "webnovel" && episodeCharTarget > 0
      ? (summary.week_avg * 7) / episodeCharTarget
      : 0;

  return (
    <section className="sec stats-section">
      <h4>{t("stats.title")}</h4>
      {loading ? (
        <p className="sec-empty">{t("common.loading")}</p>
      ) : !hasWritingDays ? (
        <p className="sec-empty">{t("stats.empty")}</p>
      ) : (
        <>
          <div className="stats-heatmap" aria-label={t("stats.title")}>
            {days.map((day) => (
              <span
                key={day.day}
                className={`stats-cell level-${statLevel(day.chars_added, maxChars)}`}
                title={`${day.day}: ${t("workspace.charCount", { count: day.chars_added.toLocaleString(locale) })}`}
              />
            ))}
          </div>
          <div className="stats-summary">
            {t("stats.summaryLine", {
              avg: summary.week_avg.toLocaleString(locale),
              days: summary.total_days.toLocaleString(locale),
            })}
          </div>
          {weeklyEpisodeCount > 0 && (
            <div className="stats-summary">
              {t("stats.weeklyEpisodes", {
                episodes: weeklyEpisodeCount.toLocaleString(locale, {
                  maximumFractionDigits: 1,
                  minimumFractionDigits: 1,
                }),
              })}
            </div>
          )}
        </>
      )}
    </section>
  );
}
