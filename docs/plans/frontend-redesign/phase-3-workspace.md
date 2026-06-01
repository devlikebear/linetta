# Phase 3 — Workspace 셸: 브레드크럼 · 아웃라인 레일 · 프로즈 에디터

> 선행: Phase 1.
> 대상 라우트: `/workspace/:projectId` (routes/Workspace)
> 대상 컴포넌트: `components/OutlinePanel`, `components/editor/Tiptap.css`, `components/editor/AITargetExtension.css`, `components/ZenMode`, (상단 바는 Workspace.tsx 내 레이아웃)

## 목업 참조 (Workspace 섹션)
- **상단 바**: breadcrumb(라이브러리 › 프로젝트 › 챕터) `--font-ui`/`--muted`, 우측에 모드/액션. 배경 `--surface`, 하단 `--line`.
- **아웃라인 레일(좌)**: 트리(챕터 → 씬). 선택 항목 `--accent-tint` 배경 + `--accent` 좌측 인디케이터. 들여쓰기 가이드 라인 `--line-soft`. 폰트 `--font-ui`.
- **프로즈 에디터(중앙)**: 본문 `--font-edit`(Newsreader), 넉넉한 `line-height`(~1.7), 측정폭(measure) 제한(~68ch). 단락 첫 글자 드롭캡: `.prose p.lead::first-letter` 대형 세리프. **씬 마커**: 씬 구분선/심볼 `--muted` 중앙 정렬. 선택/AI 타겟 하이라이트는 `--accent-tint`.
- 배경: 에디터 캔버스 `--paper` 또는 `--surface`, 페이지 느낌의 여백.

## 작업
1. **Workspace.tsx + 해당 CSS**: 상단 바를 breadcrumb 레이아웃으로(마크업 최소 추가 가능), 배경/보더 토큰화. 3분할(레일 / 에디터 / 우패널) 그리드 간격 정리. *기존 메모리 사용자 선호(중첩 split 지양·여백 넉넉)* 존중.
2. **OutlinePanel.css**: 트리 선택 상태 `--accent-tint`/`--accent`, 들여쓰기 가이드 `--line-soft`, hover `--surface-2`.
3. **Tiptap.css**:
   - `.prose` (또는 해당 에디터 루트): `font-family: var(--font-edit)`, `line-height: 1.7`, `max-width: 68ch`, `color: var(--ink)`.
   - 드롭캡: `.prose p.lead::first-letter { font-family: var(--font-serif); font-size: 3.2em; float: left; line-height: .8; padding-right: .08em; color: var(--ink); }` (목업 셀렉터 확인 후 매칭).
   - 씬 마커: 구분 단락에 `--muted` 중앙 심볼.
   - 선택/플레이스홀더 색 토큰화.
4. **AITargetExtension.css**: AI 타겟 하이라이트 `--accent-tint`/`--accent`.
5. **ZenMode.css**: 풀스크린 집필 오버레이 — 배경 `--paper`, 본문 `--font-edit`, 군더더기 UI 숨김, 페이드 인.

> 드롭캡은 본문 전체가 아니라 챕터/씬 첫 단락에만. 현재 마크업에 `.lead` 클래스가 없으면 에디터가 첫 단락에 부여하는 방식이 있는지 확인 후 셀렉터 결정(없으면 `.prose > p:first-of-type` 폴백).

## 체크포인트
- [ ] 상단 breadcrumb 바 토큰화
- [ ] OutlinePanel 트리 선택/들여쓰기 스타일
- [ ] 에디터 Newsreader + measure + line-height
- [ ] 드롭캡 + 씬 마커
- [ ] ZenMode 페이퍼 배경
- [ ] 해당 파일 하드코딩 hex 토큰화
- [ ] `make test-desktop` 통과
- [ ] `/workspace/:id` 에서 집필 화면 육안 확인 (한글 본문이 명조 폴백으로 정상 렌더되는지 포함)

## 검증
```bash
cd apps/desktop && pnpm test && pnpm build
```
