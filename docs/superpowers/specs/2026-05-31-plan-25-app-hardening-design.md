# Linetta Plan 25 — App Hardening & Product Readiness Analysis

> Date: 2026-05-31  
> Status: Implemented through 1차 개발 closure slice
> Scope: 현재 Linetta 앱 전체를 대상으로 한 부족한 부분 분석과 개선 방향. 코드 변경 전 개발 계획의 근거 문서.
> Closure note: 아래 `Key Gaps`는 구현 전 기준 분석이며, 2026-05-31 종료 시점 반영 사항은 문서 하단 `Implementation Update`와 로드맵 addendum을 기준으로 본다.

## Executive Summary

Linetta는 이미 "작가가 직접 쓰고, AI가 옆에서 돕는" 데스크톱 글쓰기 앱의 핵심 세로 슬라이스를 갖췄다. Tauri shell, React workspace, Go engine, SQLite persistence, AI generation, companion loop, markdown import/export, snapshots, backup, Git sync까지 MVP 기능면은 넓다.

지금 가장 부족한 부분은 새 기능의 양보다 **제품 안정성의 증거**다. Go engine은 테스트가 풍부하지만, writer-facing React/Tauri 경계는 테스트와 실패 상태 UX가 얇다. 특히 editor, AI modal, companion, import/export, settings, Git sync는 상태 조합이 많은데 현재 검증은 `pnpm build`와 수동 확인에 의존한다.

따라서 다음 개발은 새 기능을 넓히기보다 "작가가 하루 종일 써도 데이터가 사라지지 않고, 문제가 생기면 원인을 알 수 있고, 릴리스 전에 자동으로 잡힌다"는 신뢰 기반을 쌓는 쪽이 우선이다.

## Current Architecture Snapshot

### Runtime Shape

- Desktop app: Tauri 2 Rust shell + React/Vite UI.
- Engine: bundled Go sidecar `linetta-engine --stdio`.
- IPC: Web UI -> Tauri command `engine_call` -> Rust JSONRPC client -> Go JSONRPC server.
- Storage: SQLite under `LINETTA_HOME` or OS app support dir.
- AI: engine uses `github.com/devlikebear/tars` LLM provider abstraction; settings select `claude-code-cli` or `openai-codex`.

### Main Feature Surface

- Library, all-library, workspace, thread view, settings routes.
- Recursive project/node tree with Tiptap editor.
- Mentions, entities, relationships, threads, beats, notes, plot spine.
- AI inline generation, variations, context preview.
- Companion chat with proposal application and read-query loop.
- Snapshots, restore, markdown import/export, daily backup, Git sync.

### Verification Observed

Commands run on 2026-05-31:

```bash
cd engine && go test ./...
cd engine && go test ./... -coverprofile=/tmp/linetta-cover.out
cd apps/desktop && pnpm build
cd apps/desktop/src-tauri && cargo check
```

Results:

- Go tests passed across engine packages. Coverage is generally healthy in core packages, with lower spots in `gitsync` and `paths`.
- Frontend production build passed.
- Rust/Tauri `cargo check` passed.
- Vite emitted a bundle warning: minified JS chunk is about 641 KB, above the default 500 KB warning threshold.

## Key Gaps

### GAP-001: Frontend Has No Test Harness

Evidence:

- `apps/desktop/package.json` has `dev`, `build`, `preview`, `tauri` scripts only.
- `apps/desktop/src` currently has no `*.test.*` or `*.spec.*` files.
- `Workspace.tsx` concentrates a large amount of routing, editor state, AI state, command palette behavior, dialog behavior, autosave, snapshots, ZEN, companion, notes, and side panels.

Risk:

The most important writer workflows are fragile to regressions that TypeScript cannot catch: async event ordering, stale selection offsets, save/restore round trips, stream event races, modal close behavior, and command palette actions.

Improvement:

Add Vitest + React Testing Library + Tauri mocks, then cover the smallest high-value seams first:

- Pure helpers: tree flatten/build, proposal application, import/export save wrappers, stream block stripping.
- Hooks: `useAIGeneration`, `useCompanion`, debounced/throttled callbacks.
- Components: `CommandPalette`, `AIPanel`, `CompanionPanel`, `VersionSheet`, `NewProjectModal`.
- Route smoke tests with mocked RPC: Library empty state, Workspace load, Settings load/save.

### GAP-002: No Root Verification Contract or CI

Evidence:

- There is no root `Makefile` in the current checkout.
- No `.github/workflows` files were found.
- Verification requires remembering three working directories and commands.

Risk:

The repo already spans Go, Rust, TypeScript, pnpm, Tauri, SQLite migrations, and sidecar packaging. Without one command and CI, regressions will slip between stacks, especially during rapid feature phases.

Improvement:

Create a root-level verification contract:

- `make test`: Go tests, frontend build/test, Rust check.
- `make build-engine`: wraps `scripts/build-engine.sh`.
- `make ci`: same as CI, no dev-only assumptions.
- GitHub Actions workflow that installs Go/pnpm/Rust, runs the above, and caches dependencies.

### GAP-003: Engine Startup and JSONRPC Failure UX Is Too Thin

Evidence:

- Rust setup logs engine spawn failure to stderr but continues app setup.
- `engine_call` depends on managed `EngineState`; if setup failed, the UI only discovers it later through command failures.
- JSONRPC client has no per-call timeout and does not explicitly fail all pending requests if the sidecar exits.

Risk:

If the bundled engine is missing, blocked, incompatible, or crashes, the writer sees generic load errors rather than a recovery-oriented state. A hanging request can leave the UI stuck in loading/saving/streaming state.

Improvement:

Make engine health a first-class app state:

- Store startup error in Tauri-managed state.
- Add `engine_status` command with `{ ok, version, db_path, error }`.
- Add user-facing fatal/diagnostic screen before Library/Workspace RPCs.
- Add JSONRPC call timeouts and pending-request drain on reader EOF.
- Add Rust unit tests for response routing, notification forwarding, timeout, and EOF behavior.

### GAP-004: Persistence Invariants Are Mostly Application-Level

Evidence:

- Initial SQL schema uses free-form `kind`, `status`, `length_target`, `default_pov`, `reason`, and relationship fields without `CHECK` constraints.
- Node create handlers accept `kind` from RPC params and pass it through to repo methods.
- `nodes.update_content` can be called with any node ID; leaf-only semantics are documented elsewhere but not enforced at the handler/repo boundary.
- `nodes.set_last_opened` updates a project with a node ID but does not validate that the node belongs to the project.

Risk:

The UI usually sends valid values, but companion actions, future automation, import paths, or manual JSONRPC calls can create impossible state. Once invalid state enters SQLite, later UI assumptions become harder to defend.

Improvement:

Codify invariants in both Go validation and migrations:

- Validate enum-like fields in handlers/repos.
- Restrict `UpdateContent` to leaf nodes.
- Validate `last_opened_node_id` belongs to the target project.
- Add safe SQLite `CHECK` constraints in a migration where possible, or add table-copy migration if SQLite requires it.
- Add property/round-trip tests for import, restore, delete, and move operations.

### GAP-005: Background Work Is Invisible to Users

Evidence:

- Summarizer logs failures to stderr.
- Backup and Git sync scheduler logs failures to stderr.
- Companion still has a TODO to surface persistence errors instead of silently ignoring them.
- Settings shows backup path and Git sync controls, but not last backup/sync status.

Risk:

The app can silently lose an important convenience layer: stale summaries reduce AI context quality, backup/Git sync can fail for days, and the writer has no obvious signal until they need recovery.

Improvement:

Add a lightweight operations status model:

- Persist last backup run, last backup error, last Git sync result, and last summarizer error/count.
- Surface concise status in Settings and a small non-intrusive Workspace status entry.
- Emit events for background job failures so active sessions show a toast only when actionable.
- Add `diagnostics.get` RPC for support/debug snapshots.

### GAP-006: Product Onboarding and Recovery Flows Are Underdeveloped

Evidence:

- README still describes the app as a rebuild and only gives `./scripts/dev.sh`.
- Library's menu button is disabled.
- First-run behavior opens the new project modal, but there is no onboarding that explains where data lives, how backups/Git sync work, or how to recover.
- Export/import and Git sync exist, but recovery from backup is not a first-class UI path.

Risk:

The user may trust the app less than the implementation deserves. For a writing app, perceived safety is product functionality, not polish.

Improvement:

Add a product-readiness layer:

- First-run "Writing Safety" checklist: local DB path, backup path, optional Git sync.
- Library menu actions: reveal data folder, reveal backups folder, import, settings.
- Backup restore flow: pick backup DB, preview metadata, restore to a new copy or replace with confirmation.
- README sections for install/dev/verify/data locations/troubleshooting.

### GAP-007: Bundle and Route Loading Are Starting to Creak

Evidence:

- `pnpm build` warns that the minified JS chunk is about 641 KB.
- App routes/components are imported eagerly from `App.tsx` and `Workspace.tsx`.

Risk:

This is not urgent for a desktop app, but it will become more noticeable as AI panels, companion, import preview, and sheets grow. Eager loading also makes route-level tests slower.

Improvement:

Introduce route-level and panel-level lazy loading:

- Lazy-load Settings, ThreadView, LibraryAll, companion, AI panel, and heavy sheets.
- Configure Rollup chunks for Tiptap/ProseMirror and Tauri plugin bindings if needed.
- Add a bundle budget check to CI so growth is intentional.

## Prioritization

| Priority | Gap | Why Now |
|---|---|---|
| P0 | Frontend tests + verification contract | Prevents regressions in the writer-facing surface before more features land. |
| P0 | Engine startup / JSONRPC failure UX | A broken sidecar currently becomes vague UI failure. |
| P1 | Persistence invariants | Protects writer data and automation paths. |
| P1 | Background job visibility | Makes backup, Git sync, and summarizer trustworthy. |
| P2 | Onboarding / recovery UX | Improves product confidence and supportability. |
| P2 | Bundle split / performance budgets | Keeps the growing desktop app responsive. |

## Proposed Roadmap Shape

This is larger than a weekend slice and has clear verification checkpoints, so implement as 5 phases:

1. Quality Gate Foundation
2. Desktop Runtime Resilience
3. Data Integrity Hardening
4. Operations Visibility & Recovery
5. Product Readiness Polish

The phase-level implementation plan is in:

`docs/superpowers/plans/2026-05-31-plan-25-app-hardening-roadmap.md`

## Implementation Update — 2026-05-31

Plan 25 now includes the 1차 개발 종료 slice:

- App-wide SQLite-backed search with Library/Workspace UI entry points.
- Version bump command aligned across package, Tauri, Cargo, lockfile, and engine diagnostics.
- OS-specific GitHub Actions desktop build workflow for macOS, Linux, and Windows artifacts.
- TARS `pkg/tools` 기반 companion built-in tools:
  - `web_search` and `web_fetch` are available to the `cmd+j` companion loop.
  - Settings stores the `web_search` provider (`brave` or `perplexity`) and API key locally.
  - `linetta_apply_ops` lets the AI directly update outline, storylines, beats, characters, relationships, places, scenes, summaries, and durable memories when the writer's intent is clear.
- README refresh for search, versioning, local builds, CI, and release-build workflow.
- GAP-001/GAP-002 are materially addressed by the root verification contract, CI workflow, and frontend regression tests.
- GAP-005 is materially addressed by operations-status persistence, Settings surfacing, and companion persistence error recording.
- GAP-006 is partially addressed by README refresh, Library menu actions, and first-run writing-safety checklist; backup restore remains a later roadmap item.
