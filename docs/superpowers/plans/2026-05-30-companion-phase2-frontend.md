# Companion Phase 2 — Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 집필 화면에 컴패니언 채팅 패널을 붙여 `companion.*` 스트리밍을 표시하고, `linetta-proposal` 제안을 검토 카드로 보여주고 작가가 기존 RPC로 적용하게 한다.

**Architecture:** Rust 브리지에 companion 이벤트 allowlist 추가 → FE `companion` rpc 네임스페이스 + 이벤트 타입 → 순수 `applyProposal` 실행기 → `useCompanion` 훅(이벤트 구독+상태) → `CompanionPanel`/`ProposalCard` → Workspace 토글·도킹.

**Tech Stack:** React 18 + TS + Vite, Tauri 2(Rust 브리지), `useEngineEvent`(listen 기반).

---

## 사전 지식 (구현자 필독)

- 경로 루트: `/Users/changheonshin/workspace/myworks/linetta`. FE: `apps/desktop`. 검증: `cd apps/desktop && npx tsc --noEmit`. FE 테스트 인프라 없음 → tsc + (실행기는) 타입 보장.
- `main` 브랜치, `--no-verify` 금지, push 금지.
- LSP stale 진단 무시, tsc 출력만 신뢰.
- **이벤트 전달은 allowlist 기반**: `apps/desktop/src-tauri/src/engine.rs`의 notification match가 `ai.*`만 emit. companion 이벤트는 거기 등록해야 FE 도달(`.`→`-`).
- `useEngineEvent(event, handler)` (`apps/desktop/src/hooks/useEngineEvent.ts`): `listen`으로 구독, ref+cancelled 가드. FE는 `companion-delta` 등(하이픈) 이름으로 구독.
- rpc: `rpcCall<T>(method, params)`; 네임스페이스 객체 패턴(`apps/desktop/src/lib/rpc.ts`).
- 기존 입력 타입(types.ts): `NewThreadInput{project_id,name,color?}`, `UpdateThreadInput{id,name?,color?,summary?}`, `NewBeatInput{thread_id,node_id?,label?,description?,intensity?}`, `UpdateBeatInput{id,label?,description?,intensity?}`, `UpdateProjectInput{id,outline?}`. rpc: `threads.create/update`, `beats.create/update/delete`, `projects.update`.
- Workspace 키다운 핸들러(`apps/desktop/src/routes/Workspace.tsx` ~324): Cmd+R/P/I 처리. **Cmd+J는 비어 있음** → companion 토글에 사용.
- `.ws-body` 레이아웃: `aiModal ? "with-ai-panel" : (entitySheetId||threadSheetId) ? "with-sheet" : ""` 클래스로 우측 컬럼 폭 조절. 우측 패널은 상호배타 렌더(AIPanel/EntitySheet/ThreadSheet/ContextPanel).
- 제안 op 스키마(engine `companion.proposal` 이벤트 payload): `{run_id, valid, summary?, ops?, error?}`, op = `{op, ref?, name?, color?, summary?, thread_id?, thread_ref?, node_id?, beat_id?, label?, description?, intensity?, outline?}`.

## File Structure
- Modify: `apps/desktop/src-tauri/src/engine.rs` — companion 이벤트 allowlist.
- Modify: `apps/desktop/src/lib/types.ts` — companion 타입.
- Modify: `apps/desktop/src/lib/rpc.ts` — companion 네임스페이스.
- Create: `apps/desktop/src/lib/applyProposal.ts` — 제안 실행기.
- Create: `apps/desktop/src/hooks/useCompanion.ts` — 채팅 상태 훅.
- Create: `apps/desktop/src/components/companion/CompanionPanel.tsx` (+ `.css`), `ProposalCard.tsx`.
- Modify: `apps/desktop/src/routes/Workspace.tsx` — 토글·도킹·새로고침.

---

## Task 1: Rust 브리지 — companion 이벤트 allowlist

**Files:** `apps/desktop/src-tauri/src/engine.rs`

- [ ] **Step 1: match 확장**

`engine.rs`의 notification match(현재 ai.* 5종)에 companion 6종 추가:
```rust
        let event = match method.as_str() {
            "ai.delta" => "ai-delta",
            "ai.reset" => "ai-reset",
            "ai.done" => "ai-done",
            "ai.error" => "ai-error",
            "ai.cancelled" => "ai-cancelled",
            "companion.delta" => "companion-delta",
            "companion.reset" => "companion-reset",
            "companion.done" => "companion-done",
            "companion.error" => "companion-error",
            "companion.cancelled" => "companion-cancelled",
            "companion.proposal" => "companion-proposal",
            _ => return, // ignore unknown
        };
```

- [ ] **Step 2: 컴파일 확인 + 커밋**

Run: `cd apps/desktop/src-tauri && cargo build 2>&1 | tail -5`
Expected: 컴파일 성공(경고 무방). (cargo가 느리면 `cargo check`도 가능.)
```bash
git add apps/desktop/src-tauri/src/engine.rs
git commit -m "feat(tauri): forward companion.* engine notifications to FE"
```

---

## Task 2: FE 타입 + rpc 네임스페이스

**Files:** `apps/desktop/src/lib/types.ts`, `apps/desktop/src/lib/rpc.ts`

- [ ] **Step 1: types.ts에 companion 타입 추가**

```ts
// Companion (Phase 2) — mirrors engine companion.* payloads.
export interface CompanionMessage {
  role: string;
  content: string;
  timestamp: number;
}

export type ProposalOpType =
  | "create_thread" | "update_thread"
  | "add_beat" | "update_beat" | "delete_beat"
  | "set_outline";

export interface ProposalOp {
  op: ProposalOpType;
  ref?: string;
  name?: string;
  color?: string;
  summary?: string;
  thread_id?: string;
  thread_ref?: string;
  node_id?: string;
  beat_id?: string;
  label?: string;
  description?: string;
  intensity?: number;
  outline?: string;
}

export interface CompanionProposal {
  run_id: string;
  valid: boolean;
  summary?: string;
  ops?: ProposalOp[];
  error?: string;
}

export interface CompanionDelta { run_id: string; text: string; }
export interface CompanionReset { run_id: string; text: string; }
export interface CompanionDone { run_id: string; full_text: string; }
export interface CompanionError { run_id: string; message: string; }
export interface CompanionCancelled { run_id: string; }
```

- [ ] **Step 2: rpc.ts에 companion 네임스페이스 추가**

import 타입 목록에 `CompanionMessage` 추가. 그리고:
```ts
export const companion = {
  send: (projectId: string, nodeId: string, text: string) =>
    rpcCall<{ run_id: string }>("companion.send", { project_id: projectId, node_id: nodeId, text }),
  history: (projectId: string) =>
    rpcCall<{ messages: CompanionMessage[] }>("companion.history", { project_id: projectId })
      .then((r) => r.messages ?? []),
  cancel: (runId: string) =>
    rpcCall<{ ok: true }>("companion.cancel", { run_id: runId }),
};
```

- [ ] **Step 3: tsc + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: types.ts/rpc.ts 에러 없음(나머지 파일은 후속 task).
```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts
git commit -m "feat(desktop): companion FE types + rpc namespace"
```

---

## Task 3: applyProposal 실행기

**Files:** Create `apps/desktop/src/lib/applyProposal.ts`

- [ ] **Step 1: 실행기 작성**

```ts
import type { ProposalOp } from "./types";
import { threads as threadsApi, beats as beatsApi, projects as projectsApi } from "./rpc";

export interface ApplyFailure {
  index: number;
  op: ProposalOp;
  error: string;
}

export interface ApplyResult {
  applied: number;
  failures: ApplyFailure[];
}

// applyProposal executes the selected ops in order against existing RPCs,
// resolving create_thread.ref -> created thread id for later add_beat.thread_ref.
// A failing op is recorded and execution continues (partial apply; no rollback).
export async function applyProposal(ops: ProposalOp[], projectId: string): Promise<ApplyResult> {
  const refMap = new Map<string, string>(); // ref -> created thread id
  const failures: ApplyFailure[] = [];
  let applied = 0;

  for (let i = 0; i < ops.length; i++) {
    const op = ops[i];
    try {
      switch (op.op) {
        case "create_thread": {
          const th = await threadsApi.create({
            project_id: projectId,
            name: op.name ?? "",
            color: op.color,
          });
          if (op.ref) refMap.set(op.ref, th.id);
          if (op.summary) {
            await threadsApi.update({ id: th.id, summary: op.summary });
          }
          break;
        }
        case "update_thread": {
          if (!op.thread_id) throw new Error("thread_id 없음");
          await threadsApi.update({
            id: op.thread_id, name: op.name, color: op.color, summary: op.summary,
          });
          break;
        }
        case "add_beat": {
          const tid = op.thread_id ?? (op.thread_ref ? refMap.get(op.thread_ref) : undefined);
          if (!tid) throw new Error("스토리라인 참조를 해소할 수 없음");
          await beatsApi.create({
            thread_id: tid,
            node_id: op.node_id,
            label: op.label,
            description: op.description,
            intensity: op.intensity,
          });
          break;
        }
        case "update_beat": {
          if (!op.beat_id) throw new Error("beat_id 없음");
          await beatsApi.update({
            id: op.beat_id, label: op.label, description: op.description, intensity: op.intensity,
          });
          break;
        }
        case "delete_beat": {
          if (!op.beat_id) throw new Error("beat_id 없음");
          await beatsApi.delete(op.beat_id);
          break;
        }
        case "set_outline": {
          await projectsApi.update({ id: projectId, outline: op.outline ?? "" });
          break;
        }
        default: {
          throw new Error(`알 수 없는 op: ${(op as ProposalOp).op}`);
        }
      }
      applied++;
    } catch (e) {
      failures.push({ index: i, op, error: e instanceof Error ? e.message : String(e) });
    }
  }
  return { applied, failures };
}
```

- [ ] **Step 2: tsc + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: applyProposal.ts 에러 없음.
```bash
git add apps/desktop/src/lib/applyProposal.ts
git commit -m "feat(desktop): companion proposal apply executor"
```

---

## Task 4: useCompanion 훅

**Files:** Create `apps/desktop/src/hooks/useCompanion.ts`

- [ ] **Step 1: 훅 작성**

```ts
import { useCallback, useEffect, useRef, useState } from "react";
import { companion as companionApi } from "../lib/rpc";
import { useEngineEvent } from "./useEngineEvent";
import type {
  CompanionMessage, CompanionProposal,
  CompanionDelta, CompanionReset, CompanionDone, CompanionError, CompanionCancelled,
} from "../lib/types";

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
  proposal?: CompanionProposal; // attached to the assistant turn that produced it
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
  const [status, setStatus] = useState<CompanionStatus>("idle");
  const runIdRef = useRef<string | null>(null);
  const streamingRef = useRef("");

  // Load history on project change.
  useEffect(() => {
    let cancelled = false;
    companionApi.history(projectId)
      .then((msgs: CompanionMessage[]) => {
        if (cancelled) return;
        setMessages(msgs.map((m) => ({ role: m.role === "assistant" ? "assistant" : "user", content: m.role === "assistant" ? stripProposalBlock(m.content) : m.content })));
      })
      .catch(() => { if (!cancelled) setMessages([]); });
    return () => { cancelled = true; };
  }, [projectId]);

  const setStreamingBoth = (v: string) => { streamingRef.current = v; setStreaming(v); };

  useEngineEvent<CompanionDelta>("companion-delta", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setStreamingBoth(streamingRef.current + p.text);
  });
  useEngineEvent<CompanionReset>("companion-reset", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setStreamingBoth(p.text);
  });
  useEngineEvent<CompanionProposal>("companion-proposal", (p) => {
    if (p.run_id !== runIdRef.current) return;
    // stash on a ref to attach when done arrives
    pendingProposalRef.current = p;
  });
  useEngineEvent<CompanionDone>("companion-done", (p) => {
    if (p.run_id !== runIdRef.current) return;
    const prose = stripProposalBlock(p.full_text);
    setMessages((prev) => [...prev, { role: "assistant", content: prose, proposal: pendingProposalRef.current ?? undefined }]);
    pendingProposalRef.current = null;
    setStreamingBoth("");
    setStatus("idle");
    runIdRef.current = null;
  });
  useEngineEvent<CompanionError>("companion-error", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setMessages((prev) => [...prev, { role: "assistant", content: p.message, errored: true }]);
    setStreamingBoth("");
    setStatus("idle");
    runIdRef.current = null;
  });
  useEngineEvent<CompanionCancelled>("companion-cancelled", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setStreamingBoth("");
    setStatus("idle");
    runIdRef.current = null;
  });

  const pendingProposalRef = useRef<CompanionProposal | null>(null);

  const send = useCallback(async (text: string) => {
    const trimmed = text.trim();
    if (!trimmed || status === "streaming") return;
    setMessages((prev) => [...prev, { role: "user", content: trimmed }]);
    setStatus("streaming");
    setStreamingBoth("");
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

  return { messages, streaming, status, send, cancel };
}
```
NOTE: `pendingProposalRef` is referenced before its declaration in the listing above — when implementing, DECLARE `const pendingProposalRef = useRef<CompanionProposal | null>(null);` near the top (with the other refs), not after the listeners. Fix ordering so it compiles.

- [ ] **Step 2: tsc + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: useCompanion.ts 에러 없음(CompanionPanel은 후속).
```bash
git add apps/desktop/src/hooks/useCompanion.ts
git commit -m "feat(desktop): useCompanion hook (stream events + history + send/cancel)"
```

---

## Task 5: CompanionPanel + ProposalCard + CSS

**Files:** Create `apps/desktop/src/components/companion/ProposalCard.tsx`, `CompanionPanel.tsx`, `CompanionPanel.css`

- [ ] **Step 1: ProposalCard.tsx**

```tsx
import { useState } from "react";
import type { CompanionProposal, ProposalOp } from "../../lib/types";
import { applyProposal, type ApplyResult } from "../../lib/applyProposal";

function opLabel(op: ProposalOp): string {
  switch (op.op) {
    case "create_thread": return `스토리라인 생성: ${op.name ?? ""}`;
    case "update_thread": return `스토리라인 수정`;
    case "add_beat": return `비트 추가: ${op.label ?? ""}`;
    case "update_beat": return `비트 수정: ${op.label ?? ""}`;
    case "delete_beat": return `비트 삭제`;
    case "set_outline": return `작품 개요 설정`;
    default: return op.op;
  }
}

interface Props {
  proposal: CompanionProposal;
  projectId: string;
  onApplied: () => void;
}

export function ProposalCard({ proposal, projectId, onApplied }: Props) {
  const ops = proposal.ops ?? [];
  const [sel, setSel] = useState<boolean[]>(ops.map(() => true));
  const [result, setResult] = useState<ApplyResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [rejected, setRejected] = useState(false);

  if (!proposal.valid) {
    return (
      <div className="companion-proposal invalid">
        <div className="companion-proposal-head">AI 제안 형식 오류</div>
        {proposal.error && <div className="companion-proposal-error">{proposal.error}</div>}
      </div>
    );
  }
  if (rejected) {
    return <div className="companion-proposal done">제안 거절됨</div>;
  }

  const apply = async () => {
    setBusy(true);
    const chosen = ops.filter((_, i) => sel[i]);
    try {
      const res = await applyProposal(chosen, projectId);
      setResult(res);
      onApplied();
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="companion-proposal">
      {proposal.summary && <div className="companion-proposal-head">{proposal.summary}</div>}
      <ul className="companion-proposal-ops">
        {ops.map((op, i) => (
          <li key={i}>
            <label>
              <input
                type="checkbox"
                checked={sel[i]}
                disabled={!!result || busy}
                onChange={(e) => setSel((prev) => prev.map((v, j) => (j === i ? e.target.checked : v)))}
              />
              <span>{opLabel(op)}</span>
            </label>
          </li>
        ))}
      </ul>
      {result ? (
        <div className="companion-proposal-result">
          적용됨 {result.applied}건{result.failures.length > 0 ? ` · 실패 ${result.failures.length}건` : ""}
        </div>
      ) : (
        <div className="companion-proposal-actions">
          <button type="button" onClick={() => setRejected(true)} disabled={busy}>거절</button>
          <button type="button" className="primary" onClick={apply} disabled={busy || sel.every((v) => !v)}>
            {busy ? "적용 중…" : "적용"}
          </button>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: CompanionPanel.tsx**

```tsx
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

export function CompanionPanel({ projectId, nodeIdRef, onClose, onApplied }: Props) {
  const { messages, streaming, status, send, cancel } = useCompanion(projectId, nodeIdRef);
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
            {m.proposal && <ProposalCard proposal={m.proposal} projectId={projectId} onApplied={onApplied} />}
          </div>
        ))}
        {status === "streaming" && (
          <div className="companion-msg assistant">
            <div className="companion-bubble">{streaming || "…"}</div>
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
```

- [ ] **Step 3: CompanionPanel.css** (AIPanel.css 톤 — 우측 도크, 메시지/말풍선/제안 카드)

```css
.companion-panel { display: flex; flex-direction: column; height: 100%; border-left: 1px solid #ece9e0; background: #f6f4ee; }
.companion-head { display: flex; align-items: center; justify-content: space-between; padding: 0.7rem 0.9rem; border-bottom: 1px solid #ece9e0; font-weight: 600; }
.companion-close { background: none; border: none; cursor: pointer; color: #6b675e; }
.companion-messages { flex: 1; overflow-y: auto; padding: 0.8rem; display: flex; flex-direction: column; gap: 0.6rem; }
.companion-empty { color: #b3aea4; font-size: 0.85rem; }
.companion-msg { display: flex; flex-direction: column; gap: 0.3rem; }
.companion-msg.user { align-items: flex-end; }
.companion-bubble { max-width: 90%; padding: 0.5rem 0.7rem; border-radius: 10px; white-space: pre-wrap; line-height: 1.5; font-size: 0.9rem; }
.companion-msg.user .companion-bubble { background: #2980b9; color: #fff; }
.companion-msg.assistant .companion-bubble { background: #fff; border: 1px solid #e6e3da; color: #2c2a26; }
.companion-msg.errored .companion-bubble { background: #fdecec; border-color: #e6b3b3; color: #a33; }
.companion-input { display: flex; gap: 0.4rem; padding: 0.7rem; border-top: 1px solid #ece9e0; align-items: flex-end; }
.companion-input textarea { flex: 1; resize: none; font: inherit; padding: 0.45rem; border: 1px solid #d8d6cf; border-radius: 6px; }
.companion-input button { border: 1px solid #d8d6cf; background: #fff; border-radius: 6px; padding: 0.4rem 0.7rem; cursor: pointer; }
.companion-input button.primary { background: #2980b9; color: #fff; border-color: #2980b9; }
.companion-input button:disabled { opacity: 0.5; cursor: default; }
.companion-proposal { border: 1px solid #d8d6cf; border-radius: 8px; padding: 0.6rem; background: #fbfaf6; font-size: 0.85rem; max-width: 95%; }
.companion-proposal.invalid { border-color: #e6b3b3; background: #fdf3f3; }
.companion-proposal.done { color: #9a958b; font-size: 0.8rem; }
.companion-proposal-head { font-weight: 600; margin-bottom: 0.4rem; }
.companion-proposal-error { color: #a33; font-size: 0.8rem; }
.companion-proposal-ops { list-style: none; margin: 0 0 0.5rem; padding: 0; display: flex; flex-direction: column; gap: 0.25rem; }
.companion-proposal-ops label { display: flex; align-items: center; gap: 0.4rem; cursor: pointer; }
.companion-proposal-actions { display: flex; justify-content: flex-end; gap: 0.4rem; }
.companion-proposal-actions button { border: 1px solid #d8d6cf; background: #fff; border-radius: 6px; padding: 0.3rem 0.6rem; cursor: pointer; }
.companion-proposal-actions button.primary { background: #2980b9; color: #fff; border-color: #2980b9; }
.companion-proposal-result { color: #3e8e41; font-size: 0.8rem; }
```

- [ ] **Step 4: tsc + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: companion 컴포넌트 에러 없음(Workspace 배선은 후속). `X` 아이콘이 `../../lib/icons`에 있는지 확인(ThreadSheet가 사용 중).
```bash
git add apps/desktop/src/components/companion/
git commit -m "feat(desktop): CompanionPanel + ProposalCard + styles"
```

---

## Task 6: Workspace 배선 (토글·도킹·새로고침)

**Files:** `apps/desktop/src/routes/Workspace.tsx`

- [ ] **Step 1: import + 상태 + nodeIdRef**

- import 추가: `import { CompanionPanel } from "../components/companion/CompanionPanel";`
- 상태 추가(다른 패널 상태 근처): `const [companionOpen, setCompanionOpen] = useState(false);`
- 현재 노드 id를 담는 ref(이벤트/전송에서 최신 노드 참조): 기존에 `loadRef`가 있으면 그걸 쓰고, 없으면:
```tsx
  const companionNodeRef = useRef<string | null>(null);
  useEffect(() => { companionNodeRef.current = load?.node.id ?? null; }, [load]);
```
(`load`/`loadRef` 실제 형태 확인 후 맞출 것. `load.node.id`가 현재 씬.)

- [ ] **Step 2: Cmd+J 토글을 키다운 핸들러에 추가**

`Workspace.tsx` ~337의 `else if (e.key.toLowerCase() === "i")` 블록 뒤에 추가:
```tsx
      } else if (e.key.toLowerCase() === "j") {
        if (aiModalOpenRef.current) { e.preventDefault(); return; }
        e.preventDefault();
        setCompanionOpen((v) => !v);
      }
```

- [ ] **Step 3: 명령 팔레트 항목 추가**

`cmds` 배열(AI 섹션)에 추가(기존 항목 형식에 맞춰):
```tsx
        { id: "toggle-companion", label: companionOpen ? "컴패니언 닫기" : "집필 컴패니언 열기", section: "AI", run: () => setCompanionOpen((v) => !v) },
```
(실제 cmd 객체 필드명/시그니처를 기존 항목에서 확인해 맞출 것.)

- [ ] **Step 4: 렌더 슬롯 + 레이아웃 클래스**

`.ws-body` className 식을 companion 우선으로 확장:
```tsx
      <div className={`ws-body${
        companionOpen ? " with-companion-panel" :
        aiModal ? " with-ai-panel" : (entitySheetId || threadSheetId) ? " with-sheet" : ""
      }`}>
```
우측 패널 조건 렌더의 최상위에 companion을 추가(상호배타, 다른 패널보다 우선):
```tsx
        {companionOpen && load ? (
          <CompanionPanel
            projectId={load.project.id}
            nodeIdRef={companionNodeRef}
            onClose={() => { setCompanionOpen(false); focusEditor(); }}
            onApplied={() => {
              // 플롯/스토리라인/멘션 새로고침: 기존 새로고침 훅 사용
              if (load) refreshMentioned(load.node.id);
              // PlotPanel은 자체 reload (node 변경/리마운트 시) — 강제 새로고침이 필요하면
              // 간단히 setLoad로 동일 객체 갱신하거나 별도 신호 사용. 최소: refreshMentioned.
            }}
          />
        ) : aiModal && load ? (
          /* 기존 AIPanel */
        ) : entitySheetId ? (
          /* 기존 EntitySheet */
        ) : threadSheetId ? (
          /* 기존 ThreadSheet */
        ) : (
          /* 기존 ContextPanel */
        )}
```
(기존 분기 본문은 그대로 두고 companion 분기만 맨 앞에 추가. `refreshMentioned`/`focusEditor`는 기존 Workspace 함수 — 실제 이름 확인.)

- [ ] **Step 5: CSS — with-companion-panel 폭**

Workspace 레이아웃 CSS(예: `App.css` 또는 Workspace 관련 css)에서 `.ws-body.with-ai-panel`/`with-sheet`가 정의된 곳을 찾아 옆에 추가:
```css
.ws-body.with-companion-panel { grid-template-columns: 1fr 480px; }
```
(기존 with-ai-panel 규칙의 위치/문법을 그대로 따를 것.)

- [ ] **Step 6: tsc 클린 + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: 에러 0.
```bash
git add apps/desktop/src/routes/Workspace.tsx apps/desktop/src/App.css
git commit -m "feat(desktop): wire CompanionPanel into Workspace (Cmd+J toggle, dock, refresh)"
```

---

## Task 7: 최종 검증

- [ ] **Step 1: 전체 tsc**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: 에러 0.

- [ ] **Step 2: Rust 컴파일 + 엔진 무변경 확인**

Run: `cd apps/desktop/src-tauri && cargo build 2>&1 | tail -3`
Expected: 성공.
Run: `cd /Users/changheonshin/workspace/myworks/linetta && git diff --name-only da478e6..HEAD | grep '^engine/' || echo "engine untouched in phase2"`
(엔진 코드는 Phase 2에서 안 건드림 — 단 go.mod는 Phase 1에서 이미 변경됨; phase2 커밋 범위에 engine/ 변경이 없어야 함.)

- [ ] **Step 3: 커밋(있으면)** — 이미 task별 커밋됨.

## 최종 검증 (모든 Task 후)
- [ ] `cd apps/desktop && npx tsc --noEmit` 클린
- [ ] `cargo build`(src-tauri) 성공
- [ ] 수동 스모크(사용자): Cmd+J로 패널 토글, 이전 대화 로드, 전송→스트리밍(제안 블록 미표시), 제안 카드 적용→플롯 패널 반영, 거절, 무효 제안 표시, 중지

## 범위 밖
- 메모리(Phase 3), 관계·엔티티 op, undo, 멀티 세션 UI.
