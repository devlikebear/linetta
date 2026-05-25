import { type FormEvent } from "react";
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
  options: { tone_preset: boolean; short_form: boolean };
  onOptionsChange: (o: { tone_preset: boolean; short_form: boolean }) => void;
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
        <div className="aimode-toolbar">
          <label className="aimode-check">
            <input
              type="checkbox"
              checked={props.options.tone_preset}
              onChange={(e) => props.onOptionsChange({ ...props.options, tone_preset: e.target.checked })}
            /> 톤 프리셋 "내 톤"
          </label>
          <label className="aimode-check">
            <input
              type="checkbox"
              checked={props.options.short_form}
              onChange={(e) => props.onOptionsChange({ ...props.options, short_form: e.target.checked })}
            /> 길이: 한 문단
          </label>
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
