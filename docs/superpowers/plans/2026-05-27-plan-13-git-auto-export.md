# Plan 13 — Git Auto-Export (Daily Markdown Sync to a Git Repo)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans or subagent-driven-development.

**Goal:** Linetta auto-exports every non-archived project as `{slug(title)}.md` into a user-chosen local git repo folder, then `git add -A && git commit && git push` — once a day (piggybacked on the existing backup scheduler) and on demand via Cmd+K. Authentication relies entirely on the user's existing git config (SSH keys, gh credential helper, etc.). One repo contains all projects; git itself is the history.

**Scope replacement:** This plan **replaces** the spec §11.2 P3 item "External backup folder (iCloud/Dropbox)" with a more specific git-sync workflow. Snapshot-based local backups (Plan 6) remain untouched and continue working in parallel.

**Architecture:**
- Backend: one new `engine/internal/gitsync/` package holding a `Syncer` with `RunOnce(ctx) (ResultSummary, error)`. Two new fields on `settings.Config`/`Patch`: `git_sync_dir` and `git_sync_commit_template`. One new RPC `git_sync.run`. The existing `backup.Start` scheduler's `retentionFn` callback is wrapped to ALSO call `syncer.RunOnce` after `snapshot.Thin` succeeds.
- Frontend: `Settings.tsx` gains a "GitHub 동기화" section with a folder picker + commit template input. `Workspace.tsx` adds a Cmd+K command `지금 GitHub로 동기화` that calls `gitSync.run()` and shows a differentiated toast.

**Locked decisions:**
1. Export format = the same markdown `export.ExportProject` already produces (slug-based filename).
2. One repo holds ALL non-archived projects; one file per project. Archived projects are NOT pushed (and are NOT auto-deleted from the repo — git history keeps them).
3. Trigger = daily once via existing scheduler + on-demand via Cmd+K.
4. Auth = whatever the user's terminal already uses (SSH key, credential helper, gh). The app never sees a token.
5. Commit message template = `Linetta sync {date}` by default; only `{date}` (formatted `YYYY-MM-DD HH:MM`) is substituted in MVP.
6. `git_sync_dir == ""` means feature disabled (default).
7. No retention/cleanup; git owns the history.
8. Git commands run via `os/exec.CommandContext` with `cmd.Dir = git_sync_dir` (never via `cd`), 60s timeout each.
9. Push failures (no remote, auth, network) return a structured `ResultSummary{Pushed: false, Error: "..."}` — NOT a hard error. The frontend surfaces the message in a toast.
10. Scheduler integration = wrap the existing `retentionFn` (no new goroutine).

---

> Full task content is in this file. For brevity in the in-repo plan we link to the granular task descriptions inline below; the implementer agents will read each task section before starting.

## Tasks

### Task 1: settings.Config gains `git_sync_dir` / `git_sync_commit_template`
- Modify: `engine/internal/settings/settings.go` + `settings_test.go`
- Add both fields to `Config` and `Patch` (pointer in Patch). Mirror `FocusDefault` pattern. Include in `persistable`, in `load`, and in `Set`.
- New test `TestSet_gitSync_persists` (round-trip through disk) and `TestSet_gitSync_emptyMeansDisabled`.
- Commit: `feat(settings): add git_sync_dir and git_sync_commit_template`.

### Task 2: `engine/internal/gitsync/` package
- Create `gitsync.go` and `gitsync_test.go`.
- `ResultSummary{Skipped, FilesWritten, Committed, Pushed, Message, Error}`.
- `Syncer{Settings, Projects, Nodes, Entities, Run CmdRunner, Now func() time.Time}` with `New(...)` factory.
- `RunOnce(ctx)`:
  1. Read settings; if `GitSyncDir==""` → return `{Skipped:true}`.
  2. Stat `{dir}/.git`; on failure return summary with `Error`.
  3. List non-archived projects; for each, `export.ExportProject` → write `{dir}/{Payload.SuggestedFilename}` (overwrite). Count.
  4. Run `git -C dir add -A`; on error → summary.Error.
  5. Run `git status --porcelain`; if empty → return (no commit, not an error).
  6. Build message: substitute `{date}` with `Now().Format("2006-01-02 15:04")` in `GitSyncCommitTemplate` (default `"Linetta sync {date}"` when empty).
  7. `git commit -m msg`; on error → summary.Error.
  8. `git push`; on error → summary.Error, leave `Committed:true`.
- `runGitProd` uses `exec.CommandContext` with 60s timeout, `cmd.Dir = dir`, captures stderr in returned error.
- TDD with mock `CmdRunner` capturing calls; tests cover: skipped-when-empty-dir; error-when-not-a-git-repo; full success path (files written + commit + push); no-op when status empty; push failure captured in summary; default template applied when blank.
- Commit: `feat(gitsync): Syncer.RunOnce writes markdown + git add/commit/push`.

### Task 3: RPC handler + main.go wiring
- Create `engine/internal/rpc/handlers/gitsync.go` (handler returns ResultSummary as JSON; hard errors only on `RunOnce` returning a non-nil error).
- Modify `engine/cmd/linetta-engine/main.go`:
  - Import `"github.com/devlikebear/linetta/engine/internal/gitsync"`.
  - After repos are wired: `syncer := gitsync.New(settingsStore, projects, nodes, entities)`.
  - Wrap existing `retentionFn` so after `snapshot.Thin` it also calls `syncer.RunOnce(ctx)`, logging `res.Error` (if any) to stderr but never returning an error from `retentionFn`.
  - Register `s.Handle("git_sync.run", handlers.RunGitSync(syncer))`.
- Commit: `feat(engine): git_sync.run RPC + daily sync piggyback on backup scheduler`.

### Task 4: TS types + RPC binding + Settings UI
- `apps/desktop/src/lib/types.ts`: extend `Settings`/`SettingsPatch` with `git_sync_dir`, `git_sync_commit_template`. New `GitSyncResult` interface matching engine `ResultSummary`.
- `apps/desktop/src/lib/rpc.ts`: `export const gitSync = { run: () => rpcCall<GitSyncResult>("git_sync.run") }`.
- `apps/desktop/src-tauri/capabilities/default.json`: add `"dialog:allow-open"` permission so the folder picker works.
- `apps/desktop/src/routes/Settings.tsx`:
  - Import `open as openDialog` from `@tauri-apps/plugin-dialog`.
  - New "GitHub 동기화" section between editor and backup sections.
  - Text input for `git_sync_dir` with a "폴더 선택…" button calling `openDialog({ directory: true, multiple: false })`; if a string is returned, `apply({ git_sync_dir: picked })`.
  - Text input for `git_sync_commit_template` with placeholder `Linetta sync {date}`.
  - Hint paragraph: only `{date}` placeholder supported.
- Commit: `feat(settings-ui): GitHub 동기화 폴더/커밋 템플릿 입력`.

### Task 5: Cmd+K "지금 GitHub로 동기화"
- `apps/desktop/src/routes/Workspace.tsx`:
  - Import `gitSync` from `../lib/rpc`.
  - Inside the `commands` useMemo, after the `go-settings` push, add `git-sync-now` in the `프로젝트` section. The `run` calls `await gitSync.run()` and shows a differentiated toast for each of: skipped, no-change, committed-and-pushed, committed-only, error.
- Commit: `feat(workspace): Cmd+K 명령 '지금 GitHub로 동기화'`.

### Task 6: Smoke + tag
1. `./scripts/build-engine.sh && LINETTA_HOME=/tmp/linetta-plan13 ./scripts/dev.sh`.
2. In a separate terminal: `mkdir /tmp/linetta-plan13-repo && cd /tmp/linetta-plan13-repo && git init -b main`; add a GitHub remote via `gh repo create` or manually.
3. In Linetta Settings, set `git_sync_dir` via the folder picker. Create a project and write content.
4. Cmd+K → "지금 GitHub로 동기화" → expect a differentiated toast.
5. `cd /tmp/linetta-plan13-repo && git log --oneline && git ls-tree HEAD` — verify commit and the `{slug}.md` file.
6. Edit + sync again → second commit. Sync with no changes → toast "변경 없음".
7. Negative: set dir to `/tmp` → toast "git_sync_dir is not a git repo"; clear dir → toast "동기화 비활성화".
8. Tag: `git tag plan-13-git-auto-export-done`.

---

## Done conditions
- [ ] `go test ./... -race` green.
- [ ] `pnpm tsc -b && pnpm build` green.
- [ ] All 4 toast variants observed (skipped, no-change, committed+pushed, committed-only, error).
- [ ] `git log` in the chosen repo shows commits with subject `Linetta sync YYYY-MM-DD HH:MM`.
- [ ] `plan-13-git-auto-export-done` tag exists.

## Out of scope
- Slug collisions when two project titles slugify the same.
- Auto-removing `.md` for archived projects.
- Token-based GitHub auth.
- Per-project sync toggles.
- Placeholders other than `{date}`.
- Conflict resolution on push failure.
