import { useCallback, useEffect, useRef, useState } from "react";
import { companion as companionApi } from "../lib/rpc";
import { useEngineEvent } from "./useEngineEvent";
import type {
  CompanionMessage, CompanionProposal,
  CompanionDelta, CompanionReset, CompanionDone, CompanionError, CompanionCancelled,
  CompanionThinking,
} from "../lib/types";

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
  proposal?: CompanionProposal;
  errored?: boolean;
}

export type CompanionStatus = "idle" | "streaming";

// stripProposalBlock removes the fenced linetta-proposal block from displayed prose.
export function stripProposalBlock(text: string): string {
  return text.replace(/```linetta-proposal[\s\S]*?```/g, "").replace(/\n{3,}/g, "\n\n").trim();
}

export function useCompanion(projectId: string, nodeIdRef: { current: string | null }) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [streaming, setStreaming] = useState("");
  const [thinking, setThinking] = useState("");
  const [status, setStatus] = useState<CompanionStatus>("idle");
  const runIdRef = useRef<string | null>(null);
  const streamingRef = useRef("");
  const pendingProposalRef = useRef<CompanionProposal | null>(null);

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
        setMessages(
          msgs.map((m) => ({
            role: m.role === "assistant" ? "assistant" : "user",
            content: m.role === "assistant" ? stripProposalBlock(m.content) : m.content,
          })),
        );
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
  useEngineEvent<CompanionProposal>("companion-proposal", (p) => {
    if (p.run_id !== runIdRef.current) return;
    pendingProposalRef.current = p;
  });
  useEngineEvent<CompanionDone>("companion-done", (p) => {
    if (p.run_id !== runIdRef.current) return;
    const prose = stripProposalBlock(p.full_text);
    const proposal = pendingProposalRef.current ?? undefined;
    setMessages((prev) => [...prev, { role: "assistant", content: prose, proposal }]);
    pendingProposalRef.current = null;
    setStreamingBoth("");
    setThinking("");
    setStatus("idle");
    runIdRef.current = null;
  });
  useEngineEvent<CompanionError>("companion-error", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setMessages((prev) => [...prev, { role: "assistant", content: p.message, errored: true }]);
    setStreamingBoth("");
    setThinking("");
    setStatus("idle");
    runIdRef.current = null;
  });
  useEngineEvent<CompanionCancelled>("companion-cancelled", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setStreamingBoth("");
    setThinking("");
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

  return { messages, streaming, thinking, status, send, cancel };
}
