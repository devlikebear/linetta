# Linetta 프론트엔드 리디자인 — 로드맵

> **업데이트(실제 구현):** 초기 계획은 "토큰/색 리스킨"이었으나, 목업은 **자체 마크업+클래스 체계를 가진 완성 프로토타입**임이 확인되어
> 접근을 **전체 뷰 포팅**으로 변경했다. 목업의 디자인 시스템 CSS(1033줄)를 `apps/desktop/src/App.css`에 전역 포팅하고,
> 8개 목업 JSX(Library/Workspace/패널/오버레이/Settings)의 마크업 구조로 각 실제 컴포넌트를 재구성하되 실제 RPC/router/Tiptap/데이터/테스트를 보존했다.
> 폰트(Newsreader, IBM Plex Mono)는 `@fontsource`로 로컬 번들. 아래 "페이즈"는 초기 리스킨 단위 기록이며, 실제 작업은 화면 단위로 진행됨.


> 기준 목업: 루트 `Linetta (standalone).html` (디코드본 = 리디자인 디자인 시스템 "A warm, editorial writing instrument for long-form fiction")
> 결정 사항: **단계적 적용 · 디자인 언어 채택(픽셀 일치 아님) · 폰트 로컬 번들**

## 목표

기존 React/Tauri 데스크톱 앱의 비주얼을 목업의 "따뜻한 에디토리얼" 디자인 언어로 전환한다.
핵심은 **하드코딩 색상 제거 → CSS 토큰 시스템 도입**이며, 그 위에 화면별 스타일을 단계적으로 입힌다.
기능 동작(라우팅, RPC, 에디터 로직)은 건드리지 않는다 — **CSS/마크업 클래스/토큰만** 변경한다.

## 현재 상태 (분석 결과)

| 항목 | 현재 | 목표 |
|---|---|---|
| 디자인 토큰 | 없음 (`:root`에 `color-scheme`/`font-family`/`font-size`만) | oklch 기반 토큰 시스템 (`paper/ink/accent/thread/geometry`) |
| 색상 | CSS 20개 파일에 하드코딩 hex ~319회 (distinct 수십 개) | 전부 `var(--token)` 참조 |
| 폰트 | `ui-serif, Georgia` 시스템 폰트만, `@font-face` 0 | Newsreader + IBM Plex Mono woff2 로컬 번들 |
| 테마 | 라이트 전용 (목업도 라이트 전용 → 변경 없음) | 라이트 전용 유지 |
| 강조색 | `#a8312f` (붉은색) | `oklch(0.56 0.13 47)` 번트 시에나 |

### 컨벤션 (유지할 것)
- 컴포넌트마다 **colocated `.css`** (`Foo.tsx` + `Foo.css`). 새 파일 만들지 말고 기존 파일을 수정한다.
- 전역 스타일은 `apps/desktop/src/App.css`, `main.tsx`가 import.
- `index.html`은 `lang="ko"` — 한글 본문 폰트 폴백(명조 계열)을 토큰에 반드시 포함.
- 라우트: `/`(Library), `/library/all`, `/workspace/:projectId`, `/workspace/:projectId/threads`, `/settings`.

## 검증 (모든 페이즈 공통 체크포인트)

```bash
make test-desktop          # = cd apps/desktop && pnpm test && pnpm build
```

빌드가 통과해야 하고, 각 페이즈 종료 시 `pnpm tauri dev`로 해당 화면을 육안 확인한다.

## 페이즈

| # | 파일 | 범위 | 의존 |
|---|---|---|---|
| 1 | [phase-1-foundation.md](phase-1-foundation.md) | 토큰 시스템 + 폰트 번들 + 전역 타이포/바디. 하드코딩 색→토큰 매핑 표 확립 | — |
| 2 | [phase-2-library.md](phase-2-library.md) | Library / LibraryAll: 워드마크 히어로, 책등(book-spine) 카드, ProjectCard | 1 |
| 3 | [phase-3-workspace.md](phase-3-workspace.md) | Workspace 셸: 상단 브레드크럼 바, 아웃라인 레일 트리, 프로즈 에디터(드롭캡·씬 마커), ZenMode | 1 |
| 4 | [phase-4-panels.md](phase-4-panels.md) | 우측 패널군: Plot/Context/AI/Companion/Entity/Thread/Version 시트 | 1 |
| 5 | [phase-5-overlays.md](phase-5-overlays.md) | 오버레이: CommandPalette, SearchModal, ShortcutsModal, Toast, Settings, Import/NewProject 모달 | 1 |

페이즈 1은 **필수 선행**. 2~5는 1 이후 독립적이라 순서 무관(병렬 가능).

## 비범위 (Out of scope)
- 기능/상태/RPC/에디터 로직 변경
- 다크 모드
- 라우팅 구조 변경
- 루트 `Linetta (standalone).html` 자체 수정 (참고 자료로만 둔다)
