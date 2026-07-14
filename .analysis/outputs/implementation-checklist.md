# 감사 후 구현 체크리스트

## Release blockers

- [x] stale autosave cross-scene regression test를 먼저 추가
- [x] node/version-bound save, cancel/flush, optimistic lock 구현
- [x] daily backup 실패 후 당일 retry, SQLite quick_check, cancel-aware scheduler 구현
- [x] 앱 안 privacy policy 링크와 third-party AI 명시 동의 추가
- [x] Go/React Router/Rust patch update 후 세 dependency audit 재실행

## This month

- [x] sync manifest/atomic write/partial failure reporting
- [x] Markdown “backup” 명칭 정정과 versioned `.linetta` recovery format
- [x] startup recovery/pre-migration backup/migration checksum
- [x] atomic import cleanup, FTS/mention startup reconcile
- [x] IPC error envelope 및 i18n key/placeholder executable contract tests
- [x] Workspace persistence queue, typed app events, React Hooks lint gate
- [ ] Tiptap walker/OpenRouter controller 추출 — 기능 변경과 맞닿을 때 수행할 비차단 리팩터링

## Verification (2026-07-14)

- [x] `make ci`
- [x] `go test -race ./...`
- [x] `cargo test --features mas`
- [x] `govulncheck ./...`, `pnpm audit --prod`, `cargo audit`
- [x] `make validate-distribution`, `git diff --check`

전체 완료 기준과 비권장 항목은 `full-audit.md`를 참조한다.
