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

export function AIContextChecklist({ anchor, counts, onClose }: Props) {
  const items: { label: string; present: boolean; caption?: string }[] = [
    { label: "현재 씬 본문", present: true },
    {
      label: "인근 씬 요약 (직전 2개 + 직후 1개)",
      present: counts.nearbyScenes > 0,
      caption: `${counts.nearbyScenes}개`,
    },
    {
      label: "같은 장 다른 씬",
      present: counts.sameChapter > 0,
      caption: `${counts.sameChapter}개`,
    },
    {
      label: "형제 장 요약",
      present: counts.otherChapter > 0,
      caption: `${counts.otherChapter}개`,
    },
    {
      label: "형제 부 요약",
      present: counts.otherPart > 0,
      caption: `${counts.otherPart}개`,
    },
    { label: "작품 시놉시스", present: counts.hasSynopsis },
    {
      label: "관련 과거 씬 (멘션 RAG)",
      present: counts.relatedScenes > 0,
      caption: `${counts.relatedScenes}개`,
    },
    {
      label: "등장 인물·장소",
      present: counts.entities > 0,
      caption: `${counts.entities}개`,
    },
    {
      label: "활성 스토리라인",
      present: counts.activeThreads > 0,
      caption: `${counts.activeThreads}개`,
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
    <>
      <div
        className="ai-context-checklist"
        style={{ top: anchor.top, left: anchor.left }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <h5>AI에게 전달되는 컨텍스트</h5>
        <ul>
          {items.map((it, i) => (
            <li key={i} className={it.present ? "" : "item-disabled"}>
              <span>{it.present ? "✓" : "—"} {it.label}</span>
              {it.caption && <span className="item-count">{it.caption}</span>}
            </li>
          ))}
        </ul>
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
    counts.sameChapter +
    counts.otherChapter +
    counts.otherPart +
    (counts.hasSynopsis ? 1 : 0) +
    counts.relatedScenes +
    counts.entities +
    counts.activeThreads +
    counts.notes +
    counts.projectMetaFields +
    (counts.hasStyleNotes ? 1 : 0)
  );
}
