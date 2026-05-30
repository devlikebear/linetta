# Linetta 컴패니언 — Phase 2: 프런트엔드 (채팅 패널 + 제안 적용) 설계

> 작성일: 2026-05-30
> 구현 레포: **Linetta** (`apps/desktop` FE + `src-tauri` 브리지 1줄군). engine은 Phase 1에서 완료(`companion.send/history/cancel` + `companion.delta/reset/done/error/cancelled/proposal`).
> 사용자 목표("남은 페이즈 모두 개발 완료") 하에 아키텍처는 컴패니언 spec에서 확정된 것을 따르며, 미결 UI 결정은 본 spec에서 확정한다.

## 상위 맥락

컴패니언 = 집필 동료. Phase 1 백엔드 완료. **Phase 2(본 spec)**: 집필 화면에 채팅 패널을 붙여 대화·스트리밍을 표시하고, AI가 낸 `linetta-proposal`(플롯 변경 제안)을 **검토 카드**로 보여주고 **작가가 적용**(기존 RPC 호출)한다. **Phase 3**: 메모리.

## 확정된 Phase 2 결정

1. **패널 위치/토글:** 우측 도킹 패널(AIPanel과 동일한 도킹 패턴, 상호배타). 명령 팔레트 "컴패니언" 항목 + 단축키 **Cmd/Ctrl+J**로 토글. 열리면 우측 컬럼을 차지(ContextPanel/시트와 상호배타), `.ws-body.with-companion-panel { grid-template-columns: 1fr 480px }`.
2. **이벤트 전달:** `src-tauri/src/engine.rs`의 notification allowlist에 `companion.delta/reset/done/error/cancelled/proposal` 6개를 추가(→ `companion-delta` 등으로 emit). 이 Rust 변경 없이는 FE가 이벤트를 못 받음.
3. **프로즈 표시:** 스트리밍 텍스트에서 ` ```linetta-proposal ... ``` ` 블록을 **표시에서 제거**(프로즈만 보여줌). 제안은 별도 카드로.
4. **제안 적용:** `companion.proposal {valid, ops}` 도착 시 카드 렌더. op별 체크박스(기본 전체 선택) + [적용]/[거절]. 적용 = `applyProposal(selectedOps, projectId)`가 ops를 순서대로 **기존 RPC**로 실행하며 `create_thread.ref`→생성된 thread.id를 후속 `add_beat.thread_ref`에 해소. 적용 후 콜백으로 Workspace가 플롯/멘션 데이터 새로고침.
5. **세션 모델:** 프로젝트별 1세션(engine `EnsureWorker(project_id)`). 패널 열 때 `companion.history(project_id)`로 이전 대화 로드. `companion.send`에 현재 씬 `node_id` 전달(씬 중심 컨텍스트).

## 아키텍처 / 컴포넌트

**Rust (브리지):**
- `src-tauri/src/engine.rs` — notification match에 companion 6종 추가.

**FE 타입/클라이언트:**
- `apps/desktop/src/lib/types.ts` — `CompanionMessage{role,content,timestamp}`, 이벤트 페이로드(`CompanionDelta/Reset/Done/Error/Cancelled`), `CompanionProposal{run_id,valid,summary?,ops?,error?}`, `ProposalOp`(plot-core op union).
- `apps/desktop/src/lib/rpc.ts` — `companion` 네임스페이스: `send(projectId, nodeId, text)`→`{run_id}`, `history(projectId)`→`{messages}`(→ CompanionMessage[]), `cancel(runId)`.

**FE 로직/뷰:**
- `apps/desktop/src/lib/applyProposal.ts` — 순수 실행기: `applyProposal(ops: ProposalOp[], projectId: string) → Promise<ApplyResult>`; ref 해소 + 기존 RPC 호출(create_thread→threads.create, update_thread→threads.update, add_beat→beats.create, update_beat→beats.update, delete_beat→beats.delete, set_outline→projects.update). op별 성공/실패 수집, 실패해도 다음 op 진행(부분 성공 리포트).
- `apps/desktop/src/hooks/useCompanion.ts` — 채팅 상태 훅: `messages`, `streaming`(현재 assistant 텍스트), `proposals`(run_id별), `status`; `send(text)`, `cancel()`; companion-* 이벤트 구독(`useEngineEvent`, ref+cancelled 가드). delta append / reset replace / done 확정(프로즈에서 제안 블록 strip) / proposal은 proposals에 추가.
- `apps/desktop/src/components/companion/CompanionPanel.tsx` (+ `.css`) — 헤더 + 메시지 리스트(user/assistant 말풍선, 스트리밍 표시) + 제안 카드 인라인 + 입력창(Enter 전송, Cmd+Enter 줄바꿈 or 반대) + 취소/닫기.
- `apps/desktop/src/components/companion/ProposalCard.tsx` — ops를 한국어로 요약 렌더(op별 체크박스) + [적용]/[거절] + 적용 결과 표시(성공 n/실패 m).
- `apps/desktop/src/routes/Workspace.tsx` — companion 패널 상태(`companionOpen`), 토글(명령 + Cmd/Ctrl+J), `.ws-body` 클래스 + 우측 슬롯에 `<CompanionPanel project nodeId onApplied/>` 렌더(다른 우측 패널과 상호배타), 적용 후 새로고침 콜백.

## 데이터 흐름

```
[열기] Cmd+J/명령 → companionOpen=true → CompanionPanel mount
  → companion.history(projectId) 로 messages 로드
[전송] 입력 → useCompanion.send(text)
  → rpc companion.send(projectId, currentNodeId, text) → {run_id}
  → 이벤트: companion-delta(append)/companion-reset(replace) → 스트리밍 말풍선
            companion-proposal → proposals[run_id] 카드
            companion-done(full_text; 프로즈에서 제안블록 strip 후 확정)/error/cancelled
[적용] 카드 [적용] → applyProposal(selectedOps, projectId)
  → ops 순차 실행(기존 RPC), create_thread.ref→id 해소
  → onApplied() → Workspace가 플롯 패널/멘션/스토리라인 새로고침
```

## 제안 op → RPC 매핑 (applyProposal)

```
create_thread{ref?,name,color?,summary?}  → threads.create({project_id, name, color?})
                                             → 이어 summary 있으면 threads.update({id, summary})
                                             ref→생성 id를 refMap에 기록
update_thread{thread_id,...}               → threads.update({id: thread_id, name?, color?, summary?})
add_beat{thread_id|thread_ref,node_id,...} → tid = thread_id ?? refMap[thread_ref]
                                             → beats.create({thread_id: tid, node_id, label, description?, intensity?})
update_beat{beat_id,...}                   → beats.update({id: beat_id, label?, description?, intensity?})
delete_beat{beat_id}                       → beats.delete(beat_id)
set_outline{outline}                       → projects.update({id: projectId, outline})
```
- threads.create는 color/summary를 다 못 받으므로(시그니처 한계) name/color로 생성 후 summary는 update로 보정.
- 미해소 thread_ref(refMap 미존재) → 해당 op 실패로 기록, 진행 계속.
- `ApplyResult{ applied: number; failed: {op, error}[] }` 반환 → 카드에 표시.

## 에러 처리

- 이벤트 전달 누락 방지: Rust allowlist 6종 등록(검증: 패널에서 send 후 delta 수신).
- `useEngineEvent`는 StrictMode 이중 마운트에 cancelled 가드(기존 패턴) — 그대로 사용.
- send 실패(RPC) → 패널에 인라인 에러, 입력 보존.
- companion-error 이벤트 → 해당 assistant 말풍선을 에러 표시로.
- 빠른 씬 전환/패널 재마운트 → 이벤트 핸들러 ref 패턴 + run_id로 현재 run만 반영.
- applyProposal 부분 실패 → 성공한 op는 유지(되돌리지 않음), 실패 목록 표시(작가가 수동 처리). 적용 후 카드는 "적용됨" 상태로 비활성화.
- 무효 제안(`valid:false`) → 카드에 "AI 제안 형식 오류" + error 사유, [적용] 비활성.

## 테스트 전략

FE 테스트 인프라 없음 → `cd apps/desktop && npx tsc --noEmit` 클린 + 수동 스모크. 단, **`applyProposal`은 순수 매핑 로직이라** 가능한 한 의존을 주입 가능하게 설계(테스트는 없지만 tsc로 타입 보장 + 매핑을 명확히). Rust 변경은 `cargo build`(또는 기존 빌드 파이프라인)로 컴파일 확인. 엔진 빌드/Go 테스트는 무변경(Phase 2는 FE+브리지).

수동 스모크(사용자): 패널 열기→이전 대화 로드, 전송→스트리밍, 제안 카드→적용→플롯 패널 반영, 거절, 무효 제안 표시, 취소.

## 성공 기준

1. Cmd/Ctrl+J·명령으로 컴패니언 패널 토글, 프로젝트별 대화 영속·로드.
2. 전송 시 `companion.send` + delta/reset/done 스트리밍이 말풍선에 표시(제안 블록은 프로즈에서 제거).
3. `companion.proposal` 카드 렌더, [적용]이 ops를 기존 RPC로 실행(ref 해소), 플롯 패널에 반영.
4. `tsc --noEmit` 클린, Rust 브리지 컴파일, 엔진 무변경.

## 범위 밖 (Phase 2 아님)

- 메모리(pkg/memory) 회상·쓰기 — Phase 3.
- 관계·엔티티 op — 후속.
- 제안 적용 되돌리기(undo), 멀티 세션 UI — 후속.
