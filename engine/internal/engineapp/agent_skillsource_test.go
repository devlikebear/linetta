//go:build !mobile

package engineapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentskills"
)

// These tests exercise agentSkillSource directly against a real
// *agentskills.Store over a temp directory. They deliberately avoid
// constructing a full App — App.New opens the keychain-backed settings
// store, which hangs in this environment — so nothing here needs the
// engineapp fixtures the rest of the package's tests do.

func newSkillTestSource(t *testing.T) (agentSkillSource, string) {
	t.Helper()
	home := t.TempDir()
	return agentSkillSource{store: agentskills.NewStore(home)}, home
}

func mustWriteSkill(t *testing.T, st *agentskills.Store, s agentskills.Skill) {
	t.Helper()
	if _, err := st.Write(s, 1700000000000); err != nil {
		t.Fatalf("Write(%q): %v", s.Name, err)
	}
}

// A nil store — the zero value of agentSkillSource, which is what a build
// that never wires deps.skills would produce — must behave like "no
// skills", not panic. Matches agentMemorySource's nil-repo case above.
func TestAgentSkillSource_nilStoreReturnsNoSkills(t *testing.T) {
	var src agentSkillSource
	got := src.Skills(context.Background(), "p1")
	if len(got) != 0 {
		t.Errorf("nil store must yield no skills, got %v", got)
	}
}

func TestAgentSkillSource_readsBothWriterAndWorkScopedSkills(t *testing.T) {
	src, _ := newSkillTestSource(t)
	mustWriteSkill(t, src.store, agentskills.Skill{
		Name: "dialogue-rhythm", Scope: agentskills.ScopeWriter,
		Description: "short beats, no dashes", Author: agentskills.AuthorWriter, Enabled: true,
	})
	mustWriteSkill(t, src.store, agentskills.Skill{
		Name: "flashback-voice", Scope: agentskills.ScopeWork, ProjectID: "p1",
		Description: "how flashbacks are written in this work", Author: agentskills.AuthorAgent, Enabled: true,
	})

	got := src.Skills(context.Background(), "p1")
	if len(got) != 2 {
		t.Fatalf("want 2 skills, got %d: %+v", len(got), got)
	}
	// Both SCOPES, not just two rows: an adapter that listed the writer
	// scope twice, or that read work skills into a writer-scoped result,
	// would satisfy a length check alone.
	byName := map[string]agentskills.Skill{}
	for _, s := range got {
		byName[s.Name] = s
	}
	for name, wantScope := range map[string]agentskills.Scope{
		"dialogue-rhythm": agentskills.ScopeWriter,
		"flashback-voice": agentskills.ScopeWork,
	} {
		s, ok := byName[name]
		if !ok {
			t.Errorf("skill %q is missing from the result: %+v", name, got)
			continue
		}
		if s.Scope != wantScope {
			t.Errorf("skill %q has scope %q, want %q", name, s.Scope, wantScope)
		}
	}
	if s := byName["flashback-voice"]; s.ProjectID != "p1" {
		t.Errorf("the work-scoped skill carries project id %q, want %q", s.ProjectID, "p1")
	}
}

// A work-scoped skill under a DIFFERENT project must not leak into this
// turn — it belongs to a manuscript that is not open.
func TestAgentSkillSource_workScopedSkillsAreProjectSpecific(t *testing.T) {
	src, _ := newSkillTestSource(t)
	mustWriteSkill(t, src.store, agentskills.Skill{
		Name: "other-work-skill", Scope: agentskills.ScopeWork, ProjectID: "other-project",
		Description: "belongs to a different manuscript", Author: agentskills.AuthorWriter, Enabled: true,
	})

	got := src.Skills(context.Background(), "p1")
	if len(got) != 0 {
		t.Errorf("a skill scoped to a different work must not appear, got %+v", got)
	}
}

// With no work open (empty projectID — a fresh session before any manuscript
// is selected), the adapter must not error or panic trying to list a work
// scope with no id; it just skips work-scoped skills.
func TestAgentSkillSource_noProjectIDSkipsWorkScope(t *testing.T) {
	src, _ := newSkillTestSource(t)
	mustWriteSkill(t, src.store, agentskills.Skill{
		Name: "habit", Scope: agentskills.ScopeWriter,
		Description: "a writer habit", Author: agentskills.AuthorWriter, Enabled: true,
	})

	got := src.Skills(context.Background(), "")
	if len(got) != 1 || got[0].Name != "habit" {
		t.Fatalf("want just the writer skill with no project open, got %+v", got)
	}
}

// Rule: only enabled skills reach the prompt.
func TestAgentSkillSource_disabledSkillsAreExcluded(t *testing.T) {
	src, _ := newSkillTestSource(t)
	mustWriteSkill(t, src.store, agentskills.Skill{
		Name: "retired", Scope: agentskills.ScopeWriter,
		Description: "no longer used", Author: agentskills.AuthorWriter, Enabled: false,
	})
	mustWriteSkill(t, src.store, agentskills.Skill{
		Name: "active", Scope: agentskills.ScopeWriter,
		Description: "still used", Author: agentskills.AuthorWriter, Enabled: true,
	})

	got := src.Skills(context.Background(), "p1")
	if len(got) != 1 || got[0].Name != "active" {
		t.Fatalf("disabled skill leaked through: %+v", got)
	}
}

// Rule: a skill body must never reach the caller — the adapter strips it
// even though Store.List already returns the parsed body, because
// systemPrompt's contract (see agent.SkillSource's doc comment) is that
// Body is never in what this method hands back.
func TestAgentSkillSource_stripsBodies(t *testing.T) {
	src, _ := newSkillTestSource(t)
	mustWriteSkill(t, src.store, agentskills.Skill{
		Name: "dialogue-rhythm", Scope: agentskills.ScopeWriter,
		Description: "short beats, no dashes", Author: agentskills.AuthorWriter, Enabled: true,
		Body: "# dialogue-rhythm\n\nNever use an em dash mid-sentence.\n",
	})

	got := src.Skills(context.Background(), "p1")
	if len(got) != 1 {
		t.Fatalf("want 1 skill, got %d", len(got))
	}
	if got[0].Body != "" {
		t.Errorf("Body must be stripped, got %q", got[0].Body)
	}
}

// Rule: Guard runs before a description reaches the prompt. Store.Write
// refuses a guard-failing skill outright, so the only way one lands on disk
// is a hand-edited (or otherwise raw) SKILL.md — which is also how a writer
// editing the file directly, or a bug elsewhere, could produce one. Store.List
// already runs Guard on every entry it reads and reports a failure as a
// Diagnostic instead of returning the skill (see agentskills'
// TestGuardFailureIsADiagnostic); this test is the end-to-end proof that the
// adapter built on top of List inherits that guarantee rather than working
// around it.
func TestAgentSkillSource_guardFailingSkillNeverReachesTheCaller(t *testing.T) {
	src, home := newSkillTestSource(t)
	mustWriteSkill(t, src.store, agentskills.Skill{
		Name: "healthy", Scope: agentskills.ScopeWriter,
		Description: "a normal skill", Author: agentskills.AuthorWriter, Enabled: true,
	})

	// A zero-width space in the description: Parse accepts it, Guard
	// refuses it. Planted directly on disk since Write itself runs Guard
	// and would refuse to create this file at all.
	broken := filepath.Join(home, "skills", "sneaky", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(broken), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := "---\nname: sneaky\ndescription: looks fine​\n---\nbody\n"
	if err := os.WriteFile(broken, []byte(raw), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := src.Skills(context.Background(), "p1")
	if len(got) != 1 || got[0].Name != "healthy" {
		t.Fatalf("the guard-failing skill must not appear, and the healthy one must still: %+v", got)
	}
}
