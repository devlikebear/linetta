# Phase 2 — Library: 워드마크 히어로 + 책등 카드

> 선행: Phase 1.
> 대상 라우트: `/` (routes/Library), `/library/all` (routes/LibraryAll)
> 대상 컴포넌트: `components/ProjectCard.(tsx|css)`, `components/NewProjectModal`(스타일만; 모달 셸 동작은 Phase 5와 겹침 — 여기선 카드/그리드/히어로에 집중)

## 목업 참조 (Library 섹션)
- **워드마크 히어로**: 상단에 큰 세리프 워드마크("LINETTA"), 부제는 muted. `--font-serif`, 큰 트래킹.
- **책등(book-spine) 카드**: 프로젝트 카드를 책등 메타포로 — 세로 강조, 좌측 thread 색 띠(`--t-*` 중 프로젝트별 배정), 제목은 세리프, 메타(단어 수/수정일)는 `--font-mono` + `--muted`.
- 카드 배경 `--surface`, 보더 `--line`, hover 시 `--shadow-md` 부상 + 살짝 translateY.
- 그리드: 넉넉한 gap, 카드 radius `--r-lg`.

## 작업
1. **Library.css / LibraryAll.css**
   - 히어로 영역 타이포를 `--font-serif`로, 색은 `--ink` / 부제 `--muted`.
   - 그리드 gap·패딩을 목업 수준으로 여유있게.
   - 하드코딩 hex 전부 Phase 1 매핑 표대로 토큰화.
2. **ProjectCard.css**
   - 책등 메타포: 좌측 4~6px thread 색 띠(`border-left` 또는 `::before`). 색은 프로젝트 식별자 해시로 `--t-sienna/teal/blue/plum/olive` 중 배정(JS에서 클래스/인라인 변수로 전달하거나 nth 기반).
   - 제목 `--font-serif`, 메타 `--font-mono`/`--muted-2`.
   - hover: `box-shadow: var(--shadow-md)`, `transform: translateY(-2px)`, `transition`.
   - radius `--r-lg`, 배경 `--surface`.
3. 빈 상태/CTA 버튼: `--accent` 배경 + `--surface` 텍스트, radius `--r-md`.

> thread 색 배정에 마크업 변경이 필요하면 ProjectCard.tsx에 `style={{'--spine': ...}}` 또는 `className`만 추가 — 데이터/로직은 불변.

## 체크포인트
- [ ] 히어로 워드마크 세리프 적용
- [ ] ProjectCard 책등 스타일 + thread 색 띠
- [ ] hover 인터랙션
- [ ] Library/LibraryAll 하드코딩 hex 0 (토큰화)
- [ ] `make test-desktop` 통과
- [ ] `/` 와 `/library/all` 육안 확인

## 검증
```bash
cd apps/desktop && pnpm test && pnpm build
grep -rn "#[0-9a-fA-F]\{3,8\}" src/routes/Library.css src/routes/LibraryAll.css src/components/ProjectCard.css   # 0건 목표
```
