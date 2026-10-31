import { ScenePlanField } from "./ScenePlanField";
import { useEffect, useRef, useState } from "react";
import type { NodeRow, Project, Entity, EntityKind } from "../lib/types";
import { changesContinuity } from "../lib/agentTools";
import { projects as projectsApi, continuity as continuityApi } from "../lib/rpc";
import { PlotPanel } from "./PlotPanel";
import { User, MapPin, Box, Lightbulb, Book, Search } from "../lib/icons";
import { InlineEditableText } from "./InlineEditableText";
import { localeForLanguage, useI18n } from "../lib/i18n";
import type { EpisodeStatusCounts } from "../hooks/useFirstLeaf";
import { StatsSection } from "./StatsSection";
import { DirectionSection } from "./DirectionSection";
import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import type { ContinuityChangedPayload } from "../lib/types";

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
  /** Registered names found in the prose but not yet linked. */
  autoMentionFound?: number;
  onOpenThread: (threadId: string) => void;
  onProjectChanged?: (project: Project) => void;
  onProjectTitleChange?: (title: string) => void | Promise<void>;
  tourTarget?: string;
  /** Opens Story World's continuity section, scoped to this scene
   *  (story-assist Phase 2). Omitted entirely when the caller has not
   *  wired it, the same optional-prop convention onAutoMention follows. */
  onOpenContinuity?: () => void;
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

// Story-assist Phase 1's collapse state. Read/write failures (storage
// disabled, quota, a non-browser test environment) are ignored — the
// section simply falls back to open every time, which is the safer default.
const DIRECTION_COLLAPSED_KEY = "linetta:direction:collapsed";

function readDirectionCollapsed(): boolean {
  try {
    return window.localStorage.getItem(DIRECTION_COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

function storeDirectionCollapsed(collapsed: boolean) {
  try {
    window.localStorage.setItem(DIRECTION_COLLAPSED_KEY, collapsed ? "1" : "0");
  } catch {
    /* benign; the collapse state just does not persist this session */
  }
}

export function ContextPanel({ project, node, charCount, todayChars = null, episodeStock = null, statsRefreshKey = null, typewriter, onToggleTypewriter, saveStatus, mentionedEntities, onMentionClick, onAutoMention, autoMentionBusy, autoMentionFound = 0, onOpenThread, onProjectChanged, onProjectTitleChange, tourTarget, onOpenContinuity }: Readonly<Props>) {
  const { language, t } = useI18n();
  const locale = localeForLanguage(language);
  const target = TARGET_WORDS[project.length_target] ?? 90000;
  const pct = target > 0 ? Math.min(100, Math.round((project.word_count / target) * 100)) : 0;
  const isWebnovelProject = project.outline_preset === "webnovel";
  const episodeCharTarget = project.episode_char_target > 0 ? project.episode_char_target : 5000;

  // story-assist Phase 2: how many open continuity observations this scene
  // carries, for a badge the writer can act on without opening Story World
  // first just to find out. Fetched work-wide (continuity.open_counts) and
  // looked up by node id, rather than once per scene, since the writer
  // moves between scenes far more often than an agent reports a new check.
  const [continuityCounts, setContinuityCounts] = useState<Record<string, number>>({});
  useEffect(() => {
    let cancelled = false;
    continuityApi
      .openCounts(project.id)
      .then((res) => {
        if (!cancelled) setContinuityCounts(res.counts);
      })
      .catch(() => {
        /* The badge just stays off; it is a convenience, not a status the
           writer depends on to know their work is safe. */
      });
    return () => {
      cancelled = true;
    };
  }, [project.id]);
  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.project_id && event.project_id !== project.id) return;
    if (!changesContinuity(event.tool)) return;
    continuityApi
      .openCounts(project.id)
      .then((res) => setContinuityCounts(res.counts))
      .catch(() => {});
  });
  // continuity.set_status (the writer resolving or dismissing an
  // observation from Story World, in this window or another) does not fire
  // mcp-changed — that event is for an agent's own tool calls — so the
  // badge would otherwise keep showing a count the writer already acted on
  // until something unrelated remounted this panel.
  useEngineEvent<ContinuityChangedPayload>("continuity-changed", (event) => {
    if (event.project_id && event.project_id !== project.id) return;
    continuityApi
      .openCounts(project.id)
      .then((res) => setContinuityCounts(res.counts))
      .catch(() => {});
  });
  const continuityOpenCount = continuityCounts[node.id] ?? 0;
  const [overview, setOverview] = useState(project.outline ?? "");
  const [synopsis, setSynopsis] = useState(project.synopsis ?? "");
  const [synopsisBusy, setSynopsisBusy] = useState<"rewrite" | "clear" | null>(null);
  const overviewSaveTimer = useRef<number | null>(null);
  const synopsisSaveTimer = useRef<number | null>(null);
  const [directionCollapsed, setDirectionCollapsed] = useState<boolean>(() => readDirectionCollapsed());

  const toggleDirectionCollapsed = (collapsed: boolean) => {
    setDirectionCollapsed(collapsed);
    storeDirectionCollapsed(collapsed);
  };

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
        {/* 작가 방향 — the writer's own current focus and this scene's intent
            (story-assist Phase 1). First, so it is seen before anything
            else, the same reason the story brief renders it first. */}
        <DirectionSection
          projectId={project.id}
          nodeId={node.id}
          collapsed={directionCollapsed}
          onToggleCollapsed={toggleDirectionCollapsed}
        />
        <ScenePlanField nodeId={node.id} plan={node.plan ?? ""} />

        {/* 이 씬 — stats */}
        <div className="sec">
          <h4>{t("workspace.thisScene")}</h4>
          <div className="stat-row">
            <span className="stat-big">{charCount.toLocaleString(locale)}</span>
            <span className="stat-unit">{t("workspace.charUnit")}</span>
            <span style={{ flex: 1 }} />
            <SaveStatusPill status={saveStatus} />
          </div>
          {continuityOpenCount > 0 && onOpenContinuity && (
            <button type="button" className="chip" data-testid="continuity-open-badge" onClick={onOpenContinuity}>
              {t("continuity.openBadge", { count: String(continuityOpenCount) })}
            </button>
          )}
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

        {/* 시놉시스 — the writer's own story summary; it also feeds the MCP brief.
            Deriving one from scene summaries needed a model, so that button is
            gone and this is edited by hand or written by an agent. */}
        <div className="sec">
          <h4>
            <span>{t("workspace.synopsis")}</span>
            <span className="sec-actions">
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
                  <Search size={11} /> {autoMentionBusy
                    ? t("workspace.scanning")
                    : autoMentionFound > 0
                      ? t("workspace.scanSceneFound", { count: autoMentionFound })
                      : t("workspace.scanScene")}
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
