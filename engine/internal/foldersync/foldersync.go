// Package foldersync exports every non-archived project as a markdown file
// into a user-chosen destination folder. It mirrors the gitsync package but
// skips the git operations — it simply writes files and records ops status.
package foldersync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	// ImageDirs names every exported work's picture folder, whether or not
	// it holds anything now, so the copy can remove pictures no scene uses.
	ImageDirs []string `json:"image_dirs"`
}

// ReportInput is sent by Tauri (MAS) after the privileged copy completes.
type ReportInput struct {
	StartedAt   int64  `json:"started_at"`
	FinishedAt  int64  `json:"finished_at"`
	Ok          bool   `json:"ok"`
	FilesCopied int    `json:"files_copied"`
	Error       string `json:"error"`
}

// MirrorDirName is the folder under the Linetta home that holds the sealed copy
// on a build that cannot write to the writer's folder itself (MAS).
const MirrorDirName = "folder-mirror"

// sealedDirName is the sealed copy's folder, as vault's folder store lays it out.
const sealedDirName = ".linetta"

// Syncer exports projects as markdown into a target folder. Mirrors gitsync.Syncer.
type Syncer struct {
	Settings      *settings.Store
	Projects      *project.Repo
	Nodes         *node.Repo
	Entities      *entity.Repo
	Relationships *relationship.Repo
	Extras        export.Extras
	// Illustrations supplies the scenes' pictures, written beside each
	// work's file in its export.SyncImageDir. Nil leaves them out.
	Illustrations export.IllustrationSource
	Now           func() time.Time
	Ops           *opsstatus.Repo
}

// New builds a Syncer with production defaults.
func New(s *settings.Store, p *project.Repo, n *node.Repo, e *entity.Repo, r *relationship.Repo) *Syncer {
	return &Syncer{Settings: s, Projects: p, Nodes: n, Entities: e, Relationships: r, Now: time.Now}
}

// exportAll writes every non-archived project's markdown, and its pictures,
// into destDir. It returns the picture folder of every project it wrote.
func (s *Syncer) exportAll(ctx context.Context, destDir, language string) (int, []string, error) {
	projs, err := s.Projects.List(ctx, project.ListFilter{IncludeArchived: false, Limit: 1000})
	if err != nil {
		return 0, nil, err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return 0, nil, err
	}
	written := 0
	imageDirs := []string{}
	failures := make([]string, 0)
	for _, p := range projs {
		imageDir := export.SyncImageDir(p.Title, p.ID)
		payload, err := export.ExportProject(ctx, export.Sources{
			Projects:      s.Projects,
			Nodes:         s.Nodes,
			Entities:      s.Entities,
			Relationships: s.Relationships,
			Extras:        s.Extras,
			Illustrations: s.Illustrations,
			ImageDir:      imageDir,
		}, p.ID, language)
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
		if err := export.WriteImageDir(filepath.Join(destDir, imageDir), payload.Images); err != nil {
			msg := fmt.Sprintf("project %s pictures: %v", p.ID, err)
			fmt.Fprintf(os.Stderr, "folder sync: %s\n", msg)
			failures = append(failures, msg)
			continue
		}
		imageDirs = append(imageDirs, imageDir)
		written++
	}
	if len(failures) > 0 {
		return written, imageDirs, fmt.Errorf("%d project(s) failed: %s", len(failures), strings.Join(failures, "; "))
	}
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	manifest, err := export.BuildSyncManifest(projs, nowFn())
	if err != nil {
		return written, imageDirs, fmt.Errorf("build sync manifest: %w", err)
	}
	if err := atomicfile.Write(filepath.Join(destDir, export.SyncManifestFilename), manifest, 0o644); err != nil {
		return written, imageDirs, fmt.Errorf("write sync manifest: %w", err)
	}
	return written, imageDirs, nil
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
	n, _, werr := s.exportAll(ctx, cfg.FolderSyncDir, cfg.Language)
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
	_, imageDirs, err := s.exportAll(ctx, staging, cfg.Language)
	if err != nil {
		return StageResult{}, err
	}
	sealed, err := sealedFiles(filepath.Join(home, MirrorDirName, sealedDirName))
	if err != nil {
		return StageResult{}, err
	}
	if len(sealed) > 0 {
		if err := os.CopyFS(filepath.Join(staging, sealedDirName), os.DirFS(filepath.Join(home, MirrorDirName, sealedDirName))); err != nil {
			return StageResult{}, err
		}
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return StageResult{}, err
	}
	// Top-level files, then each picture folder's files as "<dir>/<name>" —
	// the only nesting the copy accepts.
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
			continue
		}
		if e.Name() == sealedDirName {
			files = append(files, sealed...)
			continue
		}
		pictures, err := os.ReadDir(filepath.Join(staging, e.Name()))
		if err != nil {
			return StageResult{}, err
		}
		for _, pic := range pictures {
			if !pic.IsDir() {
				files = append(files, e.Name()+"/"+pic.Name())
			}
		}
	}
	return StageResult{StagingDir: staging, Files: files, ImageDirs: imageDirs}, nil
}

// sealedFiles lists the sealed copy's files as "<.linetta>/<path>", the one
// nesting besides a picture folder that the copy accepts.
func sealedFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			rel, err := filepath.Rel(root, file)
			if err != nil {
				return err
			}
			files = append(files, sealedDirName+"/"+filepath.ToSlash(rel))
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return files, err
}

// Report records ops status after Tauri completes the MAS copy.
func (s *Syncer) Report(ctx context.Context, in ReportInput) error {
	if s.Ops == nil {
		return nil
	}
	return s.Ops.Record(ctx, opsstatus.JobFolderSync, in.StartedAt, in.FinishedAt,
		in.Ok, in.Error, map[string]any{"files_copied": in.FilesCopied})
}
