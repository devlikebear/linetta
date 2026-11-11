//go:build !mobile

package gitsync

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

// A real remote and two real clones: the point here is what git does, which a
// recording runner cannot show.

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

type clone struct {
	dir      string
	syncer   *Syncer
	projects *project.Repo
}

func newClone(t *testing.T, remote, name string) *clone {
	t.Helper()
	ctx := context.Background()
	home := t.TempDir()
	cfg, err := settings.NewForHomeWithSecretStore(home, settings.NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	st, err := storetest.Open(ctx, filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	dir := filepath.Join(t.TempDir(), name)
	git(t, filepath.Dir(dir), "clone", "--quiet", remote, dir)
	git(t, dir, "config", "user.name", name)
	git(t, dir, "config", "user.email", name+"@test.invalid")
	git(t, dir, "config", "commit.gpgsign", "false")
	git(t, dir, "config", "core.autocrlf", "false")
	git(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	if _, err := cfg.Set(ctx, settings.Patch{GitSyncDir: &dir}); err != nil {
		t.Fatal(err)
	}
	projects := project.NewRepo(st)
	return &clone{dir: dir, projects: projects, syncer: &Syncer{
		Settings: cfg, Projects: projects, Nodes: node.NewRepo(st), Entities: entity.NewRepo(st),
	}}
}

func twoClones(t *testing.T) (*clone, *clone) {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	git(t, filepath.Dir(remote), "init", "--quiet", "--bare", "--initial-branch=main", remote)
	a := newClone(t, remote, "a")
	writeRepoFile(t, a.dir, "README.txt", "시작\n")
	git(t, a.dir, "add", "-A")
	git(t, a.dir, "commit", "--quiet", "-m", "first")
	git(t, a.dir, "push", "--quiet", "-u", "origin", "main")
	b := newClone(t, remote, "b")
	return a, b
}

func writeRepoFile(t *testing.T, dir, name, body string) {
	t.Helper()
	file := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repoHas(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func (c *clone) work(t *testing.T, title string) {
	t.Helper()
	if _, err := c.projects.Create(context.Background(), 1, project.NewInput{
		Title: title, Genres: []string{"literary"}, LengthTarget: "short", DefaultPOV: "first",
	}); err != nil {
		t.Fatal(err)
	}
}

func (c *clone) sync(t *testing.T) ResultSummary {
	t.Helper()
	summary, err := c.syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Error != "" {
		t.Fatalf("sync: %s", summary.Error)
	}
	return summary
}

func TestSecondDevicePushIsNoLongerRefused(t *testing.T) {
	a, b := twoClones(t)
	a.work(t, "첫 작품")
	b.work(t, "둘째 작품")
	writeRepoFile(t, a.dir, ".linetta/items/a/one.json", "a")
	writeRepoFile(t, b.dir, ".linetta/items/b/two.json", "b")

	a.sync(t)
	if summary := b.sync(t); !summary.Pushed {
		t.Fatalf("summary = %+v", summary)
	}
	a.sync(t)
	for _, c := range []*clone{a, b} {
		for _, name := range []string{".linetta/items/a/one.json", ".linetta/items/b/two.json"} {
			if !repoHas(c.dir, name) {
				t.Errorf("%s has no %s", c.dir, name)
			}
		}
	}
}

func TestPushRefusedGoesRoundAgain(t *testing.T) {
	a, b := twoClones(t)
	a.work(t, "작품")
	writeRepoFile(t, b.dir, "other.txt", "먼저 올린 것")
	git(t, b.dir, "add", "-A")
	git(t, b.dir, "commit", "--quiet", "-m", "other")

	// The other device lands between this device's pull and its push.
	real := runGitProd
	raced := false
	a.syncer.Run = func(ctx context.Context, dir string, args ...string) (string, error) {
		if args[0] == "push" && !raced {
			raced = true
			git(t, b.dir, "push", "--quiet", "origin", "main")
		}
		return real(ctx, dir, args...)
	}

	summary := a.sync(t)

	if !raced || !summary.Pushed {
		t.Fatalf("summary = %+v, raced %v", summary, raced)
	}
	if !repoHas(a.dir, "other.txt") {
		t.Fatal("the other device's commit did not come in")
	}
}

func markdownConflict(t *testing.T, a, b *clone, name string) {
	t.Helper()
	writeRepoFile(t, a.dir, name, "여기 판\n")
	git(t, a.dir, "add", "-A")
	git(t, a.dir, "commit", "--quiet", "-m", "here")
	writeRepoFile(t, b.dir, name, "저기 판\n")
	git(t, b.dir, "add", "-A")
	git(t, b.dir, "commit", "--quiet", "-m", "there")
	git(t, b.dir, "push", "--quiet", "origin", "main")
}

func TestMarkdownConflictTakesTheRemoteAndExportsAgain(t *testing.T) {
	a, b := twoClones(t)
	markdownConflict(t, a, b, "작품.md")

	if err := a.syncer.Prepare(context.Background()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(a.dir, "작품.md"))
	if err != nil || string(data) != "저기 판\n" {
		t.Fatalf("file = %q, %v", data, err)
	}
	if rebasing(a.dir) {
		t.Fatal("a rebase is still open")
	}
	if !strings.Contains(git(t, a.dir, "log", "--format=%s"), "there") {
		t.Fatal("the remote's commit is not in the history")
	}
}

func TestConflictOutsideMarkdownStopsAndLeavesTheRepository(t *testing.T) {
	a, b := twoClones(t)
	markdownConflict(t, a, b, "내 노트.txt")
	before := git(t, a.dir, "rev-parse", "HEAD")

	err := a.syncer.Prepare(context.Background())

	if !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
	if rebasing(a.dir) {
		t.Fatal("a rebase is still open")
	}
	if got := git(t, a.dir, "rev-parse", "HEAD"); got != before {
		t.Fatalf("HEAD moved: %s → %s", before, got)
	}
	data, err := os.ReadFile(filepath.Join(a.dir, "내 노트.txt"))
	if err != nil || string(data) != "여기 판\n" {
		t.Fatalf("file = %q, %v", data, err)
	}
}

func TestWritersFilesStayAndTheMCPConfigNeverGoes(t *testing.T) {
	a, b := twoClones(t)
	a.work(t, "작품")
	writeRepoFile(t, a.dir, "내 메모.txt", "지우면 안 됨")
	writeRepoFile(t, a.dir, MCPConfigFilename, `{"token":"비밀"}`)

	a.sync(t)
	b.sync(t)

	if !repoHas(a.dir, "내 메모.txt") || !repoHas(a.dir, MCPConfigFilename) {
		t.Fatal("a writer's file was removed")
	}
	if !repoHas(b.dir, "내 메모.txt") {
		t.Fatal("a writer's file did not travel")
	}
	if repoHas(b.dir, MCPConfigFilename) {
		t.Fatal("the MCP config reached the remote")
	}
}

func TestEmptyRemoteIsNotAPullFailure(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	git(t, filepath.Dir(remote), "init", "--quiet", "--bare", "--initial-branch=main", remote)
	a := newClone(t, remote, "a")
	a.work(t, "작품")
	if err := a.syncer.Prepare(context.Background()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
}

func TestExportedFiles(t *testing.T) {
	for path, want := range map[string]bool{
		"작품--abc.md":                true,
		export.SyncManifestFilename: true,
		"작품--abc.images/a.png":      true,
		"내 노트.txt":                  false,
		".linetta/format.json":      false,
	} {
		if got := exportedFile(path); got != want {
			t.Errorf("%s = %v", path, got)
		}
	}
}
