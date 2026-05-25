# Linetta Pro Writer Plan

이 폴더는 Linetta를 "장편 웹소설 작가를 위한 Mac-native AI 협업 스튜디오"로 발전시키기 위한 실행 계획이다.

## 읽는 순서

### MVP (완료)
1. [linetta-pro-writer-roadmap.md](./linetta-pro-writer-roadmap.md)
2. [phase-1-work-library-and-engine.md](./phase-1-work-library-and-engine.md)
3. [phase-2-canon-memory-core.md](./phase-2-canon-memory-core.md)
4. [phase-3-episode-workbench.md](./phase-3-episode-workbench.md)
5. [phase-4-continuity-review-loop.md](./phase-4-continuity-review-loop.md)
6. [phase-5-publication-polish.md](./phase-5-publication-polish.md)

### macOS App Completion (Phase 6~9)
7. [linetta-macos-app-completion-roadmap.md](./linetta-macos-app-completion-roadmap.md)
8. [phase-6-embedded-engine-lifecycle.md](./phase-6-embedded-engine-lifecycle.md)
- [Phase 6.5 — UI Redesign Spec](./phase-6.5-ui-redesign.md)
- [Phase 6.5 — UI Redesign Plan](./phase-6.5-ui-redesign-plan.md)
9. [phase-7-settings-studio.md](./phase-7-settings-studio.md)
10. [phase-8-app-polish-workflow.md](./phase-8-app-polish-workflow.md)
11. [phase-9-live-run-and-editor.md](./phase-9-live-run-and-editor.md)

## 진행 규칙

- 각 phase는 수직 슬라이스다. phase 끝에는 반드시 사용자가 눈으로 확인할 수 있는 동작이 있어야 한다.
- 구현 전 항상 `git status --short --branch`로 변경 범위를 확인한다.
- 테스트 우선: 가능하면 실패 테스트를 먼저 추가한 뒤 구현한다.
- phase 안에서도 기능 단위로 커밋한다.
- 커밋 메시지는 `feat:`, `fix:`, `chore:` 규칙을 따른다.
- AI/Tessera는 Canon memory를 직접 변경하지 않는다. 모든 Canon 변경은 diff로 제안되고 사람이 승인해야 반영된다.
