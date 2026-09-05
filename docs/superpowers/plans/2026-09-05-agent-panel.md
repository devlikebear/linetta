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
