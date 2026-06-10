# Phase 2: 구조·에디터 환경 — 작업지시서

_이 페이즈의 목표를 달성하기 위한 구체적 작업 목록. Claude Code가 위에서부터 순차 실행하되, 각 체크포인트에서 사용자 확인을 받는다._

_작성일: 2026-06-10_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 4~5일_

## 페이즈 목표

회차 50개 이상의 연재 프로젝트를 마찰 없이 관리한다: **회차 상태(초고~발행)와 비축분이
한눈에 보이고, 아웃라인에서 인라인 rename·드래그 앤 드롭이 되며, 웹소설 프리셋에서는
씬 분할 없이 화에 바로 글을 쓸 수 있다. 다크 모드와 에디터 타이포 설정으로 장시간
집필 환경을 갖춘다.**

## 전제 조건

- [ ] Phase 1 완료 및 사용자 승인 (화 합산 글자수·웹소설 온보딩이 동작하는 상태)
- [ ] working tree 미커밋 변경 확인

## 포함 기능

1. **회차 상태 확장 + 비축분** — `published` 상태 추가, 아웃라인 상태 점, 비축분 카운트
2. **화 직접 집필** — 웹소설 프리셋에서 권>화(leaf) 2단계 구조 허용
3. **아웃라인 인라인 rename** — 2연속 prompt 다이얼로그 제거
4. **아웃라인 드래그 앤 드롭** — 같은 부모 내 재배치 + 컨테이너로 이동
5. **다크 모드 + 에디터 타이포 설정** — 테마/폰트 크기/줄간격
6. **단축키 도움말 보강** — ⌘J/⌘I/⌘F 누락 수정

## 이 페이즈에서 하지 않는 것

- 발행일 기록·연재 캘린더 → Out of Scope (상태 변경 시각만 `updated_at`으로)
- 집필 통계 시각화 → Phase 3
- 우측 패널 동시 표시(스택) → Out of Scope
- 에디터 폰트 패밀리 커스텀(임의 폰트 로드) → 크기·줄간격·테마까지만. 폰트 선택은 기존 serif/sans 변수 내 토글

## 작업 체크리스트

### 작업 그룹 A: 회차 상태 + 비축분

- [ ] **T2.A.1** — `published` 상태 추가 (엔진)
  - 파일: `engine/internal/node/node.go` (status 검증), 관련 테스트
  - 내용: `NodeStatus`에 `published` 허용. 기존 `draft | revision | final` 검증 로직에 추가. 마이그레이션 불필요(TEXT 컬럼)
  - 검증: `make test-go`

- [ ] **T2.A.2** — 프론트 상태 타입·라벨
  - 파일: `apps/desktop/src/lib/types.ts` (`NodeStatus`), `apps/desktop/src/lib/i18n.tsx`
  - 내용: `"published"` 추가. `workspace.status.published` ko("발행")/en("Published")/ja 라벨. 기존 `workspace.status.*` 키 패턴 확인
  - 검증: `pnpm tsc --noEmit`

- [ ] **T2.A.3** — 아웃라인 상태 점 + 상태 변경 메뉴
  - 파일: `apps/desktop/src/components/OutlinePanel.tsx`, `OutlinePanel.css`, `apps/desktop/src/routes/Workspace.tsx`
  - 내용:
    - 화 단위 노드 행 앞에 4색 상태 점 (draft=무채색, revision=올리브, final=틸, published=잉크). 기존 `--t-*` 변수만 사용
    - 컨텍스트 메뉴에 "상태" 서브 항목 4개 → `nodes.setStatus` RPC (없으면 기존 rename 핸들러 패턴으로 엔진에 추가)
    - 화-as-container의 상태는 컨테이너 노드 자체의 `status` 사용 (화-as-leaf는 leaf의 status)
  - 검증: Vitest + 수동 — 상태 변경 후 재시작에도 유지

- [ ] **T2.A.4** — 비축분 카운트
  - 파일: `apps/desktop/src/components/ContextPanel.tsx`
  - 내용: 웹소설 프리셋일 때 "이 씬" 섹션에 한 줄 — `"발행 {n}화 · 비축 {m}화"` (발행=published 화 수, 비축=final 상태이고 미발행인 화 수). 트리에서 계산하는 순수 함수는 `useFirstLeaf.ts`에 (`countEpisodeStatus`), 테스트 포함
  - i18n: `workspace.episodeStock` ko/en/ja
  - 의존: T2.A.3
  - 검증: 단위 테스트 + 수동

### 작업 그룹 B: 화 직접 집필 (권>화 2단계)

- [ ] **T2.B.1** — 웹소설 프리셋에서 "새 화(본문)" 생성 경로
  - 파일: `apps/desktop/src/routes/Workspace.tsx` (`handleCreateChapterFromOutline` 분기), `apps/desktop/src/components/OutlinePanel.tsx` (메뉴)
  - 내용:
    - 웹소설 프리셋일 때 컨텍스트 메뉴의 "새 화"가 **씬을 시드하지 않고** 권 아래에 `kind: "leaf"`로 `{n}화`를 직접 생성
    - 씬 분할을 원하는 사용자를 위해 "화에 씬 추가" 메뉴는 유지 — leaf 화에 씬을 추가하면 화를 컨테이너로 승격(`nodes.convertToContainer` — `outlineRepair.ts`의 `OutlineRepairRPC`에 이미 존재)하고 기존 본문을 씬1로 이동하는 흐름 확인
  - 검증: 수동 — 새 화 생성 → 바로 본문 작성 가능, 화 글자수 게이지(Phase 1)가 leaf 화에도 표시

- [ ] **T2.B.2** — 아웃라인 닥터 규칙 프리셋 예외
  - 파일: `apps/desktop/src/components/OutlinePanel.tsx` (`analyzeOutline`), `apps/desktop/src/lib/outlineRepair.ts`
  - 내용:
    - 웹소설 프리셋에서 depth 1의 leaf 화(`isStructuralChapterLabel` 매칭 + `word_count > 0`)를 `sceneUnderPart`/`chapterAsScene` 이슈에서 제외
    - `repairOutlineTree`가 본문 있는 leaf 화를 컨테이너로 강제 변환하지 않는지 테스트로 고정
  - 검증: `outlineRepair.test.ts`에 웹소설 2단계 구조 케이스 추가, Vitest 통과

### 작업 그룹 C: 아웃라인 조작 마찰 제거

- [ ] **T2.C.1** — 인라인 rename
  - 파일: `apps/desktop/src/components/OutlinePanel.tsx`, `apps/desktop/src/components/InlineEditableText.tsx`(재사용), `apps/desktop/src/routes/Workspace.tsx`
  - 내용:
    - 컨텍스트 메뉴 "이름 바꾸기" 클릭 시 해당 행이 인라인 입력으로 전환되어 **표시 제목(title)** 을 편집 (라벨 `{n}화` 등 구조 라벨은 자동 번호이므로 직접 편집 대상에서 제외)
    - `handleRenameNode`의 2연속 `promptDialog` 제거. 라벨 자체를 바꿔야 하는 경우는 아웃라인 닥터의 정리 기능에 맡긴다
  - 참조: 에디터 상단 `scene-title`의 `InlineEditableText` 사용 방식
  - 검증: `OutlinePanel.test.tsx`에 인라인 편집 커밋/ESC 취소 테스트, Vitest 통과

- [ ] **T2.C.2** — 드래그 앤 드롭 재배치
  - 파일: `apps/desktop/src/components/OutlinePanel.tsx`, `OutlinePanel.css`, `apps/desktop/src/lib/rpc.ts`, 엔진 `engine/internal/rpc/handlers/nodes.go`
  - 내용:
    - 신규 의존성 없이 HTML5 DnD로: 같은 부모 내 순서 변경 + 컨테이너 위에 드롭하면 그 컨테이너의 마지막 자식으로 이동
    - 드롭 인디케이터(행 사이 1px 라인)와 드래그 중 행 반투명 처리
    - 엔진에 `nodes.moveTo(id, parentId, ordinal)` RPC가 없으면 추가 (기존 `moveUp/moveDown`·`moveToParent` 로직 재조합)
    - 드롭 완료 후 `refreshTreeKeepNode` 호출, 실패 시 toast
  - 의존: T2.C.1과 독립
  - 검증: Go 테스트(ordinal 재배치) + 수동 — 화 10개 프로젝트에서 끌어서 재배치, 새로고침 후 순서 유지

### 작업 그룹 D: 다크 모드 + 에디터 타이포 설정

- [ ] **T2.D.1** — 설정 필드 (엔진)
  - 파일: `engine/internal/settings/settings.go`(+test)
  - 내용: `theme: "system" | "light" | "dark"` (기본 system), `editor_font_size: number`(기본 17 — 현행 Tiptap.css 값 확인 후 일치), `editor_line_height: number`(기본 현행 값) 추가. settings.json round-trip
  - 검증: `make test-go`

- [ ] **T2.D.2** — 다크 테마 CSS
  - 파일: `apps/desktop/src/App.css`, `apps/desktop/src/components/**/*.css`(변수 누수 점검)
  - 내용:
    - `:root[data-theme="dark"]`에 다크 팔레트 정의 — 기존 변수명(`--ink`, `--surface`, `--line`, `--t-*` 등)을 그대로 재정의하는 방식. 컴포넌트 CSS는 변수만 쓰므로 원칙적으로 무수정
    - `color-scheme`을 테마에 맞게 전환. `system`이면 `prefers-color-scheme` 미디어 쿼리 추종
    - 하드코딩 색(`#211e18` 등)이 컴포넌트 CSS에 남아 있으면 변수로 치환
  - 검증: 수동 — 라이브러리/워크스페이스/ZEN/모달 전 화면 다크 확인, 대비 부족한 곳 기록

- [ ] **T2.D.3** — Settings 화면 "에디터" 섹션 + 적용
  - 파일: `apps/desktop/src/routes/Settings.tsx`, `Settings.css`, `apps/desktop/src/lib/types.ts`, `apps/desktop/src/App.tsx`(테마 적용 루트), `components/editor/Tiptap.css`
  - 내용:
    - Settings에 "에디터" 섹션: 테마 3택 칩, 폰트 크기(15~22 스테퍼), 줄간격(1.6~2.2 스테퍼)
    - 앱 루트에서 settings 로드 후 `document.documentElement.dataset.theme` + CSS 변수(`--edit-size`, `--edit-leading`) 설정. Tiptap.css가 이 변수를 사용하도록
  - i18n: `settings.editor.*` ko/en/ja
  - 의존: T2.D.1, T2.D.2
  - 검증: `Settings.test.tsx` 보강 + 수동 — 변경 즉시 에디터 반영, 재시작 후 유지

### 작업 그룹 E: 단축키 도움말

- [ ] **T2.E.1** — ShortcutsModal 누락 단축키 추가
  - 파일: `apps/desktop/src/components/ShortcutsModal.tsx`, `apps/desktop/src/lib/i18n.tsx`
  - 내용: `⌘J`(글쓰기 동료), `⌘I`(AI 생성), `⌘F`(전체 검색) 항목 추가. 라벨은 커맨드 팔레트 힌트와 동일 용어 사용
  - 검증: Vitest 통과, 수동 확인

---

## ✅ Phase 2 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] 웹소설 프로젝트: 권>화(leaf) 직접 집필 + 상태 점 + 비축분 카운트 동작
- [ ] 일반 소설 프로젝트: 부>장>씬 흐름과 아웃라인 닥터가 기존과 동일 (회귀 없음)

**자동 검증:**
- [ ] `make test` 통과

**수동 확인:**
- [ ] 화 3개를 final로 → "비축 3화" 표시 → 1개를 published로 → "발행 1화 · 비축 2화"
- [ ] 아웃라인에서 화를 드래그해 순서 변경 → 새로고침 후 유지
- [ ] 행에서 인라인으로 제목 수정 → ESC 취소 / Enter 커밋 동작
- [ ] 다크 모드 전환 → 라이브러리·워크스페이스·ZEN·컴패니언 패널 모두 정상 대비
- [ ] 폰트 크기 변경이 에디터와 ZEN에 즉시 반영
- [ ] ⌘? 단축키 도움말에 ⌘J/⌘I/⌘F 표시

**이 체크포인트를 통과하면 사용자에게 확인 요청 후 Phase 3로 진행.**
실패 시: 실패 항목 보고 → 원인 파악 → 수정 → 재검증.

---

## 참고 자료

- 로드맵: [`roadmap.md`](./roadmap.md)
- 아웃라인 닥터 규칙: `apps/desktop/src/components/OutlinePanel.tsx`의 `analyzeOutline`, `apps/desktop/src/lib/outlineRepair.ts`
- 노드 RPC: `engine/internal/rpc/handlers/nodes.go`, `engine/internal/node/repo.go`

## 메모 / 주의

- 다크 팔레트는 **변수 재정의 방식**을 지킬 것 — 컴포넌트 CSS에 다크 분기를 흩뿌리면 유지보수 불가.
- DnD는 외부 라이브러리(dnd-kit 등)를 추가하지 않는다. 요구 범위(세로 리스트 재배치)는 HTML5 DnD로 충분.
- leaf 화 ↔ 컨테이너 화 전환은 데이터 이동이 수반되므로, 전환 전 확인 다이얼로그를 띄우고 스냅샷(`snapshots.createManual`)을 먼저 남길 것.

---
_다음 페이즈: Phase 3 — 통계·플랫폼·퇴고 → [`phase-3-stats-platform.md`](./phase-3-stats-platform.md)_
