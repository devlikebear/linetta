//go:build !mobile

// Package gitsync exports every non-archived project as a markdown file into a
// user-chosen git repo and exchanges it with the remote: commit what is there,
// take what the remote has, export, commit and push. Authentication is delegated
// entirely to whatever the user's shell already uses (SSH keys, credential
// helpers, gh). Disabling is signalled by an empty GitSyncDir in settings.
package gitsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// gitNetworkTimeout is for the steps that move a repository: a first push
	// carries every sealed blob of the library.
	gitNetworkTimeout = 10 * time.Minute
	// maxRebaseSteps bounds resolving Markdown one commit at a time.
	maxRebaseSteps = 100
)

// ErrConflict means a pull ran into something other than Markdown, which is the
// writer's to sort out. The repository is back as it was.
var ErrConflict = errors.New("gitsync: the remote conflicts with files Linetta does not write")

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

// RunOnce performs one end-to-end sync cycle: take what the remote has, write
// each non-archived project's markdown into GitSyncDir, commit and push, once
// more if the remote moved under it. A hard Go error is only returned for
// unrecoverable configuration/IO failures before git runs; everything else
// surfaces via ResultSummary.
func (s *Syncer) RunOnce(ctx context.Context) (summary ResultSummary, err error) {
	nowFn := s.now()
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
	for range 2 {
		if perr := s.Prepare(ctx); perr != nil {
			if errors.Is(perr, ErrNotSetUp) {
				break
			}
			return ResultSummary{Error: perr.Error()}, nil
		}
		var pushRefused bool
		summary, pushRefused, err = s.Publish(ctx)
		if !pushRefused {
			return summary, err
		}
	}
	summary, _, err = s.Publish(ctx)
	return summary, err
}

// ErrNotSetUp means there is no repository to exchange with. Publish reports that
// the way it always has.
var ErrNotSetUp = errors.New("gitsync: no repository")

func (s *Syncer) now() func() time.Time {
	if s.Now != nil {
		return s.Now
	}
	return time.Now
}

func (s *Syncer) runner() CmdRunner {
	if s.Run != nil {
		return s.Run
	}
	return runGitProd
}

// repoDir is the repository, or ErrNotSetUp.
func (s *Syncer) repoDir(ctx context.Context) (string, error) {
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("settings.Get: %w", err)
	}
	dir := strings.TrimSpace(cfg.GitSyncDir)
	if dir == "" {
		return "", ErrNotSetUp
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return "", ErrNotSetUp
	}
	return dir, nil
}

func (s *Syncer) commitMessage(ctx context.Context) (string, error) {
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return "", err
	}
	tmpl := cfg.GitSyncCommitTemplate
	if strings.TrimSpace(tmpl) == "" {
		tmpl = defaultCommitTemplate
	}
	return strings.ReplaceAll(tmpl, "{date}", s.now()().Format("2006-01-02 15:04")), nil
}

// stageAll stages the folder and says whether anything is staged. `add -A`
// takes everything in the directory, including files Linetta did not write. The
// one we must never stage is an MCP client config: the recommended workflow puts
// .mcp.json in the synced folder so an agent can find the server, and a literal
// bearer token committed here would be pushed to the remote. Excluded at the
// staging call rather than by editing the writer's .gitignore, which is their
// file to manage.
func stageAll(ctx context.Context, run CmdRunner, dir string) (bool, error) {
	addArgs := append([]string{"add", "-A", "--"}, excludeFromStaging...)
	if _, err := run(ctx, dir, addArgs...); err != nil {
		return false, fmt.Errorf("git add: %w", err)
	}
	status, err := run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(status) != "", nil
}

// Prepare gets the repository ready for a sync: what is in the folder is
// committed, so nothing is lost to the pull, and the remote's commits come in.
// It never runs reset --hard or clean. The folder also holds the writer's own
// files, and those are not ours to remove.
func (s *Syncer) Prepare(ctx context.Context) error {
	dir, err := s.repoDir(ctx)
	if err != nil {
		return err
	}
	run := s.runner()
	dirty, err := stageAll(ctx, run, dir)
	if err != nil {
		return err
	}
	if dirty {
		msg, err := s.commitMessage(ctx)
		if err != nil {
			return err
		}
		if _, err := run(ctx, dir, "commit", "-m", msg); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}
	}
	return pull(ctx, run, dir)
}

// nothingToPull is a repository with no remote branch to take: no remote yet, or
// one that is still empty. The push that follows says what is wrong, if anything.
func nothingToPull(err error) bool {
	text := err.Error()
	for _, sign := range []string{"no tracking information", "No remote repository", "no such ref was fetched", "does not appear to be a git repository", "couldn't find remote ref"} {
		if strings.Contains(text, sign) {
			return true
		}
	}
	return false
}

func pull(ctx context.Context, run CmdRunner, dir string) error {
	_, err := run(ctx, dir, "pull", "--ff-only")
	if err == nil || nothingToPull(err) {
		return nil
	}
	_, err = run(ctx, dir, "pull", "--rebase")
	if err == nil || nothingToPull(err) {
		return nil
	}
	if !rebasing(dir) {
		return fmt.Errorf("git pull: %w", err)
	}
	if err := resolveMarkdown(ctx, run, dir); err != nil {
		_, _ = run(ctx, dir, "rebase", "--abort")
		return err
	}
	return nil
}

func rebasing(dir string) bool {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(dir, ".git", name)); err == nil {
			return true
		}
	}
	return false
}

// exportedFile says a path is something the export writes again right after the
// pull, so losing this side of a conflict in it loses nothing.
func exportedFile(path string) bool {
	return strings.HasSuffix(path, ".md") || path == export.SyncManifestFilename || strings.Contains(path, ".images/")
}

// resolveMarkdown walks a stopped rebase through conflicts in exported files by
// taking the remote's side. In a rebase that side is called "ours".
func resolveMarkdown(ctx context.Context, run CmdRunner, dir string) error {
	for range maxRebaseSteps {
		// -z, because git quotes a non-ASCII path otherwise, and Korean titles are
		// the normal case here.
		out, err := run(ctx, dir, "diff", "--name-only", "--diff-filter=U", "-z")
		if err != nil {
			return fmt.Errorf("git diff: %w", err)
		}
		if strings.Trim(out, "\x00") == "" {
			return fmt.Errorf("%w: the rebase stopped with nothing to resolve", ErrConflict)
		}
		conflicted := strings.Split(strings.Trim(out, "\x00"), "\x00")
		for _, path := range conflicted {
			if !exportedFile(path) {
				return fmt.Errorf("%w: %s", ErrConflict, path)
			}
		}
		if _, err := run(ctx, dir, append([]string{"checkout", "--ours", "--"}, conflicted...)...); err != nil {
			return fmt.Errorf("git checkout: %w", err)
		}
		if _, err := run(ctx, dir, append([]string{"add", "--"}, conflicted...)...); err != nil {
			return fmt.Errorf("git add: %w", err)
		}
		// Taking the remote's side can leave the commit with nothing in it.
		step := []string{"-c", "core.editor=true", "rebase", "--continue"}
		if _, err := run(ctx, dir, "diff", "--cached", "--quiet"); err == nil {
			step = []string{"rebase", "--skip"}
		}
		_, err = run(ctx, dir, step...)
		if !rebasing(dir) {
			return nil
		}
		if err != nil && !strings.Contains(err.Error(), "CONFLICT") && !strings.Contains(err.Error(), "could not apply") {
			return fmt.Errorf("git rebase: %w", err)
		}
	}
	return fmt.Errorf("%w: the rebase did not finish", ErrConflict)
}

// Publish exports the Markdown beside whatever is already in the folder, then
// commits and pushes all of it. pushRefused says the remote moved ahead, so the
// caller can go round again from Prepare.
func (s *Syncer) Publish(ctx context.Context) (out ResultSummary, pushRefused bool, err error) {
	nowFn := s.now()
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return ResultSummary{}, false, fmt.Errorf("settings.Get: %w", err)
	}
	dir := strings.TrimSpace(cfg.GitSyncDir)
	if dir == "" {
		return ResultSummary{Skipped: true}, false, nil
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return ResultSummary{Error: fmt.Sprintf("git_sync_dir is not a git repo: %v", err)}, false, nil
	}
	projs, err := s.Projects.List(ctx, project.ListFilter{IncludeArchived: false, Limit: 1000})
	if err != nil {
		return ResultSummary{}, false, fmt.Errorf("projects.List: %w", err)
	}
	written := 0
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
		}, p.ID, cfg.Language)
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
		if err := export.WriteImageDir(filepath.Join(dir, imageDir), payload.Images); err != nil {
			msg := fmt.Sprintf("project %s pictures: %v", p.ID, err)
			fmt.Fprintf(os.Stderr, "gitsync: %s\n", msg)
			failures = append(failures, msg)
			continue
		}
		written++
	}
	summary := ResultSummary{FilesWritten: written}
	if len(failures) > 0 {
		summary.Error = fmt.Sprintf("%d project(s) failed: %s", len(failures), strings.Join(failures, "; "))
		return summary, false, nil
	}
	manifest, err := export.BuildSyncManifest(projs, nowFn())
	if err != nil {
		summary.Error = "build sync manifest: " + err.Error()
		return summary, false, nil
	}
	if err := atomicfile.Write(filepath.Join(dir, export.SyncManifestFilename), manifest, 0o644); err != nil {
		summary.Error = "write sync manifest: " + err.Error()
		return summary, false, nil
	}
	run := s.runner()
	dirty, err := stageAll(ctx, run, dir)
	if err != nil {
		summary.Error = err.Error()
		return summary, false, nil
	}
	if !dirty {
		// A commit from Prepare may still be waiting to go up.
		ahead, err := run(ctx, dir, "rev-list", "--count", "@{u}..HEAD")
		if err != nil || strings.TrimSpace(ahead) == "0" || strings.TrimSpace(ahead) == "" {
			return summary, false, nil
		}
	} else {
		msg, err := s.commitMessage(ctx)
		if err != nil {
			summary.Error = err.Error()
			return summary, false, nil
		}
		summary.Message = msg
		if _, err := run(ctx, dir, "commit", "-m", msg); err != nil {
			summary.Error = "git commit: " + err.Error()
			return summary, false, nil
		}
		summary.Committed = true
	}
	if _, err := run(ctx, dir, "push"); err != nil {
		summary.Error = "git push: " + err.Error()
		return summary, rejected(err), nil
	}
	summary.Pushed = true
	return summary, false, nil
}

func rejected(err error) bool {
	text := err.Error()
	return strings.Contains(text, "[rejected]") || strings.Contains(text, "fetch first") || strings.Contains(text, "non-fast-forward")
}

// MCPConfigFilename is the client config an agent-assisted writer may drop in
// the synced folder. Named here so the exclusion is greppable from both sides.
const MCPConfigFilename = ".mcp.json"

// excludeFromStaging is the pathspec tail for `git add`: everything under the
// sync dir except files that must never reach the remote.
var excludeFromStaging = []string{".", ":(exclude)" + MCPConfigFilename}

func gitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	hideConsole(cmd)
	return cmd
}

// runGitProd is the production CmdRunner. It runs git from `dir`, captures
// stderr into the returned error, and enforces a per-call timeout.
func runGitProd(ctx context.Context, dir string, args ...string) (string, error) {
	timeout := gitTimeout
	if slices.Contains(args, "push") || slices.Contains(args, "pull") || slices.Contains(args, "rebase") {
		timeout = gitNetworkTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := gitCommand(cctx, dir, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("timeout after %s: %s", timeout, strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()+stdout.String()))
	}
	return stdout.String(), nil
}
