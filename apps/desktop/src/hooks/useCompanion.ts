import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { useCallback, useEffect, useState } from "react";
import { companion as companionApi } from "../lib/rpc";
import { extractApplyOpsProposal, stripProposalBlock } from "../lib/companionDisplay";
import type {
  CompanionMessage, CompanionProposal, CompanionChoices,
  CompanionDelta, CompanionReset, CompanionDone, CompanionError, CompanionCancelled,
  CompanionApplied, CompanionThinking, CompanionReasoning, AIContextSelection, CompanionImageAttachment, CompanionIntent,
  CompanionHistoryScope, AISetupIssue,
} from "../lib/types";

export interface ChatMessage {
  id?: string;
  role: "user" | "assistant";
  content: string;
  nodeId?: string | null;
  nodeLabel?: string;
  runId?: string;
  scope?: CompanionMessage["scope"];
  status?: string;
  proposal?: CompanionProposal;
  choices?: CompanionChoices;
  errored?: boolean;
  retryText?: string;
  aiSetupIssue?: AISetupIssue;
  rawError?: string;
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
  appliedListeners: Set<(event: CompanionApplied) => void>;
  historyLoaded: boolean;
  historyLoading: boolean;
  historySeq: number;
  historyRetryCount: number;
  historyRetryTimer: ReturnType<typeof setTimeout> | null;
}

type CompanionRunEvent = { run_id: string; project_id?: string; node_id?: string; scope?: CompanionMessage["scope"] };
type CompanionStoreKey = string;
type CompanionNodeTarget = { current: string | null } | string | null | undefined;

const PENDING_RUN_ID = "__linetta_pending_run__";

const stores = new Map<CompanionStoreKey, CompanionSessionStore>();
const runStores = new Map<string, CompanionStoreKey>();
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
  const base = {
    id: m.id,
    nodeId: m.node_id,
    nodeLabel: m.node_label,
    runId: m.run_id,
    scope: m.scope,
    status: m.status,
  };
  if (m.role !== "assistant") {
    return { ...base, role: "user", content: m.content };
  }
  const runId = m.run_id || `history-${m.timestamp}`;
  return {
    ...base,
    role: "assistant",
    content: stripProposalBlock(m.content),
    proposal: extractApplyOpsProposal(m.content, runId) ?? undefined,
  };
}

export function classifyAISetupIssue(message: string): AISetupIssue | undefined {
  const text = message.toLowerCase();
  if (!text.trim()) return undefined;
  if (
    text.includes("api key is required") ||
    text.includes("auth mode api-key") ||
    text.includes("missing api key") ||
    text.includes("api_key is required") ||
    text.includes("credential") && text.includes("required")
  ) {
    return "missing_key";
  }
  if (
    /\b(401|403)\b/.test(text) ||
    text.includes("unauthorized") ||
    text.includes("forbidden") ||
    text.includes("invalid api key") ||
    text.includes("invalid token")
  ) {
    return "auth_required";
  }
  if (
    text.includes("model not found") ||
    text.includes("invalid model") ||
    text.includes("unknown model") ||
    text.includes("model_unavailable")
  ) {
    return "model_unavailable";
  }
  if (
    text.includes("rate limit") ||
    text.includes("quota") ||
    text.includes("insufficient credits") ||
    text.includes("spend limit") ||
    text.includes("billing hard limit") ||
    text.includes("exceeded your current quota")
  ) {
    return "rate_or_spend_limit";
  }
  if (
    text.includes("provider") ||
    text.includes("llm") ||
    text.includes("openai") ||
    text.includes("anthropic") ||
    text.includes("gemini") ||
    text.includes("claude") ||
    text.includes("codex") ||
    text.includes("openrouter") ||
    text.includes("api ") ||
    text.includes(" api")
  ) {
    return "unknown_provider_error";
  }
  return undefined;
}

function setupErrorMessage(message: string, retryText?: string): ChatMessage {
  const issue = classifyAISetupIssue(message);
  return {
    role: "assistant",
    content: message,
    errored: true,
    ...(retryText ? { retryText } : {}),
    ...(issue ? { aiSetupIssue: issue, rawError: message } : {}),
  };
}

function companionStoreKey(projectId: string, scope: CompanionHistoryScope, nodeId?: string | null): CompanionStoreKey {
  if (scope === "scene" && nodeId) return `${projectId}:scene:${nodeId}`;
  return `${projectId}:project:all`;
}

function currentNodeIdFrom(target: CompanionNodeTarget): string | null {
  if (typeof target === "string") return target || null;
  if (target && typeof target === "object" && "current" in target) return target.current ?? null;
  return null;
}

function getStore(key: CompanionStoreKey): CompanionSessionStore {
  let store = stores.get(key);
  if (!store) {
    store = {
      state: initialState(),
      listeners: new Set(),
      appliedListeners: new Set(),
      historyLoaded: false,
      historyLoading: false,
      historySeq: 0,
      historyRetryCount: 0,
      historyRetryTimer: null,
    };
    stores.set(key, store);
  }
  return store;
}

function notifyStore(store: CompanionSessionStore) {
  const snapshot = snapshotFromState(store.state);
  for (const listener of store.listeners) {
    listener(snapshot);
  }
}

function updateStore(key: CompanionStoreKey, update: (state: CompanionSessionState) => CompanionSessionState) {
  const store = getStore(key);
  const next = update(store.state);
  if (next === store.state) return;
  store.state = next;
  notifyStore(store);
}

function subscribe(key: CompanionStoreKey, listener: (snapshot: CompanionSessionSnapshot) => void) {
  ensureEngineListeners();
  const store = getStore(key);
  store.listeners.add(listener);
  listener(snapshotFromState(store.state));
  return () => {
    store.listeners.delete(listener);
  };
}

function loadHistory(projectId: string, nodeId: string | null, scope: CompanionHistoryScope, key: CompanionStoreKey) {
  const store = getStore(key);
  if (store.historyLoaded || store.historyLoading) return;
  if (store.historyRetryTimer) {
    clearTimeout(store.historyRetryTimer);
    store.historyRetryTimer = null;
  }
  store.historyLoading = true;
  const seq = ++store.historySeq;
  companionApi.history(projectId, scope === "scene" ? nodeId : null, scope)
    .then((msgs: CompanionMessage[]) => {
      const current = getStore(key);
      if (seq !== current.historySeq) return;
      current.historyLoading = false;
      current.historyLoaded = true;
      current.historyRetryCount = 0;
      updateStore(key, (state) => {
        if (state.status === "streaming" || state.messages.length > 0) return state;
        return { ...state, messages: msgs.map(toChatMessage) };
      });
    })
    .catch(() => {
      const current = getStore(key);
      if (seq !== current.historySeq) return;
      current.historyLoading = false;
      current.historyLoaded = false;
      if (current.historyRetryCount < 3) {
        current.historyRetryCount += 1;
        const delayMs = 250 * current.historyRetryCount;
        current.historyRetryTimer = setTimeout(() => {
          const retryStore = getStore(key);
          retryStore.historyRetryTimer = null;
          loadHistory(projectId, nodeId, scope, key);
        }, delayMs);
      }
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
    const key = storeKeyForRunEvent(p);
    if (!key || !acceptRunEvent(key, p.run_id)) return;
    updateStore(key, (state) => ({ ...state, streaming: state.streaming + p.text }));
  });
  registerEngineEvent<CompanionReset>("companion-reset", (p) => {
    const key = storeKeyForRunEvent(p);
    if (!key || !acceptRunEvent(key, p.run_id)) return;
    updateStore(key, (state) => ({ ...state, streaming: p.text }));
  });
  registerEngineEvent<CompanionThinking>("companion-thinking", (p) => {
    const key = storeKeyForRunEvent(p);
    if (!key || !acceptRunEvent(key, p.run_id)) return;
    updateStore(key, (state) => ({ ...state, thinking: p.text }));
  });
  registerEngineEvent<CompanionReasoning>("companion-reasoning", (p) => {
    const key = storeKeyForRunEvent(p);
    if (!key || !acceptRunEvent(key, p.run_id)) return;
    updateStore(key, (state) => ({ ...state, reasoning: state.reasoning + p.text }));
  });
  registerEngineEvent<CompanionProposal>("companion-proposal", (p) => {
    const key = storeKeyForRunEvent(p);
    if (!key || !acceptRunEvent(key, p.run_id)) return;
    updateStore(key, (state) => ({ ...state, pendingProposal: p }));
  });
  registerEngineEvent<CompanionChoices>("companion-choices", (p) => {
    const key = storeKeyForRunEvent(p);
    if (!key || !acceptRunEvent(key, p.run_id)) return;
    updateStore(key, (state) => ({ ...state, pendingChoices: p }));
  });
  registerEngineEvent<CompanionApplied>("companion-applied", (p) => {
    const key = storeKeyForRunEvent(p);
    if (!key || !acceptRunEvent(key, p.run_id)) return;
    const store = getStore(key);
    for (const listener of store.appliedListeners) {
      listener(p);
    }
  });
  registerEngineEvent<CompanionDone>("companion-done", (p) => {
    const key = storeKeyForRunEvent(p);
    if (!key || !acceptRunEvent(key, p.run_id)) return;
    updateStore(key, (state) => {
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
    runStores.delete(p.run_id);
  });
  registerEngineEvent<CompanionError>("companion-error", (p) => {
    const key = storeKeyForRunEvent(p);
    if (!key || !acceptRunEvent(key, p.run_id)) return;
    updateStore(key, (state) => ({
      ...state,
      messages: [...state.messages, setupErrorMessage(p.message, latestUserMessage(state.messages))],
      streaming: "",
      thinking: "",
      reasoning: "",
      status: "idle",
      runId: null,
      sending: false,
      pendingProposal: null,
      pendingChoices: null,
    }));
    runStores.delete(p.run_id);
  });
  registerEngineEvent<CompanionCancelled>("companion-cancelled", (p) => {
    const key = storeKeyForRunEvent(p);
    if (!key || !acceptRunEvent(key, p.run_id)) return;
    updateStore(key, (state) => ({
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
    runStores.delete(p.run_id);
  });
}

function latestUserMessage(messages: ChatMessage[]): string | undefined {
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === "user" && messages[i].content.trim() !== "") {
      return messages[i].content;
    }
  }
  return undefined;
}

function storeKeyForRunEvent(event: CompanionRunEvent): CompanionStoreKey | null {
  const known = runStores.get(event.run_id);
  if (known) return known;
  if (event.project_id && (event.scope === "scene" || event.scope === "project")) {
    const key = companionStoreKey(event.project_id, event.scope, event.node_id ?? null);
    runStores.set(event.run_id, key);
    return key;
  }

  const candidates: string[] = [];
  for (const [key, store] of stores.entries()) {
    if (store.state.runId === event.run_id || store.state.runId === PENDING_RUN_ID) {
      candidates.push(key);
    }
  }
  if (candidates.length !== 1) return null;
  const key = candidates[0];
  runStores.set(event.run_id, key);
  return key;
}

function acceptRunEvent(key: CompanionStoreKey, runId: string): boolean {
  const store = getStore(key);
  if (store.state.runId === runId) return true;
  if (!store.state.runId && runStores.get(runId) === key) {
    store.state = { ...store.state, runId };
    notifyStore(store);
    return true;
  }
  if (store.state.runId === PENDING_RUN_ID) {
    store.state = { ...store.state, runId };
    runStores.set(runId, key);
    notifyStore(store);
    return true;
  }
  return false;
}

export function useCompanion(
  projectId: string,
  nodeTarget: CompanionNodeTarget,
  onApplied?: (event: CompanionApplied) => void,
  contextSelection?: AIContextSelection,
  outlineStructure?: string,
  historyScope: CompanionHistoryScope = "scene",
) {
  const currentNodeId = currentNodeIdFrom(nodeTarget);
  const effectiveScope: CompanionHistoryScope = historyScope === "scene" && currentNodeId ? "scene" : "project";
  const storeKey = companionStoreKey(projectId, effectiveScope, currentNodeId);
  const [snapshot, setSnapshot] = useState<CompanionSessionSnapshot>(() => snapshotFromState(getStore(storeKey).state));

  useEffect(() => {
    const unsubscribe = subscribe(storeKey, setSnapshot);
    loadHistory(projectId, currentNodeId, effectiveScope, storeKey);
    return unsubscribe;
  }, [currentNodeId, effectiveScope, projectId, storeKey]);

  useEffect(() => {
    if (!onApplied) return undefined;
    const store = getStore(storeKey);
    store.appliedListeners.add(onApplied);
    return () => {
      store.appliedListeners.delete(onApplied);
    };
  }, [onApplied, storeKey]);

  const send = useCallback(async (text: string, images: CompanionImageAttachment[] = [], intent?: CompanionIntent) => {
    ensureEngineListeners();
    const trimmed = text.trim();
    const store = getStore(storeKey);
    if (!trimmed || store.state.status === "streaming" || store.state.sending) return;
    updateStore(storeKey, (state) => ({
      ...state,
      messages: [...state.messages, {
        role: "user",
        content: trimmed,
        nodeId: effectiveScope === "scene" ? currentNodeId : null,
        scope: effectiveScope,
      }],
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
      const payload = contextSelection || outlineStructure || images.length > 0 || intent || effectiveScope === "project"
        ? {
            ...(contextSelection ? { context: contextSelection } : {}),
            ...(outlineStructure ? { outline_structure: outlineStructure } : {}),
            ...(images.length > 0 ? { images } : {}),
            ...(intent ? { intent } : {}),
            ...(effectiveScope === "project" ? { scope: "project" as const } : {}),
          }
        : undefined;
      const { run_id } = payload
        ? await companionApi.send(projectId, effectiveScope === "scene" ? currentNodeId ?? "" : "", trimmed, payload)
        : await companionApi.send(projectId, currentNodeId ?? "", trimmed);
      updateStore(storeKey, (state) => {
        if (state.runId !== PENDING_RUN_ID) return state;
        runStores.set(run_id, storeKey);
        return { ...state, runId: run_id };
      });
    } catch (e) {
      updateStore(storeKey, (state) => ({
        ...state,
        messages: [...state.messages, setupErrorMessage(String(e), latestUserMessage(state.messages))],
        status: "idle",
        runId: null,
        sending: false,
      }));
    }
  }, [contextSelection, currentNodeId, effectiveScope, outlineStructure, projectId, storeKey]);

  const cancel = useCallback(() => {
    const id = getStore(storeKey).state.runId;
    if (id && id !== PENDING_RUN_ID) companionApi.cancel(id).catch(() => {});
  }, [storeKey]);

  const clear = useCallback(async () => {
    await companionApi.clear(projectId, effectiveScope === "scene" ? currentNodeId : null, effectiveScope);
    const store = getStore(storeKey);
    store.historyLoaded = true;
    updateStore(storeKey, () => initialState());
  }, [currentNodeId, effectiveScope, projectId, storeKey]);

  const compact = useCallback(async () => {
    const msgs = await companionApi.compact(projectId, effectiveScope === "scene" ? currentNodeId : null, effectiveScope);
    const store = getStore(storeKey);
    store.historyLoaded = true;
    updateStore(storeKey, (state) => ({
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
  }, [currentNodeId, effectiveScope, projectId, storeKey]);

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
  for (const store of stores.values()) {
    if (store.historyRetryTimer) clearTimeout(store.historyRetryTimer);
  }
  stores.clear();
  runStores.clear();
}
