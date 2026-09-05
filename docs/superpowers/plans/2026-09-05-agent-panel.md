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
