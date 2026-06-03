import { useEffect, useRef, useState } from "react";
import "./ZenMode.css";
import { TiptapEditor, type TiptapHandle } from "./editor/Tiptap";
import { localeForLanguage, useI18n } from "../lib/i18n";

interface Props {
  initialDoc: object;
  initialSelection?: { from: number; to: number } | null;
  charCount: number;
  sceneLabel: string;
  /** Target word count for the progress bar; 0 disables the bar. */
  target?: number;
  onChange: (doc: object) => void;
  onCharCount: (n: number) => void;
  onManualSave: (doc: object) => void;
  onMountEditor?: (handle: TiptapHandle | null) => void;
  onExit: () => void;
}

export function ZenMode({
  initialDoc,
  initialSelection,
  charCount,
  sceneLabel,
  target = 0,
  onChange,
  onCharCount,
  onManualSave,
  onMountEditor,
  onExit,
}: Props) {
  const { language, t } = useI18n();
  const locale = localeForLanguage(language);
  const [showBar, setShowBar] = useState(false);
  const editorRef = useRef<TiptapHandle | null>(null);
  const hideTimer = useRef<number | null>(null);

  // ESC and Cmd+Period exit.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const isMac = navigator.platform.toLowerCase().includes("mac");
      const mod = isMac ? e.metaKey : e.ctrlKey;
      if (e.key === "Escape") {
        e.preventDefault();
        onExit();
      } else if (mod && e.key === ".") {
        e.preventDefault();
        onExit();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onExit]);

  // Restore the saved selection (or focus the end) immediately on mount.
  useEffect(() => {
    window.requestAnimationFrame(() => {
      if (initialSelection) {
        editorRef.current?.setSelection(initialSelection);
      } else {
        editorRef.current?.focus();
      }
    });
    // We intentionally only fire this once on mount; initialSelection is a
    // snapshot at entry time and must not retrigger restores while typing.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Hover top 8px → flash progress bar for 2s.
  const onPointerMove = (e: React.PointerEvent) => {
    if (e.clientY > 8) return;
    setShowBar(true);
    if (hideTimer.current != null) window.clearTimeout(hideTimer.current);
    hideTimer.current = window.setTimeout(() => setShowBar(false), 2000);
  };

  const progressPercent =
    target > 0 ? Math.min(100, Math.round((charCount / target) * 100)) : 0;

  return (
    <div className="zen-overlay" onPointerMove={onPointerMove}>
      {showBar && target > 0 && (
        <div className="zen-progress">
          <div className="zen-progress-fill" style={{ width: `${progressPercent}%` }} />
        </div>
      )}
      <div className="zen-bar">
        <span>ZEN</span>
        <span>{t("workspace.zenStatus", { count: charCount.toLocaleString(locale), scene: sceneLabel })}</span>
      </div>
      <div className="zen-col">
        <TiptapEditor
          ref={(h) => {
            editorRef.current = h;
            onMountEditor?.(h);
          }}
          initialDoc={initialDoc}
          onChange={onChange}
          onCharCount={onCharCount}
          onManualSave={onManualSave}
        />
      </div>
    </div>
  );
}
