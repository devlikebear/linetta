# 이야기 점검·방향 도구 — 로드맵

_작성일: 2026-10-24_
_상태: 계획 (구현 전)_
_근거: [`inkos-analysis.md`](./inkos-analysis.md) — InkOS 소스 분석_
_관련: `engine/internal/contextualedit` — 이름·설정 변경 뒤의 결정적 일관성 재검사(구현 완료. 계획서는 git 이력에 있음)_

## Overview

InkOS에서 가져올 네 가지 아이디어를 리네타의 원칙("작가가 쓰고, AI는 부를 때만, 바꾸기 전에 묻는다")에 맞게 다시 설계한다.

1. **작가 방향.** 지금 몇 회차의 초점과 씬마다 꼭 지킬 것·피할 것을 작가가 적어 두면, 모든 에이전트가 브리프 맨 위에서 읽는다.
2. **연속성 점검.** 에이전트가 씬을 Story World와 대조해 어긋나는 대목을 근거 인용과 함께 보고한다. 원고는 바꾸지 않는다.
3. **제안함과 가져온 원고의 설정 추출.** 에이전트가 만든 인물·장소·관계·팩트 후보가 바로 적용되지 않고 제안함에 쌓인다. 작가가 항목별로 승인한다. 이미 쓴 원고를 가져왔을 때 Story World를 채우는 데 쓴다.
4. **갈래 예측.** 다음 회차의 가능한 전개 2~5개를 비정사(non-canon)로 만들어 비교한다. 고른 갈래는 계획으로만 남고, 개요 반영은 제안함을 거친다.

## 문제 정의

- **작가 방향이 흩어져 있다.**
  - 지금 에이전트가 읽는 방향 정보는 작품 메모(work notes), 문체 메모, 시놉시스, 개요다.
  - 이 중 **작품 메모는 에이전트가 기록하는 문서**다. 작가가 "앞으로 3화는 둘의 오해를 풀지 말 것"이라고 못 박아 둘 자리가 없다.
- **연속성 확인이 손에 맡겨져 있다.**
  - 맥락 편집 Phase 3의 일관성 재검사는 이름 변경 뒤 남은 표현만 찾는 결정적 검사다.
  - "3화에서 왼팔을 다쳤는데 5화에서 양손으로 검을 든다" 같은 의미상의 어긋남은 작가가 직접 기억해야 한다.
- **에이전트의 설정 쓰기는 즉시 적용된다.**
  - `linetta_apply_story_ops`의 `create_entity`·`create_relationship`·`create_fact_card`는 바로 반영되고, 묶음 undo도 안 된다.
  - 가져온 원고 40화에서 인물을 뽑아 달라고 하면 수십 개 항목이 검토 없이 Story World에 들어간다.
- **다음 전개를 비교할 도구가 없다.**
  - 에이전트에게 물어보면 대화에 답이 흘러가 버린다.
  - 정사가 바뀐 뒤 그 답이 아직 유효한지 알 방법도 없다.

## 가치 제안

> 장편·웹소설 작가는 리네타에서 자기 방향을 못 박아 두고, 쓴 씬의 어긋남을 근거와 함께 보고받고, AI가 만든 설정 후보와 전개 갈래를 하나씩 골라 받아들인다. 원고와 정사는 작가가 승인한 만큼만 바뀐다.

## 핵심 원칙

이 로드맵의 모든 페이즈가 지킨다. 이미 이 코드베이스의 길이기도 하다.

1. **엔진은 LLM을 부르지 않는다.**
   - LLM 판단은 내장 에이전트나 MCP 클라이언트가 한 턴 안에서 한다(`summarizer` 패키지 주석 참고).
   - 엔진은 결정적 읽기 도구, 검증, 저장만 맡는다.
2. **에이전트의 산출물은 초안이고, 승인은 앱 RPC로만 한다.**
   - 본보기는 `style` 패키지의 문체 프로필이다.
   - `linetta_propose_style_profile` → `project_style_drafts` → Story World에서 승인·편집·폐기 → `style.approve_draft` RPC.
   - **어떤 MCP 도구도 승인하지 못한다.**
3. **id는 엔진이 소유한다.**
   - 모델은 존재하는 entity / fact / thread / beat / node id만 참조할 수 있다.
   - 새 항목은 id 없는 후보로 받는다.
   - 모르는 id는 도구 오류로 돌려보낸다. 오류 메시지는 모델이 고칠 수 있는 문장으로 쓴다(`toolErr` 관례).
4. **근거 인용은 엔진이 검증한다.**
   - 모델이 낸 인용은 해당 씬의 해당 문단에 실제로 있어야 한다. 없으면 도구 오류로 되돌린다.
   - InkOS의 `REVIEW_SOURCE_MISMATCH` 교훈이다.
5. **작가 소유 문서는 에이전트가 쓰지 못한다.**
   - 작가 방향(초점·씬 의도)은 앱 UI에서만 쓴다.
   - InkOS의 `write_truth_file`처럼 통째로 덮어쓰는 길은 만들지 않는다.
6. **낡음을 숨기지 않는다.**
   - 점검과 예측은 만들 때의 입력 지문을 저장한다.
   - 정사나 본문이 바뀌면 "왜 낡았는지"와 함께 표시한다.
7. **도구는 상시 비용이다.**
   - 새 MCP 도구는 선택 그룹 하나("점검·제안", `review_tools_enabled`)로 묶는다. Settings → Tool budget에서 끌 수 있다.
   - 기존 도구로 되는 일은 기존 도구에 옵션을 더한다.

## 현재 코드베이스 근거

| 영역 | 파일 | 이 로드맵에서의 쓰임 |
|---|---|---|
| 초안→승인 본보기 | `engine/internal/style/draft.go`, `engine/internal/mcphost/tools_style.go`, `engine/internal/rpc/handlers/style.go`, `apps/desktop/src/components/StyleSection.tsx` | Phase 2·3·4의 저장, 도구, 승인 흐름 |
| 문단 번호 관례 | `engine/internal/style/scenes.go` `sceneOf` (export.DocToPlainText → `"\n\n"` 분할, 비지 않은 문단 1부터) | Phase 2 근거 위치 |
| 브리프 | `engine/internal/storycontext` (`types.go` ContextKey 사슬, `render.go`) | Phase 1 방향 섹션, Phase 4 선택한 갈래 |
| MCP 도구 | `engine/internal/mcphost/tools*.go`, `budget.go` (`ToolGroups`) | 새 도구와 선택 그룹 |
| 설정 연산 | `engine/internal/storyops` (`ops.go` 파서, `apply.go`) | Phase 3 제안함은 같은 op 형식을 재사용 |
| 내장 에이전트 | `engine/internal/agent/prompt.go` `systemPrompt`, `apps/desktop/src/components/agent/AgentPanel.tsx` starter 칩 | 사용 지침 한 줄, 시작 칩 |
| 이름 변경 뒤 일관성 검사 | `engine/internal/contextualedit` | Phase 2와 역할 구분: 결정적 검사는 거기, 의미 점검은 여기 |
| 회차 판별 | `engine/internal/episode` (`List`, `Resolve`) | Phase 1 초점 낡음, Phase 4 기준 회차 |
| Markdown 가져오기 | `engine/internal/importmd`, `apps/desktop/src/components/ImportPreviewModal.tsx` | Phase 3 추출 진입점 |
| 작품 단위 복원 | `engine/internal/restore/restore.go` `MergeProject` | 새 작품 단위 테이블은 여기서 함께 복사 |
| 마이그레이션 | `engine/internal/store/migrations/` (현재 `0023_publish.sql`) | `0024`부터 |
| RPC 허용 목록 | `apps/desktop/src-tauri/src/lib.rs` `RENDERER_ENGINE_METHODS` (`lib/rpcAllowlist.test.ts`가 대조) | 새 RPC마다 추가 |
| 다국어 | `apps/desktop/src/lib/i18n.tsx` (ko/en/ja), `storycontext/render.go` `langPick` | 모든 문구 3개 언어 |

## MVP 기능

| 기능 | 설명 | 페이즈 | 복잡도 | MVP |
|---|---|---|---:|---:|
| 현재 초점 | 작품당 한 칸(500자). 설정 시점의 기준 회차를 기억하고 3회차가 지나면 "아직 유효한가요?" 표시 | 1 | Easy | ✅ |
| 씬 의도 | 씬마다 목표 한 줄, 지킬 것과 피할 것 각 5개까지 | 1 | Medium | ✅ |
| 브리프 "작가 방향" 섹션 | 초점과 현재 씬 의도를 브리프 맨 위에 둔다. 비어 있으면 생략 | 1 | Easy | ✅ |
| 연속성 점검 기록 | 에이전트가 씬 단위로 관찰을 보고. 엔진이 인용과 참조 id를 검증 | 2 | Medium | ✅ |
| 점검 목록 화면 | 씬별 관찰, 근거 문단 열기, 무시와 해결, 본문 변경 시 낡음 표시 | 2 | Medium | ✅ |
| 제안함 | 에이전트가 낸 story op 후보를 쌓고, 작가가 항목별로 수락·수정·거절 | 3 | Hard | ✅ |
| 중복 후보 표시 | 새 인물·장소 후보가 기존 이름이나 별칭과 겹치면 "같은 대상일 수 있음" | 3 | Medium | ✅ |
| 가져온 원고 설정 추출 | 가져오기 직후 동의를 받고 에이전트가 회차 순서대로 추출해 제안함에 쌓는다. 진행률 표시 | 3 | Medium | ✅ |
| 갈래 예측 저장·비교 | 2~5개 갈래, 비정사, 입력 지문으로 낡음 판정 | 4 | Medium | ✅ |
| 갈래 선택과 개요 반영 제안 | 선택은 RPC로. 개요 반영은 제안함 op로 | 4 | Medium | ✅ |
| 에이전트의 초점 제안 | 에이전트가 초점 문안을 초안으로 제안 | — | Easy | Next |
| 떡밥 장부 | 열림 / 진행 / 보류 / 회수 / 철회 상태와 회수 안 된 떡밥 보고 | — | Hard | Later |
| 평문 원고 회차 분할 | `.txt`의 제N화 / 第N章 / Chapter N / 프롤로그 인식, 번호 누락 감지 | — | Medium | Later |
| 시점별 사실 | 사실에 유효 회차 범위를 둔다 ("3화부터 왼팔 부상") | — | Hard | Later |

## 페이즈

| Phase | 문서 | 끝나면 동작하는 것 | 예상 |
|---|---|---|---|
| 1 | [`phase-1-direction.md`](./phase-1-direction.md) | 작가가 초점과 씬 의도를 적고, 에이전트와 MCP 클라이언트가 그것을 브리프 맨 위에서 읽는다 | 1.5~2일 |
| 2 | [`phase-2-continuity.md`](./phase-2-continuity.md) | "이 씬 연속성 점검" → 근거가 검증된 관찰이 점검 목록에 쌓이고, 작가가 문단으로 이동하거나 무시·해결한다 | 2~3일 |
| 3 | [`phase-3-proposals-import.md`](./phase-3-proposals-import.md) | 에이전트의 설정 후보가 제안함에 쌓이고, 작가가 골라 적용한다. 원고를 가져온 직후 추출을 시작할 수 있다 | 3~4일 |
| 4 | [`phase-4-forecast.md`](./phase-4-forecast.md) | 다음 회차 갈래를 비교해 고르고, 개요 반영을 제안함으로 받는다. 정사가 바뀌면 이유와 함께 낡음 표시 | 2~3일 |

의존 관계:
- Phase 2는 Phase 1의 씬 의도("피할 것")를 점검 기준으로 쓴다.
- Phase 3은 Phase 2가 만든 Review 도구 그룹과 인용 검증을 재사용한다.
- Phase 4는 Phase 1의 방향(지문 입력, `episode.LastWritten`)과 Phase 3의 제안함(개요 반영)을 쓴다.

순서는 1 → 2 → 3 → 4다. 각 페이즈가 끝나면 그 자체로 쓸 수 있는 기능이 남는다.
Phase 1만 하고 멈춰도 방향 문서는 동작한다.

## Now / Next / Later

### Now
- Phase 1~4 (위 표)

### Next
- 에이전트가 초점과 씬 의도를 초안으로 제안하고(`propose_*`), 작가가 승인한다.
- 씬 의도와 초점을 Markdown 내보내기·가져오기(`mdmeta`)에 포함한다.
- 점검 관찰을 에디터 여백에 표시한다 (지금은 목록과 문단 이동만).
- 제안함 수락을 하나의 묶음 undo로 되돌린다 (지금 entity / fact op는 묶음 undo가 안 된다).

### Later
- 떡밥 장부 (InkOS hook ledger의 상태 규칙: 회수는 확정, 철회는 사유 필수, 보류는 진전이 아님)
- 시점별 사실(유효 회차 범위)과 "N화 시점" 점검
- 평문 원고 회차 분할과 프롤로그 보존
- 갈래별로 LLM 호출을 따로 하는 진짜 격리 생성

## Out of Scope

- 엔진 안에서 직접 LLM을 호출하는 파이프라인 (InkOS의 Planner·Composer·Settler 같은 고정 파이프라인)
- AI가 원고를 자동으로 고치는 수정 루프
- 일정에 맞춰 혼자 연재를 쓰는 데몬 모드
- 단편 패키지, 인터랙티브 Play, 표지 생성 같은 다른 매체
- 시장 조사(Radar)

## 공통 구현 규칙 (모든 페이즈)

- **브랜치와 PR**
  - 페이즈마다 `main`에서 새 브랜치를 딴다 (`feat/story-assist-phase-N-…`).
  - 기능 단위로 커밋하고(`feat:` / `fix:` / `chore:`), PR을 연다. `main`에 직접 푸시하지 않는다.
  - 머지는 작가(사용자) 승인 뒤에 한다.
- **마이그레이션**
  - `engine/internal/store/migrations/00NN_*.sql`. 파일 머리에 "왜 이 테이블이 따로 있는가"를 주석으로 쓴다 (`0022_style_drafts.sql` 관례).
- **작품 단위 복원**
  - 새 작품 단위 테이블은 `restore.MergeProject`에 복사 규칙을 추가한다.
  - node id를 참조하면 재매핑한다.
  - 이 처리를 빠뜨리면 백업에서 복원한 작품에서 그 데이터가 사라진다.
- **MCP 도구**
  - `record(d, name, handler)`로 감싸 활동 로그에 남긴다.
  - 쓰기가 성공하면 `notifyChanged`를 부른다.
  - 쓰기 도구는 `settings.MCPModeFull`에서만 등록한다.
  - 새 도구는 `ToolGroups`의 새 그룹 `Review`와 `ReviewToolNames`에 넣는다.
  - `TestToolGroupNamesAreAllRealTools`와 Tool budget 화면이 이것을 대조한다.
- **RPC**
  - `engine/internal/engineapp/engineapp.go`에서 `s.Handle`로 등록한다.
  - `apps/desktop/src/lib/rpc.ts`에 래퍼를 둔다.
  - `apps/desktop/src-tauri/src/lib.rs`의 `RENDERER_ENGINE_METHODS`에 추가한다.
  - `lib/rpcAllowlist.test.ts`가 누락을 잡는다.
- **문구**
  - ko / en / ja를 모두 채운다 (`lib/i18n.tsx`, `i18n.catalog.test.ts`).
  - 브리프 문구는 `langPick`으로 쓴다.
- **에이전트 지침**
  - `agent/prompt.go` `systemPrompt`에 도구마다 한 줄을 쓴다.
  - 도구 그룹이 꺼지면 그 줄도 뺀다 (기존 memory / skills 처리와 같은 방식).
- **문서**
  - 사용자에게 보이는 동작은 README 해당 절과 `docs/privacy-policy.md`를 갱신한다 (새로 저장되는 데이터가 있으면).

## 공통 검증 명령

```sh
make test                                                   # 전체 게이트
cd engine && CGO_ENABLED=0 go test ./internal/<pkg>/...      # 패키지 단위 (cgo 테스트는 로그인 키체인을 건드린다)
make test-mobile-engine                                     # 엔진 패키지를 추가·수정했을 때 (mcphost는 !mobile)
pnpm --dir apps/desktop test -- <파일>.test.tsx --run
pnpm --dir apps/desktop exec tsc --noEmit
git diff --check
```

수동 확인은 격리된 데이터 디렉터리로 한다. 실제 서재를 건드리지 않는다.

```sh
LINETTA_HOME=/tmp/linetta-story-assist ./scripts/dev.sh
```
