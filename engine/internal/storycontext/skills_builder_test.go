package storycontext

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
)

type fakeSkillSource struct {
	skills    []SkillBrief
	projectID string
}

func (f *fakeSkillSource) ContextSkills(_ context.Context, projectID string) []SkillBrief {
	f.projectID = projectID
	return f.skills
}

func TestBuildFullFetchesTheSkills(t *testing.T) {
	f := newCtxFixture(t)
	p := f.project(t, project.NewInput{LengthTarget: "novel", DefaultPOV: "first"})
	src := &fakeSkillSource{skills: twoSkills()}
	b := f.builder().WithSkillSource(src)

	c, err := b.BuildFull(context.Background(), *p.LastOpenedNodeID, "", "", Options{})
	if err != nil {
		t.Fatalf("BuildFull: %v", err)
	}
	if len(c.Skills) != 2 {
		t.Fatalf("got %d skills, want 2: %+v", len(c.Skills), c.Skills)
	}
	// The scope the source is asked about must be the work the brief is for.
	// A skill source handed "" would silently answer with writer-scoped
	// skills only, and the work's own skills would never reach any client.
	if src.projectID != p.ID {
		t.Errorf("skill source asked about project %q, want the brief's own %q", src.projectID, p.ID)
	}
}

// Optional, like every other source: a build that never wires one still
// produces a brief.
func TestBuildFullSurvivesNoSkillSource(t *testing.T) {
	f := newCtxFixture(t)
	p := f.project(t, project.NewInput{LengthTarget: "novel", DefaultPOV: "first"})
	c, err := f.builder().BuildFull(context.Background(), *p.LastOpenedNodeID, "", "", Options{})
	if err != nil {
		t.Fatalf("BuildFull with no skill source must still build: %v", err)
	}
	if len(c.Skills) != 0 {
		t.Errorf("got %+v, want no skills", c.Skills)
	}
}
