import { useEffect, useRef, useState } from "react";
import { ArrowLeft, ArrowRight, Layers, Sparkles, X } from "lucide-react";
import type { AIContextPreview, AIContextSelection, AIOptions } from "../../lib/types";
import type { CommitMode } from "../../lib/editor/commitGenerated";
import type { GenStatus, GenVariation } from "../../lib/editor/useAIGeneration";
import { TONE_PRESETS } from "../../lib/tonePresets";
import { AIContextChecklistList } from "./AIContextChecklist";
import "./AIPanel.css";

interface Props {
  mode: CommitMode;
  canChooseMode: boolean; // true when no selection → show 삽입/전체교체 radio
  options: AIOptions;
  contextItemCount: number;
  contextPreview: AIContextPreview;
  contextSelection: AIContextSelection;
  variations: GenVariation[];
  currentIdx: number;
  status: GenStatus;
  onModeChange: (m: CommitMode) => void;
  onOptionsChange: (o: AIOptions) => void;
  onContextSelectionChange: (next: AIContextSelection) => void;
  onRun: (prompt: string, variationsOn: boolean) => void;
  onSwitch: (direction: -1 | 1) => void;
  onAccept: () => void;
  onCancel: () => void;
  onContextClick: () => void;
  showChecklist: boolean;
}

const MODE_LABEL: Record<CommitMode, string> = {
  replace: "대체",
  insert: "삽입",
  replaceAll: "전체교체",
};

export function AIPanel(props: Props) {
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
  const acceptable = !!current && current.done && !current.error && current.text.trim().length > 0;
  const canRun = !isRunning;

  const run = () => {
    if (!canRun) return;
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
    <aside className="panel" onKeyDown={onKeyDown}>
      <div className="panel-head">
        <span className="ttl"><span className="ic"><Sparkles size={16} /></span> AI 생성</span>
        <button type="button" className="panel-close" onClick={props.onCancel} aria-label="닫기"><X size={16} /></button>
      </div>

      <div className="panel-scroll" style={{ padding: 16, display: "flex", flexDirection: "column", gap: 14 }}>
        <div className="ai-modes">
          {props.canChooseMode ? (
            <>
              <button
                type="button"
                className={`ai-mode-pill${props.mode === "insert" ? " on" : ""}`}
                onClick={() => props.onModeChange("insert")}
                disabled={isRunning}
              >
                삽입
              </button>
              <button
                type="button"
                className={`ai-mode-pill${props.mode === "replaceAll" ? " on" : ""}`}
                onClick={() => props.onModeChange("replaceAll")}
                disabled={isRunning}
              >
                전체교체
              </button>
            </>
          ) : (
            <span className="ai-mode-pill on">모드 · {MODE_LABEL[props.mode]}</span>
          )}
        </div>

        <textarea
          ref={textareaRef}
          className={`ai-textarea${shake ? " shake" : ""}`}
          placeholder="프롬프트를 입력하세요…"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          onKeyDown={onTextareaKeyDown}
          rows={3}
          disabled={isRunning}
        />

        <div className="ai-chiprow">
          <span className="chip">
            <select
              value={props.options.tone}
              onChange={(e) => props.onOptionsChange({ ...props.options, tone: e.target.value as AIOptions["tone"] })}
              disabled={isRunning}
            >
              {TONE_PRESETS.map((t) => (
                <option key={t.id} value={t.id}>톤: {t.label}</option>
              ))}
            </select>
          </span>
          <button
            type="button"
            className={`chip${props.options.short_form ? " on" : ""}`}
            onClick={() => props.onOptionsChange({ ...props.options, short_form: !props.options.short_form })}
            aria-pressed={props.options.short_form}
            disabled={isRunning}
          >
            {props.options.short_form ? "길이: 한 문단" : "길이: 자유"}
          </button>
          <button
            type="button"
            className={`chip${variationsOn ? " on" : ""}`}
            onClick={() => setVariationsOn((v) => !v)}
            aria-pressed={variationsOn}
            title="3개 변형 병렬 생성 (토큰 3배)"
            disabled={isRunning}
          >
            변형 ×3
          </button>
          <button type="button" className="chip ctx" onClick={props.onContextClick}>
            <Layers size={13} /> ctx {props.contextItemCount}
          </button>
        </div>

        {props.showChecklist && (
          <AIContextChecklistList
            preview={props.contextPreview}
            selection={props.contextSelection}
            onSelectionChange={props.onContextSelectionChange}
            disabled={isRunning}
          />
        )}

        {isRunning && !current?.text && !current?.error ? (
          <div className="ai-result">
            <span className="ai-working">
              <span className="ai-working-dot" aria-hidden="true" /> AI 생성 중…
            </span>
          </div>
        ) : hasResult ? (
          <div className="ai-result">
            {current?.error ? (
              <span className="ai-result-empty">(오류: {current.error})</span>
            ) : (
              <>
                {current?.text}
                {isRunning && !current?.done && <span className="ai-cursor">&nbsp;</span>}
              </>
            )}
          </div>
        ) : (
          <div className="ai-result">
            <span className="ai-result-empty">
              생성 결과가 여기에 나타납니다. 선택한 컨텍스트 {props.contextItemCount}개가 함께 전달돼요.
            </span>
          </div>
        )}
      </div>

      <div className="panel-foot">
        {hasResult && props.variations.length > 1 && (
          <div className="ai-nav">
            <button type="button" onClick={() => props.onSwitch(-1)} aria-label="이전 변형"><ArrowLeft size={13} /></button>
            <div className="ai-dots">
              {props.variations.map((_, i) => (
                <i key={i} className={i === props.currentIdx ? "on" : ""} />
              ))}
            </div>
            <button type="button" onClick={() => props.onSwitch(1)} aria-label="다음 변형"><ArrowRight size={13} /></button>
          </div>
        )}
        <span className="spacer" />
        <button type="button" className="btn ghost sm" onClick={props.onCancel}>취소</button>
        {!hasResult ? (
          <button type="button" className="btn accent sm" onClick={run} disabled={!canRun}>
            생성 <span className="kbd" style={{ marginLeft: 4 }}>⌘↵</span>
          </button>
        ) : (
          <>
            <button type="button" className="btn ghost sm" onClick={run} title="다시 생성" disabled={!canRun}>다시</button>
            <button type="button" className="btn accent sm" onClick={props.onAccept} disabled={!acceptable}>
              수락 <span className="kbd" style={{ marginLeft: 4 }}>Tab</span>
            </button>
          </>
        )}
      </div>
    </aside>
  );
}
