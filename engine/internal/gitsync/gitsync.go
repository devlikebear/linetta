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

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

const (
	defaultCommitTemplate = "Linetta sync {date}"
	gitTimeout            = 60 * time.Second
)

// ResultSummary is the wire shape returned to the UI. Soft errors (no remote,
// auth, network) land in Error; only configuration/IO problems before git runs
// are reported as a Go error from RunOnce.
type ResultSummary struct {
	Skipped      bool   `json:"skipped"`
	FilesWritten int    `json:"files_written"`
	Committed    bool   `json:"committed"`
	Pushed       bool   `json:"pushed"`
	Message      string `json:"message"`
	Error        string `json:"error"`
}

// CmdRunner is the seam used by tests to record/stub `git` invocations.
type CmdRunner func(ctx context.Context, dir string, args ...string) (string, error)

// Syncer composes the dependencies needed to read settings, list projects,
// build markdown payloads and shell out to git.
type Syncer struct {
	Settings      *settings.Store
	Projects      *project.Repo
	Nodes         *node.Repo
	Entities      *entity.Repo
	Relationships *relationship.Repo
	Run           CmdRunner
	Now           func() time.Time
	Ops           *opsstatus.Repo
}

// New constructs a Syncer with the real `git` runner and wall-clock time.
func New(s *settings.Store, p *project.Repo, n *node.Repo, e *entity.Repo, rels ...*relationship.Repo) *Syncer {
	var rr *relationship.Repo
	if len(rels) > 0 {
		rr = rels[0]
	}
	return &Syncer{
		Settings: s, Projects: p, Nodes: n, Entities: e, Relationships: rr,
		Run: runGitProd, Now: time.Now,
	}
}

// InitResult is the structured outcome of one Init call.
type InitResult struct {
	Skipped     bool   `json:"skipped"`      // GitSyncDir was empty
	AlreadyRepo bool   `json:"already_repo"` // dir was already a git repo
	Created     bool   `json:"created"`      // we ran `git init`
	Dir         string `json:"dir"`          // resolved path
	Error       string `json:"error"`
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
	for _, p := range projs {
		payload, err := export.ExportProject(ctx, s.Projects, s.Nodes, s.Entities, s.Relationships, p.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gitsync: export project %s: %v\n", p.ID, err)
			continue
		}
		target := filepath.Join(dir, payload.SuggestedFilename)
		if err := os.WriteFile(target, []byte(payload.Markdown), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "gitsync: write %s: %v\n", target, err)
			continue
		}
		written++
	}
	summary = ResultSummary{FilesWritten: written}
	run := s.Run
	if run == nil {
		run = runGitProd
	}
	if _, err := run(ctx, dir, "add", "-A"); err != nil {
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
