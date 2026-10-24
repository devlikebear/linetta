package restore

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/illustration"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// A merged work brings its gallery along: new ids, the scene each picture was
// drawn from mapped to the copy of that scene, the bytes still a BLOB.
func TestMergeProjectCopiesIllustrations(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{Title: "Gallery", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	scene, err := node.NewRepo(st).CreateRoot(ctx, p.ID, node.KindLeaf, "씬 1", "비", 1)
	if err != nil {
		t.Fatal(err)
	}
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 6, 9)), nil); err != nil {
		t.Fatal(err)
	}
	repo := illustration.NewRepo(st)
	drawn, err := repo.Add(ctx, 5, illustration.NewInput{ProjectID: p.ID, SourceNodeID: scene.ID, Caption: "우산", Prompt: "p", Provider: "gemini-native", Model: "m", Data: jpg.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Add(ctx, 6, illustration.NewInput{ProjectID: p.ID, Prompt: "q", Provider: "openai-codex", Model: "n", Data: jpg.Bytes()}); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(home, "backup.db")
	if _, err := st.DB().ExecContext(ctx, "VACUUM INTO ?", backupPath); err != nil {
		t.Fatal(err)
	}

	live, err := store.Open(ctx, filepath.Join(home, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	merged, err := MergeProject(ctx, live, backupPath, home, p.ID, " copy", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	got, err := illustration.NewRepo(live).ListByProject(ctx, merged.ProjectID)
	if err != nil || len(got) != 2 {
		t.Fatalf("gallery = %+v, %v", got, err)
	}
	copyOfDrawn := got[1] // oldest last
	if copyOfDrawn.ID == drawn.ID || copyOfDrawn.Caption != "우산" || copyOfDrawn.Width != 6 || copyOfDrawn.Height != 9 {
		t.Fatalf("copied illustration = %+v", copyOfDrawn)
	}
	if copyOfDrawn.SourceNodeID == nil || *copyOfDrawn.SourceNodeID == scene.ID {
		t.Fatalf("source scene = %v, want the copy of %s", copyOfDrawn.SourceNodeID, scene.ID)
	}
	if _, err := node.NewRepo(live).Get(ctx, *copyOfDrawn.SourceNodeID); err != nil {
		t.Fatalf("source scene is not in the merged work: %v", err)
	}
	full, err := illustration.NewRepo(live).Get(ctx, copyOfDrawn.ID)
	if err != nil || !bytes.Equal(full.Data, jpg.Bytes()) {
		t.Fatalf("bytes: %v", err)
	}
	var typ string
	if err := live.DB().QueryRowContext(ctx, "SELECT typeof(data) FROM illustrations LIMIT 1").Scan(&typ); err != nil || typ != "blob" {
		t.Fatalf("storage type: %s %v", typ, err)
	}
}
