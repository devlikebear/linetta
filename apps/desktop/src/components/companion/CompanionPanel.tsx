import { useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  Archive,
  Book,
  Check,
  Copy,
  CornerDownLeft,
  HelpCircle,
  Lightbulb,
  MessageSquare,
  Pencil,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { useCompanion, type ChatMessage } from "../../hooks/useCompanion";
import { useSmoothStream } from "../../hooks/useSmoothStream";
import { ProposalCard } from "./ProposalCard";
import { ChoiceCard } from "./ChoiceCard";
import { Markdown } from "./Markdown";
import { useI18n } from "../../lib/i18n";
import "./CompanionPanel.css";

interface Props {
  projectId: string;
  nodeIdRef: { current: string | null };
  onClose: () => void;
  onApplied: () => void;
  beforeSend?: () => Promise<void> | void;
}

// hide proposal/query blocks (even partial/unclosed) from the live stream preview,
// cutting at whichever fence appears first.
function streamProse(text: string): string {
  let idx = -1;
  for (const fence of ["```linetta-proposal", "```linetta-query", "```linetta-choices"]) {
    const i = text.indexOf(fence);
    if (i >= 0 && (idx < 0 || i < idx)) idx = i;
  }
  return (idx >= 0 ? text.slice(0, idx) : text).trimEnd();
}

type Translate = ReturnType<typeof useI18n>["t"];

const PROMPT_EXAMPLE_KEYS = [
  "companion.example.conflict",
  "companion.example.outline",
  "companion.example.search",
  "companion.example.fetch",
  "companion.example.apply",
] as const;

function CompanionEmpty({
  t,
  onPick,
}: {
  t: Translate;
  onPick: (prompt: string) => void;
}) {
  return (
    <section className="companion-empty-card" aria-label={t("companion.examplesLabel")}>
      <div className="companion-empty-kicker">
        <Lightbulb size={14} />
        <span>{t("companion.examplesTitle")}</span>
      </div>
      <h3>{t("companion.emptyTitle")}</h3>
      <p>{t("companion.empty")}</p>
      <div className="companion-example-list">
        {PROMPT_EXAMPLE_KEYS.map((key) => {
          const prompt = t(key);
          return (
            <button
              key={key}
              type="button"
              className="companion-example"
              onClick={() => onPick(prompt)}
            >
              <Lightbulb size={13} />
              <span>{prompt}</span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

function CompanionHelp({ t }: { t: Translate }) {
  return (
    <section className="companion-help-card" aria-label={t("companion.help")}>
      <div className="companion-help-title">
        <HelpCircle size={15} />
        <span>{t("companion.helpTitle")}</span>
      </div>
      <p>{t("companion.helpBody")}</p>
      <div className="companion-tool-list">
        <div className="companion-tool-row">
          <Search size={14} />
          <span>{t("companion.tool.webSearch")}</span>
        </div>
        <div className="companion-tool-row">
          <Book size={14} />
          <span>{t("companion.tool.webFetch")}</span>
        </div>
        <div className="companion-tool-row">
          <Pencil size={14} />
          <span>{t("companion.tool.applyOps")}</span>
        </div>
      </div>
    </section>
  );
}

function formatTranscript(messages: ChatMessage[], liveProse: string, t: Translate): string {
  const rows = messages.map((m) => `${m.role === "user" ? t("companion.transcript.user") : t("companion.transcript.assistant")}:\n${m.content.trim()}`);
  const live = liveProse.trim();
  if (live) rows.push(`${t("companion.transcript.assistant")}:\n${live}`);
  return rows.filter((row) => row.trim()).join("\n\n");
}

export function CompanionPanel({ projectId, nodeIdRef, onClose, onApplied, beforeSend }: Props) {
  const { t } = useI18n();
  const { messages, streaming, thinking, reasoning, status, send, cancel, clear, compact } = useCompanion(projectId, nodeIdRef, onApplied);
  const [draft, setDraft] = useState("");
  const [flushing, setFlushing] = useState(false);
  const [copied, setCopied] = useState(false);
  const [showHelp, setShowHelp] = useState(false);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const focusInput = () => inputRef.current?.focus();

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
  }, [messages, streaming]);

  useLayoutEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    input.style.height = "auto";
    input.style.height = `${input.scrollHeight}px`;
  }, [draft]);

  const sendWithFreshContext = async (text: string) => {
    if (!text.trim() || flushing) return false;
    setFlushing(true);
    try {
      await beforeSend?.();
      await send(text);
      return true;
    } catch {
      return false;
    } finally {
      setFlushing(false);
    }
  };

  const submit = () => {
    const text = draft;
    if (!text.trim()) return;
    void sendWithFreshContext(text).then((sent) => {
      if (sent) setDraft("");
    });
  };

  const isStreaming = status === "streaming";
  const isBusy = isStreaming || flushing;
  // Smooth out chunky/bursty provider deltas so the prose reveals evenly
  // instead of jumping. The completed message still uses the full text.
  const smoothStreaming = useSmoothStream(streaming, isStreaming);
  const liveProse = streamProse(smoothStreaming);
  const hasTranscript = messages.length > 0 || liveProse.trim().length > 0;

  const copyTranscript = async () => {
    const text = formatTranscript(messages, liveProse, t);
    if (!text) return;
    await navigator.clipboard.writeText(text);
    setCopied(true);
  };

  const pickExample = (prompt: string) => {
    setDraft(prompt);
    window.requestAnimationFrame(() => focusInput());
  };

  return (
    <aside className="panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl"><span className="ic"><MessageSquare size={16} /></span> {t("companion.title")}</span>
        <div className="companion-head-actions">
          <button
            type="button"
            className="panel-close companion-action"
            onClick={copyTranscript}
            disabled={!hasTranscript}
            aria-label={t("companion.copy")}
            title={t("companion.copy")}
          >
            {copied ? <Check size={15} /> : <Copy size={15} />}
          </button>
          <button
            type="button"
            className="panel-close companion-action"
            onClick={() => { void compact(); }}
            disabled={!hasTranscript || isStreaming}
            aria-label={t("companion.compact")}
            title={t("companion.compact")}
          >
            <Archive size={15} />
          </button>
          <button
            type="button"
            className="panel-close companion-action"
            onClick={() => { void clear(); }}
            disabled={!hasTranscript || isStreaming}
            aria-label={t("companion.clear")}
            title={t("companion.clear")}
          >
            <Trash2 size={15} />
          </button>
          <button
            type="button"
            className={`panel-close companion-action${showHelp ? " is-active" : ""}`}
            onClick={() => setShowHelp((v) => !v)}
            aria-label={t("companion.help")}
            aria-pressed={showHelp}
            title={t("companion.help")}
          >
            <HelpCircle size={15} />
          </button>
          <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}><X size={16} /></button>
        </div>
      </div>

      <div className="panel-scroll cmp-stream" ref={scrollRef}>
        {showHelp && <CompanionHelp t={t} />}
        {messages.length === 0 && (
          <CompanionEmpty t={t} onPick={pickExample} />
        )}
        {messages.map((m, i) => {
          const isUser = m.role === "user";
          if (isUser) {
            return (
              <div key={i} className="msg user">
                <div className={`msg-bubble${m.errored ? " errored" : ""}`}>{m.content}</div>
              </div>
            );
          }
          const hasCard = !!m.proposal || !!m.choices;
          return (
            <div key={i} className="msg bot">
              {(m.content || !hasCard) && (
                <>
                  <span className="msg-who">companion</span>
                  <div className={`msg-bubble${m.errored ? " errored" : ""}`}>
                    {m.errored ? m.content : <Markdown text={m.content} />}
                  </div>
                </>
              )}
              {m.proposal && (
                <ProposalCard proposal={m.proposal} projectId={projectId} nodeIdRef={nodeIdRef} onApplied={onApplied} />
              )}
              {m.choices && (
                <ChoiceCard choices={m.choices} disabled={isBusy} onPick={(text) => { void sendWithFreshContext(text); }} onCustom={focusInput} />
              )}
            </div>
          );
        })}
        {isStreaming && (
          <div className="msg bot">
            <span className="msg-who">companion</span>
            <div className="companion-thinking">
              <span className="ai-working-dot" aria-hidden="true" />
              {thinking || (liveProse ? t("companion.writing") : t("companion.thinking"))}
            </div>
            {reasoning && (
              <details className="companion-reasoning">
                <summary>{t("companion.reasoning")}</summary>
                <div className="companion-reasoning-body">{reasoning}</div>
              </details>
            )}
            <div className="msg-bubble">{liveProse ? <Markdown text={liveProse} /> : <span className="ai-cursor">&nbsp;</span>}</div>
          </div>
        )}
      </div>

      <div className="cmp-input-wrap">
        <div className="cmp-input">
          <textarea
            ref={inputRef}
            value={draft}
            placeholder={t("companion.placeholder")}
            rows={1}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); submit(); }
            }}
          />
          {isStreaming ? (
            <button type="button" className="cmp-send cmp-stop" onClick={cancel} aria-label={t("companion.stop")}>{t("companion.stop")}</button>
          ) : (
            <button type="button" className="cmp-send" onClick={submit} disabled={!draft.trim() || flushing} aria-label={t("companion.send")}>
              <CornerDownLeft size={16} />
            </button>
          )}
        </div>
        <div className="cmp-hint"><span>web_search</span><span>web_fetch</span><span>linetta_apply_ops</span></div>
      </div>
    </aside>
  );
}
