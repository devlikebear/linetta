import { useEffect, useState } from "react";
import type { NodeRow, Project, Entity, EntityKind } from "../lib/types";
import { PlotPanel } from "./PlotPanel";
import { User, MapPin, Box, Lightbulb, Book } from "../lib/icons";

export type SaveStatus =
  | { kind: "idle" }
  | { kind: "saving" }
  | { kind: "saved"; at: number }
  | { kind: "error"; message: string };

interface Props {
  project: Project;
  node: NodeRow;
  charCount: number;
  typewriter: boolean;
  onToggleTypewriter: () => void;
  saveStatus: SaveStatus;
  mentionedEntities: Entity[];
  onMentionClick: (entityId: string) => void;
  onOpenThread: (threadId: string) => void;
  onProjectChanged?: (project: Project) => void;
}

const STATUS_LABEL: Record<NodeRow["status"], string> = {
  draft: "초고",
  revision: "퇴고",
  final: "완성",
};

const KIND_META: Record<EntityKind, { label: string; color: string; Icon: typeof User }> = {
  character: { label: "인물", color: "var(--t-sienna)", Icon: User },
  place: { label: "장소", color: "var(--t-teal)", Icon: MapPin },
  item: { label: "물건", color: "var(--t-olive)", Icon: Box },
  concept: { label: "개념", color: "var(--t-plum)", Icon: Lightbulb },
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

export function ContextPanel({ project, node, charCount, typewriter, onToggleTypewriter, saveStatus, mentionedEntities, onMentionClick, onOpenThread, onProjectChanged }: Props) {
  const target = TARGET_WORDS[project.length_target] ?? 90000;
  const pct = target > 0 ? Math.min(100, Math.round((project.word_count / target) * 100)) : 0;

  return (
    <aside className="panel">
      <div className="panel-head">
        <span className="ttl">
          <span className="ic"><Book size={16} /></span>
          {project.title}
        </span>
        <span className="sub">{STATUS_LABEL[node.status]}</span>
      </div>

      <div className="panel-scroll">
        {/* 이 씬 — stats */}
        <div className="sec">
          <h4>이 씬</h4>
          <div className="stat-row">
            <span className="stat-big">{charCount.toLocaleString("ko-KR")}</span>
            <span className="stat-unit">자</span>
            <span style={{ flex: 1 }} />
            <SaveStatusPill status={saveStatus} />
          </div>
          <div className="progress">
            <div className="progress-track">
              <div className="progress-fill" style={{ width: `${pct}%` }} />
            </div>
            <div className="progress-meta">
              <span>작품 전체 {project.word_count.toLocaleString("ko-KR")}</span>
              <span>{pct}% / {Math.round(target / 1000)}k</span>
            </div>
          </div>
          <div className="toggles">
            <button type="button" className="toggle-row" onClick={onToggleTypewriter}>
              <span className="lbl">
                <span className="ic"><Book size={15} /></span> 타자기 모드
              </span>
              <span className={"switch" + (typewriter ? " on" : "")} />
            </button>
          </div>
        </div>

        {/* 등장 — mentioned entities */}
        <div className="sec">
          <h4>등장 <span style={{ color: "var(--muted-2)" }}>{mentionedEntities.length}</span></h4>
          {mentionedEntities.length === 0 && (
            <p className="sec-empty">이 씬에 언급된 인물이 없어요</p>
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

        {/* 플롯 — outline + spine */}
        <PlotPanel
          project={project}
          nodeId={node.id}
          onOpenThread={onOpenThread}
          onProjectChanged={onProjectChanged}
        />
      </div>
    </aside>
  );
}

function SaveStatusPill({ status }: { status: SaveStatus }) {
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
          <span className="d" />저장됨
        </span>
      );
    case "saving":
      return (
        <span className="save-pill saving">
          <span className="d" />저장 중
        </span>
      );
    case "saved": {
      const seconds = Math.max(0, Math.floor((Date.now() - status.at) / 1000));
      const label = seconds < 1 ? "방금 저장됨" : `${seconds}초 전 저장됨`;
      return (
        <span className="save-pill">
          <span className="d" />{label}
        </span>
      );
    }
    case "error":
      return (
        <span className="save-pill saving" title={status.message}>
          <span className="d" />저장 실패
        </span>
      );
  }
}
