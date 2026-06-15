# 맥락 편집 워크벤치 — 로드맵

_작성일: 2026-06-15_
_상태: Phase 1~3 구현 완료 / 릴리즈 검증 중_
_관련: [`companion-writing-actions.md`](./companion-writing-actions.md), [`companion-scene-edit-reliability.md`](./companion-scene-edit-reliability.md), [`../../superpowers/specs/2026-06-15-manuscript-rag-continuity-design.md`](../../superpowers/specs/2026-06-15-manuscript-rag-continuity-design.md)_
_전제: `engine/internal/manuscript` FTS5 기반 본문 검색이 main에 포함되어 있거나 현재 작업 브랜치에 존재한다._

## Overview

Linetta의 편집기는 현재 "현재 씬을 쓰는 공간"과 "컴패니언이 현재 씬을 고쳐주는 공간"에 강하다.
다음 단계는 작품 전체를 대상으로 한 **맥락 편집**이다.

맥락 편집은 단순한 찾기/바꾸기가 아니다. 작가가 "민호를 민준으로 바꿔야겠다", "왕성 이름을 바꿨다",
"주요 아이템 설정을 수정했다"라고 결정했을 때, Linetta가 작품 전체에서 관련 대목을 찾아 보여주고,
장면별로 바꿀지 말지 검토하게 하고, 변경 후에도 설정 일관성이 깨지지 않았는지 다시 확인하는 흐름이다.

## 문제 정의

소설 원고가 길어지면 다음 작업이 급격히 위험해진다.

- 캐릭터 이름, 별칭, 호칭 변경
- 장소명, 조직명, 왕국명 변경
- 주요 아이템/스킬/마법/능력 이름 변경
- 설정 문장 변경 후 앞 회차와 뒤 회차의 묘사 일치 여부 확인
- 현재 씬 안의 작은 치환과 작품 전체 치환 구분

일반 텍스트 편집기의 "Replace all"은 너무 무섭다. 반대로 수동 검색은 놓치는 대목이 생긴다.
Linetta는 이미 작품 구조, 씬 단위 저장, entity/fact/relationship, 컴패니언, 로컬 RAG를 갖고 있으므로
**검색 → 후보 검토 → 선택 적용 → 일관성 재검사**를 하나의 작가용 워크플로로 만들 수 있다.

## 가치 제안

> 한국어 장편/웹소설 작가는 Linetta의 맥락 편집 워크벤치로 현재 씬 또는 작품 전체에서 이름·장소·소품·설정 변경 후보를 찾고, 장면별 근거와 diff를 확인한 뒤, 안전하게 선택 적용하고 일관성까지 점검한다.

## 핵심 원칙

- **절대 조용히 전체 적용하지 않는다.** 작품 전체 변경은 항상 preview와 선택 적용을 거친다.
- **현재 씬 편집과 작품 전체 편집을 분리한다.** 같은 UI 안에서 scope를 명확히 보여준다.
- **텍스트 치환과 맥락 변경을 구분한다.** 단순 문자열은 빠르게, 설정/의미 변경은 근거와 확인을 더 요구한다.
- **본문 변경 전 snapshot을 남긴다.** 기존 `snapshot`/VersionSheet 흐름으로 복구 가능해야 한다.
- **컴패니언은 판단을 돕고, 적용은 구조화된 엔진이 한다.** LLM이 "적용했다"고 말하는 것을 신뢰하지 않는다.
- **MVP는 lexical 검색 중심이다.** 의미 검색/임베딩은 Later. 단, LLM이 검색어 변형을 제안하는 보조 UX는 허용한다.

## 현재 코드베이스 근거

- 현재 씬 에디터: `apps/desktop/src/components/editor/Tiptap.tsx`
  - ProseMirror/Tiptap selection과 imperative handle이 이미 있다.
  - 선택 영역 컨텍스트 메뉴로 컴패니언 퇴고를 호출한다.
- 워크스페이스 shell: `apps/desktop/src/routes/Workspace.tsx`
  - `SearchModal`, `CommandPalette`, `CompanionPanel`, `VersionSheet`가 이곳에서 연결된다.
  - Cmd+F는 현재 앱 전역 검색 모달을 연다.
- 앱 전역 검색: `engine/internal/search`
  - 프로젝트/노드 이동용 LIKE 검색이다. 작품 내부 편집 검색에는 너무 넓고 정밀도가 낮다.
- 본문 RAG 검색: `engine/internal/manuscript`
  - FTS5 trigram + 2자 이하 LIKE fallback.
  - project scope, node_id, breadcrumb, snippet을 제공한다.
- 컴패니언 적용: `engine/internal/companion/tools.go`
  - `set_scene_text`는 씬 전체 교체에 강하지만, 작품 전체 후보별 부분 치환에는 별도 엔진이 필요하다.
- proposal UI: `apps/desktop/src/components/companion/ProposalCard.tsx`
  - 승인 후 적용이라는 UX 패턴은 재사용할 수 있다.

## MVP 기능

| 기능 | 설명 | 핵심 가치 | 복잡도 | MVP |
|---|---|---:|---:|---:|
| 현재 씬 찾기/바꾸기 | 현재 Tiptap 문서에서 단어 찾기, 이전/다음, 현재 항목 바꾸기, 씬 내 전체 바꾸기 | Yes | Medium | ✅ |
| 작품 전체 본문 검색 | 로컬 RAG/FTS로 현재 작품의 본문 검색, 씬별 결과 표시, 클릭 시 해당 씬 이동 | Yes | Medium | ✅ |
| 작품 전체 바꾸기 Preview | 검색 결과를 기반으로 장면별 후보와 diff를 만들고 선택 적용 | Yes | Hard | ✅ |
| 맥락 변경 마법사 | entity/place/item/concept 기반으로 이름·별칭·설정 변경 후보를 만들고 관련 metadata 업데이트 제안 | Yes | Hard | ✅ |
| 일관성 재검사 | 변경 전후 `search_manuscript`와 entity/fact를 대조해 미변경/모순 후보를 보고 | Yes | Medium | ✅ |
| 의미 검색/임베딩 | 패러프레이즈까지 자동 탐색 | No | Hard | Later |
| 자동 전체 재작성 | 여러 씬을 LLM이 알아서 다시 씀 | No | Hard | Later |

## UX 모델

새 표면은 `맥락 편집` 워크벤치다.

- 진입:
  - Command Palette: `맥락 편집 열기`
  - 검색 아이콘 옆 별도 아이콘 또는 editor toolbar 버튼
  - EntitySheet에서 `작품 전체 이름 변경...`
- 위치:
  - 우측 패널로 시작한다. 컴패니언과 같은 오른쪽 영역을 쓰되, 컴패니언 대화와 상태를 섞지 않는다.
  - 넓은 diff 검토가 필요할 때는 modal/full-height sheet로 확장 가능하게 설계한다.
- 주요 컨트롤:
  - scope segmented control: `현재 씬` / `작품 전체`
  - mode tabs: `찾기`, `바꾸기`, `맥락 변경`, `일관성`
  - result list grouped by scene
  - candidate checkbox
  - inline snippet + before/after diff
  - `선택 적용`, `스냅샷 만들고 적용`, `컴패니언에게 검토 요청`

## Now / Next / Later

### Now

1. Phase 1 — 현재 씬 찾기/바꾸기 + 작품 검색 RPC ✅
   - 현재 씬 안의 안전한 편집 기본기를 만든다.
   - `manuscript.search` RPC로 작품 내부 본문 검색을 UI에서 쓸 수 있게 한다.
2. Phase 2 — 작품 전체 바꾸기 preview/apply ✅
   - 장면별 후보, diff, 선택 적용, snapshot을 만든다.
3. Phase 3 — 맥락 변경 마법사 + 일관성 재검사 ✅
   - entity/fact/manuscript를 묶어 이름/장소/아이템/설정 변경을 작품 전체 작업으로 만든다.

### Next

- `SearchModal`과 `ContextualEditPanel`의 역할 정리: 앱 이동 검색 vs 작품 편집 검색을 명확히 안내.
- 변경 batch history 저장: 언제 어떤 후보를 적용했는지 작업 로그로 남기기.
- 컴패니언 액션 팔레트에 `작품 전체 이름 변경`, `설정 변경 영향 확인` 프리셋 추가.

### Later

- 임베딩 기반 의미 검색.
- 교정/스타일 변경 batch rewrite.
- regex 고급 모드.
- 여러 프로젝트 cross-search.
- Git-style visual diff viewer 고도화.

## 페이즈 문서

- [`contextual-editor-phase-1-find-search.md`](./contextual-editor-phase-1-find-search.md)
- [`contextual-editor-phase-2-batch-replace.md`](./contextual-editor-phase-2-batch-replace.md)
- [`contextual-editor-phase-3-context-consistency.md`](./contextual-editor-phase-3-context-consistency.md)

## 공통 Out of Scope

- LLM이 작품 전체 본문을 즉시 자동 수정하는 기능
- 임베딩/벡터 DB
- 외부 검색 서비스 연동
- cross-project 변경
- 모바일 대응
- regex replace MVP 포함
- undo 시스템 신규 구현. 복구는 기존 snapshot/VersionSheet에 의존한다.

## 공통 검증

- `git status --short --branch`
- `cd engine && go test ./...`
- `pnpm --dir apps/desktop test -- ContextualEditPanel.test.tsx Tiptap.test.tsx SearchModal.test.tsx --run`
- `pnpm --dir apps/desktop exec tsc --noEmit`
- `make test`
- `git diff --check`
- `LINETTA_HOME=/tmp/linetta-contextual-editor ./scripts/dev.sh`

## 수동 검증 시나리오

1. 작품에 3개 이상의 씬을 만들고 `민호`, `민호 형`, `민호의 시계` 같은 변형 표현을 넣는다.
2. 현재 씬 모드에서 `민호`를 찾고 이전/다음 이동이 되는지 확인한다.
3. 현재 씬에서 현재 항목만 `민준`으로 바꾸고 저장되는지 확인한다.
4. 작품 전체 모드에서 `민호` 검색 결과가 씬별로 묶이는지 확인한다.
5. 일부 후보만 선택해 `민준`으로 적용한다.
6. VersionSheet에서 적용 전 snapshot이 보이는지 확인한다.
7. 일관성 탭에서 아직 남은 `민호` 또는 관련 별칭 후보가 보고되는지 확인한다.

## 구현 순서 메모

방금 구현된 Manuscript RAG 변경이 아직 커밋되지 않았다면 먼저 별도 커밋으로 닫는다.
그 다음 이 로드맵을 새 기능 브랜치/커밋으로 시작한다. 두 작업을 한 커밋에 섞으면 마이그레이션, 검색 RPC,
편집 UI가 한꺼번에 얽혀 리뷰와 회귀 추적이 어려워진다.
