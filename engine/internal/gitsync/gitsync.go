//go:build !mobile

// Package gitsync exports every non-archived project as a markdown file into a
// user-chosen git repo and then runs `git add -A && git commit && git push`.
// Authentication is delegated entirely to whatever the user's shell already
// uses (SSH keys, credential helpers, gh). Disabling is signalled by an empty
// GitSyncDir in settings.
package gitsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/atomicfile"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
)

const (
	defaultCommitTemplate = "Linetta sync {date}"
	gitTimeout            = 60 * time.Second
)

func defaultRunner() CmdRunner {
	return runGitProd
}

// Init creates GitSyncDir if missing and runs `git init -b main` there if it
// is not already a repo. Safe to call repeatedly. Does NOT add a remote — the
// user adds that with `git remote add origin <url>` or `gh repo create`.
func (s *Syncer) Init(ctx context.Context) (InitResult, error) {
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return InitResult{}, fmt.Errorf("settings.Get: %w", err)
	}
	dir := strings.TrimSpace(cfg.GitSyncDir)
	if dir == "" {
		return InitResult{Skipped: true}, nil
	}
	res := InitResult{Dir: dir}
	if _, err := os.Stat(dir); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			res.Error = fmt.Sprintf("stat dir: %v", err)
			return res, nil
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			res.Error = fmt.Sprintf("mkdir dir: %v", err)
			return res, nil
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		res.AlreadyRepo = true
		return res, nil
	}
	run := s.Run
	if run == nil {
		run = runGitProd
	}
	if _, err := run(ctx, dir, "init", "-b", "main"); err != nil {
		res.Error = "git init: " + err.Error()
		return res, nil
	}
	res.Created = true
	return res, nil
}

// RunOnce performs one end-to-end sync cycle: read settings → write each
// non-archived project's markdown into GitSyncDir → git add/status/commit/push.
// A hard Go error is only returned for unrecoverable configuration/IO failures
// before git runs; everything else surfaces via ResultSummary.
func (s *Syncer) RunOnce(ctx context.Context) (summary ResultSummary, err error) {
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	startedAt := nowFn().UnixMilli()
	if s.Ops != nil {
		defer func() {
			finishedAt := nowFn().UnixMilli()
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
			} else if summary.Error != "" {
				errMsg = summary.Error
			}
			_ = s.Ops.Record(ctx, opsstatus.JobGitSync, startedAt, finishedAt, errMsg == "", errMsg, map[string]any{
				"skipped":       summary.Skipped,
				"files_written": summary.FilesWritten,
				"committed":     summary.Committed,
				"pushed":        summary.Pushed,
				"message":       summary.Message,
			})
		}()
	}

	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return ResultSummary{}, fmt.Errorf("settings.Get: %w", err)
	}
	dir := strings.TrimSpace(cfg.GitSyncDir)
	if dir == "" {
		return ResultSummary{Skipped: true}, nil
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return ResultSummary{Error: fmt.Sprintf("git_sync_dir is not a git repo: %v", err)}, nil
	}
	projs, err := s.Projects.List(ctx, project.ListFilter{IncludeArchived: false, Limit: 1000})
	if err != nil {
		return ResultSummary{}, fmt.Errorf("projects.List: %w", err)
	}
	written := 0
	failures := make([]string, 0)
	for _, p := range projs {
		payload, err := export.ExportProject(ctx, s.Projects, s.Nodes, s.Entities, s.Relationships, p.ID, cfg.Language)
		if err != nil {
			msg := fmt.Sprintf("project %s export: %v", p.ID, err)
			fmt.Fprintf(os.Stderr, "gitsync: %s\n", msg)
			failures = append(failures, msg)
			continue
		}
		target := filepath.Join(dir, export.SyncFilename(p.Title, p.ID))
		if err := atomicfile.Write(target, []byte(payload.Markdown), 0o644); err != nil {
			msg := fmt.Sprintf("project %s write: %v", p.ID, err)
			fmt.Fprintf(os.Stderr, "gitsync: %s\n", msg)
			failures = append(failures, msg)
			continue
		}
		written++
	}
	summary = ResultSummary{FilesWritten: written}
	if len(failures) > 0 {
		summary.Error = fmt.Sprintf("%d project(s) failed: %s", len(failures), strings.Join(failures, "; "))
		return summary, nil
	}
	manifest, err := export.BuildSyncManifest(projs, nowFn())
	if err != nil {
		summary.Error = "build sync manifest: " + err.Error()
		return summary, nil
	}
	if err := atomicfile.Write(filepath.Join(dir, export.SyncManifestFilename), manifest, 0o644); err != nil {
		summary.Error = "write sync manifest: " + err.Error()
		return summary, nil
	}
	run := s.Run
	if run == nil {
		run = runGitProd
	}
	// `add -A` stages everything in the directory, including files Linetta did
	// not write. The one we must never stage is an MCP client config: the
	// recommended workflow puts .mcp.json in the synced folder so an agent can
	// find the server, and a literal bearer token committed here would be
	// pushed to the remote. Excluded at the staging call rather than by editing
	// the writer's .gitignore, which is their file to manage.
	addArgs := append([]string{"add", "-A", "--"}, excludeFromStaging...)
	if _, err := run(ctx, dir, addArgs...); err != nil {
		summary.Error = "git add: " + err.Error()
		return summary, nil
	}
	status, err := run(ctx, dir, "status", "--porcelain")
	if err != nil {
		summary.Error = "git status: " + err.Error()
		return summary, nil
	}
	if strings.TrimSpace(status) == "" {
		return summary, nil
	}
	tmpl := cfg.GitSyncCommitTemplate
	if strings.TrimSpace(tmpl) == "" {
		tmpl = defaultCommitTemplate
	}
	msg := strings.ReplaceAll(tmpl, "{date}", nowFn().Format("2006-01-02 15:04"))
	summary.Message = msg
	if _, err := run(ctx, dir, "commit", "-m", msg); err != nil {
		summary.Error = "git commit: " + err.Error()
		return summary, nil
	}
	summary.Committed = true
	if _, err := run(ctx, dir, "push"); err != nil {
		summary.Error = "git push: " + err.Error()
		return summary, nil
	}
	summary.Pushed = true
	return summary, nil
}

// MCPConfigFilename is the client config an agent-assisted writer may drop in
// the synced folder. Named here so the exclusion is greppable from both sides.
const MCPConfigFilename = ".mcp.json"

// excludeFromStaging is the pathspec tail for `git add`: everything under the
// sync dir except files that must never reach the remote.
var excludeFromStaging = []string{".", ":(exclude)" + MCPConfigFilename}

// runGitProd is the production CmdRunner. It runs git from `dir`, captures
// stderr into the returned error, and enforces a per-call timeout.
func runGitProd(ctx context.Context, dir string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("timeout after %s: %s", gitTimeout, strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
