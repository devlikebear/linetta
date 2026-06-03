import { useEffect, useRef, useState } from "react";
import { Archive, Check, Copy, CornerDownLeft, MessageSquare, Trash2, X } from "lucide-react";
import { useCompanion, type ChatMessage } from "../../hooks/useCompanion";
import { useSmoothStream } from "../../hooks/useSmoothStream";
import { ProposalCard } from "./ProposalCard";
import { ChoiceCard } from "./ChoiceCard";
import { Markdown } from "./Markdown";
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

function formatTranscript(messages: ChatMessage[], liveProse: string): string {
  const rows = messages.map((m) => `${m.role === "user" ? "나" : "컴패니언"}:\n${m.content.trim()}`);
  const live = liveProse.trim();
  if (live) rows.push(`컴패니언:\n${live}`);
  return rows.filter((row) => row.trim()).join("\n\n");
}

export function CompanionPanel({ projectId, nodeIdRef, onClose, onApplied }: Props) {
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
    const text = formatTranscript(messages, liveProse);
    if (!text) return;
    await navigator.clipboard.writeText(text);
    setCopied(true);
  };

  return (
    <aside className="panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl"><span className="ic"><MessageSquare size={16} /></span> 집필 컴패니언</span>
        <div className="companion-head-actions">
          <button
            type="button"
            className="panel-close companion-action"
            onClick={copyTranscript}
            disabled={!hasTranscript}
            aria-label="대화 복사"
            title="대화 복사"
          >
            {copied ? <Check size={15} /> : <Copy size={15} />}
          </button>
          <button
            type="button"
            className="panel-close companion-action"
            onClick={() => { void compact(); }}
            disabled={!hasTranscript || isStreaming}
            aria-label="대화 압축"
            title="대화 압축"
          >
            <Archive size={15} />
          </button>
          <button
            type="button"
            className="panel-close companion-action"
            onClick={() => { void clear(); }}
            disabled={!hasTranscript || isStreaming}
            aria-label="대화 클리어"
            title="대화 클리어"
          >
            <Trash2 size={15} />
          </button>
          <button type="button" className="panel-close" onClick={onClose} aria-label="닫기"><X size={16} /></button>
        </div>
      </div>

      <div className="panel-scroll cmp-stream" ref={scrollRef}>
        {messages.length === 0 && (
          <p className="companion-empty">무엇이든 물어보거나 플롯을 함께 구상해요.</p>
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
              {thinking || (liveProse ? "작성 중…" : "생각 중…")}
            </div>
            {reasoning && (
              <details className="companion-reasoning">
                <summary>추론 중…</summary>
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
            placeholder="메시지… (Enter 전송, Shift+Enter 줄바꿈)"
            rows={1}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); submit(); }
            }}
          />
          {isStreaming ? (
            <button type="button" className="cmp-send cmp-stop" onClick={cancel} aria-label="중지">중지</button>
          ) : (
            <button type="button" className="cmp-send" onClick={submit} disabled={!draft.trim()} aria-label="전송">
              <CornerDownLeft size={16} />
            </button>
          )}
        </div>
        <div className="cmp-hint"><span>web_search</span><span>web_fetch</span><span>linetta_apply_ops</span></div>
      </div>
    </aside>
  );
}
