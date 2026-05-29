import { useEffect } from "react";
import { X } from "../lib/icons";
import "./ShortcutsModal.css";

interface Shortcut {
  keys: string;
  label: string;
}

const SHORTCUTS: Shortcut[] = [
  { keys: "Cmd+P", label: "명령 팔레트 열기" },
  { keys: "Cmd+S", label: "수동 스냅샷 저장" },
  { keys: "Cmd+.", label: "ZEN 모드 종료 / 다이얼로그 취소" },
  { keys: "ESC", label: "다이얼로그 닫기 · ZEN 종료 · 선택 해제" },
  { keys: "Cmd+Shift+F", label: "Focus 모드 토글" },
  { keys: "Cmd+Z", label: "본문 되돌리기" },
  { keys: "Cmd+Shift+Z", label: "본문 다시 실행" },
  { keys: "@", label: "엔티티 멘션 검색" },
  { keys: "ESC (✱ 위에서)", label: "노트 popover 닫기" },
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
    <div className="shortcuts-backdrop" onClick={onClose}>
      <div
        className="shortcuts-modal"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="단축키 도움말"
      >
        <header className="shortcuts-head">
          <h3>단축키</h3>
          <button
            type="button"
            className="shortcuts-close"
            onClick={onClose}
            aria-label="닫기"
          >
            <X size={16} />
          </button>
        </header>
        <ul className="shortcuts-list">
          {SHORTCUTS.map((s) => (
            <li key={s.keys} className="shortcuts-row">
              <kbd className="shortcuts-keys">{s.keys}</kbd>
              <span className="shortcuts-label">{s.label}</span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
