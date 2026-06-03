import { useCallback, useEffect, useRef, useState } from "react";
import { companion as companionApi } from "../lib/rpc";
import { useEngineEvent } from "./useEngineEvent";
import type {
  CompanionMessage, CompanionProposal, CompanionChoices,
  CompanionDelta, CompanionReset, CompanionDone, CompanionError, CompanionCancelled,
  CompanionApplied, CompanionThinking, CompanionReasoning,
} from "../lib/types";

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
  proposal?: CompanionProposal;
  choices?: CompanionChoices;
  errored?: boolean;
}

export type CompanionStatus = "idle" | "streaming";

// stripProposalBlock removes fenced machine-control blocks from displayed prose.
export function stripProposalBlock(text: string): string {
  return text
    .replace(/```linetta-(?:proposal|query|choices)[\s\S]*?```/g, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function toChatMessage(m: CompanionMessage): ChatMessage {
  return {
    role: m.role === "assistant" ? "assistant" : "user",
    content: m.role === "assistant" ? stripProposalBlock(m.content) : m.content,
  };
}

export function useCompanion(projectId: string, nodeIdRef: { current: string | null }, onApplied?: () => void) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [streaming, setStreaming] = useState("");
  const [thinking, setThinking] = useState("");
  const [reasoning, setReasoning] = useState("");
  const reasoningRef = useRef("");
  const setReasoningBoth = (v: string) => { reasoningRef.current = v; setReasoning(v); };
  const [status, setStatus] = useState<CompanionStatus>("idle");
  const runIdRef = useRef<string | null>(null);
  const streamingRef = useRef("");
  const pendingProposalRef = useRef<CompanionProposal | null>(null);
  const pendingChoicesRef = useRef<CompanionChoices | null>(null);

  const setStreamingBoth = (v: string) => { streamingRef.current = v; setStreaming(v); };

  // Load history on project change.
  useEffect(() => {
    let cancelled = false;
    // History stores only role/content/timestamp; past proposals render as
    // prose (no ProposalCard). Proposals are session-ephemeral by design —
    // re-applying a historical proposal is not supported in Phase 2.
    companionApi.history(projectId)
      .then((msgs: CompanionMessage[]) => {
        if (cancelled) return;
        setMessages(msgs.map(toChatMessage));
      })
      .catch(() => { if (!cancelled) setMessages([]); });
    return () => { cancelled = true; };
  }, [projectId]);

  useEngineEvent<CompanionDelta>("companion-delta", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setStreamingBoth(streamingRef.current + p.text);
  });
  useEngineEvent<CompanionReset>("companion-reset", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setStreamingBoth(p.text);
  });
  useEngineEvent<CompanionThinking>("companion-thinking", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setThinking(p.text);
  });
  useEngineEvent<CompanionReasoning>("companion-reasoning", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setReasoningBoth(reasoningRef.current + p.text);
  });
  useEngineEvent<CompanionProposal>("companion-proposal", (p) => {
    if (p.run_id !== runIdRef.current) return;
    pendingProposalRef.current = p;
  });
  useEngineEvent<CompanionChoices>("companion-choices", (p) => {
    if (p.run_id !== runIdRef.current) return;
    pendingChoicesRef.current = p;
  });
  useEngineEvent<CompanionApplied>("companion-applied", (p) => {
    if (p.run_id !== runIdRef.current) return;
    onApplied?.();
  });
  useEngineEvent<CompanionDone>("companion-done", (p) => {
    if (p.run_id !== runIdRef.current) return;
    const prose = stripProposalBlock(p.full_text);
    const proposal = pendingProposalRef.current ?? undefined;
    const choices = pendingChoicesRef.current ?? undefined;
    setMessages((prev) => [...prev, { role: "assistant", content: prose, proposal, choices }]);
    pendingProposalRef.current = null;
    pendingChoicesRef.current = null;
    setStreamingBoth("");
    setThinking("");
    setReasoningBoth("");
    setStatus("idle");
    runIdRef.current = null;
  });
  useEngineEvent<CompanionError>("companion-error", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setMessages((prev) => [...prev, { role: "assistant", content: p.message, errored: true }]);
    setStreamingBoth("");
    setThinking("");
    setReasoningBoth("");
    setStatus("idle");
    runIdRef.current = null;
  });
  useEngineEvent<CompanionCancelled>("companion-cancelled", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setStreamingBoth("");
    setThinking("");
    setReasoningBoth("");
    setStatus("idle");
    runIdRef.current = null;
  });

  const send = useCallback(async (text: string) => {
    const trimmed = text.trim();
    if (!trimmed || status === "streaming") return;
    setMessages((prev) => [...prev, { role: "user", content: trimmed }]);
    setStatus("streaming");
    setStreamingBoth("");
    setThinking("");
    setReasoningBoth("");
    try {
      const { run_id } = await companionApi.send(projectId, nodeIdRef.current ?? "", trimmed);
      runIdRef.current = run_id;
    } catch (e) {
      setMessages((prev) => [...prev, { role: "assistant", content: String(e), errored: true }]);
      setStatus("idle");
    }
  }, [projectId, status, nodeIdRef]);

  const cancel = useCallback(() => {
    const id = runIdRef.current;
    if (id) companionApi.cancel(id).catch(() => {});
  }, []);

  const clear = useCallback(async () => {
    await companionApi.clear(projectId);
    setMessages([]);
    setStreamingBoth("");
    setThinking("");
    setReasoningBoth("");
    setStatus("idle");
    runIdRef.current = null;
    pendingProposalRef.current = null;
    pendingChoicesRef.current = null;
  }, [projectId]);

  const compact = useCallback(async () => {
    const msgs = await companionApi.compact(projectId);
    setMessages(msgs.map(toChatMessage));
    setStreamingBoth("");
    setThinking("");
    setReasoningBoth("");
    setStatus("idle");
    runIdRef.current = null;
    pendingProposalRef.current = null;
    pendingChoicesRef.current = null;
  }, [projectId]);

  return { messages, streaming, thinking, reasoning, status, send, cancel, clear, compact };
}
