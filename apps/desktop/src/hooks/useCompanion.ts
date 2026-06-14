import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { useCallback, useEffect, useState } from "react";
import { companion as companionApi } from "../lib/rpc";
import { extractApplyOpsProposal, stripProposalBlock } from "../lib/companionDisplay";
import type {
  CompanionMessage, CompanionProposal, CompanionChoices,
  CompanionDelta, CompanionReset, CompanionDone, CompanionError, CompanionCancelled,
  CompanionApplied, CompanionThinking, CompanionReasoning, AIContextSelection, CompanionImageAttachment,
} from "../lib/types";

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
  proposal?: CompanionProposal;
  choices?: CompanionChoices;
  errored?: boolean;
}

export type CompanionStatus = "idle" | "streaming";

interface CompanionSessionState {
  messages: ChatMessage[];
  streaming: string;
  thinking: string;
  reasoning: string;
  status: CompanionStatus;
  runId: string | null;
  sending: boolean;
  pendingProposal: CompanionProposal | null;
  pendingChoices: CompanionChoices | null;
}

type CompanionSessionSnapshot = Pick<
  CompanionSessionState,
  "messages" | "streaming" | "thinking" | "reasoning" | "status"
>;

interface CompanionSessionStore {
  state: CompanionSessionState;
  listeners: Set<(snapshot: CompanionSessionSnapshot) => void>;
  appliedListeners: Set<() => void>;
  historyLoaded: boolean;
  historyLoading: boolean;
  historySeq: number;
}

type CompanionRunEvent = { run_id: string; project_id?: string };

const PENDING_RUN_ID = "__linetta_pending_run__";

const stores = new Map<string, CompanionSessionStore>();
const runProjects = new Map<string, string>();
let engineListenersStarted = false;
let engineUnlisteners: UnlistenFn[] = [];
let listenerGeneration = 0;

function initialState(): CompanionSessionState {
  return {
    messages: [],
    streaming: "",
    thinking: "",
    reasoning: "",
    status: "idle",
    runId: null,
    sending: false,
    pendingProposal: null,
    pendingChoices: null,
  };
}

function snapshotFromState(state: CompanionSessionState): CompanionSessionSnapshot {
  return {
    messages: state.messages,
    streaming: state.streaming,
    thinking: state.thinking,
    reasoning: state.reasoning,
    status: state.status,
  };
}

function toChatMessage(m: CompanionMessage): ChatMessage {
  if (m.role !== "assistant") {
    return { role: "user", content: m.content };
  }
  const runId = `history-${m.timestamp}`;
  return {
    role: "assistant",
    content: stripProposalBlock(m.content),
    proposal: extractApplyOpsProposal(m.content, runId) ?? undefined,
  };
}

function getStore(projectId: string): CompanionSessionStore {
  let store = stores.get(projectId);
  if (!store) {
    store = {
      state: initialState(),
      listeners: new Set(),
      appliedListeners: new Set(),
      historyLoaded: false,
      historyLoading: false,
      historySeq: 0,
    };
    stores.set(projectId, store);
  }
  return store;
}

function notifyStore(store: CompanionSessionStore) {
  const snapshot = snapshotFromState(store.state);
  for (const listener of store.listeners) {
    listener(snapshot);
  }
}

function updateStore(projectId: string, update: (state: CompanionSessionState) => CompanionSessionState) {
  const store = getStore(projectId);
  const next = update(store.state);
  if (next === store.state) return;
  store.state = next;
  notifyStore(store);
}

function subscribe(projectId: string, listener: (snapshot: CompanionSessionSnapshot) => void) {
  ensureEngineListeners();
  const store = getStore(projectId);
  store.listeners.add(listener);
  listener(snapshotFromState(store.state));
  return () => {
    store.listeners.delete(listener);
  };
}

function loadHistory(projectId: string) {
  const store = getStore(projectId);
  if (store.historyLoaded || store.historyLoading) return;
  store.historyLoading = true;
  const seq = ++store.historySeq;
  companionApi.history(projectId)
    .then((msgs: CompanionMessage[]) => {
      const current = getStore(projectId);
      if (seq !== current.historySeq) return;
      current.historyLoading = false;
      current.historyLoaded = true;
      updateStore(projectId, (state) => {
        if (state.status === "streaming" || state.messages.length > 0) return state;
        return { ...state, messages: msgs.map(toChatMessage) };
      });
    })
    .catch(() => {
      const current = getStore(projectId);
      if (seq !== current.historySeq) return;
      current.historyLoading = false;
      current.historyLoaded = true;
      updateStore(projectId, (state) => {
        if (state.status === "streaming" || state.messages.length > 0) return state;
        return { ...state, messages: [] };
      });
    });
}

function registerEngineEvent<T>(event: string, handler: (payload: T) => void) {
  const generation = listenerGeneration;
  void listen<T>(event, (e) => {
    if (generation !== listenerGeneration) return;
    handler(e.payload);
  }).then((unlisten) => {
    if (generation !== listenerGeneration) {
      unlisten();
      return;
    }
    engineUnlisteners.push(unlisten);
  });
}

function ensureEngineListeners() {
  if (engineListenersStarted) return;
  engineListenersStarted = true;
  registerEngineEvent<CompanionDelta>("companion-delta", (p) => {
    const projectId = projectIdForRunEvent(p);
    if (!projectId || !acceptRunEvent(projectId, p.run_id)) return;
    updateStore(projectId, (state) => ({ ...state, streaming: state.streaming + p.text }));
  });
  registerEngineEvent<CompanionReset>("companion-reset", (p) => {
    const projectId = projectIdForRunEvent(p);
    if (!projectId || !acceptRunEvent(projectId, p.run_id)) return;
    updateStore(projectId, (state) => ({ ...state, streaming: p.text }));
  });
  registerEngineEvent<CompanionThinking>("companion-thinking", (p) => {
    const projectId = projectIdForRunEvent(p);
    if (!projectId || !acceptRunEvent(projectId, p.run_id)) return;
    updateStore(projectId, (state) => ({ ...state, thinking: p.text }));
  });
  registerEngineEvent<CompanionReasoning>("companion-reasoning", (p) => {
    const projectId = projectIdForRunEvent(p);
    if (!projectId || !acceptRunEvent(projectId, p.run_id)) return;
    updateStore(projectId, (state) => ({ ...state, reasoning: state.reasoning + p.text }));
  });
  registerEngineEvent<CompanionProposal>("companion-proposal", (p) => {
    const projectId = projectIdForRunEvent(p);
    if (!projectId || !acceptRunEvent(projectId, p.run_id)) return;
    updateStore(projectId, (state) => ({ ...state, pendingProposal: p }));
  });
  registerEngineEvent<CompanionChoices>("companion-choices", (p) => {
    const projectId = projectIdForRunEvent(p);
    if (!projectId || !acceptRunEvent(projectId, p.run_id)) return;
    updateStore(projectId, (state) => ({ ...state, pendingChoices: p }));
  });
  registerEngineEvent<CompanionApplied>("companion-applied", (p) => {
    const projectId = projectIdForRunEvent(p);
    if (!projectId || !acceptRunEvent(projectId, p.run_id)) return;
    const store = getStore(projectId);
    for (const listener of store.appliedListeners) {
      listener();
    }
  });
  registerEngineEvent<CompanionDone>("companion-done", (p) => {
    const projectId = projectIdForRunEvent(p);
    if (!projectId || !acceptRunEvent(projectId, p.run_id)) return;
    updateStore(projectId, (state) => {
      const prose = stripProposalBlock(p.full_text);
      const proposal = state.pendingProposal ?? extractApplyOpsProposal(p.full_text, p.run_id) ?? undefined;
      const choices = state.pendingChoices ?? undefined;
      return {
        ...state,
        messages: [...state.messages, { role: "assistant", content: prose, proposal, choices }],
        streaming: "",
        thinking: "",
        reasoning: "",
        status: "idle",
        runId: null,
        sending: false,
        pendingProposal: null,
        pendingChoices: null,
      };
    });
    runProjects.delete(p.run_id);
  });
  registerEngineEvent<CompanionError>("companion-error", (p) => {
    const projectId = projectIdForRunEvent(p);
    if (!projectId || !acceptRunEvent(projectId, p.run_id)) return;
    updateStore(projectId, (state) => ({
      ...state,
      messages: [...state.messages, { role: "assistant", content: p.message, errored: true }],
      streaming: "",
      thinking: "",
      reasoning: "",
      status: "idle",
      runId: null,
      sending: false,
      pendingProposal: null,
      pendingChoices: null,
    }));
    runProjects.delete(p.run_id);
  });
  registerEngineEvent<CompanionCancelled>("companion-cancelled", (p) => {
    const projectId = projectIdForRunEvent(p);
    if (!projectId || !acceptRunEvent(projectId, p.run_id)) return;
    updateStore(projectId, (state) => ({
      ...state,
      streaming: "",
      thinking: "",
      reasoning: "",
      status: "idle",
      runId: null,
      sending: false,
      pendingProposal: null,
      pendingChoices: null,
    }));
    runProjects.delete(p.run_id);
  });
}

function projectIdForRunEvent(event: CompanionRunEvent): string | null {
  if (event.project_id) {
    runProjects.set(event.run_id, event.project_id);
    return event.project_id;
  }
  const known = runProjects.get(event.run_id);
  if (known) return known;

  const candidates: string[] = [];
  for (const [projectId, store] of stores.entries()) {
    if (store.state.runId === event.run_id || store.state.runId === PENDING_RUN_ID) {
      candidates.push(projectId);
    }
  }
  if (candidates.length !== 1) return null;
  const projectId = candidates[0];
  runProjects.set(event.run_id, projectId);
  return projectId;
}

function acceptRunEvent(projectId: string, runId: string): boolean {
  const store = getStore(projectId);
  if (store.state.runId === runId) return true;
  if (store.state.runId === PENDING_RUN_ID) {
    store.state = { ...store.state, runId };
    runProjects.set(runId, projectId);
    notifyStore(store);
    return true;
  }
  return false;
}

export function useCompanion(projectId: string, nodeIdRef: { current: string | null }, onApplied?: () => void, contextSelection?: AIContextSelection, outlineStructure?: string) {
  const [snapshot, setSnapshot] = useState<CompanionSessionSnapshot>(() => snapshotFromState(getStore(projectId).state));

  useEffect(() => {
    const unsubscribe = subscribe(projectId, setSnapshot);
    loadHistory(projectId);
    return unsubscribe;
  }, [projectId]);

  useEffect(() => {
    if (!onApplied) return undefined;
    const store = getStore(projectId);
    store.appliedListeners.add(onApplied);
    return () => {
      store.appliedListeners.delete(onApplied);
    };
  }, [onApplied, projectId]);

  const send = useCallback(async (text: string, images: CompanionImageAttachment[] = []) => {
    ensureEngineListeners();
    const trimmed = text.trim();
    const store = getStore(projectId);
    if (!trimmed || store.state.status === "streaming" || store.state.sending) return;
    updateStore(projectId, (state) => ({
      ...state,
      messages: [...state.messages, { role: "user", content: trimmed }],
      status: "streaming",
      runId: PENDING_RUN_ID,
      sending: true,
      streaming: "",
      thinking: "",
      reasoning: "",
      pendingProposal: null,
      pendingChoices: null,
    }));
    try {
      const payload = contextSelection || outlineStructure || images.length > 0
        ? {
            ...(contextSelection ? { context: contextSelection } : {}),
            ...(outlineStructure ? { outline_structure: outlineStructure } : {}),
            ...(images.length > 0 ? { images } : {}),
          }
        : undefined;
      const { run_id } = payload
        ? await companionApi.send(projectId, nodeIdRef.current ?? "", trimmed, payload)
        : await companionApi.send(projectId, nodeIdRef.current ?? "", trimmed);
      updateStore(projectId, (state) => {
        if (state.runId !== PENDING_RUN_ID) return state;
        runProjects.set(run_id, projectId);
        return { ...state, runId: run_id };
      });
    } catch (e) {
      updateStore(projectId, (state) => ({
        ...state,
        messages: [...state.messages, { role: "assistant", content: String(e), errored: true }],
        status: "idle",
        runId: null,
        sending: false,
      }));
    }
  }, [projectId, nodeIdRef, contextSelection, outlineStructure]);

  const cancel = useCallback(() => {
    const id = getStore(projectId).state.runId;
    if (id && id !== PENDING_RUN_ID) companionApi.cancel(id).catch(() => {});
  }, [projectId]);

  const clear = useCallback(async () => {
    await companionApi.clear(projectId);
    const store = getStore(projectId);
    store.historyLoaded = true;
    updateStore(projectId, () => initialState());
  }, [projectId]);

  const compact = useCallback(async () => {
    const msgs = await companionApi.compact(projectId);
    const store = getStore(projectId);
    store.historyLoaded = true;
    updateStore(projectId, (state) => ({
      ...state,
      messages: msgs.map(toChatMessage),
      streaming: "",
      thinking: "",
      reasoning: "",
      status: "idle",
      runId: null,
      sending: false,
      pendingProposal: null,
      pendingChoices: null,
    }));
  }, [projectId]);

  return {
    messages: snapshot.messages,
    streaming: snapshot.streaming,
    thinking: snapshot.thinking,
    reasoning: snapshot.reasoning,
    status: snapshot.status,
    send,
    cancel,
    clear,
    compact,
  };
}

export function __resetCompanionSessionStoreForTests() {
  listenerGeneration += 1;
  for (const unlisten of engineUnlisteners) {
    unlisten();
  }
  engineUnlisteners = [];
  engineListenersStarted = false;
  stores.clear();
  runProjects.clear();
}
