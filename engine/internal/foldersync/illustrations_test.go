package foldersync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	return "image/jpeg", data, nil
}

func sceneShowing(ids ...string) string {
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"비"}]}`
	for _, id := range ids {
		doc += `,{"type":"illustration","attrs":{"id":"` + id + `","caption":"c"}}`
	}
	return doc + "]}"
}

// The synced folder holds each work's pictures beside its file: written once,
// left alone by the next sync, removed when no scene shows them.
func TestRunOnceWritesPicturesBesideTheWork(t *testing.T) {
	s, st, _ := newFixture(t)
	ctx := context.Background()
	s.Illustrations = pictures{"a": []byte("AAAA"), "b": []byte("BB")}
	target := t.TempDir()
	enabled := true
	if _, err := st.Set(ctx, settings.Patch{FolderSyncDir: &target, FolderSyncEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	projs, err := s.Projects.List(ctx, project.ListFilter{Limit: 10})
	if err != nil || len(projs) != 1 {
		t.Fatalf("projects: %v %v", projs, err)
	}
	p := projs[0]
	if err := s.Nodes.UpdateContent(ctx, *p.LastOpenedNodeID, sceneShowing("a", "b"), 2); err != nil {
		t.Fatal(err)
	}
	if res, err := s.RunOnce(ctx); err != nil || res.Error != "" {
		t.Fatalf("RunOnce: %+v %v", res, err)
	}
	imageDir := filepath.Join(target, export.SyncImageDir(p.Title, p.ID))
	md, err := os.ReadFile(filepath.Join(target, export.SyncFilename(p.Title, p.ID)))
	if err != nil || !strings.Contains(string(md), "]("+export.SyncImageDir(p.Title, p.ID)+"/a.jpg)") {
		t.Fatalf("markdown does not link the picture file: %s %v", md, err)
	}
	if got, err := os.ReadFile(filepath.Join(imageDir, "a.jpg")); err != nil || string(got) != "AAAA" {
		t.Fatalf("a.jpg = %q %v", got, err)
	}

	old := time.Unix(1, 0)
	if err := os.Chtimes(filepath.Join(imageDir, "a.jpg"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := s.Nodes.UpdateContent(ctx, *p.LastOpenedNodeID, sceneShowing("a"), 3); err != nil {
		t.Fatal(err)
	}
	if res, err := s.RunOnce(ctx); err != nil || res.Error != "" {
		t.Fatalf("second RunOnce: %+v %v", res, err)
	}
	if info, err := os.Stat(filepath.Join(imageDir, "a.jpg")); err != nil || !info.ModTime().Equal(old) {
		t.Fatalf("an unchanged picture was rewritten: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(imageDir, "b.jpg")); !os.IsNotExist(err) {
		t.Fatalf("a picture no scene shows stayed: %v", err)
	}
}

// The MAS copy gets the pictures as "<dir>/<name>" and every work's picture
// folder, so it can remove the ones no scene uses even when none are left.
func TestStageListsPicturesAndTheirFolders(t *testing.T) {
	s, st, _ := newFixture(t)
	ctx := context.Background()
	s.Illustrations = pictures{"a": []byte("AAAA")}
	target := t.TempDir()
	enabled := true
	if _, err := st.Set(ctx, settings.Patch{FolderSyncDir: &target, FolderSyncEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	projs, _ := s.Projects.List(ctx, project.ListFilter{Limit: 10})
	p := projs[0]
	if err := s.Nodes.UpdateContent(ctx, *p.LastOpenedNodeID, sceneShowing("a"), 2); err != nil {
		t.Fatal(err)
	}
	res, err := s.Stage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := export.SyncImageDir(p.Title, p.ID)
	want := dir + "/a.jpg"
	found := false
	for _, f := range res.Files {
		found = found || f == want
	}
	if !found {
		t.Fatalf("staged files %v lack %s", res.Files, want)
	}
	if len(res.ImageDirs) != 1 || res.ImageDirs[0] != dir {
		t.Fatalf("image dirs = %v", res.ImageDirs)
	}
}
