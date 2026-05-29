import { useEffect, useRef, useState } from "react";
import type { AIOptions } from "../../lib/types";
import { TONE_PRESETS } from "../../lib/tonePresets";
import "./AIPromptBar.css";

export type PresetID = "rewrite" | "expand" | "compact" | null;

const PRESET_SEED: Record<Exclude<PresetID, null>, string> = {
  rewrite: "이 단락을 더 자연스럽게 다시 써줘.",
  expand: "이 장면을 더 감각적으로 확장해줘.",
  compact: "이 단락을 한 문장으로 요약해줘.",
};

interface Props {
  anchor: { top: number; left: number } | null;
  hasSelection: boolean;
  busy: boolean;
  options: AIOptions;
  contextItemCount: number;
  errorMessage?: string;
  onOptionsChange: (o: AIOptions) => void;
  onRun: (preset: PresetID, prompt: string, variationsOn: boolean) => void;
  onCancel: () => void;
  onClose: () => void;
  onContextClick: () => void;
}

export function AIPromptBar({
  anchor,
  hasSelection,
  busy,
  options,
  contextItemCount,
  errorMessage,
  onOptionsChange,
  onRun,
  onCancel,
  onClose,
  onContextClick,
}: Props) {
  const [prompt, setPrompt] = useState("");
  const [shake, setShake] = useState(false);
  const [variationsOn, setVariationsOn] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  if (!anchor) return null;

  const submit = (preset: PresetID) => {
    const seed = preset ? PRESET_SEED[preset] : "";
    const text = preset ? seed : prompt.trim();
    if (!text) {
      setShake(true);
      setTimeout(() => setShake(false), 350);
      textareaRef.current?.focus();
      return;
    }
    if (preset && !prompt) setPrompt(seed);
    onRun(preset, text, variationsOn);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      submit(null);
    } else if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    }
  };

  return (
    <div
      className="ai-prompt-bar"
      style={{ top: anchor.top, left: anchor.left }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <div className="ai-prompt-bar-row">
        <div className="ai-prompt-bar-presets">
          <button
            type="button"
            className="ai-prompt-bar-preset-chip"
            disabled={!hasSelection}
            title={!hasSelection ? "선택 영역이 필요합니다" : ""}
            onClick={() => submit("rewrite")}
          >
            재작성
          </button>
          <button
            type="button"
            className="ai-prompt-bar-preset-chip"
            onClick={() => submit("expand")}
          >
            확장
          </button>
          <button
            type="button"
            className="ai-prompt-bar-preset-chip"
            disabled={!hasSelection}
            title={!hasSelection ? "선택 영역이 필요합니다" : ""}
            onClick={() => submit("compact")}
          >
            요약
          </button>
        </div>
        <ToneDropdown options={options} onChange={onOptionsChange} />
        <LengthChip options={options} onChange={onOptionsChange} />
        <button
          type="button"
          className={`ai-prompt-bar-preset-chip${variationsOn ? " active" : ""}`}
          onClick={() => setVariationsOn((v) => !v)}
          aria-pressed={variationsOn}
          title="3개 변형 병렬 생성 (토큰 3배)"
        >
          변형 ×3
        </button>
      </div>

      <textarea
        ref={textareaRef}
        className={`ai-prompt-bar-textarea${shake ? " shake" : ""}`}
        placeholder="프롬프트를 입력하세요…"
        value={prompt}
        onChange={(e) => setPrompt(e.target.value)}
        onKeyDown={onKeyDown}
        rows={2}
      />

      {errorMessage && <p className="ai-prompt-bar-error">오류: {errorMessage}</p>}

      <div className="ai-prompt-bar-footer">
        <button type="button" className="ai-prompt-bar-ctx" onClick={onContextClick}>
          ⓘ ctx: {contextItemCount}개
        </button>
        {busy ? (
          <button type="button" className="ai-prompt-bar-run" onClick={onCancel}>
            취소 Esc
          </button>
        ) : (
          <button type="button" className="ai-prompt-bar-run" onClick={() => submit(null)}>
            생성 ⌘↵
          </button>
        )}
      </div>
    </div>
  );
}

function ToneDropdown({ options, onChange }: { options: AIOptions; onChange: (o: AIOptions) => void }) {
  return (
    <select
      className="ai-prompt-bar-preset-chip"
      value={options.tone}
      onChange={(e) => onChange({ ...options, tone: e.target.value as AIOptions["tone"] })}
    >
      {TONE_PRESETS.map((t) => (
        <option key={t.id} value={t.id}>톤: {t.label}</option>
      ))}
    </select>
  );
}

function LengthChip({ options, onChange }: { options: AIOptions; onChange: (o: AIOptions) => void }) {
  return (
    <button
      type="button"
      className="ai-prompt-bar-preset-chip"
      onClick={() => onChange({ ...options, short_form: !options.short_form })}
      aria-pressed={options.short_form}
    >
      {options.short_form ? "길이: 한 문단" : "길이: 자유"}
    </button>
  );
}
