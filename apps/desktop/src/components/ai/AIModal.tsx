import { useEffect, useRef, useState } from "react";
import type { AIOptions, ContextCounts } from "../../lib/types";
import type { CommitMode } from "../../lib/editor/commitGenerated";
import type { GenStatus, GenVariation } from "../../lib/editor/useAIGeneration";
import { TONE_PRESETS } from "../../lib/tonePresets";
import { AIContextChecklistList } from "./AIContextChecklist";
import "./AIModal.css";

interface Props {
  mode: CommitMode;
  canChooseMode: boolean; // true when no selection → show 삽입/전체교체 radio
  options: AIOptions;
  contextItemCount: number;
  variations: GenVariation[];
  currentIdx: number;
  status: GenStatus;
  onModeChange: (m: CommitMode) => void;
  onOptionsChange: (o: AIOptions) => void;
  onRun: (prompt: string, variationsOn: boolean) => void;
  onSwitch: (direction: -1 | 1) => void;
  onAccept: () => void;
  onCancel: () => void;
  onContextClick: () => void;
  showChecklist: boolean;
  checklistCounts: ContextCounts;
}

const MODE_LABEL: Record<CommitMode, string> = {
  replace: "대체",
  insert: "삽입",
  replaceAll: "전체교체",
};

export function AIModal(props: Props) {
  const [prompt, setPrompt] = useState("");
  const [variationsOn, setVariationsOn] = useState(false);
  const [shake, setShake] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const hasResult = props.variations.length > 0;
  const isRunning = props.status.kind === "running";
  const current = props.variations[props.currentIdx];
  const acceptable = !!current && !current.error;

  const run = () => {
    const text = prompt.trim();
    if (!text) {
      setShake(true);
      setTimeout(() => setShake(false), 350);
      textareaRef.current?.focus();
      return;
    }
    props.onRun(text, variationsOn);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      props.onCancel();
      return;
    }
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      run();
      return;
    }
    if (hasResult && e.key === "Tab") {
      e.preventDefault();
      if (acceptable) props.onAccept();
      return;
    }
    if (hasResult && props.variations.length > 1) {
      if (e.key === "ArrowLeft") {
        e.preventDefault();
        props.onSwitch(-1);
      } else if (e.key === "ArrowRight") {
        e.preventDefault();
        props.onSwitch(1);
      }
    }
  };

  const onTextareaKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
      e.preventDefault();
      run();
    }
  };

  return (
    <div className="ai-modal-backdrop" onMouseDown={props.onCancel}>
      <div
        className="ai-modal"
        onMouseDown={(e) => e.stopPropagation()}
        onKeyDown={onKeyDown}
      >
        <div className="ai-modal-modes">
          {props.canChooseMode ? (
            <>
              <label className="ai-modal-mode-radio">
                <input
                  type="radio"
                  name="ai-mode"
                  checked={props.mode === "insert"}
                  onChange={() => props.onModeChange("insert")}
                />
                삽입
              </label>
              <label className="ai-modal-mode-radio">
                <input
                  type="radio"
                  name="ai-mode"
                  checked={props.mode === "replaceAll"}
                  onChange={() => props.onModeChange("replaceAll")}
                />
                전체교체
              </label>
            </>
          ) : (
            <span className="ai-modal-mode-label">모드: {MODE_LABEL[props.mode]}</span>
          )}
        </div>

        <textarea
          ref={textareaRef}
          className={`ai-modal-textarea${shake ? " shake" : ""}`}
          placeholder="프롬프트를 입력하세요…"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          onKeyDown={onTextareaKeyDown}
          rows={3}
        />

        <div className="ai-modal-chiprow">
          <select
            className="ai-modal-chip"
            value={props.options.tone}
            onChange={(e) => props.onOptionsChange({ ...props.options, tone: e.target.value as AIOptions["tone"] })}
          >
            {TONE_PRESETS.map((t) => (
              <option key={t.id} value={t.id}>톤: {t.label}</option>
            ))}
          </select>
          <button
            type="button"
            className="ai-modal-chip"
            onClick={() => props.onOptionsChange({ ...props.options, short_form: !props.options.short_form })}
            aria-pressed={props.options.short_form}
          >
            {props.options.short_form ? "길이: 한 문단" : "길이: 자유"}
          </button>
          <button
            type="button"
            className={`ai-modal-chip${variationsOn ? " active" : ""}`}
            onClick={() => setVariationsOn((v) => !v)}
            aria-pressed={variationsOn}
            title="3개 변형 병렬 생성 (토큰 3배)"
          >
            변형 ×3
          </button>
          <button type="button" className="ai-modal-ctx" onClick={props.onContextClick}>
            ⓘ ctx: {props.contextItemCount}개
          </button>
        </div>

        {props.showChecklist && (
          <AIContextChecklistList counts={props.checklistCounts} />
        )}

        {hasResult && (
          <div className="ai-modal-result">
            {current?.error ? (
              <span className="ai-modal-error">(오류: {current.error})</span>
            ) : (
              <>
                {current?.text}
                {isRunning && !current?.done && <span className="ai-modal-result-cursor">▌</span>}
              </>
            )}
          </div>
        )}

        <div className="ai-modal-footer">
          {hasResult && props.variations.length > 1 && (
            <div className="ai-modal-nav">
              <button type="button" className="ai-modal-chip" onClick={() => props.onSwitch(-1)}>◀</button>
              <span>{props.currentIdx + 1}/{props.variations.length}</span>
              <button type="button" className="ai-modal-chip" onClick={() => props.onSwitch(1)}>▶</button>
            </div>
          )}
          <div className="ai-modal-actions">
            <button type="button" className="ai-modal-btn" onClick={props.onCancel}>
              취소
            </button>
            {!hasResult ? (
              <button type="button" className="ai-modal-btn primary" onClick={run}>
                생성 ⌘↵
              </button>
            ) : (
              <>
                <button type="button" className="ai-modal-btn" onClick={run} title="다시 생성">
                  다시
                </button>
                <button
                  type="button"
                  className="ai-modal-btn primary"
                  onClick={props.onAccept}
                  disabled={!acceptable}
                >
                  수락 Tab
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
