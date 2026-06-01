import { useEffect } from "react";
import "./ShortcutsModal.css";

interface Shortcut {
  keys: string;
  label: string;
}

const SHORTCUTS: Shortcut[] = [
  { keys: "⌘P", label: "명령 팔레트 열기" },
  { keys: "⌘S", label: "수동 스냅샷 저장" },
  { keys: "⌘.", label: "ZEN 모드 종료 / 다이얼로그 취소" },
  { keys: "esc", label: "다이얼로그 닫기 · ZEN 종료 · 선택 해제" },
  { keys: "⌘⇧F", label: "Focus 모드 토글" },
  { keys: "⌘Z", label: "본문 되돌리기" },
  { keys: "⌘⇧Z", label: "본문 다시 실행" },
  { keys: "@", label: "엔티티 멘션 검색" },
  { keys: "esc (✱)", label: "노트 popover 닫기" },
];

interface Props {
  open: boolean;
  onClose: () => void;
}

export function ShortcutsModal({ open, onClose }: Props) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="backdrop center" onClick={onClose}>
      <div
        className="modal sc-modal"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="단축키 도움말"
      >
        <h2>단축키</h2>
        <p className="modal-sub">대부분의 작업은 키보드만으로 가능해요.</p>
        <div className="sc-grid">
          {SHORTCUTS.map((s) => (
            <div className="sc-item" key={s.keys}>
              <span>{s.label}</span>
              <span className="kbd">{s.keys}</span>
            </div>
          ))}
        </div>
        <div className="modal-actions">
          <button type="button" className="btn accent" onClick={onClose}>닫기</button>
        </div>
      </div>
    </div>
  );
}
