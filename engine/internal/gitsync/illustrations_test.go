//go:build !mas && !mobile

package gitsync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

type pictures map[string][]byte

func (p pictures) IllustrationImage(_ context.Context, id string) (string, []byte, error) {
	data, ok := p[id]
	if !ok {
		return "", nil, export.ErrIllustrationMissing
	}
	return "image/png", data, nil
}

// The repository gets each work's pictures as files the Markdown links to,
// and `git add -A` takes them along.
func TestRunOnce_writesPicturesIntoTheRepo(t *testing.T) {
	s, runner, repoDir, _ := newFixture(t)
	ctx := context.Background()
	s.Illustrations = pictures{"a": []byte("PNG")}
	if _, err := s.Settings.Set(ctx, settings.Patch{GitSyncDir: &repoDir}); err != nil {
		t.Fatal(err)
	}
	projs, _ := s.Projects.List(ctx, project.ListFilter{Limit: 10})
	p := projs[0]
	doc := `{"type":"doc","content":[{"type":"illustration","attrs":{"id":"a","caption":"표지"}}]}`
	if err := s.Nodes.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	dir := export.SyncImageDir(p.Title, p.ID)
	if got, err := os.ReadFile(filepath.Join(repoDir, dir, "a.png")); err != nil || string(got) != "PNG" {
		t.Fatalf("picture file = %q %v", got, err)
	}
	md, _ := os.ReadFile(filepath.Join(repoDir, export.SyncFilename(p.Title, p.ID)))
	if !strings.Contains(string(md), "![표지]("+dir+"/a.png)") {
		t.Fatalf("markdown = %s", md)
	}
	if !strings.Contains(strings.Join(runner.cmdNames(), " "), "add") {
		t.Fatalf("git add was not run: %v", runner.cmdNames())
	}
}
