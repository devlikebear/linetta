// Package foldersync exports every non-archived project as a markdown file
// into a user-chosen destination folder. It mirrors the gitsync package but
// skips the git operations — it simply writes files and records ops status.
package foldersync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/atomicfile"
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
	failures := make([]string, 0)
	for _, p := range projs {
		payload, err := export.ExportProject(ctx, s.Projects, s.Nodes, s.Entities, s.Relationships, p.ID)
		if err != nil {
			msg := fmt.Sprintf("project %s export: %v", p.ID, err)
			fmt.Fprintf(os.Stderr, "folder sync: %s\n", msg)
			failures = append(failures, msg)
			continue
		}
		dest := filepath.Join(destDir, export.SyncFilename(p.Title, p.ID))
		if err := atomicfile.Write(dest, []byte(payload.Markdown), 0o644); err != nil {
			msg := fmt.Sprintf("project %s write: %v", p.ID, err)
			fmt.Fprintf(os.Stderr, "folder sync: %s\n", msg)
			failures = append(failures, msg)
			continue
		}
		written++
	}
	if len(failures) > 0 {
		return written, fmt.Errorf("%d project(s) failed: %s", len(failures), strings.Join(failures, "; "))
	}
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	manifest, err := export.BuildSyncManifest(projs, nowFn())
	if err != nil {
		return written, fmt.Errorf("build sync manifest: %w", err)
	}
	if err := atomicfile.Write(filepath.Join(destDir, export.SyncManifestFilename), manifest, 0o644); err != nil {
		return written, fmt.Errorf("write sync manifest: %w", err)
	}
	return written, nil
}

// RunOnce writes directly to FolderSyncDir (non-MAS) and records ops status.
func (s *Syncer) RunOnce(ctx context.Context) (summary ResultSummary, err error) {
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	startedAt := nowFn().UnixMilli()
	if s.Ops != nil {
		defer func() {
			finishedAt := nowFn().UnixMilli()
			errMsg := summary.Error
			_ = s.Ops.Record(ctx, opsstatus.JobFolderSync, startedAt, finishedAt,
				errMsg == "", errMsg, map[string]any{"files_written": summary.FilesWritten})
		}()
	}
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
