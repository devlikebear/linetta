# Plan 25 — Linetta App Hardening & Product Readiness Roadmap

> **For agentic workers:** Follow repo `AGENTS.md`: check `git status` before edits, keep commits feature-sized, use `feat/fix/chore` commit messages, and write failing tests before implementation when behavior changes.  
> **Analysis reference:** `docs/superpowers/specs/2026-05-31-plan-25-app-hardening-design.md`

## Goal

Turn the current feature-rich MVP into a safer daily writing app by adding automated frontend/runtime verification, better engine failure handling, stronger persistence invariants, visible background-job health, and basic recovery/onboarding flows.

## Non-goals

- Do not add new creative-writing features in this plan.
- Do not replace the Go engine / Tauri / React architecture.
- Do not redesign the entire UI visual language.
- Do not introduce cloud sync or accounts.

## Current Baseline

Verified on 2026-05-31:

```bash
cd engine && go test ./...
cd apps/desktop && pnpm build
cd apps/desktop/src-tauri && cargo check
```

All three passed. `pnpm build` warned that the main JS chunk is about 641 KB.

## Phase 1: Quality Gate Foundation

**Outcome:** One repo-level command and CI can verify the engine, frontend, and Tauri shell. Frontend gets its first useful regression tests.

### Tasks

- [ ] Add a root `Makefile` with documented targets:
  - `make test`: Go tests + frontend tests/build + Rust check.
  - `make test-go`: `cd engine && go test ./...`.
  - `make test-desktop`: `cd apps/desktop && pnpm test && pnpm build`.
  - `make test-tauri`: `cd apps/desktop/src-tauri && cargo check`.
  - `make build-engine`: `bash scripts/build-engine.sh`.
- [ ] Add Vitest + React Testing Library to `apps/desktop`.
  - Add `test` script to `apps/desktop/package.json`.
  - Add `src/test/setup.ts` for DOM and Tauri mocks.
  - Mock `@tauri-apps/api/core.invoke`, dialog, and fs plugin calls.
- [ ] Write failing tests first for pure/high-risk frontend seams:
  - `useFirstLeaf` tree building and leaf navigation helpers.
  - `applyProposal` happy path and invalid-op path.
  - `useCompanion.stripProposalBlock` and query/proposal stream hiding.
  - `commitGenerated` insert/replace/replaceAll behavior with a small editor test double.
- [ ] Add component smoke tests:
  - `CommandPalette` search/disabled command/keyboard selection.
  - `AIPanel` empty prompt guard, run shortcut, accept shortcut.
  - `CompanionPanel` streaming/thinking/proposal rendering.
- [ ] Add `.github/workflows/ci.yml`.
  - Use pnpm cache, Go cache, Cargo cache.
  - Run `make test`.
  - Upload frontend build artifact only on failure if useful.

### Checkpoint

**Automated:**

- [ ] `make test` passes locally.
- [ ] `cd apps/desktop && pnpm test` passes.
- [ ] GitHub Actions passes on the branch.

**Manual:**

- [ ] Confirm CI failure output is readable when one frontend test is intentionally broken and reverted.

**Stop for user confirmation before Phase 2.**

## Phase 2: Desktop Runtime Resilience

**Outcome:** Engine startup, engine crash, JSONRPC timeout, and notification forwarding have explicit behavior and user-facing diagnostics.

### Tasks

- [ ] Add Rust-side engine status state.
  - In `apps/desktop/src-tauri/src/lib.rs`, keep startup error in managed state instead of only logging it.
  - Add `engine_status` command returning `{ ok, error, version? }`.
- [ ] Add Go engine metadata RPC.
  - Add `version` or `diagnostics.version` handler returning engine version, db path, home dir, and migration status.
  - Keep it side-effect-free.
- [ ] Harden JSONRPC client.
  - Add per-call timeout to `jsonrpc::Client::call`.
  - On stdout EOF, drain pending calls with a structured error.
  - Keep notification forwarding behavior for `ai.*` and `companion.*`.
- [ ] Add frontend startup gate.
  - Add `EngineGate` around routes.
  - Show a friendly diagnostic screen when the engine is unavailable.
  - Include "Retry", "Copy diagnostics", and "Open data folder" when available.
- [ ] Add tests.
  - Rust unit tests for JSONRPC response routing, timeout, EOF drain, and notification handler.
  - Frontend tests for EngineGate ok/error/retry states.

### Checkpoint

**Automated:**

- [ ] `make test` passes.
- [ ] A Rust test proves pending JSONRPC calls fail when the reader exits.
- [ ] A frontend test proves engine startup errors do not render Library as if the app loaded normally.

**Manual:**

- [ ] Temporarily rename `apps/desktop/src-tauri/binaries/linetta-engine-<triple>` and run the app; confirm the diagnostic screen appears.

**Stop for user confirmation before Phase 3.**

## Phase 3: Data Integrity Hardening

**Outcome:** The engine rejects impossible state at RPC/repo boundaries and migrations encode durable invariants where practical.

### Tasks

- [ ] Add validation helpers in engine domain packages.
  - `node.ValidKind`, `node.ValidStatus`.
  - `project.ValidLengthTarget`, `project.ValidDefaultPOV`.
  - `snapshot.ValidReason`.
- [ ] Write failing tests first for invalid inputs:
  - `nodes.create_sibling` rejects unknown kind.
  - `nodes.create_child` rejects unknown kind.
  - `nodes.update_content` rejects container nodes.
  - `nodes.set_last_opened` rejects a node from a different project.
  - `projects.create` rejects unknown length/POV values.
- [ ] Implement repo/handler validation.
  - Return `rpc.CodeInvalidParams` for user/input errors.
  - Keep internal DB errors as internal errors.
- [ ] Add migration for constraints where safe.
  - If SQLite cannot add a `CHECK` constraint in place, document which invariants remain application-enforced.
  - Add indexes needed by new validation queries.
- [ ] Add import/restore regression tests.
  - Import cannot create invalid node kinds.
  - Restore recomputes mentions/word count consistently.
  - Delete/move operations keep sibling ordinals coherent.

### Checkpoint

**Automated:**

- [ ] `make test` passes.
- [ ] New invalid-input tests fail before implementation and pass after.
- [ ] Existing DB migration tests cover a fresh DB and an upgraded DB.

**Manual:**

- [ ] Use a small JSONRPC smoke script or Tauri console call to confirm invalid values return user-readable errors.

**Stop for user confirmation before Phase 4.**

## Phase 4: Operations Visibility & Recovery

**Outcome:** Backup, Git sync, summarizer, and companion persistence failures become visible and recoverable without scanning stderr.

### Tasks

- [ ] Add `ops_status` persistence.
  - New migration table with job name, last_started_at, last_finished_at, last_ok, last_error, metadata_json.
  - Package `engine/internal/opsstatus`.
- [ ] Wire background jobs.
  - Backup scheduler records success/error.
  - Git sync records files written, commit, push, error.
  - Summarizer records recent failures and count.
  - Companion persistence TODO is resolved by surfacing or recording persistence errors.
- [ ] Add RPC handlers:
  - `diagnostics.get`
  - `ops_status.get`
  - `ops_status.clear_error`
- [ ] Update Settings UI.
  - Show last backup status and folder.
  - Show last Git sync result.
  - Show summarizer status only when degraded.
- [ ] Add backup recovery flow.
  - Library menu action: reveal backups folder.
  - Optional restore MVP: pick backup DB, validate schema, restore to a new file or replace current DB after confirmation.

### Checkpoint

**Automated:**

- [ ] `make test` passes.
- [ ] Go tests cover ops status persistence and scheduler write paths.
- [ ] Frontend tests cover Settings status rendering.

**Manual:**

- [ ] Force Git sync failure with a repo missing remote and confirm Settings shows the failure.
- [ ] Confirm backup status updates after app startup daily run.

**Stop for user confirmation before Phase 5.**

## Phase 5: Product Readiness Polish

**Outcome:** The app communicates its safety model and is easier to start, recover, and support.

### Tasks

- [ ] Refresh README.
  - Current product summary.
  - Dev commands.
  - Verification commands.
  - Data locations.
  - Backup/Git sync behavior.
  - Troubleshooting engine startup and AI provider issues.
- [ ] Implement Library menu actions.
  - Open data folder.
  - Open backups folder.
  - Import markdown.
  - Settings.
  - About/diagnostics.
- [ ] Add first-run safety checklist.
  - Show where local data lives.
  - Explain daily backups.
  - Offer Git sync setup.
  - Make it dismissible and stored in settings.
- [ ] Reduce bundle warning.
  - Lazy-load non-critical routes and heavy panels.
  - Consider manual chunks for Tiptap/ProseMirror.
  - Add CI bundle budget or a documented accepted threshold.
- [ ] Add one smoke/e2e path if feasible.
  - Use a Tauri-capable test approach or a mocked Vite route test.
  - Scenario: create project -> type text -> save -> snapshot -> export.

### Checkpoint

**Automated:**

- [ ] `make test` passes.
- [ ] Frontend bundle warning is either resolved or converted into an explicit budget.
- [ ] README command examples have been run locally.

**Manual:**

- [ ] Fresh `LINETTA_HOME=$(mktemp -d)` run shows first-run checklist and can create a project.
- [ ] Library menu opens data/backups locations.
- [ ] A writer can understand backup and recovery options without reading source.

## Suggested Commit Boundaries

- `chore: add repo verification and frontend test harness`
- `fix: surface engine startup and jsonrpc failures`
- `fix: enforce core persistence invariants`
- `feat: add operations status and diagnostics`
- `feat: add writing safety onboarding`
- `chore: refresh README and bundle budget`

## Final Definition of Done

- `make test` is the single local verification command.
- CI runs the same verification on PRs.
- Frontend has meaningful tests around editor/AI/companion flows.
- Engine startup/crash/hang states are visible and recoverable.
- Invalid persistence states are rejected before they enter SQLite.
- Backup/Git sync/summarizer status is visible in the UI.
- README explains current app behavior, data safety, and troubleshooting.
