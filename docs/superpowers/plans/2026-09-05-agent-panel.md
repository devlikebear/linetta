# 에이전트 패널 (#95)

에픽 #90 서브프로젝트 1의 다섯 번째 단계. 설계 문서
`docs/superpowers/specs/2026-09-02-builtin-agent-byok-design.md` 8.2절·10절.

#93이 엔진을 끝냈고 #94가 프로바이더를 연결할 수 있게 했다. 이 이슈가 작가에게
에이전트를 부를 방법을 준다. 이게 머지되기 전까지 내장 에이전트에 닿는 유일한
길은 수동 JSON-RPC 호출이다.

## 조사에서 확인된 사실

배관은 대부분 깔려 있다. 확인한 것:

| 항목 | 상태 |
| --- | --- |
| `agent.*` RPC 래퍼와 타입 | `rpc.ts`, `types.ts`에 존재. 허용 목록도 등재됨 |
| 알림 5종의 Tauri 전달 | `ffi.rs`가 전부 전달하고 Rust 테스트가 이름을 고정 |
| `useEngineEvent` | 존재. `useMcpChanges`가 실사용 중 |
| `useSmoothStream` | 존재하지만 **호출부가 하나도 없다.** 이 이슈가 첫 소비자 |
| `react-markdown` + `remark-gfm` | 의존성에 있지만 import 하는 곳이 없다. 삭제된 1.0 컴패니언의 `Markdown.tsx`가 이 이슈를 위해 남겨진 것으로 보인다 |
| Cmd/Ctrl+J | 의도적으로 비어 있음. `Workspace.tsx`와 `ShortcutsModal.tsx` 양쪽에 "컴패니언과 함께 사라졌고 재할당하지 않는다"는 주석이 있다 |

### 함정 여섯

**1. `agent.tool` 알림은 툴 호출당 두 번 온다.** `state: "started"` 다음에
`"done"` 또는 `"error"`. **호출별 id가 없다** — `run_id`와 `name`뿐이다. 한 턴에
같은 툴을 여러 번 부르면 도착 순서가 유일한 상관자다. 접힌 한 줄로 보이려면
패널이 이 둘을 하나로 합쳐야 한다.

**2. `AgentHistoryRow.content`는 `role === "tool"`일 때 JSON 문자열이다.**
`{name, summary, ok, batch_id, node_ids}` 모양의 `toolEvent`다. 히스토리를
복원하려면 tool 행만 `JSON.parse` 해야 한다. user/assistant 행은 평범한 문자열.

**3. 같은 밀리초에 쓰인 행이 uuid 순서로 돌아온다.** `companion/history.go`의
`List`가 `created_at ASC, CASE role WHEN 'user' THEN 0 ELSE 1 END, id ASC`로
정렬하는데, 한 턴의 assistant와 tool 행은 둘 다 'user'가 아니라 tie-break이
uuid v4다. 윈도우의 거친 타이머에서 실제로 발생한다. **패널이 혼자 못 고친다.**
작업 1이 엔진에서 고친다.

**4. 오른쪽 패널 슬롯의 "하나만 열림"이 두 곳에서 따로 유지된다.** 각
`toggle*` 콜백이 나머지를 직접 닫고, `reconcileInspector`는 `sizeClass ===
"ipad"`일 때만 개입한다. 네 번째 패널은 `InspectorState`, `PRIORITY`, 기존
`toggle*` 셋 **전부**에 들어가야 한다. 하나라도 빠지면 iPad에서 조용히 겹친다.

**5. `ws-body`의 폭 클래스가 네 번째 패널을 모른다.**
`(factBookOpen||contextualEditOpen||canonOpen) ? " right-wide" : ""`에 새 불리언을
넣지 않으면 패널이 좁은 기본 폭으로 그려진다.

**6. 충돌 배너가 누가 썼든 "외부 에이전트"라고 말한다.** `mcp.changed`에
`source`가 실려 오지만(#93이 넣었다) `useMcpChanges`가 그걸 **반환하지도 않고**
배너가 분기하지도 않는다. 작가가 방금 패널에서 시킨 변경에 대해 "외부 에이전트가
이 씬을 고쳤습니다"를 읽게 된다.

## Global Constraints

- **작업 1만 엔진을 건드린다.** 나머지는 전부 `apps/desktop`이다. 다른 작업에서
  엔진 변경이 필요해 보이면 계획서의 오류이니 멈추고 보고할 것.
- **툴 호출은 접힌 한 줄로만 보인다.** 인자 전체를 보여주지 않는다. 옛 컴패니언의
  액션 팔레트·인텐트 칩·스코프 선택기는 돌아오지 않는다.
- **패널을 닫아도 실행은 계속된다.** 엔진의 턴은 RPC 호출의 수명과 무관하게 돈다
  (#93이 `context.WithoutCancel`로 그렇게 만들었다). 패널은 다시 열 때
  `agent.history`로 복원한다.
- **이유 코드는 번역해서 보여준다.** 원문 메시지를 화면에 쓰지 않는다. 특히
  `agent_internal_error`는 영문 메시지에 Go 패닉 값이 실려 있다.
- **i18n 키는 세 카탈로그(ko/en/ja)에 같은 커밋에서 들어간다.** `agentPanel.*`
  네임스페이스를 쓴다.
- **`data-testid`로 단언하고, i18n은 키 에코로 모킹한다.** 이 저장소의 관습이다.
- 새 `rpcCall` 래퍼를 만들면 `RENDERER_ENGINE_METHODS`에 정렬된 위치로 넣는다.
  이 계획서는 만들지 않으므로 해당 없음이어야 한다.
- CI는 윈도우에서도 Go 전체 스위트를 돌린다. 작업 1은 거기 걸린다.

## 작업 목록

1. 엔진: 트랜스크립트 순서를 결정적으로 (마이그레이션 + `seq` 열)
2. `useMcpChanges`가 출처를 알려주고, 배너가 그걸로 분기
3. 패널 껍데기: 슬롯, Cmd/Ctrl+J, 미설정 안내
4. 메시지 목록과 스트리밍
5. 툴 줄과 되돌리기
6. 작성기, 정지, 시작 칩, 사용량
7. 히스토리 복원과 오류 처리

---

## Task 1: 트랜스크립트 순서를 결정적으로

한 턴의 assistant 행과 tool 행이 같은 밀리초에 쓰이면 uuid 순서로 돌아온다.
패널이 답변을 그걸 만든 툴 칩보다 먼저 그릴 수 있다.

**조사에서 밝혀진 것: 마이그레이션이 필요 없다.** `companion_messages`는
`WITHOUT ROWID`가 아니므로 SQLite의 암묵적 `rowid`가 이미 있고, 그건 정확히
삽입 순서다. 새 열도, 카운터도, 마이그레이션도 필요 없다 — `ORDER BY`의
tie-break를 `id`에서 `rowid`로 바꾸면 된다.

`id`는 uuid v4라 무작위다. `rowid`는 단조 증가한다. 트랜스크립트가 알고 싶은
것은 "어느 쪽이 먼저 쓰였나"이고, 그건 `rowid`가 그대로 답한다.

### 파일

- `engine/internal/companion/history.go` — `ORDER BY` 두 곳
- `engine/internal/companion/history_test.go` — 순서를 고정하는 테스트

### 1-1. 정렬

`listHistorySQL`(현재 122·126행)의 두 `ORDER BY`를 바꾼다.

안쪽(최근 N개를 고르는 서브셀렉트):
```sql
ORDER BY created_at DESC, CASE role WHEN 'assistant' THEN 0 ELSE 1 END, rowid DESC
```
바깥쪽(시간순 복원):
```sql
ORDER BY m.created_at ASC, CASE m.role WHEN 'user' THEN 0 ELSE 1 END, m.rowid ASC
```

**`role` tie-break는 남긴다.** 그건 무작위가 아니라 의도적이다 — 같은 밀리초의
user 행은 assistant 행보다 먼저 온다. 무작위인 건 그 다음 단계뿐이다.

**서브셀렉트가 `SELECT *`이므로 `m.rowid`가 바깥에서 보이지 않는다.** 안쪽
셀렉트에 `rowid`를 명시적으로 꺼내야 한다: `SELECT *, rowid FROM ...`. 구현할 때
확인할 것 — 안 되면 서브셀렉트를 `SELECT rowid AS _rowid, * FROM`로 바꾸고
바깥에서 `m._rowid`를 쓴다.

### 1-2. 테스트

```go
func TestList_ordersRowsWrittenInTheSameMillisecondByInsertion(t *testing.T) {
    // A turn writes assistant then tool then tool, and on a coarse clock —
    // Windows' timer granularity is ~15ms — all three land on one
    // millisecond. Ordering by the uuid id then decides at random, so the
    // panel can draw a reply before the tool chips that produced it.
    // Insertion order is the only thing that is actually true here.
}
```

세 행을 **고정된 같은 `created_at`**으로 쓰고, 여러 번 돌려도(`-count=20`) 항상
쓴 순서로 나오는지 본다. uuid는 매번 다르므로, `id` tie-break로는 이 테스트가
반드시 언젠가 실패한다 — 그게 이 테스트가 진짜라는 증거다.

### 검증

```
cd engine && go test ./internal/companion/ -run TestList -count=20
cd engine && go test ./internal/companion/ ./internal/agent/ ./internal/export/ -count=1
cd engine && go build ./... && go vet ./...
cd engine && go build -tags mas ./... && GOOS=windows go build ./...
bash scripts/validate-story-core-deps.sh
make test-mobile-engine
```

키체인이 잠겨 있으므로 `go test ./...`나 `internal/engineapp` 전체는 돌리지 말 것.

### 커밋

```
fix(companion): order a turn's rows by when they were written, not by uuid (#95)
```

---

## Task 2: 충돌 배너가 누가 썼는지 안다

작가가 방금 패널에서 시킨 변경에 대해 "외부 에이전트가 이 씬을 고쳤습니다"를
읽는 상황을 없앤다.

### 파일

- `apps/desktop/src/hooks/useMcpChanges.ts` — 충돌의 출처를 함께 반환
- `apps/desktop/src/hooks/useMcpChanges.test.tsx` — 테스트
- `apps/desktop/src/routes/Workspace.tsx` — 배너 분기
- `apps/desktop/src/lib/i18n.tsx` — 키 하나 × 세 카탈로그

### 2-1. 훅

`McpChangedPayload.source`는 이미 타입에 있다(`"agent" | "external"`). 훅이
`conflictNodeId`만 반환하고 출처를 버린다. 출처도 같이 담는다.

```ts
// The conflict and the source it came from move together: a banner that
// names the wrong writer is worse than a generic one, and they can only
// disagree if they are stored apart.
const [conflict, setConflict] = useState<{ nodeId: string; source: string } | null>(null);
```

**하나의 상태로 묶는 이유.** 두 `useState`로 나누면 둘이 어긋난 렌더가 존재할 수
있고, 그 순간 배너는 이전 충돌의 출처로 이번 충돌을 설명한다.

### 2-2. 배너

`source === "agent"`일 때만 에이전트 문구를 쓴다. 나머지는 전부 기존 문구다 —
빈 값도, 모르는 값도. 내장 에이전트가 한 일을 외부가 한 것으로 보여주는 쪽이
그 반대보다 안전하다. #94의 활동 로그 출처 열과 같은 판단이다.

```
workspace.mcp.conflict.agentBody
  ko "에이전트가 이 씬을 고쳤습니다. 편집 중인 내용이 있어 아직 반영하지 않았습니다."
  en "The agent changed this scene. Your unsaved edits are still here, so nothing was replaced."
  ja "エージェントがこのシーンを変更しました。編集中の内容があるため、まだ反映していません。"
```

### 2-3. 테스트

```tsx
it("names the built-in agent when the change came from it", ...);
it("keeps the external wording for an unknown or missing source", ...);
```

두 번째가 핵심이다. `source`가 없는 이벤트(옛 엔진, 또는 필드가 빠진 경로)가
에이전트로 읽히면 안 된다.

### 커밋

```
feat(desktop): the conflict banner says whether the agent or an outside client wrote (#95)
```

---

## Task 3: 패널 껍데기

패널이 열리고 닫히고, 프로바이더가 없으면 그렇게 말한다.

**이 작업이 끝나면** Cmd/Ctrl+J로 패널이 토글되고, 다른 패널과 겹치지 않고,
프로바이더가 설정되지 않았으면 설정으로 가는 안내 하나가 본문 대신 나온다.
메시지도 스트리밍도 아직 없다.

### 파일

- `apps/desktop/src/components/agent/AgentPanel.tsx` — 신규
- `apps/desktop/src/components/agent/AgentPanel.css` — 신규
- `apps/desktop/src/components/agent/AgentPanel.test.tsx` — 신규
- `apps/desktop/src/routes/Workspace.tsx` — 슬롯, 토글, 단축키
- `apps/desktop/src/hooks/inspector.ts` — `InspectorState`, `PRIORITY`
- `apps/desktop/src/components/ShortcutsModal.tsx` — 항목 하나
- `apps/desktop/src/lib/i18n.tsx` — `agentPanel.*` 시작

### 3-1. 컴포넌트

`FactBookPanel`의 구조를 따른다. 공용 클래스는 `App.css`에 있다.

```tsx
<aside className="panel agent-panel" onMouseDown={(e) => e.stopPropagation()}>
  <div className="panel-head">
    <span className="ttl"><span className="ic"><Bot /></span> {t("agentPanel.title")}</span>
    <button className="panel-close" onClick={onClose}><X /></button>
  </div>
  {ready ? (
    <div className="panel-scroll agent-log" data-testid="agent-log">…</div>
  ) : (
    <p className="agent-empty" data-testid="agent-unconfigured">
      {t("agentPanel.unconfigured")}
    </p>
  )}
</aside>
```

`onMouseDown` 전파 차단은 다른 패널이 전부 하는 것이다 — 패널 안을 클릭했다고
워크스페이스의 선택이 풀리면 안 된다.

### 3-2. 미설정 상태

**무엇이 "설정됨"인가.** `providers.list`에서 `active` 행이 `configured &&
consented`일 때만이다. 자격증명만 있고 동의가 없으면 첫 턴이 서버에서
`provider_consent_required`로 거절된다 — 그걸 보내놓고 오류를 보여주느니
처음부터 안내하는 게 낫다.

안내는 문장 하나다. 설정 화면으로 가는 링크를 포함한다. **여기에 프로바이더
선택 UI를 만들지 않는다** — 그건 #94의 화면이고, 두 벌이 되면 반드시 어긋난다.

### 3-3. 슬롯 배선

`Workspace.tsx`에서:

1. `agentOpen` 상태와 `toggleAgent` 콜백. 기존 `toggleFactBook`,
   `toggleContextualEdit`, `toggleCanon`이 서로를 닫는 방식 그대로, **셋 전부에**
   `setAgentOpen(false)`를 추가하고 `toggleAgent`는 나머지 셋을 닫는다.
2. `hooks/inspector.ts`의 `InspectorState`에 `agent` 필드, `PRIORITY` 배열에도.
   빠뜨리면 iPad에서만 조용히 겹친다.
3. 오른쪽 슬롯 삼항 사슬에 분기를 넣는다. `ContextPanel`(else 대체) **위**,
   나머지 토글 패널들과 같은 층.
4. `ws-body` 클래스 계산식의 `right-wide` 조건에 `agentOpen`을 넣는다.

### 3-4. 단축키

`Workspace.tsx`의 전역 keydown 핸들러에 분기를 추가한다.

```tsx
} else if (e.key.toLowerCase() === "j") {
  e.preventDefault();
  toggleAgent();
}
```

`ShortcutsModal.tsx`의 `SHORTCUTS`에 항목 하나. **그리고 두 파일에 있는
"컴패니언과 함께 사라졌고 의도적으로 비워둔다"는 주석을 지운다** — 이제 거짓이다.
`Cmd+I`(AI 초고)는 여전히 비어 있으므로 그 부분은 남긴다.

### 3-5. 테스트

```tsx
it("renders the unconfigured notice when no provider is consented", ...);
it("renders the log once the active provider is configured and consented", ...);
it("does not offer provider setup of its own", ...);  // no key input, no choices
```

그리고 `Workspace.mcpChanges.test.ts`가 쓰는 소스 텍스트 단언 방식으로
(그 파일은 라우트가 커서 마운트 대신 원문을 검사한다):

```ts
it("closes the other inspector panels when the agent panel opens", ...);
it("binds Cmd/Ctrl+J", ...);
```

### 커밋

```
feat(desktop): the agent panel's shell, its slot, and Cmd/Ctrl+J (#95)
```

---

## Task 4: 메시지 목록과 스트리밍

작가의 말과 에이전트의 답이 보인다. 답은 흐르듯 나타난다.

### 4-1. 메시지 모델

패널이 들고 있는 것은 세 종류의 줄이다.

```ts
type Line =
  | { kind: "user"; id: string; text: string }
  | { kind: "assistant"; id: string; text: string; usage?: { input: number; output: number } }
  | { kind: "tool"; id: string; name: string; summary: string; state: "running" | "ok" | "error";
      batchId?: string; undone?: boolean };
```

**`id`가 필요한 이유.** 툴 줄은 두 이벤트(`started` → `done`/`error`)가 하나로
합쳐지고, `agent.tool` 페이로드에는 호출별 id가 없다. `run_id + name + 그 이름이
이 턴에서 몇 번째인가`로 만든다. 순서가 유일한 상관자다.

### 4-2. 스트리밍

`agent.delta`가 조각을 보낸다. 누적한 문자열을 `useSmoothStream(text, streaming)`에
넘긴다. `streaming`은 이 실행이 아직 끝나지 않았을 때 참이다.

```tsx
const shown = useSmoothStream(assistantText, running);
```

**`useSmoothStream`은 이 저장소에서 처음 쓰인다** — 존재하지만 호출부가 없었다.
시그니처는 `(target: string, active: boolean) => string`이고, `active`가 거짓이면
`target`을 그대로 돌려준다. `target`이 `shown`으로 시작하지 않으면 즉시
스냅한다(스트림 리셋).

### 4-3. 다른 실행의 이벤트를 무시한다

모든 페이로드에 `run_id`가 있다. **현재 실행의 것이 아니면 버린다.** 패널을 닫고
다시 열거나, 한 작품에서 다른 작품으로 옮겨간 뒤 옛 실행의 늦은 이벤트가 도착할 수
있다. #94에서 같은 부류의 결함을 세 라운드에 걸쳐 고쳤으므로 여기서는 처음부터
건다.

### 4-4. 마크다운

`react-markdown`과 `remark-gfm`이 의존성에 있고 쓰는 곳이 없다. 삭제된 1.0
컴패니언의 `Markdown.tsx`(15줄)가 이 이슈를 위해 남겨진 것으로 보인다. 같은
모양으로 `components/agent/Markdown.tsx`를 만든다 — 링크는
`target="_blank" rel="noreferrer"`.

**컴패니언 패널의 나머지는 되살리지 않는다.** 제안 카드, 선택 카드, 이미지 첨부,
액션 프리셋은 전부 1.0의 개념이고 MCP 전환이 없앤 것이다.

### 4-5. 테스트

`useMcpChanges.test.tsx`가 쓰는 방식으로 `@tauri-apps/api/event`를 직접 모킹하고
리스너를 손으로 부른다.

```tsx
it("accumulates agent.delta into one reply", ...);
it("ignores events from a run that is not the current one", ...);
it("renders markdown in a reply", ...);
```

### 커밋

```
feat(desktop): the message list, and a reply that streams (#95)
```

---

## Task 5: 툴 줄과 되돌리기

에이전트가 무엇을 읽고 무엇을 썼는지 한 줄씩 보인다. 쓴 것은 되돌릴 수 있다.

### 5-1. 두 이벤트를 한 줄로

`agent.tool`이 `state: "started"`로 한 번, `"done"`이나 `"error"`로 다시 온다.
같은 줄을 갱신한다.

```
읽음 · 4-2 씬 / 스토리 컨텍스트
씀 · 4-2 씬                      [되돌리기]
```

**`batch_id`가 있는 줄에만 되돌리기가 붙는다.** 읽기 툴은 배치를 만들지 않는다.
그리고 #93의 리뷰가 확인한 것 — 작가가 쓰기 도중에 정지를 누르면 결과에
`batch_id`가 없다. 그 경우 줄은 남지만 버튼은 없다. 정직한 상태다.

### 5-2. 되돌리기

`agent.undo(batchId)`. 성공하면 그 줄을 되돌림 상태로 바꾼다.

**실패는 정상 경로다.** 되돌리기 배치는 서비스 메모리에 최대 8개만 산다. 재시작
후나 턴을 몇 번 더 돌린 뒤에는 `agent_undo_unavailable`이 온다 — #94가 이미
세 언어로 번역해뒀다. 그 문구를 줄 옆에 보여주고 버튼을 없앤다.

### 5-3. 테스트

```tsx
it("merges the started and done events into one line", ...);
it("offers undo only on a line that carries a batch id", ...);
it("marks the line undone after agent.undo succeeds", ...);
it("explains an expired undo window instead of failing silently", ...);
```

### 커밋

```
feat(desktop): one line per tool call, and undo where there is a batch (#95)
```

---

## Task 6: 작성기, 정지, 시작 칩, 사용량

### 6-1. 작성기

여러 줄 입력. `Enter` 전송, `Shift+Enter` 줄바꿈. 실행 중에는 전송이 **정지**로
바뀐다(`agent.cancel`).

`agent.run`에 `project_id`와 **현재 에디터의 `node_id`**를 넘긴다. 그게 #93의
스코프 라인 재료다 — 에이전트가 "지금 어느 씬 얘기인지"를 아는 유일한 경로.

### 6-2. 시작 칩

"현재 씬 초고", "연속성 점검", "다음 씬 제안". **작성기를 채우기만 한다.**
바로 보내지 않는다 — 작가가 문장을 고칠 기회를 남긴다.

### 6-3. 사용량

`agent.done`의 `usage.input`/`usage.output`을 턴 끝에 한 줄로. **비용 계산은
하지 않는다.** 가격은 프로바이더마다 다르고 자주 바뀌며, 틀린 금액은 없는
금액보다 나쁘다.

### 6-4. 정지

`agent.cancel(runId)`. #93이 확인한 것: 취소는 인메모리 전송을 넘어가고, 부분
응답은 트랜스크립트에 남는다. 정지 후에도 그때까지의 답변이 화면에 남아야 한다.

### 커밋

```
feat(desktop): the composer, the stop button, and what a turn cost (#95)
```

---

## Task 7: 히스토리 복원과 오류

### 7-1. 복원

패널을 열 때 `agent.history(projectId)`. 행을 `Line`으로 바꾼다.

**`role === "tool"`인 행의 `content`는 JSON 문자열이다.** `{name, summary, ok,
batch_id, node_ids}`. 파싱에 실패하면 그 줄을 건너뛴다 — 한 줄 때문에 대화 전체가
안 보이는 것보다 낫다.

### 7-2. 오류

`agent.error`의 `reason`을 `rpcErrorMessage`로 번역한다. **원문 `message`를
화면에 쓰지 않는다** — `agent_internal_error`에는 Go 패닉 값이 실려 있다.

- 인증 실패(`provider_auth_failed`) → 설정으로 가는 링크
- `agent_busy` → 이미 도는 턴이 있다는 안내
- `agent_iteration_limit` → 부분 결과는 남아 있고 이어서 시킬 수 있다는 안내

### 7-3. 남겨진 턴

패널을 닫아도 실행은 계속된다. 다시 열었을 때 도는 턴이 있는지 알 방법은
`agent.run`을 시도해 `agent_busy`를 받는 것뿐이다 — #93은 상태 조회 메서드를
만들지 않았다. **이 계획서는 그걸 추가하지 않는다.** 복원된 히스토리의 마지막
줄이 user 행이면 "아직 도는 중일 수 있다"고만 표시하고, 작가가 보내면 그때
`agent_busy`가 정직하게 답한다.

### 커밋

```
feat(desktop): restore a conversation, and say what went wrong (#95)
```

---

## 이 계획서가 일부러 남기는 것

- **메모리(#97)와 스킬(#98).** 시작 칩은 고정 세 개다. 스킬 목록에서 오지 않는다.
- **세션 검색.** 히스토리는 예산만큼만 돌아온다. #99의 후보다.
- **에이전트 실행 상태 조회 RPC.** 위 7-3을 볼 것. 필요해지면 #99에서 만든다.
- **모바일.** `agent_available`이 거짓이면 패널 자체가 없다.
- **실패한 턴의 재시도 버튼.** 설계 스펙 §10은 "패널은 재시도 버튼을 보여준다"고
  적었지만 이 브랜치는 실패 알림만 그리고 재시도는 주지 않는다. 이 항목이 원래
  이 목록에 없었던 것은 판단의 결과가 아니라 누락이었고, 최종 리뷰에서 드러나
  여기 적는다.

  지금 작가가 겪는 것: 턴 중간에 `agent.error`가 오면 프롬프트는 이미
  `handleSend`에서 소비된 뒤다(`setDraft("")`가 요청을 보내기 전에 실행된다).
  실패 알림 아래에는 아무 버튼도 없으므로, 방금 쓴 문장을 처음부터 다시
  타이핑하는 수밖에 없다. 동기적으로 거절된 전송(`provider_not_configured` 등)만
  `setDraft(prompt)`으로 원고를 돌려준다.

  만들지 않은 이유는 "쉬워서 나중에"가 아니다. 재시도는 어떤 프롬프트를 다시
  보낼지 정해야 하고, 그 답이 지금은 화면에 없다. 라이브 실패는 `lines`의 user
  줄에서 읽을 수 있지만, 복원된 실패(`agentPanel.restore.failed`)는 트랜스크립트의
  user 행에서 읽어야 하고 — 그 행은 `linesFromHistory`가 `Line`으로 바꾸면서
  원문을 그대로 들고 있으니 가능하지만 — 그 턴이 이미 원고를 절반 고쳐 놓은
  상태라면 같은 프롬프트를 다시 보내는 것이 안전한 행동인지가 별개의 문제다.
  이 브랜치가 만들지 않은 `snapshot_id` 되돌리기 경로와 함께 설계해야 하는
  일이므로, #99에서 둘을 같이 다룬다.
