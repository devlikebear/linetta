package handlers

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
)

// A relationship changed in episode 2 is still a colleague in episode 1 and an
// enemy from 2 on after the work goes out as Markdown and comes back (#386).
func TestExportImportRoundTripKeepsRelationshipSpans(t *testing.T) {
	ctx := context.Background()
	now := int64(1000)
	src := openLibrary(t, "src.db")
	p, err := src.projects.Create(ctx, now, project.NewInput{Title: "배신", Genres: []string{"fantasy"}, LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	first := *p.LastOpenedNodeID
	if err := src.nodes.Rename(ctx, first, "1화", "", now); err != nil {
		t.Fatal(err)
	}
	second, err := src.nodes.CreateSibling(ctx, first, "leaf", "2화", "", now)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := src.entities.Create(ctx, now, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "하린"})
	b, _ := src.entities.Create(ctx, now, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "유나"})
	pair, _ := src.relationships.CreatePair(ctx, relationship.NewPairInput{ProjectID: p.ID, FromID: a.ID, ToID: b.ID, Label: "동료", InverseLabel: "동료"})
	if _, err := src.relationships.Change(ctx, relationship.ChangeInput{ID: pair[0].ID, AtNodeID: second.ID, Label: "적", InverseLabel: "적", Quote: "넌 이제 적이야."}); err != nil {
		t.Fatal(err)
	}

	params, _ := json.Marshal(map[string]string{"project_id": p.ID})
	raw, err := ExportProject(src.exportSources(), nil)(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	var payload export.Payload
	_ = json.Unmarshal(raw, &payload)

	dst := openLibrary(t, "dst.db")
	params, _ = json.Marshal(map[string]string{"file_name": payload.SuggestedFilename, "content": payload.Markdown})
	raw, err = ImportMarkdown(dst.projects, dst.nodes, dst.entities, dst.relationships, dst.importExtras(), nil, func() int64 { return 9000 })(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ProjectID string `json:"project_id"`
	}
	_ = json.Unmarshal(raw, &result)

	rels, _ := dst.relationships.ListByProject(ctx, result.ProjectID)
	if len(rels) != 4 {
		t.Fatalf("imported %d rows, want 4 (two pairs)", len(rels))
	}
	nodes, _ := dst.nodes.ListByProject(ctx, result.ProjectID)
	var scenes []string
	for _, n := range nodes {
		if n.Kind == "leaf" {
			scenes = append(scenes, n.ID)
		}
	}
	labels := func(at string) []string {
		var out []string
		for _, r := range relationship.AsOf(rels, nodes, at) {
			out = append(out, r.Label)
		}
		return out
	}
	if got := labels(scenes[0]); !slices.Equal(got, []string{"동료", "동료"}) {
		t.Fatalf("episode 1 = %v", got)
	}
	if got := labels(scenes[1]); !slices.Equal(got, []string{"적", "적"}) {
		t.Fatalf("episode 2 = %v", got)
	}
	for _, r := range rels {
		if r.Label == "적" && (r.SinceQuote != "넌 이제 적이야." || r.SupersedesID == nil) {
			t.Fatalf("successor lost its quote or predecessor: %+v", r)
		}
	}
}
