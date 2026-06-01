# Phase 4 — 우측 패널군

> 선행: Phase 1.
> 대상 컴포넌트: `components/PlotPanel`, `components/ContextPanel`, `components/ai/AIPanel`, `components/ai/AIContextChecklist`, `components/companion/CompanionPanel`, `components/companion/ProposalCard`, `components/EntitySheet`, `components/ThreadSheet`, `components/VersionSheet`, `components/RelationshipPicker`, `components/NotePopover`

## 목업 참조 (Right panels 섹션)
- **공통 패널 셸**: 배경 `--surface` 또는 `--surface-2`(함몰), 보더 `--line`, radius `--r-lg`, 섹션 헤더 `--font-ui`/`--muted` 소문자 라벨, 수치는 `--font-mono`.
- **Stats/Progress**: 진행 바 트랙 `--surface-3`, 채움 `--accent`. 수치 `--font-mono`.
- **Plot spine + beats**: 세로 spine 라인 `--line`, beat 노드 `--accent`/thread 색, 라벨 세리프.
- **AI 패널**: 모드 토글 칩(`--accent-tint` 선택), textarea `--surface`/`--line` 포커스 `--accent`, 결과 카드, 컨텍스트 체크리스트(체크 `--ok`). 칩 radius `--r-sm`.
- **Companion 챗**: 버블 — 사용자 `--accent-tint`, 어시스턴트 `--surface-2`. tool-call 블록 `--font-mono`/`--surface-3`. apply-card는 `--accent` 보더 강조.
- **Entity/Thread 시트**: 슬라이드 시트, thread 시트는 해당 `--t-*` 색 헤더.

## 작업 (파일별 토큰화 + 목업 스타일)
1. **PlotPanel.css** — spine/beat, thread 색 노드.
2. **ContextPanel.css** — 패널 셸, 섹션 라벨.
3. **AIPanel.css** — 모드 칩, textarea 포커스, 결과 카드.
4. **AIContextChecklist.css** — 체크 상태 `--ok`/`--muted`.
5. **CompanionPanel.css** — 챗 버블 좌우 구분, tool-call 모노, 스크롤 영역.
6. **ProposalCard.css** — apply-card 강조 보더 `--accent`, 액션 버튼.
7. **EntitySheet.css / ThreadSheet.css / VersionSheet.css** — 시트 셸, ThreadSheet 헤더에 `--t-*` 색(엔티티/스레드별 변수 전달).
8. **RelationshipPicker.css / NotePopover.css** — 팝오버 `--shadow-md`, 배경 `--surface`, 보더 `--line`.

> 패널이 많으므로 **공통 셸 패턴**(배경/보더/radius/헤더 라벨)을 먼저 한 패널에서 확정한 뒤 나머지에 복붙 일관 적용. 필요하면 `App.css`에 `.panel`, `.panel-label` 같은 공통 유틸 클래스를 추가해 중복을 줄여도 됨(단, 기존 클래스명 유지하며 추가만).

## 체크포인트
- [ ] 공통 패널 셸 일관 적용
- [ ] AI/Companion 칩·버블·카드 스타일
- [ ] Plot spine + thread 색
- [ ] 시트(Entity/Thread/Version) 토큰화 + thread 색 헤더
- [ ] 9개 CSS 파일 하드코딩 hex 토큰화
- [ ] `make test-desktop` 통과
- [ ] 우패널 각 모드 육안 확인

## 검증
```bash
cd apps/desktop && pnpm test && pnpm build
grep -rln "#[0-9a-fA-F]\{3,8\}" src/components/ai src/components/companion src/components/PlotPanel.css src/components/ContextPanel.css src/components/EntitySheet.css src/components/ThreadSheet.css src/components/VersionSheet.css
```
