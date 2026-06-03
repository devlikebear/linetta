import { useEffect, useRef, useState } from "react";
import { Archive, Check, Copy, CornerDownLeft, MessageSquare, Trash2, X } from "lucide-react";
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

function formatTranscript(messages: ChatMessage[], liveProse: string, t: Translate): string {
  const rows = messages.map((m) => `${m.role === "user" ? t("companion.transcript.user") : t("companion.transcript.assistant")}:\n${m.content.trim()}`);
  const live = liveProse.trim();
  if (live) rows.push(`${t("companion.transcript.assistant")}:\n${live}`);
  return rows.filter((row) => row.trim()).join("\n\n");
}

export function CompanionPanel({ projectId, nodeIdRef, onClose, onApplied }: Props) {
  const { t } = useI18n();
  const { messages, streaming, thinking, reasoning, status, send, cancel, clear, compact } = useCompanion(projectId, nodeIdRef, onApplied);
  const [draft, setDraft] = useState("");
  const [copied, setCopied] = useState(false);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const focusInput = () => inputRef.current?.focus();

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
  }, [messages, streaming]);

  const submit = () => {
    if (!draft.trim()) return;
    send(draft);
    setDraft("");
  };

  const isStreaming = status === "streaming";
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
          <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}><X size={16} /></button>
        </div>
      </div>

      <div className="panel-scroll cmp-stream" ref={scrollRef}>
        {messages.length === 0 && (
          <p className="companion-empty">{t("companion.empty")}</p>
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
                <ChoiceCard choices={m.choices} disabled={isStreaming} onPick={send} onCustom={focusInput} />
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
            <button type="button" className="cmp-send" onClick={submit} disabled={!draft.trim()} aria-label={t("companion.send")}>
              <CornerDownLeft size={16} />
            </button>
          )}
        </div>
        <div className="cmp-hint"><span>web_search</span><span>web_fetch</span><span>linetta_apply_ops</span></div>
      </div>
    </aside>
  );
}
