package companion

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func TestReferenceRepo_CreateListUpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	projects := project.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "t", Genres: []string{"f"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	repo := NewReferenceRepo(st.DB())

	ref, err := repo.Create(ctx, ReferenceInput{
		ProjectID:  p.ID,
		SourceType: "clipboard",
		Purpose:    "style",
		Title:      "자서전 문체",
		Content:    strings.Repeat("담담한 문장. ", 600),
	}, 1000)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ref.Status != ReferenceStatusSummarized {
		t.Fatalf("long reference status = %q, want summarized", ref.Status)
	}
	if ref.Summary == "" || ref.CharCount == 0 || ref.TokenEstimate == 0 {
		t.Fatalf("reference metadata not populated: %+v", ref)
	}

	list, err := repo.List(ctx, ReferenceQuery{ProjectID: p.ID, NodeID: "", Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != ref.ID {
		t.Fatalf("List = %+v, want created ref", list)
	}

	status := ReferenceStatusDisabled
	updated, err := repo.Update(ctx, ReferencePatch{ProjectID: p.ID, ID: ref.ID, Status: &status}, 2000)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status != ReferenceStatusDisabled || updated.UpdatedAt != 2000 {
		t.Fatalf("updated = %+v", updated)
	}
	active, err := repo.List(ctx, ReferenceQuery{ProjectID: p.ID, Limit: 20})
	if err != nil {
		t.Fatalf("List active: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("disabled reference should be hidden, got %+v", active)
	}

	if err := repo.Delete(ctx, p.ID, ref.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
