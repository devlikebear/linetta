import { type FormEvent } from "react";
import type { AIOptions } from "../../lib/types";
import { TONE_PRESETS } from "../../lib/tonePresets";
import "./AIMode.css";

export type AIRunStatus =
  | { kind: "idle" }
  | { kind: "running"; runId: string; text: string }
  | { kind: "done"; text: string }
  | { kind: "error"; message: string; text: string };

interface Props {
  status: AIRunStatus;
  /** Initial prompt (populated by preset chips). */
  prompt: string;
  onPromptChange: (v: string) => void;
  options: AIOptions;
  onOptionsChange: (o: AIOptions) => void;
  onPresetClick: (preset: "rewrite" | "expand" | "compact") => void;
  onRun: () => void;
  onCancel: () => void;
  onInsert: (text: string) => void;
  onReplace: (text: string) => void;
  onRegenerate: () => void;
  onDiscard: () => void;
  /** One-line summary of what the engine attached (for the meta line). */
  contextSummary: string;
}

export function AIMode(props: Props) {
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (props.status.kind === "running") props.onCancel();
    else props.onRun();
  };

  const showActions = props.status.kind === "done";
  const streamingText =
    props.status.kind === "running" || props.status.kind === "error"
      ? props.status.text
      : props.status.kind === "done"
      ? props.status.text
      : "";

  return (
    <div className="aimode">
      <form className="aimode-prompt" onSubmit={submit}>
        <div className="aimode-presets">
          <button type="button" onClick={() => props.onPresetClick("rewrite")}>재작성</button>
          <button type="button" onClick={() => props.onPresetClick("expand")}>확장</button>
          <button type="button" onClick={() => props.onPresetClick("compact")}>요약</button>
        </div>
        <label className="aimode-prompt-label">PROMPT</label>
        <textarea
          className="aimode-textarea"
          value={props.prompt}
          onChange={(e) => props.onPromptChange(e.target.value)}
          placeholder="작가의 지시를 입력하세요…"
          rows={4}
        />
        <div className="ai-chip-row">
          <span className="ai-chip-label">톤</span>
          {TONE_PRESETS.map((t) => (
            <button
              type="button"
              key={t.id}
              className={`ai-chip${props.options.tone === t.id ? " active" : ""}`}
              onClick={() => props.onOptionsChange({ ...props.options, tone: t.id })}
            >
              {t.label}
            </button>
          ))}
          <span className="ai-chip-label" style={{ marginLeft: "0.6rem" }}>길이</span>
          <button
            type="button"
            className={`ai-chip${props.options.short_form ? " active" : ""}`}
            onClick={() => props.onOptionsChange({ ...props.options, short_form: !props.options.short_form })}
            aria-pressed={props.options.short_form}
          >
            한 문단
          </button>
        </div>
        <div className="aimode-toolbar">
          <span className="aimode-spacer" />
          <button type="submit" className="aimode-run">
            {props.status.kind === "running" ? "취소" : "생성"}
          </button>
        </div>
      </form>

      <section className="aimode-output">
        <p className="aimode-meta">{streamingText ? `생성됨 · ${props.contextSummary}` : "결과가 여기에 표시됩니다"}</p>
        <div className="aimode-stream">
          {streamingText.split(/\n/).map((line, i) => (
            <p key={i}>{line || " "}</p>
          ))}
          {props.status.kind === "running" && <span className="aimode-cursor">▌</span>}
          {props.status.kind === "error" && (
            <p className="aimode-error">오류: {props.status.message}</p>
          )}
        </div>
        {showActions && (
          <div className="aimode-actions">
            <button type="button" onClick={() => props.onInsert(streamingText)}>커서에 삽입</button>
            <button type="button" onClick={() => props.onReplace(streamingText)}>선택 영역 교체</button>
            <button type="button" onClick={props.onRegenerate}>다시 생성</button>
            <button type="button" onClick={props.onDiscard}>버리기</button>
          </div>
        )}
      </section>
    </div>
  );
}
