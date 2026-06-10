import { useEffect, useRef, useState } from "react";
import type { NodeRow, Project, Entity, EntityKind } from "../lib/types";
import { projects as projectsApi } from "../lib/rpc";
import { PlotPanel } from "./PlotPanel";
import { User, MapPin, Box, Lightbulb, Book, Search } from "../lib/icons";
import { InlineEditableText } from "./InlineEditableText";
import { localeForLanguage, useI18n } from "../lib/i18n";
import type { EpisodeStatusCounts } from "../hooks/useFirstLeaf";
import { StatsSection } from "./StatsSection";

export type SaveStatus =
  | { kind: "idle" }
  | { kind: "saving" }
  | { kind: "saved"; at: number }
  | { kind: "error"; message: string };

interface Props {
  project: Project;
  node: NodeRow;
  charCount: number;
  todayChars?: number | null;
  episodeStock?: EpisodeStatusCounts | null;
  statsRefreshKey?: number | null;
  typewriter: boolean;
  onToggleTypewriter: () => void;
  saveStatus: SaveStatus;
  mentionedEntities: Entity[];
  onMentionClick: (entityId: string) => void;
  onAutoMention?: () => void;
  autoMentionBusy?: boolean;
  onOpenThread: (threadId: string) => void;
  onProjectChanged?: (project: Project) => void;
  onProjectTitleChange?: (title: string) => void | Promise<void>;
  tourTarget?: string;
}

const KIND_META: Record<EntityKind, { color: string; Icon: typeof User }> = {
  character: { color: "var(--t-sienna)", Icon: User },
  place: { color: "var(--t-teal)", Icon: MapPin },
  item: { color: "var(--t-olive)", Icon: Box },
  concept: { color: "var(--t-plum)", Icon: Lightbulb },
};

// Word-count target by length_target. Mirrors the spirit of the mockup's
// progress bar (작품 전체 단어 / 목표).
const TARGET_WORDS: Record<Project["length_target"], number> = {
  flash: 1000,
  short: 7500,
  novella: 40000,
  novel: 90000,
  series: 200000,
};

export function ContextPanel({ project, node, charCount, todayChars = null, episodeStock = null, statsRefreshKey = null, typewriter, onToggleTypewriter, saveStatus, mentionedEntities, onMentionClick, onAutoMention, autoMentionBusy, onOpenThread, onProjectChanged, onProjectTitleChange, tourTarget }: Readonly<Props>) {
  const { language, t } = useI18n();
  const locale = localeForLanguage(language);
  const target = TARGET_WORDS[project.length_target] ?? 90000;
  const pct = target > 0 ? Math.min(100, Math.round((project.word_count / target) * 100)) : 0;
  const isWebnovelProject = project.outline_preset === "webnovel";
  const episodeCharTarget = project.episode_char_target > 0 ? project.episode_char_target : 5000;
  const [overview, setOverview] = useState(project.outline ?? "");
  const [synopsis, setSynopsis] = useState(project.synopsis ?? "");
  const [synopsisBusy, setSynopsisBusy] = useState<"rewrite" | "clear" | null>(null);
  const overviewSaveTimer = useRef<number | null>(null);
  const synopsisSaveTimer = useRef<number | null>(null);

  useEffect(() => {
    setOverview(project.outline ?? "");
    setSynopsis(project.synopsis ?? "");
  }, [project.id, project.outline, project.synopsis]);

  useEffect(() => () => {
    if (overviewSaveTimer.current) window.clearTimeout(overviewSaveTimer.current);
    if (synopsisSaveTimer.current) window.clearTimeout(synopsisSaveTimer.current);
  }, []);

  const saveOverview = (next: string) => {
    setOverview(next);
    if (overviewSaveTimer.current) window.clearTimeout(overviewSaveTimer.current);
    overviewSaveTimer.current = window.setTimeout(async () => {
      try {
        const updated = await projectsApi.update({ id: project.id, outline: next });
        onProjectChanged?.(updated);
      } catch {
        /* benign; keep local draft */
      }
    }, 600);
  };

  const saveSynopsis = (next: string) => {
    setSynopsis(next);
    if (synopsisSaveTimer.current) window.clearTimeout(synopsisSaveTimer.current);
    synopsisSaveTimer.current = window.setTimeout(async () => {
      try {
        const updated = await projectsApi.update({ id: project.id, synopsis: next });
        onProjectChanged?.(updated);
      } catch {
        /* benign; keep local draft */
      }
    }, 600);
  };

  const saveEpisodeTarget = async (next: string) => {
    const compact = next.replace(/,/g, "");
    const parsed = Number.parseInt(compact, 10);
    if (!/^\d+$/.test(compact) || parsed <= 0) {
      throw new Error("invalid episode target");
    }
    const updated = await projectsApi.update({ id: project.id, episode_char_target: parsed });
    onProjectChanged?.(updated);
  };

  const rewriteSynopsis = async () => {
    if (synopsisSaveTimer.current) window.clearTimeout(synopsisSaveTimer.current);
    setSynopsisBusy("rewrite");
    try {
      const updated = await projectsApi.rewriteSynopsis(project.id);
      setSynopsis(updated.synopsis ?? "");
      onProjectChanged?.(updated);
    } catch {
      /* benign; leave current synopsis visible */
    } finally {
      setSynopsisBusy(null);
    }
  };

  const clearSynopsis = async () => {
    if (synopsisSaveTimer.current) window.clearTimeout(synopsisSaveTimer.current);
    setSynopsisBusy("clear");
    try {
      const updated = await projectsApi.clearSynopsis(project.id);
      setSynopsis(updated.synopsis ?? "");
      onProjectChanged?.(updated);
    } catch {
      /* benign; leave current synopsis visible */
    } finally {
      setSynopsisBusy(null);
    }
  };

  return (
    <aside className="panel" data-tour={tourTarget}>
      <div className="panel-head">
        <span className="ttl">
          <span className="ic"><Book size={16} /></span>
          <InlineEditableText
            value={project.title}
            ariaLabel={t("workspace.novelTitle")}
            className="panel-title-input"
            onCommit={async (title) => { await onProjectTitleChange?.(title); }}
          />
        </span>
        <span className="sub">{t(`workspace.status.${node.status}`)}</span>
      </div>

      <div className="panel-scroll">
        {/* 이 씬 — stats */}
        <div className="sec">
          <h4>{t("workspace.thisScene")}</h4>
          <div className="stat-row">
            <span className="stat-big">{charCount.toLocaleString(locale)}</span>
            <span className="stat-unit">{t("workspace.charUnit")}</span>
            <span style={{ flex: 1 }} />
            <SaveStatusPill status={saveStatus} />
          </div>
          {todayChars !== null && (
            <div className="scene-today">
              {t("workspace.todayChars", { count: todayChars.toLocaleString(locale) })}
            </div>
          )}
          {isWebnovelProject && episodeStock && (
            <div className="scene-today">
              {t("workspace.episodeStock", {
                published: episodeStock.published.toLocaleString(locale),
                stock: episodeStock.stock.toLocaleString(locale),
              })}
            </div>
          )}
          <div className="progress">
            <div className="progress-track">
              <div className="progress-fill" style={{ width: `${pct}%` }} />
            </div>
            <div className="progress-meta">
              <span>{t("workspace.totalWork", { count: project.word_count.toLocaleString(locale) })}</span>
              {isWebnovelProject ? (
                <span className="episode-target-meta">
                  <span>{t("workspace.episodeTarget")}</span>
                  <InlineEditableText
                    value={String(episodeCharTarget)}
                    ariaLabel={t("workspace.episodeTarget")}
                    className="episode-target-input"
                    onCommit={saveEpisodeTarget}
                  />
                  <span>{t("workspace.charUnit")}</span>
                </span>
              ) : (
                <span>{pct}% / {Math.round(target / 1000)}k</span>
              )}
            </div>
          </div>
          <div className="toggles">
            <button type="button" className="toggle-row" onClick={onToggleTypewriter}>
              <span className="lbl">
                <span className="ic"><Book size={15} /></span> {t("workspace.typewriterMode")}
              </span>
              <span className={"switch" + (typewriter ? " on" : "")} />
            </button>
          </div>
        </div>

        {/* 작품 개요 — writer-authored plan/intent. */}
        <div className="sec">
          <h4>{t("workspace.projectOverview")}</h4>
          <textarea
            aria-label={t("workspace.projectOverview")}
            className="project-text-edit"
            value={overview}
            rows={5}
            placeholder={t("workspace.projectOverviewPlaceholder")}
            onChange={(e) => saveOverview(e.target.value)}
          />
        </div>

        {/* 시놉시스 — editable story summary used as its own AI context item. */}
        <div className="sec">
          <h4>
            <span>{t("workspace.synopsis")}</span>
            <span className="sec-actions">
              <button type="button" onClick={rewriteSynopsis} disabled={synopsisBusy !== null}>
                {synopsisBusy === "rewrite" ? t("workspace.rewriting") : t("workspace.rewrite")}
              </button>
              <button type="button" onClick={clearSynopsis} disabled={synopsisBusy !== null || synopsis.trim() === ""}>
                {t("workspace.clear")}
              </button>
            </span>
          </h4>
          <textarea
            aria-label={t("workspace.projectSynopsis")}
            className="project-text-edit"
            value={synopsis}
            rows={6}
            placeholder={t("workspace.synopsisPlaceholder")}
            onChange={(e) => saveSynopsis(e.target.value)}
          />
        </div>

        {/* 등장 — mentioned entities */}
        <div className="sec">
          <h4>
            <span>{t("workspace.mentions")} <span style={{ color: "var(--muted-2)" }}>{mentionedEntities.length}</span></span>
            {onAutoMention && (
              <span className="sec-actions">
                <button type="button" onClick={onAutoMention} disabled={!!autoMentionBusy}>
                  <Search size={11} /> {autoMentionBusy ? t("workspace.scanning") : t("workspace.scanScene")}
                </button>
              </span>
            )}
          </h4>
          {mentionedEntities.length === 0 && (
            <p className="sec-empty">{t("workspace.noMentionedEntities")}</p>
          )}
          {mentionedEntities.map((e) => {
            const meta = KIND_META[e.kind];
            const Icon = meta.Icon;
            return (
              <button
                key={e.id}
                type="button"
                className="ent-row"
                onClick={() => onMentionClick(e.id)}
              >
                <span className="ent-av" style={{ "--av": meta.color } as React.CSSProperties}>
                  {e.name.slice(0, 1)}
                </span>
                <span className="ent-info">
                  <div className="ent-name">{e.name}</div>
                  {e.role && <div className="ent-role">{e.role}</div>}
                </span>
                <span className="ent-kind-ic"><Icon size={14} /></span>
              </button>
            );
          })}
        </div>

        {/* 플롯 — storylines + beats */}
        <PlotPanel
          project={project}
          nodeId={node.id}
          onOpenThread={onOpenThread}
        />

        <StatsSection
          project={project}
          refreshKey={statsRefreshKey}
          episodeCharTarget={episodeCharTarget}
        />
      </div>
    </aside>
  );
}

function SaveStatusPill({ status }: { status: SaveStatus }) {
  const { t } = useI18n();
  // Re-render every second when in "saved" state so the relative label updates.
  const [, tick] = useState(0);
  useEffect(() => {
    if (status.kind !== "saved") return;
    const id = window.setInterval(() => tick((n) => n + 1), 1000);
    return () => window.clearInterval(id);
  }, [status]);

  switch (status.kind) {
    case "idle":
      return (
        <span className="save-pill">
          <span className="d" />{t("workspace.saved")}
        </span>
      );
    case "saving":
      return (
        <span className="save-pill saving">
          <span className="d" />{t("common.saving")}
        </span>
      );
    case "saved": {
      const seconds = Math.max(0, Math.floor((Date.now() - status.at) / 1000));
      const label = seconds < 1 ? t("workspace.justSaved") : t("workspace.savedSecondsAgo", { seconds });
      return (
        <span className="save-pill">
          <span className="d" />{label}
        </span>
      );
    }
    case "error":
      return (
        <span className="save-pill saving" title={status.message}>
          <span className="d" />{t("workspace.saveFailed")}
        </span>
      );
  }
}
