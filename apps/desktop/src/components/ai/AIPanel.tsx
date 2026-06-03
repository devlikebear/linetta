import { useEffect, useRef, useState } from "react";
import { ArrowLeft, ArrowRight, Layers, Sparkles, X } from "lucide-react";
import type { AIContextPreview, AIContextSelection, AIOptions } from "../../lib/types";
import type { CommitMode } from "../../lib/editor/commitGenerated";
import type { GenStatus, GenVariation } from "../../lib/editor/useAIGeneration";
import { TONE_PRESETS } from "../../lib/tonePresets";
import { toneLabel, useI18n } from "../../lib/i18n";
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

export function AIPanel(props: Props) {
  const { language, t } = useI18n();
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
  const modeLabel = (mode: CommitMode) => t(`ai.mode.${mode}`);

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
        <span className="ttl"><span className="ic"><Sparkles size={16} /></span> {t("ai.title")}</span>
        <button type="button" className="panel-close" onClick={props.onCancel} aria-label={t("common.close")}><X size={16} /></button>
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
                {t("ai.mode.insert")}
              </button>
              <button
                type="button"
                className={`ai-mode-pill${props.mode === "replaceAll" ? " on" : ""}`}
                onClick={() => props.onModeChange("replaceAll")}
                disabled={isRunning}
              >
                {t("ai.mode.replaceAll")}
              </button>
            </>
          ) : (
            <span className="ai-mode-pill on">{t("ai.mode.label", { mode: modeLabel(props.mode) })}</span>
          )}
        </div>

        <textarea
          ref={textareaRef}
          className={`ai-textarea${shake ? " shake" : ""}`}
          placeholder={t("ai.promptPlaceholder")}
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
              {TONE_PRESETS.map((preset) => (
                <option key={preset.id} value={preset.id}>
                  {t("ai.tonePrefix", { tone: toneLabel(language, preset.id) })}
                </option>
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
            {props.options.short_form ? t("ai.length.short") : t("ai.length.free")}
          </button>
          <button
            type="button"
            className={`chip${variationsOn ? " on" : ""}`}
            onClick={() => setVariationsOn((v) => !v)}
            aria-pressed={variationsOn}
            title={t("ai.variationsTitle")}
            disabled={isRunning}
          >
            {t("ai.variations")}
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
              <span className="ai-working-dot" aria-hidden="true" /> {t("ai.generating")}
            </span>
          </div>
        ) : hasResult ? (
          <div className="ai-result">
            {current?.error ? (
              <span className="ai-result-empty">{t("ai.error", { error: current.error })}</span>
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
              {t("ai.emptyResult", { count: props.contextItemCount })}
            </span>
          </div>
        )}
      </div>

      <div className="panel-foot">
        {hasResult && props.variations.length > 1 && (
          <div className="ai-nav">
            <button type="button" onClick={() => props.onSwitch(-1)} aria-label={t("ai.prevVariation")}><ArrowLeft size={13} /></button>
            <div className="ai-dots">
              {props.variations.map((_, i) => (
                <i key={i} className={i === props.currentIdx ? "on" : ""} />
              ))}
            </div>
            <button type="button" onClick={() => props.onSwitch(1)} aria-label={t("ai.nextVariation")}><ArrowRight size={13} /></button>
          </div>
        )}
        <span className="spacer" />
        <button type="button" className="btn ghost sm" onClick={props.onCancel}>{t("common.cancel")}</button>
        {!hasResult ? (
          <button type="button" className="btn accent sm" onClick={run} disabled={!canRun}>
            {t("ai.generate")} <span className="kbd" style={{ marginLeft: 4 }}>⌘↵</span>
          </button>
        ) : (
          <>
            <button type="button" className="btn ghost sm" onClick={run} title={t("ai.retry")} disabled={!canRun}>{t("ai.retry")}</button>
            <button type="button" className="btn accent sm" onClick={props.onAccept} disabled={!acceptable}>
              {t("ai.accept")} <span className="kbd" style={{ marginLeft: 4 }}>Tab</span>
            </button>
          </>
        )}
      </div>
    </aside>
  );
}
