import { useEffect, useRef, useState } from "react";
import { useCompanion } from "../../hooks/useCompanion";
import { ProposalCard } from "./ProposalCard";
import { X } from "../../lib/icons";
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
  for (const fence of ["```linetta-proposal", "```linetta-query"]) {
    const i = text.indexOf(fence);
    if (i >= 0 && (idx < 0 || i < idx)) idx = i;
  }
  return (idx >= 0 ? text.slice(0, idx) : text).trimEnd();
}

export function CompanionPanel({ projectId, nodeIdRef, onClose, onApplied }: Props) {
  const { messages, streaming, thinking, status, send, cancel } = useCompanion(projectId, nodeIdRef, onApplied);
  const [draft, setDraft] = useState("");
  const scrollRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
  }, [messages, streaming]);

  const submit = () => {
    if (!draft.trim()) return;
    send(draft);
    setDraft("");
  };

  const liveProse = streamProse(streaming);

  return (
    <aside className="companion-panel" onMouseDown={(e) => e.stopPropagation()}>
      <header className="companion-head">
        <span>집필 컴패니언</span>
        <button type="button" className="companion-close" onClick={onClose} aria-label="닫기"><X size={16} /></button>
      </header>

      <div className="companion-messages" ref={scrollRef}>
        {messages.length === 0 && <p className="companion-empty">무엇이든 물어보거나 플롯을 함께 구상해요.</p>}
        {messages.map((m, i) => (
          <div key={i} className={`companion-msg ${m.role}${m.errored ? " errored" : ""}`}>
            {m.content && <div className="companion-bubble">{m.content}</div>}
            {m.proposal && <ProposalCard proposal={m.proposal} projectId={projectId} nodeIdRef={nodeIdRef} onApplied={onApplied} />}
          </div>
        ))}
        {status === "streaming" && (
          <div className="companion-msg assistant">
            {thinking && <div className="companion-thinking">🔎 {thinking}</div>}
            <div className="companion-bubble">{liveProse || "…"}</div>
          </div>
        )}
      </div>

      <div className="companion-input">
        <textarea
          value={draft}
          placeholder="메시지… (Enter 전송, Shift+Enter 줄바꿈)"
          rows={2}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); submit(); }
          }}
        />
        {status === "streaming" ? (
          <button type="button" onClick={cancel}>중지</button>
        ) : (
          <button type="button" className="primary" onClick={submit} disabled={!draft.trim()}>전송</button>
        )}
      </div>
    </aside>
  );
}
