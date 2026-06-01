import { Check } from "lucide-react";
import "./AIContextChecklist.css";
import type { ContextCounts } from "../../lib/types";

// Re-export so existing consumers that import { ContextCounts } from this file
// continue to work without changes.
export type { ContextCounts };

interface Props {
  anchor: { top: number; left: number };
  counts: ContextCounts;
  onClose: () => void;
}

export function AIContextChecklistList({ counts }: { counts: ContextCounts }) {
  const items: { label: string; present: boolean; caption?: string }[] = [
    { label: "현재 씬 본문", present: true },
    { label: "작품 개요", present: counts.hasOutline },
    { label: "작품 시놉시스(폴백)", present: counts.hasSynopsis },
    {
      label: "직전·직후 씬 발췌",
      present: counts.nearbyScenes > 0,
      caption: `${counts.nearbyScenes}개`,
    },
    {
      label: "관련 과거 씬 (멘션 RAG)",
      present: counts.relatedScenes > 0,
      caption: `${counts.relatedScenes}개`,
    },
    {
      label: "플롯 (전/현/후 씬 비트)",
      present: counts.plotBeats > 0,
      caption: `${counts.plotBeats}개`,
    },
    {
      label: "등장 인물·장소",
      present: counts.entities > 0,
      caption: `${counts.entities}개`,
    },
    {
      label: "관계",
      present: counts.relationships > 0,
      caption: `${counts.relationships}개`,
    },
    {
      label: "작가 주석",
      present: counts.notes > 0,
      caption: `${counts.notes}개`,
    },
    {
      label: "작품 설정 (장르/분량/시점)",
      present: counts.projectMetaFields > 0,
      caption: `${counts.projectMetaFields}/3`,
    },
    { label: "작가 style notes", present: counts.hasStyleNotes },
  ];

  return (
    <ul className="ai-checklist">
      {items.map((it, i) => (
        <li key={i} className={it.present ? "" : "off"}>
          <span className="ck">{it.present ? <Check size={11} /> : null}</span>
          {it.label}
          {it.caption && <span className="n">{it.caption}</span>}
        </li>
      ))}
    </ul>
  );
}

export function AIContextChecklist({ anchor, counts, onClose }: Props) {
  return (
    <>
      <div
        className="ai-context-checklist"
        style={{ top: anchor.top, left: anchor.left }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <AIContextChecklistList counts={counts} />
      </div>
      {/* invisible backdrop to capture outside click */}
      <div
        style={{ position: "fixed", inset: 0, zIndex: 55 }}
        onMouseDown={onClose}
      />
    </>
  );
}

export function totalContextItems(counts: ContextCounts): number {
  return (
    counts.nearbyScenes +
    (counts.hasOutline ? 1 : 0) +
    (counts.hasSynopsis ? 1 : 0) +
    counts.relatedScenes +
    counts.plotBeats +
    counts.entities +
    counts.relationships +
    counts.notes +
    counts.projectMetaFields +
    (counts.hasStyleNotes ? 1 : 0)
  );
}
