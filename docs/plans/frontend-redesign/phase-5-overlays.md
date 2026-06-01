# Phase 5 — 오버레이 · 설정

> 선행: Phase 1.
> 대상 컴포넌트: `components/CommandPalette`, `components/SearchModal`, `components/ShortcutsModal`, `components/ToastProvider`, `routes/Settings`, `components/NewProjectModal`, `components/ImportPreviewModal`, `routes/ThreadView`

## 목업 참조 (Overlays 섹션)
- **공통 오버레이**: 백드롭 `rgba(33,30,24,0.42)` 류 + blur, 패널 `--surface`, radius `--r-xl`, `--shadow-lg`, 페이드/스케일 인.
- **Command palette**: 중앙 상단 부유, 입력 `--font-ui` 큼직, 결과 행 hover `--accent-tint` + 좌측 아이콘, 단축키 힌트 `--font-mono`/`--muted`.
- **Search modal**: 입력 + 결과 리스트, 매치 하이라이트 `--accent`.
- **Shortcuts modal**: kbd 칩 `--surface-2`/`--line`/`--font-mono`.
- **Toast**: `--surface` + `--shadow-md`, 상태색(`--ok`/`--warn`/`--accent`) 좌측 띠.
- **Settings**: 섹션 그룹 카드 `--surface`, 라벨 `--muted`, 입력/토글 토큰화.

## 작업
1. **CommandPalette.css** — 부유 패널, 결과 행 hover, 단축키 힌트 모노.
2. **SearchModal.css** — 입력/결과/하이라이트.
3. **ShortcutsModal.css** — kbd 칩.
4. **ToastProvider.css** — 토스트 셸 + 상태 띠.
5. **Settings (routes/Settings + css)** — 섹션 카드, 입력/토글. 메모리상 사용자 선호(미니멀·플랫·여백) 반영.
6. **NewProjectModal.css / ImportPreviewModal.css** — 공통 모달 셸(백드롭+패널) 일관 적용.
7. **ThreadView.css** — thread 전용 뷰 `--t-*` 색 활용.

> 백드롭/패널/애니메이션을 한 곳(예: `App.css`의 `.overlay-backdrop`, `.overlay-panel`)에서 정의하고 각 오버레이가 재사용하면 일관성↑·중복↓. 기존 클래스는 유지하고 공통 클래스를 덧붙이는 방식.

## 체크포인트
- [ ] 공통 오버레이 셸(백드롭/패널/애니메이션)
- [ ] CommandPalette/Search/Shortcuts 스타일
- [ ] Toast 상태색
- [ ] Settings 미니멀 카드
- [ ] 모달 2종 + ThreadView 토큰화
- [ ] `make test-desktop` 통과
- [ ] `Cmd+K`(팔레트)·검색·단축키·설정 육안 확인

## 검증
```bash
cd apps/desktop && pnpm test && pnpm build
```

## 마감 (전체 리디자인 완료 시)
```bash
# 앱 전역에서 남은 하드코딩 hex 점검 — 의도된 예외만 남아야 함
grep -rn "#[0-9a-fA-F]\{3,8\}" apps/desktop/src --include="*.css" | grep -v ":root" | wc -l
```
모든 페이즈 종료 후 `pnpm tauri dev`로 라이브러리→집필→패널→오버레이 전 흐름을 한 번 통과 확인한다.
