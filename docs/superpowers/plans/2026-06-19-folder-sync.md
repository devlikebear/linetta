# Folder Sync (iCloud / Google Drive 폴더 동기화) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 사용자가 지정한 클라우드 폴더(iCloud Drive / Google Drive / 임의 폴더)에 모든 비-아카이브 프로젝트의 마크다운을 자동(일일)·수동으로 단방향 내보내기. 전 빌드(Developer ID / MAS / Linux / Windows) 지원.

**Architecture:** Git Sync의 `export.ExportProject()` 로직을 재사용해 마크다운을 생성한다. 비-MAS(비샌드박스)에서는 엔진이 대상 폴더에 직접 쓰고 엔진 스케줄러가 일일 구동한다. MAS(샌드박스)에서는 엔진이 컨테이너 staging에만 쓰고(`folder_sync.stage`), Tauri(Rust)가 macOS security-scoped bookmark로 staging→대상 폴더를 복사한 뒤 `folder_sync.report`로 ops를 기록하며, 일일 트리거는 Rust 타이머가 담당한다. 프론트엔드는 빌드를 모르고 항상 Rust 커맨드(`set_folder_sync_dir`, `folder_sync_now`)만 호출한다.

**Tech Stack:** Go(엔진), Rust/Tauri v2, Objective-C(`.m` + `cc` 빌드 브리지로 NSURL bookmark), React/TypeScript(프론트). 설계: `docs/superpowers/specs/2026-06-19-cloud-folder-sync-design.md`.

---

## File Structure

**엔진 (Go):**
- Create `engine/internal/foldersync/foldersync.go` — `Syncer`(export 코어, `RunOnce`/`Stage`/`Report`), `ResultSummary`/`StageResult`/`ReportInput`.
- Create `engine/internal/foldersync/foldersync_test.go` — direct write/stage/report 테스트.
- Modify `engine/internal/opsstatus/opsstatus.go` — `JobFolderSync` 상수 추가.
- Modify `engine/internal/settings/settings.go` — `FolderSyncDir`/`FolderSyncEnabled` (Config/Patch/Set/persist).
- Modify `engine/internal/settings/settings_test.go`(없으면 Create) — patch round-trip.
- Create `engine/internal/rpc/handlers/foldersync.go` — `RunFolderSync`/`StageFolderSync`/`ReportFolderSync`.
- Create `engine/internal/rpc/handlers/foldersync_test.go` — 핸들러 직렬화 테스트.
- Create `engine/cmd/linetta-engine/foldersync_direct.go` (`//go:build !mas`) — setup + 실제 dailySyncer.
- Create `engine/cmd/linetta-engine/foldersync_staged.go` (`//go:build mas`) — setup + no-op dailySyncer.
- Modify `engine/cmd/linetta-engine/main.go` — `setupFolderSync` 호출 + retentionFn 다중 syncer.

**Tauri (Rust):**
- Modify `apps/desktop/src-tauri/Cargo.toml` — feature `mas`, build-dep `cc`.
- Modify `apps/desktop/src-tauri/build.rs` — MAS+macOS일 때 `.m` 컴파일 + Foundation 링크.
- Create `apps/desktop/src-tauri/macos/bookmarks.m` — NSURL security-scoped bookmark C 브리지.
- Create `apps/desktop/src-tauri/src/macos_bookmarks.rs` — extern "C" + 안전 래퍼.
- Create `apps/desktop/src-tauri/src/folder_sync.rs` — 복사 헬퍼, 커맨드, (MAS) 오케스트레이션 + 타이머.
- Modify `apps/desktop/src-tauri/src/lib.rs` — 모듈 선언, 커맨드 등록, (MAS) 타이머 spawn.
- Modify `apps/desktop/src-tauri/entitlements/linetta-mas.entitlements` — `bookmarks.app-scope`.
- Modify `apps/desktop/src-tauri/tauri.mas.conf.json` — 엔타이틀먼트 경로 버그 수정.
- Modify `scripts/release-mas-local.sh` — `--features mas`.

**프론트엔드:**
- Modify `apps/desktop/src/lib/types.ts` — `FolderSyncResult`, Settings/Patch 필드.
- Modify `apps/desktop/src/lib/rpc.ts` — `folderSyncNow`/`setFolderSyncDir`.
- Modify `apps/desktop/src/routes/Settings.tsx` — Folder Sync 섹션 + 상태 + i18n 키.
- Modify i18n 파일(예: `apps/desktop/src/lib/i18n/*`) — `settings.folder.*` 키.

---

## Task 1: 엔진 settings 필드 추가

**Files:**
- Modify: `engine/internal/settings/settings.go`
- Test: `engine/internal/settings/settings_test.go`

- [ ] **Step 1: 실패 테스트 작성**

`engine/internal/settings/settings_test.go`에 추가(파일 없으면 생성, package `settings`):

```go
package settings

import (
	"context"
	"testing"
)

func TestFolderSyncPatchRoundTrip(t *testing.T) {
	t.Setenv("LINETTA_HOME", t.TempDir())
	st, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	dir := "/tmp/linetta-folder-sync"
	enabled := true
	if _, err := st.Set(ctx, Patch{FolderSyncDir: &dir, FolderSyncEnabled: &enabled}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	cfg, err := st.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cfg.FolderSyncDir != dir {
		t.Errorf("FolderSyncDir = %q, want %q", cfg.FolderSyncDir, dir)
	}
	if !cfg.FolderSyncEnabled {
		t.Errorf("FolderSyncEnabled = false, want true")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/settings/ -run TestFolderSyncPatchRoundTrip`
Expected: FAIL (compile error: `FolderSyncDir`/`FolderSyncEnabled` undefined).

- [ ] **Step 3: Config 구조체에 필드 추가**

`settings.go`의 `Config` 구조체에서 `GitSyncCommitTemplate` 줄 다음에 추가:

```go
	FolderSyncDir             string                    `json:"folder_sync_dir"`
	FolderSyncEnabled         bool                      `json:"folder_sync_enabled"`
```

- [ ] **Step 4: Patch 구조체에 필드 추가**

`settings.go`의 `Patch` 구조체에서 `GitSyncCommitTemplate` 줄 다음에 추가:

```go
	FolderSyncDir             *string                   `json:"folder_sync_dir,omitempty"`
	FolderSyncEnabled         *bool                     `json:"folder_sync_enabled,omitempty"`
```

- [ ] **Step 5: Set() 적용 로직 추가**

`settings.go`의 `Set()` 메서드에서 `GitSyncCommitTemplate` 적용 블록 다음에 추가:

```go
	if p.FolderSyncDir != nil {
		next.FolderSyncDir = *p.FolderSyncDir
	}
	if p.FolderSyncEnabled != nil {
		next.FolderSyncEnabled = *p.FolderSyncEnabled
	}
```

- [ ] **Step 6: persist 포함 확인**

`settings.go`에서 디스크 직렬화용 구조체(`persistable`/`persist()` 등 `json.MarshalIndent`로 쓰는 곳)가 `Config`를 통째로 쓰면 추가 작업 불필요. 별도 필드 나열 구조체라면 위 두 필드(`folder_sync_dir`, `folder_sync_enabled`)를 동일하게 추가한다. (확인: `git_sync_dir`가 나열돼 있으면 같은 자리에 추가.)

- [ ] **Step 7: 통과 확인**

Run: `cd engine && go test ./internal/settings/ -run TestFolderSyncPatchRoundTrip`
Expected: PASS

- [ ] **Step 8: 커밋**

```bash
git add engine/internal/settings/settings.go engine/internal/settings/settings_test.go
git commit -m "feat(engine): add folder sync settings fields"
```

---

## Task 2: foldersync 패키지 코어 + opsstatus 상수

**Files:**
- Modify: `engine/internal/opsstatus/opsstatus.go`
- Create: `engine/internal/foldersync/foldersync.go`
- Test: `engine/internal/foldersync/foldersync_test.go`

- [ ] **Step 1: opsstatus 상수 추가**

`engine/internal/opsstatus/opsstatus.go`의 const 블록(`JobGitSync` 옆)에 추가:

```go
	JobFolderSync           = "folder_sync"
```

- [ ] **Step 2: 실패 테스트 작성**

`engine/internal/foldersync/foldersync_test.go` 생성:

```go
package foldersync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newFixture(t *testing.T) (*Syncer, *settings.Store, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)

	st, err := settings.New()
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	projects := project.NewRepo(db)
	if _, err := projects.Create(context.Background(), 1, project.NewInput{
		Title: "Quiet City", Genres: []string{"literary"}, LengthTarget: "short", DefaultPOV: "first",
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	fixed := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	s := &Syncer{
		Settings: st, Projects: projects, Nodes: node.NewRepo(db),
		Entities: entity.NewRepo(db), Relationships: relationship.NewRepo(db),
		Now: func() time.Time { return fixed }, Ops: opsstatus.NewRepo(db),
	}
	return s, st, home
}

func TestRunOnceWritesMarkdown(t *testing.T) {
	s, st, _ := newFixture(t)
	target := t.TempDir()
	enabled := true
	if _, err := st.Set(context.Background(), settings.Patch{FolderSyncDir: &target, FolderSyncEnabled: &enabled}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if res.FilesWritten != 1 {
		t.Fatalf("FilesWritten = %d, want 1", res.FilesWritten)
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 1 {
		t.Fatalf("target has %d files, want 1", len(entries))
	}
}

func TestRunOnceSkipsWhenDisabled(t *testing.T) {
	s, st, _ := newFixture(t)
	target := t.TempDir()
	if _, err := st.Set(context.Background(), settings.Patch{FolderSyncDir: &target}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !res.Skipped {
		t.Fatalf("expected skipped when disabled")
	}
}

func TestStageWritesToContainer(t *testing.T) {
	s, st, home := newFixture(t)
	target := t.TempDir()
	enabled := true
	if _, err := st.Set(context.Background(), settings.Patch{FolderSyncDir: &target, FolderSyncEnabled: &enabled}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.Stage(context.Background())
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if len(res.Files) != 1 {
		t.Fatalf("Files = %d, want 1", len(res.Files))
	}
	if filepath.Dir(res.StagingDir) != home {
		t.Fatalf("staging dir %q not under home %q", res.StagingDir, home)
	}
	if _, err := os.Stat(filepath.Join(res.StagingDir, res.Files[0])); err != nil {
		t.Fatalf("staged file missing: %v", err)
	}
}
```

- [ ] **Step 3: 실패 확인**

Run: `cd engine && go test ./internal/foldersync/`
Expected: FAIL (package/types undefined).

- [ ] **Step 4: foldersync.go 구현**

`engine/internal/foldersync/foldersync.go` 생성 (모듈 경로는 기존 import 경로 prefix에 맞춰 조정; gitsync.go의 import prefix를 그대로 따른다):

```go
package foldersync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// ResultSummary is the wire shape returned by folder_sync.run.
type ResultSummary struct {
	Skipped      bool   `json:"skipped"`
	FilesWritten int    `json:"files_written"`
	FilesCopied  int    `json:"files_copied"`
	Message      string `json:"message"`
	Error        string `json:"error"`
}

// StageResult is returned by folder_sync.stage (MAS): files exported to a
// container staging dir for Tauri to copy into the user-selected cloud folder.
type StageResult struct {
	Skipped    bool     `json:"skipped"`
	StagingDir string   `json:"staging_dir"`
	Files      []string `json:"files"`
}

// ReportInput is sent by Tauri (MAS) after the privileged copy completes.
type ReportInput struct {
	StartedAt   int64  `json:"started_at"`
	FinishedAt  int64  `json:"finished_at"`
	Ok          bool   `json:"ok"`
	FilesCopied int    `json:"files_copied"`
	Error       string `json:"error"`
}

// Syncer exports projects as markdown into a target folder. Mirrors gitsync.Syncer.
type Syncer struct {
	Settings      *settings.Store
	Projects      *project.Repo
	Nodes         *node.Repo
	Entities      *entity.Repo
	Relationships *relationship.Repo
	Now           func() time.Time
	Ops           *opsstatus.Repo
}

// New builds a Syncer with production defaults.
func New(s *settings.Store, p *project.Repo, n *node.Repo, e *entity.Repo, r *relationship.Repo) *Syncer {
	return &Syncer{Settings: s, Projects: p, Nodes: n, Entities: e, Relationships: r, Now: time.Now}
}

// exportAll writes every non-archived project's markdown into destDir.
func (s *Syncer) exportAll(ctx context.Context, destDir string) (int, error) {
	projs, err := s.Projects.List(ctx, project.ListFilter{IncludeArchived: false, Limit: 1000})
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return 0, err
	}
	written := 0
	for _, p := range projs {
		payload, err := export.ExportProject(ctx, s.Projects, s.Nodes, s.Entities, s.Relationships, p.ID)
		if err != nil {
			return written, err
		}
		dest := filepath.Join(destDir, payload.SuggestedFilename)
		if err := os.WriteFile(dest, []byte(payload.Markdown), 0o644); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// RunOnce writes directly to FolderSyncDir (non-MAS) and records ops status.
func (s *Syncer) RunOnce(ctx context.Context) (summary ResultSummary, err error) {
	startedAt := s.Now().UnixMilli()
	defer func() {
		if s.Ops == nil {
			return
		}
		_ = s.Ops.Record(ctx, opsstatus.JobFolderSync, startedAt, s.Now().UnixMilli(),
			summary.Error == "", summary.Error, map[string]any{"files_written": summary.FilesWritten})
	}()
	cfg, gerr := s.Settings.Get(ctx)
	if gerr != nil {
		summary.Error = gerr.Error()
		return summary, nil
	}
	if !cfg.FolderSyncEnabled || strings.TrimSpace(cfg.FolderSyncDir) == "" {
		summary.Skipped = true
		return summary, nil
	}
	n, werr := s.exportAll(ctx, cfg.FolderSyncDir)
	summary.FilesWritten = n
	summary.FilesCopied = n
	if werr != nil {
		summary.Error = werr.Error()
	}
	return summary, nil
}

// Stage exports to a container staging dir (MAS) and returns the file list.
func (s *Syncer) Stage(ctx context.Context) (StageResult, error) {
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return StageResult{}, err
	}
	if !cfg.FolderSyncEnabled || strings.TrimSpace(cfg.FolderSyncDir) == "" {
		return StageResult{Skipped: true}, nil
	}
	home, err := paths.Home()
	if err != nil {
		return StageResult{}, err
	}
	staging := filepath.Join(home, "folder-sync-staging")
	if err := os.RemoveAll(staging); err != nil {
		return StageResult{}, err
	}
	if _, err := s.exportAll(ctx, staging); err != nil {
		return StageResult{}, err
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return StageResult{}, err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	return StageResult{StagingDir: staging, Files: files}, nil
}

// Report records ops status after Tauri completes the MAS copy.
func (s *Syncer) Report(ctx context.Context, in ReportInput) error {
	if s.Ops == nil {
		return nil
	}
	return s.Ops.Record(ctx, opsstatus.JobFolderSync, in.StartedAt, in.FinishedAt,
		in.Ok, in.Error, map[string]any{"files_copied": in.FilesCopied})
}
```

> 주의: `paths.Home()`의 실제 시그니처를 확인하라. `(string, error)`가 아니면(예: 단일 반환) 위 호출을 맞춰 조정한다. `project.List`/`project.ListFilter`/`export.ExportProject`는 `engine/internal/gitsync/gitsync.go`에서 쓰는 형태와 동일하다.

- [ ] **Step 5: 통과 확인**

Run: `cd engine && go test ./internal/foldersync/ ./internal/opsstatus/`
Expected: PASS (3 foldersync 테스트 통과)

- [ ] **Step 6: 커밋**

```bash
git add engine/internal/foldersync/ engine/internal/opsstatus/opsstatus.go
git commit -m "feat(engine): add foldersync package (export/stage/report)"
```

---

## Task 3: foldersync RPC 핸들러

**Files:**
- Create: `engine/internal/rpc/handlers/foldersync.go`
- Test: `engine/internal/rpc/handlers/foldersync_test.go`

- [ ] **Step 1: 실패 테스트 작성**

`engine/internal/rpc/handlers/foldersync_test.go` 생성:

```go
package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/foldersync"
)

func TestRunFolderSyncSerializes(t *testing.T) {
	// Syncer with nil deps: RunOnce reads settings first; use a Syncer whose
	// Settings is nil would panic, so we only assert the handler marshals a
	// ResultSummary shape via a stubbed Stage path is covered elsewhere.
	// Here we verify ReportFolderSync accepts params and returns ok.
	s := &foldersync.Syncer{}
	h := ReportFolderSync(s)
	out, err := h(context.Background(), json.RawMessage(`{"ok":true,"files_copied":2}`))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var got map[string]bool
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got["ok"] {
		t.Fatalf("expected ok=true, got %v", got)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/rpc/handlers/ -run TestRunFolderSyncSerializes`
Expected: FAIL (ReportFolderSync undefined).

- [ ] **Step 3: 핸들러 구현**

`engine/internal/rpc/handlers/foldersync.go` 생성:

```go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/foldersync"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// RunFolderSync exports directly to the configured folder (non-MAS builds).
func RunFolderSync(s *foldersync.Syncer) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		res, err := s.RunOnce(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(res)
	}
}

// StageFolderSync exports to a container staging dir (MAS builds).
func StageFolderSync(s *foldersync.Syncer) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		res, err := s.Stage(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(res)
	}
}

// ReportFolderSync records ops status after Tauri completes the MAS copy.
func ReportFolderSync(s *foldersync.Syncer) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in foldersync.ReportInput
		if len(params) > 0 {
			if err := json.Unmarshal(params, &in); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
			}
		}
		if err := s.Report(ctx, in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]bool{"ok": true})
	}
}
```

> `s.Report`는 `Ops == nil`이면 nil을 반환하므로 위 테스트(빈 Syncer)에서 안전하다.

- [ ] **Step 4: 통과 확인**

Run: `cd engine && go test ./internal/rpc/handlers/ -run TestRunFolderSyncSerializes`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add engine/internal/rpc/handlers/foldersync.go engine/internal/rpc/handlers/foldersync_test.go
git commit -m "feat(engine): add folder sync RPC handlers"
```

---

## Task 4: 빌드 태그 setup + 스케줄러 배선

**Files:**
- Create: `engine/cmd/linetta-engine/foldersync_direct.go` (`//go:build !mas`)
- Create: `engine/cmd/linetta-engine/foldersync_staged.go` (`//go:build mas`)
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: 비-MAS setup 작성**

`engine/cmd/linetta-engine/foldersync_direct.go` 생성 (gitsync_enabled.go의 import 목록을 그대로 참고):

```go
//go:build !mas

package main

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/foldersync"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

func setupFolderSync(
	s *rpc.Server,
	settingsStore *settings.Store,
	projects *project.Repo,
	nodes *node.Repo,
	entities *entity.Repo,
	relationships *relationship.Repo,
	ops *opsstatus.Repo,
) dailySyncer {
	syncer := foldersync.New(settingsStore, projects, nodes, entities, relationships)
	syncer.Ops = ops
	s.Handle("folder_sync.run", handlers.RunFolderSync(syncer))
	return realFolderSyncer{syncer}
}

type realFolderSyncer struct{ s *foldersync.Syncer }

func (r realFolderSyncer) RunOnce(ctx context.Context) (syncResult, error) {
	res, err := r.s.RunOnce(ctx)
	return syncResult{Error: res.Error}, err
}
```

- [ ] **Step 2: MAS setup 작성**

`engine/cmd/linetta-engine/foldersync_staged.go` 생성:

```go
//go:build mas

package main

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/foldersync"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

func setupFolderSync(
	s *rpc.Server,
	settingsStore *settings.Store,
	projects *project.Repo,
	nodes *node.Repo,
	entities *entity.Repo,
	relationships *relationship.Repo,
	ops *opsstatus.Repo,
) dailySyncer {
	syncer := foldersync.New(settingsStore, projects, nodes, entities, relationships)
	syncer.Ops = ops
	// MAS: Tauri owns the privileged copy + the daily timer.
	s.Handle("folder_sync.stage", handlers.StageFolderSync(syncer))
	s.Handle("folder_sync.report", handlers.ReportFolderSync(syncer))
	return noopFolderSyncer{}
}

type noopFolderSyncer struct{}

func (noopFolderSyncer) RunOnce(context.Context) (syncResult, error) {
	return syncResult{}, nil
}
```

- [ ] **Step 3: main.go retentionFn을 다중 syncer로 변경**

`engine/cmd/linetta-engine/main.go`에서 기존 `syncer := setupGitSync(...)`와 `retentionFn` 블록을 다음으로 교체:

```go
	gitSyncer := setupGitSync(s, settingsStore, projects, nodes, entities, relationships, ops)
	folderSyncer := setupFolderSync(s, settingsStore, projects, nodes, entities, relationships, ops)
	syncers := []dailySyncer{gitSyncer, folderSyncer}
	retentionFn := func(ctx context.Context) error {
		if err := snapshot.Thin(ctx, st.DB(), time.Now().UnixMilli()); err != nil {
			return err
		}
		for _, sy := range syncers {
			res, err := sy.RunOnce(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "daily sync: %v\n", err)
				continue
			}
			if res.Error != "" {
				fmt.Fprintf(os.Stderr, "daily sync: %s\n", res.Error)
			}
		}
		return nil
	}
```

> 변수명(`st`, `snapshot`, `ops` 등)은 기존 main.go의 것과 일치한다. `setupGitSync`/`backup.Start` 호출은 그대로 둔다.

- [ ] **Step 4: 비-MAS 빌드 + 테스트**

Run: `cd engine && go build ./... && go test ./...`
Expected: PASS (전체 빌드/테스트 통과)

- [ ] **Step 5: MAS 빌드 확인**

Run: `cd engine && go build -tags mas ./...`
Expected: 빌드 성공 (folder_sync.stage/report 등록, no-op syncer).

- [ ] **Step 6: 커밋**

```bash
git add engine/cmd/linetta-engine/foldersync_direct.go engine/cmd/linetta-engine/foldersync_staged.go engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): wire folder sync into scheduler (direct/staged build tags)"
```

---

## Task 5: Cargo feature `mas` + `cc` 빌드 의존성

**Files:**
- Modify: `apps/desktop/src-tauri/Cargo.toml`

- [ ] **Step 1: build-dependency `cc` 추가**

`Cargo.toml`의 `[build-dependencies]` 섹션을 다음으로 변경:

```toml
[build-dependencies]
tauri-build = { version = "2", features = [] }
cc = "1"
```

- [ ] **Step 2: `mas` feature 추가**

`[features]` 섹션을 다음으로 변경:

```toml
[features]
default = ["custom-protocol"]
custom-protocol = ["tauri/custom-protocol"]
mas = []
```

- [ ] **Step 3: 기본 빌드 확인**

Run: `cd apps/desktop/src-tauri && cargo check`
Expected: 성공 (기능 변화 없음).

- [ ] **Step 4: 커밋**

```bash
git add apps/desktop/src-tauri/Cargo.toml
git commit -m "build(desktop): add mas feature and cc build-dependency"
```

---

## Task 6: macOS security-scoped bookmark 브리지 (.m + Rust)

**Files:**
- Create: `apps/desktop/src-tauri/macos/bookmarks.m`
- Create: `apps/desktop/src-tauri/src/macos_bookmarks.rs`
- Modify: `apps/desktop/src-tauri/build.rs`

- [ ] **Step 1: Objective-C 브리지 작성**

`apps/desktop/src-tauri/macos/bookmarks.m` 생성:

```objc
#import <Foundation/Foundation.h>
#import <string.h>
#import <stdlib.h>

// Create a security-scoped bookmark for the directory at `path`.
// Returns malloc'd bytes and sets *out_len; returns NULL on error.
const void *linetta_bookmark_create(const char *path, size_t *out_len) {
    @autoreleasepool {
        if (path == NULL || out_len == NULL) return NULL;
        NSString *p = [NSString stringWithUTF8String:path];
        NSURL *url = [NSURL fileURLWithPath:p isDirectory:YES];
        NSError *err = nil;
        NSData *data = [url bookmarkDataWithOptions:NSURLBookmarkCreationWithSecurityScope
                    includingResourceValuesForKeys:nil
                                     relativeToURL:nil
                                             error:&err];
        if (data == nil) return NULL;
        size_t len = (size_t)[data length];
        void *buf = malloc(len);
        if (buf == NULL) return NULL;
        memcpy(buf, [data bytes], len);
        *out_len = len;
        return buf;
    }
}

// Resolve a bookmark and start security-scoped access.
// Returns a retained NSURL handle and sets *out_path (malloc'd UTF8 path).
// Returns NULL on error.
void *linetta_bookmark_start(const void *data, size_t len, char **out_path) {
    @autoreleasepool {
        if (data == NULL || out_path == NULL) return NULL;
        NSData *d = [NSData dataWithBytes:data length:len];
        BOOL stale = NO;
        NSError *err = nil;
        NSURL *url = [NSURL URLByResolvingBookmarkData:d
                                              options:NSURLBookmarkResolutionWithSecurityScope
                                        relativeToURL:nil
                                  bookmarkDataIsStale:&stale
                                                error:&err];
        if (url == nil) return NULL;
        if (![url startAccessingSecurityScopedResource]) return NULL;
        const char *fsr = [[url path] UTF8String];
        if (fsr == NULL) {
            [url stopAccessingSecurityScopedResource];
            return NULL;
        }
        *out_path = strdup(fsr);
        return (void *)CFBridgingRetain(url);
    }
}

// Stop security-scoped access and release the handle from linetta_bookmark_start.
void linetta_bookmark_stop(void *handle) {
    if (handle == NULL) return;
    NSURL *url = (NSURL *)CFBridgingRelease(handle);
    [url stopAccessingSecurityScopedResource];
}

// Free a buffer returned by create (bytes) or start (out_path).
void linetta_free(void *ptr) {
    if (ptr) free(ptr);
}
```

- [ ] **Step 2: build.rs에서 조건부 컴파일**

`apps/desktop/src-tauri/build.rs`를 다음으로 교체:

```rust
fn main() {
    let is_mas = std::env::var("CARGO_FEATURE_MAS").is_ok();
    let target_os = std::env::var("CARGO_CFG_TARGET_OS").unwrap_or_default();
    if is_mas && target_os == "macos" {
        cc::Build::new()
            .file("macos/bookmarks.m")
            .flag("-fobjc-arc")
            .compile("linetta_bookmarks");
        println!("cargo:rustc-link-lib=framework=Foundation");
        println!("cargo:rerun-if-changed=macos/bookmarks.m");
    }
    tauri_build::build();
}
```

- [ ] **Step 3: Rust 안전 래퍼 작성**

`apps/desktop/src-tauri/src/macos_bookmarks.rs` 생성:

```rust
#![cfg(all(target_os = "macos", feature = "mas"))]

use std::ffi::{CStr, CString};
use std::os::raw::{c_char, c_void};
use std::path::Path;

extern "C" {
    fn linetta_bookmark_create(path: *const c_char, out_len: *mut usize) -> *const c_void;
    fn linetta_bookmark_start(
        data: *const c_void,
        len: usize,
        out_path: *mut *mut c_char,
    ) -> *mut c_void;
    fn linetta_bookmark_stop(handle: *mut c_void);
    fn linetta_free(ptr: *mut c_void);
}

/// Create a persistable security-scoped bookmark for a folder the app currently
/// has access to (e.g. just selected via the open panel).
pub fn create_bookmark(path: &Path) -> Result<Vec<u8>, String> {
    let c = CString::new(path.to_string_lossy().as_bytes()).map_err(|e| e.to_string())?;
    let mut len: usize = 0;
    let ptr = unsafe { linetta_bookmark_create(c.as_ptr(), &mut len as *mut usize) };
    if ptr.is_null() {
        return Err("failed to create security-scoped bookmark".into());
    }
    let bytes = unsafe { std::slice::from_raw_parts(ptr as *const u8, len) }.to_vec();
    unsafe { linetta_free(ptr as *mut c_void) };
    Ok(bytes)
}

/// Resolve `bookmark`, start security-scoped access, run `f` with the resolved
/// path, then always stop access. Outer Err = bookmark/access failure; inner
/// Result is whatever `f` returns.
pub fn with_scoped_access<T, F>(bookmark: &[u8], f: F) -> Result<Result<T, String>, String>
where
    F: FnOnce(&Path) -> Result<T, String>,
{
    let mut out_path: *mut c_char = std::ptr::null_mut();
    let handle = unsafe {
        linetta_bookmark_start(
            bookmark.as_ptr() as *const c_void,
            bookmark.len(),
            &mut out_path as *mut *mut c_char,
        )
    };
    if handle.is_null() {
        return Err("폴더 접근 권한을 잃었습니다. 다시 선택하세요".into());
    }
    let path = unsafe { CStr::from_ptr(out_path) }.to_string_lossy().into_owned();
    unsafe { linetta_free(out_path as *mut c_void) };
    let result = f(Path::new(&path));
    unsafe { linetta_bookmark_stop(handle) };
    Ok(result)
}
```

- [ ] **Step 4: MAS feature 컴파일 확인**

Run: `cd apps/desktop/src-tauri && cargo check --features mas`
Expected: 컴파일 성공(`.m`이 cc로 빌드되고 Foundation 링크). 경고는 허용.

> 이 태스크는 본 계획에서 가장 위험도가 높다. objc API 호출이 컴파일/링크되는지 위 명령으로 확인하고, 실제 동작(권한 유지)은 Task 12 이후 실제 MAS 빌드에서 수동 QA한다. `lib.rs`에 모듈을 선언하기 전(Task 8)에는 `cargo check --features mas`가 이 파일을 컴파일하지 않을 수 있으니, Task 8에서 `mod macos_bookmarks;`를 추가한 뒤 다시 확인한다.

- [ ] **Step 5: 커밋**

```bash
git add apps/desktop/src-tauri/macos/bookmarks.m apps/desktop/src-tauri/src/macos_bookmarks.rs apps/desktop/src-tauri/build.rs
git commit -m "feat(desktop): add macOS security-scoped bookmark bridge"
```

---

## Task 7: folder_sync.rs — 복사 헬퍼 · 커맨드 · (MAS) 오케스트레이션

**Files:**
- Create: `apps/desktop/src-tauri/src/folder_sync.rs`

> 이 파일은 `EngineState`/`engine_client`를 참조하므로 Task 8에서 `lib.rs`가 이 모듈을 선언하고 `pub(crate)`로 노출한 뒤 함께 컴파일된다. 본 태스크는 파일 작성 + 복사 헬퍼 단위 테스트까지.

- [ ] **Step 1: 복사 헬퍼 + 테스트 작성**

`apps/desktop/src-tauri/src/folder_sync.rs` 생성:

```rust
use std::path::Path;

use crate::EngineState;

/// Copy each named file from `staging` into `target`. Returns the count copied.
pub(crate) fn copy_files(staging: &Path, target: &Path, files: &[String]) -> Result<usize, String> {
    let mut n = 0usize;
    for f in files {
        let src = staging.join(f);
        let dst = target.join(f);
        std::fs::copy(&src, &dst).map_err(|e| format!("copy {f}: {e}"))?;
        n += 1;
    }
    Ok(n)
}

fn now_millis() -> i64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

/// Persist the chosen folder path in engine settings, and (MAS) create + store a
/// security-scoped bookmark for later unattended access.
#[tauri::command]
pub(crate) async fn set_folder_sync_dir(
    state: tauri::State<'_, EngineState>,
    app: tauri::AppHandle,
    path: String,
) -> Result<(), String> {
    let client = crate::engine_client(&state)?;
    client
        .call("settings.set", Some(serde_json::json!({ "folder_sync_dir": path })))
        .await
        .map_err(|e| e.to_string())?;

    #[cfg(all(target_os = "macos", feature = "mas"))]
    {
        let bookmark = crate::macos_bookmarks::create_bookmark(std::path::Path::new(&path))?;
        let store = bookmark_store_path(&app)?;
        if let Some(parent) = store.parent() {
            std::fs::create_dir_all(parent).map_err(|e| e.to_string())?;
        }
        std::fs::write(&store, &bookmark).map_err(|e| e.to_string())?;
    }
    #[cfg(not(all(target_os = "macos", feature = "mas")))]
    {
        let _ = &app;
    }
    Ok(())
}

/// Run a folder sync now. Non-MAS forwards to the engine; MAS orchestrates the
/// staged copy via the security-scoped bookmark.
#[tauri::command]
pub(crate) async fn folder_sync_now(
    state: tauri::State<'_, EngineState>,
    app: tauri::AppHandle,
) -> Result<serde_json::Value, String> {
    #[cfg(all(target_os = "macos", feature = "mas"))]
    {
        return run_folder_sync_mas(&app, state.inner()).await;
    }
    #[cfg(not(all(target_os = "macos", feature = "mas")))]
    {
        let _ = &app;
        let client = crate::engine_client(&state)?;
        client
            .call("folder_sync.run", None)
            .await
            .map_err(|e| e.to_string())
    }
}

#[cfg(all(target_os = "macos", feature = "mas"))]
fn bookmark_store_path(app: &tauri::AppHandle) -> Result<std::path::PathBuf, String> {
    use tauri::Manager;
    let dir = app.path().app_data_dir().map_err(|e| e.to_string())?;
    Ok(dir.join("folder-sync.bookmark"))
}

/// MAS orchestration: stage in the engine, copy via bookmark, report back.
#[cfg(all(target_os = "macos", feature = "mas"))]
pub(crate) async fn run_folder_sync_mas(
    app: &tauri::AppHandle,
    state: &EngineState,
) -> Result<serde_json::Value, String> {
    let client = crate::engine_client(state)?;
    let started = now_millis();

    let stage = client
        .call("folder_sync.stage", None)
        .await
        .map_err(|e| e.to_string())?;
    if stage.get("skipped").and_then(|v| v.as_bool()).unwrap_or(false) {
        return Ok(serde_json::json!({ "skipped": true, "files_copied": 0, "message": "", "error": "" }));
    }
    let staging = stage
        .get("staging_dir")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    let files: Vec<String> = stage
        .get("files")
        .and_then(|v| v.as_array())
        .map(|a| a.iter().filter_map(|x| x.as_str().map(String::from)).collect())
        .unwrap_or_default();

    let store = bookmark_store_path(app)?;
    let bookmark = std::fs::read(&store)
        .map_err(|_| "폴더 접근 권한을 잃었습니다. 다시 선택하세요".to_string())?;
    let staging_pb = std::path::PathBuf::from(&staging);

    let outcome = crate::macos_bookmarks::with_scoped_access(&bookmark, |target| {
        copy_files(&staging_pb, target, &files)
    });
    let (ok, copied, errmsg) = match outcome {
        Ok(Ok(n)) => (true, n, String::new()),
        Ok(Err(e)) => (false, 0usize, e),
        Err(e) => (false, 0usize, e),
    };

    let _ = client
        .call(
            "folder_sync.report",
            Some(serde_json::json!({
                "started_at": started,
                "finished_at": now_millis(),
                "ok": ok,
                "files_copied": copied,
                "error": errmsg,
            })),
        )
        .await;

    Ok(serde_json::json!({ "skipped": false, "files_copied": copied, "message": "", "error": errmsg }))
}

#[cfg(test)]
mod tests {
    use super::copy_files;

    #[test]
    fn copies_named_files() {
        let staging = tempdir();
        let target = tempdir();
        std::fs::write(staging.join("a.md"), b"hello").unwrap();
        std::fs::write(staging.join("b.md"), b"world").unwrap();
        let files = vec!["a.md".to_string(), "b.md".to_string()];
        let n = copy_files(&staging, &target, &files).unwrap();
        assert_eq!(n, 2);
        assert_eq!(std::fs::read_to_string(target.join("a.md")).unwrap(), "hello");
        assert_eq!(std::fs::read_to_string(target.join("b.md")).unwrap(), "world");
    }

    fn tempdir() -> std::path::PathBuf {
        let mut p = std::env::temp_dir();
        // Unique-ish name without external crates: nanos since epoch + counter.
        use std::sync::atomic::{AtomicU64, Ordering};
        static C: AtomicU64 = AtomicU64::new(0);
        let n = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        p.push(format!("linetta-fs-test-{}-{}", n, C.fetch_add(1, Ordering::Relaxed)));
        std::fs::create_dir_all(&p).unwrap();
        p
    }
}
```

> 이 파일 단독으로는 `crate::EngineState`/`crate::engine_client` 미연결로 컴파일되지 않는다. Task 8에서 `lib.rs`에 모듈을 선언하고 `engine_client`/`EngineState` 가시성을 `pub(crate)`로 맞춘 뒤 빌드한다. 복사 헬퍼 테스트는 Task 8 이후 실행한다.

- [ ] **Step 2: 커밋 (Task 8과 함께 빌드 검증)**

```bash
git add apps/desktop/src-tauri/src/folder_sync.rs
git commit -m "feat(desktop): add folder_sync module (copy/commands/MAS orchestration)"
```

---

## Task 8: lib.rs 배선 (모듈·커맨드·가시성·MAS 타이머)

**Files:**
- Modify: `apps/desktop/src-tauri/src/lib.rs`

- [ ] **Step 1: 모듈 선언 추가**

`lib.rs` 상단의 다른 `mod` 선언들 근처에 추가:

```rust
mod folder_sync;
#[cfg(all(target_os = "macos", feature = "mas"))]
mod macos_bookmarks;
```

- [ ] **Step 2: EngineState / engine_client 가시성 조정**

`folder_sync.rs`가 참조할 수 있도록 `EngineState` 구조체와 `engine_client` 함수의 가시성을 `pub(crate)`로 변경:

```rust
pub(crate) struct EngineState {
    pub(crate) client: Option<Arc<jsonrpc::Client>>,
    pub(crate) startup_error: Option<String>,
    pub(crate) _engine: Option<Arc<engine::EngineHandle>>,
}
```

```rust
pub(crate) fn engine_client(state: &EngineState) -> Result<Arc<jsonrpc::Client>, String> {
    state.client.clone().ok_or_else(|| {
        state
            .startup_error
            .clone()
            .unwrap_or_else(|| "engine unavailable".to_string())
    })
}
```

- [ ] **Step 3: 커맨드 등록**

`invoke_handler`의 `generate_handler!` 목록에 추가:

```rust
        .invoke_handler(tauri::generate_handler![
            engine_ping,
            engine_call,
            engine_status,
            open_path,
            folder_sync::set_folder_sync_dir,
            folder_sync::folder_sync_now
        ])
```

- [ ] **Step 4: MAS 일일 타이머 spawn**

`setup(|app| { ... })` 클로저 안, `handle.manage(state);` 다음에 추가:

```rust
            #[cfg(all(target_os = "macos", feature = "mas"))]
            {
                let timer_handle = app.handle().clone();
                tauri::async_runtime::spawn(async move {
                    use tauri::Manager;
                    // Let the engine settle after launch, then run daily.
                    tokio::time::sleep(std::time::Duration::from_secs(30)).await;
                    loop {
                        let state = timer_handle.state::<EngineState>();
                        let _ = folder_sync::run_folder_sync_mas(&timer_handle, state.inner()).await;
                        tokio::time::sleep(std::time::Duration::from_secs(86_400)).await;
                    }
                });
            }
```

- [ ] **Step 5: 기본 빌드 + folder_sync 테스트**

Run: `cd apps/desktop/src-tauri && cargo test folder_sync`
Expected: PASS (`copies_named_files` 통과). 그리고 `cargo check`도 성공.

- [ ] **Step 6: MAS feature 빌드 확인**

Run: `cd apps/desktop/src-tauri && cargo check --features mas`
Expected: 성공 (bookmark 모듈 + 타이머 + MAS 오케스트레이션 컴파일).

- [ ] **Step 7: 커밋**

```bash
git add apps/desktop/src-tauri/src/lib.rs apps/desktop/src-tauri/src/folder_sync.rs
git commit -m "feat(desktop): register folder sync commands and MAS daily timer"
```

---

## Task 9: 엔타이틀먼트 · MAS conf · 릴리스 스크립트

**Files:**
- Modify: `apps/desktop/src-tauri/entitlements/linetta-mas.entitlements`
- Modify: `apps/desktop/src-tauri/tauri.mas.conf.json`
- Modify: `scripts/release-mas-local.sh`

- [ ] **Step 1: bookmarks.app-scope 엔타이틀먼트 추가**

`linetta-mas.entitlements`의 `files.user-selected.read-write` 다음에 추가:

```xml
	<key>com.apple.security.files.bookmarks.app-scope</key>
	<true/>
```

- [ ] **Step 2: MAS conf 엔타이틀먼트 경로 수정**

`tauri.mas.conf.json`의 `entitlements` 값을 수정 (버그 수정: DevID용 → MAS용):

```json
{
  "$schema": "https://schema.tauri.app/config/2",
  "bundle": {
    "category": "Productivity",
    "macOS": {
      "entitlements": "entitlements/linetta-mas.entitlements",
      "minimumSystemVersion": "12.0"
    }
  }
}
```

- [ ] **Step 3: 릴리스 스크립트에 `--features mas` 추가**

`scripts/release-mas-local.sh`의 빌드 줄을 수정:

```bash
pnpm tauri build --config src-tauri/tauri.mas.conf.json --bundles app --features mas
```

- [ ] **Step 4: 커밋**

```bash
git add apps/desktop/src-tauri/entitlements/linetta-mas.entitlements apps/desktop/src-tauri/tauri.mas.conf.json scripts/release-mas-local.sh
git commit -m "build(desktop): MAS entitlements + features mas for folder sync bookmarks"
```

---

## Task 10: 프론트엔드 타입

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`

- [ ] **Step 1: FolderSyncResult 추가**

`types.ts`의 `GitSyncResult` 정의 다음에 추가:

```typescript
export interface FolderSyncResult {
  skipped: boolean;
  files_copied: number;
  message: string;
  error: string;
}
```

- [ ] **Step 2: Settings 인터페이스에 필드 추가**

`Settings` 인터페이스의 `git_sync_commit_template` 다음에 추가:

```typescript
  folder_sync_dir: string;
  folder_sync_enabled: boolean;
```

- [ ] **Step 3: SettingsPatch 인터페이스에 필드 추가**

`SettingsPatch` 인터페이스의 `git_sync_commit_template?` 다음에 추가:

```typescript
  folder_sync_dir?: string;
  folder_sync_enabled?: boolean;
```

- [ ] **Step 4: 타입 체크**

Run: `cd apps/desktop && pnpm tsc --noEmit`
Expected: 에러 없음 (또는 기존과 동일).

- [ ] **Step 5: 커밋**

```bash
git add apps/desktop/src/lib/types.ts
git commit -m "feat(desktop): add folder sync types"
```

---

## Task 11: 프론트엔드 RPC 래퍼

**Files:**
- Modify: `apps/desktop/src/lib/rpc.ts`

- [ ] **Step 1: invoke 래퍼 추가**

`rpc.ts`의 `openPath` 래퍼 근처에 추가:

```typescript
export async function setFolderSyncDir(path: string): Promise<void> {
  return invoke<void>("set_folder_sync_dir", { path });
}

export async function folderSyncNow(): Promise<FolderSyncResult> {
  return invoke<FolderSyncResult>("folder_sync_now");
}
```

- [ ] **Step 2: FolderSyncResult import 확인**

`rpc.ts` 상단에서 `types`로부터 타입을 import하는 구문에 `FolderSyncResult`를 추가 (기존 `GitSyncResult` import 옆).

- [ ] **Step 3: 타입 체크**

Run: `cd apps/desktop && pnpm tsc --noEmit`
Expected: 에러 없음.

- [ ] **Step 4: 커밋**

```bash
git add apps/desktop/src/lib/rpc.ts
git commit -m "feat(desktop): add folder sync rpc wrappers"
```

---

## Task 12: Settings.tsx — Folder Sync 섹션 + i18n

**Files:**
- Modify: `apps/desktop/src/routes/Settings.tsx`
- Modify: i18n 파일 (예: `apps/desktop/src/lib/i18n/ko.ts`, `en.ts` — 기존 `settings.git.*` 키가 있는 파일)

- [ ] **Step 1: 상수 추가**

`Settings.tsx`의 job 상수(`JOB_GIT_SYNC` 등) 근처에 추가:

```typescript
const JOB_FOLDER_SYNC = "folder_sync";
```

- [ ] **Step 2: import 추가**

`rpc` import에서 `setFolderSyncDir`, `folderSyncNow`를 추가로 가져온다 (기존 `gitSync`/`openDialog` import 옆).

- [ ] **Step 3: 드래프트 상태 추가**

`gitDirDraft` 상태 근처에 추가:

```typescript
  const [folderDirDraft, setFolderDirDraft] = useState("");
```

- [ ] **Step 4: 설정 로드 시 초기화**

설정 로드 `useEffect`의 `setGitDirDraft(s.git_sync_dir);` 다음에 추가:

```typescript
      setFolderDirDraft(s.folder_sync_dir);
```

- [ ] **Step 5: Folder Sync 섹션 JSX 추가**

`Settings.tsx`의 Git Sync `</section>` 닫힘 직후에 추가:

```tsx
<section className="settings-section">
  <h3>{t("settings.folder.title")}</h3>
  <p className="sd">{t("settings.folder.description")}</p>
  <div className="modal-field">
    <label htmlFor="folder-dir">{t("settings.folder.folder")}</label>
    <div className="set-field-row">
      <input
        id="folder-dir"
        type="text"
        value={folderDirDraft}
        readOnly
        placeholder={t("settings.folder.folderPlaceholder")}
      />
      <button
        type="button"
        className="btn ghost sm"
        onClick={async () => {
          const picked = await openDialog({ directory: true, multiple: false });
          if (typeof picked === "string") {
            try {
              await setFolderSyncDir(picked);
              setFolderDirDraft(picked);
              await apply({ folder_sync_dir: picked });
            } catch (e) {
              setError(String(e));
            }
          }
        }}
        disabled={saving}
      >
        {t("settings.folder.pickFolder")}
      </button>
    </div>
  </div>
  <label className="set-toggle">
    <input
      type="checkbox"
      checked={current?.folder_sync_enabled ?? false}
      onChange={(e) => apply({ folder_sync_enabled: e.target.checked })}
      disabled={saving}
    />
    {t("settings.folder.enable")}
  </label>
  <p className="sd">{t("settings.folder.help")}</p>
  <button
    type="button"
    className="btn ghost sm"
    onClick={async () => {
      try {
        const res = await folderSyncNow();
        if (res.error) {
          setError(res.error);
          return;
        }
        setError(null);
        setSavedAt(Date.now());
        await refreshOps();
      } catch (e) {
        setError(String(e));
      }
    }}
    disabled={saving}
  >
    {t("settings.folder.runNow")}
  </button>
  <OpsStatusCard
    title={t("settings.ops.folderStatus")}
    status={opsByJob.get(JOB_FOLDER_SYNC)}
    okText={t("settings.ops.folderOk")}
    idleText={t("settings.ops.noRuns")}
    onClearError={() => clearOpsError(JOB_FOLDER_SYNC)}
    disabled={saving}
    t={t}
    language={language}
  />
</section>
```

> `setFolderSyncDir`가 엔진 settings에도 경로를 저장하지만, `apply({ folder_sync_dir })`를 함께 호출해 화면의 `current` 상태를 즉시 갱신한다(중복 저장은 무해). 입력은 `readOnly` — 경로는 항상 폴더 선택으로만 설정한다(MAS 북마크 정합성). `refreshOps`/`opsByJob`/`clearOpsError`/`apply`는 기존 Settings 컴포넌트의 것을 사용한다.

- [ ] **Step 6: i18n 키 추가**

`settings.git.*` 키가 정의된 모든 언어 파일에 다음 키를 추가한다. 한국어 예:

```typescript
  "settings.folder.title": "폴더 동기화",
  "settings.folder.description": "프로젝트를 마크다운으로 내보내 iCloud Drive·Google Drive 등 클라우드 폴더에 보관합니다.",
  "settings.folder.folder": "대상 폴더",
  "settings.folder.folderPlaceholder": "폴더를 선택하세요",
  "settings.folder.pickFolder": "폴더 선택",
  "settings.folder.enable": "자동 동기화 사용 (매일)",
  "settings.folder.help": "iCloud Drive 또는 Google Drive 동기화 폴더를 선택하면 운영체제가 업로드를 처리합니다.",
  "settings.folder.runNow": "지금 내보내기",
  "settings.ops.folderStatus": "폴더 동기화 상태",
  "settings.ops.folderOk": "마지막 폴더 동기화 성공",
```

영어 예:

```typescript
  "settings.folder.title": "Folder Sync",
  "settings.folder.description": "Export projects as Markdown into a cloud folder such as iCloud Drive or Google Drive.",
  "settings.folder.folder": "Target folder",
  "settings.folder.folderPlaceholder": "Choose a folder",
  "settings.folder.pickFolder": "Choose folder",
  "settings.folder.enable": "Sync automatically (daily)",
  "settings.folder.help": "Pick an iCloud Drive or Google Drive folder; the OS handles the upload.",
  "settings.folder.runNow": "Export now",
  "settings.ops.folderStatus": "Folder sync status",
  "settings.ops.folderOk": "Last folder sync succeeded",
```

> 프로젝트의 다른 언어 파일에도 동일 키를 추가한다(누락 시 키 문자열이 그대로 노출됨).

- [ ] **Step 7: 타입 체크 + 빌드**

Run: `cd apps/desktop && pnpm tsc --noEmit && pnpm build`
Expected: 에러 없음.

- [ ] **Step 8: 커밋**

```bash
git add apps/desktop/src/routes/Settings.tsx apps/desktop/src/lib/i18n/
git commit -m "feat(desktop): add Folder Sync settings section"
```

---

## Task 13: 통합 검증 (비-MAS 수동 QA)

**Files:** 없음 (수동 검증)

- [ ] **Step 1: 개발 빌드 실행 및 폴더 동기화 확인**

Run: `cd apps/desktop && pnpm tauri dev`
수동 확인:
1. Settings → Folder Sync → "폴더 선택"으로 임시 폴더 지정, "자동 동기화 사용" 체크.
2. "지금 내보내기" 클릭 → 대상 폴더에 프로젝트 `.md` 파일이 생성됨.
3. "폴더 동기화 상태" 카드가 성공으로 표시됨.
4. 파일을 지우고 다시 "지금 내보내기" → 재생성(덮어쓰기) 확인.

- [ ] **Step 2: 전체 빌드 스모크**

Run: `cd engine && go build ./... && go build -tags mas ./...`
Run: `cd apps/desktop/src-tauri && cargo check && cargo check --features mas`
Expected: 모두 성공.

- [ ] **Step 3: MAS 수동 QA (실기기, 별도 세션 가능)**

`make release-mas-local`로 MAS 빌드 → 설치 후:
1. Settings에서 iCloud Drive(또는 Google Drive) 폴더 선택.
2. "지금 내보내기" → 해당 클라우드 폴더에 `.md` 생성 확인.
3. 앱 재시작 후 "지금 내보내기" 다시 → 북마크로 접근이 유지되어 재기록됨(권한 재요청 없음).
4. iCloud/GDrive 클라이언트가 업로드하는지 확인.

> MAS 수동 QA는 코드 머지 후 릴리스 검증 단계에서 수행한다. 실패 시 Task 6(bookmark 브리지)을 우선 점검한다.

---

## Self-Review 결과

- **Spec 커버리지**: settings(T1), export 코어/stage/report(T2), 핸들러(T3), 빌드태그·스케줄러(T4), cargo feature(T5), 북마크 브리지(T6), 복사·커맨드·오케스트레이션(T7), lib.rs·타이머(T8), 엔타이틀먼트·conf·릴리스(T9), 타입(T10), rpc(T11), UI·i18n(T12), 검증(T13). spec의 모든 산출물 매핑됨.
- **빌드 분기**: 비-MAS = 엔진 직접 쓰기 + 엔진 스케줄러; MAS = Rust stage→copy→report + Rust 타이머. 두 경로 모두 태스크로 존재.
- **타입 일관성**: `ResultSummary`(files_copied/error) ↔ 프론트 `FolderSyncResult`(files_copied/error), `folder_sync.run`/`stage`/`report` 메서드명, `JobFolderSync="folder_sync"` ↔ `JOB_FOLDER_SYNC` 일치.
- **알려진 검증 의존**: `paths.Home()` 시그니처, settings `Set`/`Get` 시그니처, i18n 파일 경로는 구현 시 실제 코드로 확인(각 태스크에 주석 명시).
