# Phase 1: 연재 핵심 단위 — 작업지시서

_이 페이즈의 목표를 달성하기 위한 구체적 작업 목록. Claude Code가 위에서부터 순차 실행하되, 각 체크포인트에서 사용자 확인을 받는다._

_작성일: 2026-06-10_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 3~4일_

## 페이즈 목표

웹소설가의 기본 작업 루프가 앱 안에서 닫힌다: **새 프로젝트를 "웹소설 연재"로 시작하고,
화(회차) 단위 글자수가 아웃라인·에디터 푸터·ZEN에 항상 보이며, 다 쓴 회차를
플레인 텍스트로 복사해 플랫폼에 붙여넣고, 오늘 쓴 글자수를 확인한다.**

## 전제 조건

- [ ] 프로젝트 환경 세팅 완료 (`make dev` 실행 가능, `make test` 통과 상태)
- [ ] working tree 미커밋 변경 처리 방침을 사용자에게 확인 (roadmap의 주의 사항)

## 포함 기능

1. **회차 글자수 목표 필드** — 프로젝트에 `episode_char_target` 추가 (기본 5,000자)
2. **화(컨테이너) 합산 글자수 + 게이지** — 아웃라인 레일 표시
3. **에디터 푸터 회차 누적 표시** — "이번 화 4,213 / 5,000자 · 공백 포함"
4. **회차/씬 본문 플레인 텍스트 복사** — 클립보드, 컨텍스트 메뉴 + 커맨드 팔레트
5. **새 프로젝트 작품 유형 선택** — 웹소설 연재 / 일반 소설 + 한국 웹소설 장르 칩
6. **ZEN 목표 진행바 연결** — 죽어 있는 `target` prop 활성화
7. **오늘 쓴 글자수** — 엔진에 일별 집필량 기록 + 컨텍스트 패널 표시

## 이 페이즈에서 하지 않는 것

- 회차 상태(발행/비축) → Phase 2
- 화 직접 집필(권>화 2단계) → Phase 2 — 이번 페이즈에서 화 합산은 **컨테이너 기준**으로만 계산
- 일별 통계의 시각화(히트맵·평균·예상) → Phase 3 — 이번 페이즈는 "오늘" 숫자 한 줄까지만
- 플랫폼별 텍스트 변환 규칙 → Phase 3 — 이번 페이즈 복사는 "문단 사이 빈 줄 1개"의 단순 규칙만

## 작업 체크리스트

### 작업 그룹 A: 회차 글자수 목표 (엔진 → 프론트)

- [ ] **T1.A.1** — 프로젝트에 `episode_char_target` 필드 추가 (엔진)
  - 파일: `engine/internal/project/` (struct·repo), `engine/internal/store/` (마이그레이션), `engine/internal/rpc/handlers/projects*.go`
  - 내용:
    - `projects` 테이블에 `episode_char_target INTEGER NOT NULL DEFAULT 5000` 컬럼 추가 (기존 마이그레이션 패턴을 따라 신규 마이그레이션으로)
    - Project struct/scan/update에 필드 반영. `projects.update` RPC가 patch로 받을 수 있게
  - 참조: 기존 `outline_preset` 필드가 추가된 커밋/코드 경로를 그대로 모방
  - 검증: `make test-go` 통과 + 신규 필드 round-trip 테스트 1개 추가

- [ ] **T1.A.2** — 프론트 타입·RPC 반영
  - 파일: `apps/desktop/src/lib/types.ts` (`Project`, `ProjectPatch`), `apps/desktop/src/lib/rpc.ts`
  - 내용: `episode_char_target: number` 추가. 기존 `projects.update` 호출 시그니처에 포함
  - 검증: `cd apps/desktop && pnpm tsc --noEmit` (또는 `make test-desktop`의 빌드 단계)

- [ ] **T1.A.3** — 목표 편집 UI
  - 파일: `apps/desktop/src/components/ContextPanel.tsx`
  - 내용:
    - "이 씬" 섹션의 progress 메타 줄에 회차 목표를 표시하고, 클릭 시 인라인 숫자 입력으로 수정 (`InlineEditableText` 패턴 참조, 숫자 검증)
    - 저장은 기존 `saveOverview`처럼 debounce 후 `projectsApi.update`
  - i18n: `workspace.episodeTarget` 등 신규 키 ko/en/ja 3종
  - 검증: `ContextPanel` 관련 Vitest 통과 + 수동으로 값 변경 → 재시작 후 유지 확인

### 작업 그룹 B: 화 합산 글자수 표시

- [ ] **T1.B.1** — 트리 합산 헬퍼
  - 파일: `apps/desktop/src/hooks/useFirstLeaf.ts` (+ `useFirstLeaf.test.ts`)
  - 내용: `sumLeafChars(node: TreeNode): number` — 하위 leaf `word_count` 합산. 순수 함수로 작성
  - 검증: 단위 테스트 3케이스 (leaf 단독 / 중첩 컨테이너 / 빈 컨테이너)

- [ ] **T1.B.2** — 아웃라인 레일에 화 글자수 + 미니 게이지
  - 파일: `apps/desktop/src/components/OutlinePanel.tsx`, `OutlinePanel.css`
  - 내용:
    - `RailNode`의 챕터 헤더(`tree-chapter-row`)에 합산 글자수(`sumLeafChars`)와 목표 대비 비율의 얇은 게이지 바 추가
    - 게이지는 웹소설 프리셋(`outlinePresetId === "webnovel"`)일 때만 표시. props로 `episodeCharTarget` 전달 (Workspace에서 `load.project.episode_char_target`)
    - 100% 이상이면 게이지 색을 채움 상태로 (기존 `--t-*` 변수 활용, 새 색 추가 금지)
  - 참조: 기존 `.sc-words` 표시 방식, `ContextPanel`의 `.progress-track/.progress-fill` CSS 패턴
  - 검증: `OutlinePanel.css.test.ts`에 신규 클래스 계약 추가, Vitest 통과

- [ ] **T1.B.3** — 에디터 푸터 회차 누적 표시
  - 파일: `apps/desktop/src/routes/Workspace.tsx`
  - 내용:
    - 현재 씬의 부모 컨테이너(화)를 찾아 `sumLeafChars(화) - 현재씬.word_count + charCount`(라이브 보정)로 회차 누적 계산. `useMemo`로
    - `editor-foot`을 웹소설 프리셋일 때 `"{씬제목} · 이번 화 {n} / {target}자 · 공백 포함"`으로, 일반 프리셋이면 현행 유지
  - i18n: `workspace.episodeCharCount` (예: `"이번 화 {count} / {target}자"`), `workspace.charCountWithSpaces` ko/en/ja
  - 검증: 수동 — 같은 화의 다른 씬으로 이동해도 합산이 일관됨, 타이핑 시 실시간 증가

### 작업 그룹 C: 본문 플레인 텍스트 복사

- [ ] **T1.C.1** — 엔진 `export.nodeText` RPC
  - 파일: `engine/internal/export/` 신규 `text.go`(+`text_test.go`), `engine/internal/rpc/handlers/export.go`
  - 내용:
    - 노드(leaf 또는 컨테이너 서브트리)의 Tiptap doc들을 순회해 **플레인 텍스트** 생성: 문단 사이 빈 줄 1개, 마크다운 문법·멘션 마크 제거(멘션은 표시 텍스트만), 씬 사이는 빈 줄 2개
    - 응답: `{ text: string, char_count: number }`
  - 참조: `export/markdown.go`의 doc 순회 로직, `node/word_count.go`의 `CountChars` 텍스트 추출 방식
  - 검증: Go 테스트 — leaf 1개 / 컨테이너(씬 2개) / 멘션 포함 doc 3케이스, `make test-go`

- [ ] **T1.C.2** — 프론트 복사 동작
  - 파일: `apps/desktop/src/lib/rpc.ts` (`exportApi.nodeText`), `apps/desktop/src/routes/Workspace.tsx`, `apps/desktop/src/components/OutlinePanel.tsx`
  - 내용:
    - 아웃라인 컨텍스트 메뉴에 "본문 복사" 항목 추가 (`onCopyText?: (node) => void` prop, leaf·컨테이너 모두)
    - 커맨드 팔레트에 "이 화 본문 복사" / "이 씬 본문 복사" — **섹션은 기존 `내보내기`(`workspace.command.section.export`) 사용, 신규 섹션 금지**
    - 복사 성공 시 toast: `"{n}자 복사됨"` (Tauri 환경이므로 `navigator.clipboard.writeText` 사용, 실패 시 toast로 에러)
  - i18n: `workspace.copyText`, `workspace.toast.copyTextSuccess`, `workspace.toast.copyTextFailed` ko/en/ja
  - 의존: T1.C.1
  - 검증: 수동 — 화 우클릭 → 본문 복사 → 외부 에디터 붙여넣기에서 문단 구조 확인

### 작업 그룹 D: 새 프로젝트 온보딩

- [ ] **T1.D.1** — 작품 유형 선택 + 웹소설 장르
  - 파일: `apps/desktop/src/components/NewProjectModal.tsx`, `apps/desktop/src/lib/i18n.tsx`, `apps/desktop/src/lib/types.ts`
  - 내용:
    - 모달 최상단에 작품 유형 칩 2개: **웹소설 연재 / 일반 소설** (기본: 웹소설 연재)
    - 웹소설 선택 시: `NewProjectInput.outline_preset = "webnovel"`, 분량 칩을 숨기고 `length_target: "series"` 고정, 장르 칩을 웹소설 세트로 교체 — 현대판타지·로맨스판타지·무협·판타지·회귀/빙의/환생·헌터/게이트·아카데미·로맨스
    - 일반 소설 선택 시: 현행 UI 유지 (`outline_preset = "novel"`)
    - `NewProjectInput`에 `outline_preset` 필드가 없으면 추가하고 엔진 `projects.create`가 받도록 (기존 `projects.update`의 `outline_preset` 처리 재사용)
  - i18n: `newProject.kind.*`, `newProject.webnovelGenre.*` ko/en/ja
  - 검증: `NewProjectModal` 신규 Vitest (유형 전환 시 칩 세트 변경) + 수동으로 웹소설 프로젝트 생성 → 아웃라인 닥터에서 프리셋이 "웹소설"인지 확인

### 작업 그룹 E: ZEN 목표 연결 + 오늘 쓴 글자수

- [ ] **T1.E.1** — ZEN에 회차 목표 전달
  - 파일: `apps/desktop/src/routes/Workspace.tsx`, `apps/desktop/src/components/ZenMode.tsx`
  - 내용:
    - Workspace가 `<ZenMode target={...}>`에 웹소설 프리셋이면 `episode_char_target`, 아니면 0 전달
    - ZenMode 진행바는 이미 구현돼 있음 (상단 8px 호버 시 표시) — 회차 누적(T1.B.3과 동일 계산)을 기준으로 동작하도록 `charCount` 의미를 점검하고, 필요하면 `episodeCount` prop 추가
    - `zen-bar` 문구에 목표 대비 표시: `"이번 화 {n} / {target}자"`
  - 의존: T1.B.3
  - 검증: 수동 — ZEN 진입 → 상단 호버 → 진행바 표시, 5,000자 도달 시 100%

- [ ] **T1.E.2** — 엔진 일별 집필량 기록
  - 파일: `engine/internal/store/` (마이그레이션), 신규 `engine/internal/stats/` 패키지, `engine/internal/rpc/handlers/nodes.go`(updateContent 훅), 신규 `engine/internal/rpc/handlers/stats.go`
  - 내용:
    - 테이블 `writing_stats (project_id TEXT, day TEXT /* YYYY-MM-DD 로컬 */, chars_added INTEGER, PRIMARY KEY(project_id, day))`
    - `nodes.updateContent` 처리 시 기존 doc과 새 doc의 `CountChars` 차이를 계산해 **양수일 때만** 해당 일자에 누적 (삭제는 0으로 클램프 — 퇴고로 줄어든 날을 음수로 보여주지 않는다)
    - RPC `stats.today(project_id)` → `{ chars_added: number }`
  - 참조: 핸들러 등록은 `handlers/notes.go`처럼 작은 패키지 패턴
  - 검증: Go 테스트 — 같은 날 2회 저장 누적 / 글자 삭제 시 0 / 날짜 분리, `make test-go`

- [ ] **T1.E.3** — 컨텍스트 패널 "오늘" 표시
  - 파일: `apps/desktop/src/lib/rpc.ts`, `apps/desktop/src/components/ContextPanel.tsx`
  - 내용: "이 씬" 섹션 stat-row 아래에 한 줄 — `"오늘 {n}자"`. 노드 저장 성공 시(saveStatus가 saved로 바뀔 때) 재조회, 과도한 폴링 금지
  - i18n: `workspace.todayChars` ko/en/ja
  - 의존: T1.E.2
  - 검증: 수동 — 타이핑 → 저장 후 숫자 증가, 앱 재시작 후에도 당일 값 유지

---

## ✅ Phase 1 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] 웹소설 프로젝트에서 화 게이지·푸터 회차 카운트·ZEN 진행바가 동일한 합산 값을 보여줌
- [ ] 일반 소설 프로젝트에서는 기존 UI가 그대로임 (회귀 없음)

**자동 검증:**
- [ ] `make test` 통과 (Go + Vitest + Vite build + cargo check)

**수동 확인:**
- [ ] 새 프로젝트 → "웹소설 연재" → 1권>1화>씬1 생성: 아웃라인에 화 게이지 표시
- [ ] 씬에 글 작성 → 푸터 "이번 화 n / 5,000자" 실시간 갱신 → 같은 화에 씬 추가 후 합산 일관성 확인
- [ ] 화 우클릭 → 본문 복사 → 외부 텍스트 에디터 붙여넣기: 문단 사이 빈 줄, 멘션이 일반 텍스트로
- [ ] 컨텍스트 패널 "오늘 n자"가 작성량만큼 증가
- [ ] 회차 목표를 5,500으로 변경 → 게이지·푸터·ZEN 모두 반영

**이 체크포인트를 통과하면 사용자에게 확인 요청 후 Phase 2로 진행.**
실패 시: 실패 항목 보고 → 원인 파악 → 수정 → 재검증.

---

## 참고 자료

- 로드맵: [`roadmap.md`](./roadmap.md)
- 글자수 계산: `engine/internal/node/word_count.go`, 프론트 카운터는 `components/editor/Tiptap.tsx`
- 아웃라인 프리셋: `apps/desktop/src/lib/outlineRepair.ts`

## 메모 / 주의

- `charCount`(프론트 라이브)와 `word_count`(엔진 저장값)는 저장 debounce(800ms) 동안 어긋난다.
  회차 합산 시 **현재 씬만 라이브 값으로 치환**하는 T1.B.3 방식을 ZEN에서도 동일하게 쓸 것.
- 커맨드 팔레트 섹션은 8개 고정 — `Workspace.tsx`의 주석(Phase-15 cleanup) 참조. 복사 명령은 `내보내기` 섹션에 넣는다.
- i18n 키 추가 시 `MessageKey` union에 누락되면 타입 에러로 빌드가 깨진다. ko/en/ja 3곳 모두 추가.

---
_다음 페이즈: Phase 2 — 구조·에디터 환경 → [`phase-2-structure-editor.md`](./phase-2-structure-editor.md)_
