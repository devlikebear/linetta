package illustration

import (
	"errors"
	"fmt"
	"testing"
)

func sceneWith(ids ...string) string {
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"비"}]}`
	for _, id := range ids {
		doc += fmt.Sprintf(`,{"type":"illustration","attrs":{"id":%q,"caption":""}}`, id)
	}
	return doc + "]}"
}

func TestUsageAndTheDeleteGuard(t *testing.T) {
	f := fixture(t)
	data := encodeJPEG(t, 2, 2)
	used, err := f.repo.Add(f.ctx, 1, NewInput{ProjectID: f.project, Prompt: "p", Provider: "x", Model: "y", Data: data})
	if err != nil {
		t.Fatal(err)
	}
	unused, err := f.repo.Add(f.ctx, 2, NewInput{ProjectID: f.project, Prompt: "p", Provider: "x", Model: "y", Data: data})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB().ExecContext(f.ctx, "UPDATE nodes SET content_doc = ? WHERE id = ?", sceneWith(used.ID), f.scene); err != nil {
		t.Fatal(err)
	}

	usage, err := f.repo.UsageByProject(f.ctx, f.project)
	if err != nil {
		t.Fatal(err)
	}
	if got := usage[used.ID]; len(got) != 1 || got[0].NodeID != f.scene || got[0].Label != "씬 1" {
		t.Fatalf("usage of the used picture = %+v", got)
	}
	if len(usage[unused.ID]) != 0 {
		t.Fatalf("the unused picture is reported in use: %+v", usage[unused.ID])
	}

	err = f.repo.Delete(f.ctx, used.ID)
	var inUse *InUseError
	if !errors.Is(err, ErrInUse) || !errors.As(err, &inUse) || inUse.Scenes[0].NodeID != f.scene {
		t.Fatalf("deleting a picture a scene shows: %v", err)
	}
	if _, err := f.repo.Get(f.ctx, used.ID); err != nil {
		t.Fatalf("a refused delete removed the picture: %v", err)
	}
	if err := f.repo.Delete(f.ctx, unused.ID); err != nil {
		t.Fatalf("deleting an unused picture: %v", err)
	}

	// Out of the scene, the picture can go.
	if _, err := f.s.DB().ExecContext(f.ctx, "UPDATE nodes SET content_doc = ? WHERE id = ?", sceneWith(), f.scene); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.Delete(f.ctx, used.ID); err != nil {
		t.Fatalf("deleting after the scene let go: %v", err)
	}
	if err := f.repo.Delete(f.ctx, used.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting twice: %v", err)
	}
}
