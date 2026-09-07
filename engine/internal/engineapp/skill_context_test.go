package engineapp

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentskills"
)

// skillBriefSource is the story brief's half of the same reduction
// agentSkillSource performs for the system prompt. These tests exercise it
// directly against a real *agentskills.Store over a temp directory, avoiding
// the engineapp fixtures for the same reason agent_skillsource_test.go does.

func newBriefSource(t *testing.T) skillBriefSource {
	t.Helper()
	return skillBriefSource{store: agentskills.NewStore(t.TempDir())}
}

func writeBriefSkill(t *testing.T, st *agentskills.Store, s agentskills.Skill) {
	t.Helper()
	if _, err := st.Write(s, 1700000000000); err != nil {
		t.Fatalf("Write(%q): %v", s.Name, err)
	}
}

// A build that never wires the store must answer "no skills", not panic —
// matching agentSkillSource's nil-store case.
func TestSkillBriefSource_nilStoreReturnsNoSkills(t *testing.T) {
	var src skillBriefSource
	if got := src.ContextSkills(context.Background(), "p1"); len(got) != 0 {
		t.Errorf("nil store must yield no skills, got %v", got)
	}
}

func TestSkillBriefSource_readsBothScopesAsNamesAndDescriptions(t *testing.T) {
	src := newBriefSource(t)
	writeBriefSkill(t, src.store, agentskills.Skill{
		Name: "dialogue-rhythm", Scope: agentskills.ScopeWriter,
		Description: "short beats, no dashes", Author: agentskills.AuthorWriter,
		Enabled: true, Body: "a body that must never reach the brief",
	})
	writeBriefSkill(t, src.store, agentskills.Skill{
		Name: "flashback-voice", Scope: agentskills.ScopeWork, ProjectID: "p1",
		Description: "how flashbacks are written here", Author: agentskills.AuthorAgent,
		Enabled: true, Body: "another body that must never reach the brief",
	})

	got := src.ContextSkills(context.Background(), "p1")
	if len(got) != 2 {
		t.Fatalf("want 2 skills, got %d: %+v", len(got), got)
	}
	scopes := map[string]string{}
	for _, s := range got {
		scopes[s.Name] = s.Scope
		if s.Description == "" {
			t.Errorf("%q reached the brief with no description — the list would name it and say nothing", s.Name)
		}
	}
	if scopes["dialogue-rhythm"] != string(agentskills.ScopeWriter) {
		t.Errorf("dialogue-rhythm scope = %q, want %q", scopes["dialogue-rhythm"], agentskills.ScopeWriter)
	}
	if scopes["flashback-voice"] != string(agentskills.ScopeWork) {
		t.Errorf("flashback-voice scope = %q, want %q", scopes["flashback-voice"], agentskills.ScopeWork)
	}
}

// A skill the writer switched off must not be listed, in the brief any more
// than in the prompt — the same clause SkillSource's contract states.
func TestSkillBriefSource_skipsDisabledSkills(t *testing.T) {
	src := newBriefSource(t)
	writeBriefSkill(t, src.store, agentskills.Skill{
		Name: "switched-off", Scope: agentskills.ScopeWriter,
		Description: "the writer turned this one off", Author: agentskills.AuthorAgent,
		Enabled: false,
	})
	if got := src.ContextSkills(context.Background(), "p1"); len(got) != 0 {
		t.Errorf("a disabled skill reached the brief: %+v", got)
	}
}

// With no work open there is nothing for a work-scoped skill to belong to, so
// only the writer's own are offered — the same rule agentSkillSource follows.
func TestSkillBriefSource_withNoWorkOpenListsOnlyTheWritersOwn(t *testing.T) {
	src := newBriefSource(t)
	writeBriefSkill(t, src.store, agentskills.Skill{
		Name: "dialogue-rhythm", Scope: agentskills.ScopeWriter,
		Description: "short beats, no dashes", Author: agentskills.AuthorWriter, Enabled: true,
	})
	writeBriefSkill(t, src.store, agentskills.Skill{
		Name: "flashback-voice", Scope: agentskills.ScopeWork, ProjectID: "p1",
		Description: "how flashbacks are written here", Author: agentskills.AuthorAgent, Enabled: true,
	})
	got := src.ContextSkills(context.Background(), "  ")
	if len(got) != 1 || got[0].Name != "dialogue-rhythm" {
		t.Errorf("want only the writer-scoped skill, got %+v", got)
	}
}
