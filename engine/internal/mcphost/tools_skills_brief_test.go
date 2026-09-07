//go:build !mobile

package mcphost

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/storycontext"
)

// fakeBriefSkills stands in for engineapp's skillBriefSource, which mcphost
// must not import. It carries names and descriptions only — the same
// reduction the real adapter performs, so nothing here can accidentally prove
// a body reaches the brief.
type fakeBriefSkills struct{ items []storycontext.SkillBrief }

func (f fakeBriefSkills) ContextSkills(_ context.Context, _ string) []storycontext.SkillBrief {
	return f.items
}

func briefSkills() []storycontext.SkillBrief {
	return []storycontext.SkillBrief{
		{Name: "dialogue-rhythm", Description: "short beats, no dashes", Scope: "writer"},
		{Name: "flashback-voice", Description: "how flashbacks are written here", Scope: "work"},
	}
}

// The point of the whole task. A connected Claude Desktop or Claude Code never
// receives Linetta's system prompt, so linetta_get_story_context is the one
// channel that can tell it skills exist. Without this, linetta_read_skill is
// undiscoverable and the client would have to guess the tool is there and call
// it blind — the same silent omission ElementsNotInBrief was added for (#72).
func TestGetStoryContextTellsAnExternalClientItsSkillsExist(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, nil, nil)
	d.Context = d.Context.WithSkillSource(fakeBriefSkills{items: briefSkills()})
	d.Settings = languageSettings(t, "en")

	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID})
	if err != nil {
		t.Fatalf("getStoryContext: %v", err)
	}
	for _, want := range []string{
		"## Skills you can read (2)",
		"dialogue-rhythm",
		"flashback-voice",
		"linetta_read_skill",
	} {
		if !strings.Contains(out.Brief, want) {
			t.Errorf("external client's brief is missing %q; got:\n%s", want, out.Brief)
		}
	}
	if !slices.Contains(out.IncludedSections, "skills") {
		t.Errorf("the report must list skills as present: included=%v", out.IncludedSections)
	}
	if slices.Contains(out.EmptySections, "skills") {
		t.Errorf("skills is in both lists: empty=%v", out.EmptySections)
	}
}

// A work with no skills must say so in the report rather than leaving the
// section unmentioned: "skills" in empty_sections is how a client learns the
// list is empty and not merely unbuilt.
func TestSectionReportListsSkillsEmptyWhenThereAreNone(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, nil, nil)
	d.Settings = languageSettings(t, "en")

	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID})
	if err != nil {
		t.Fatalf("getStoryContext: %v", err)
	}
	if !slices.Contains(out.EmptySections, "skills") {
		t.Errorf("the report must list skills as empty: empty=%v", out.EmptySections)
	}
	if slices.Contains(out.IncludedSections, "skills") {
		t.Errorf("skills is in both lists: included=%v", out.IncludedSections)
	}
}

// ContextKeySkills exists only because a real switch reaches it. This is that
// switch: include_skills=false must take the block out of the brief AND out of
// the report, or the key is advertising a control that does not exist — the
// thing sectionReport's own comment warns against.
func TestIncludeSkillsFalseRemovesTheBlockAndTheReportSaysSo(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, nil, nil)
	d.Context = d.Context.WithSkillSource(fakeBriefSkills{items: briefSkills()})
	d.Settings = languageSettings(t, "en")

	off := false
	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID, IncludeSkills: &off})
	if err != nil {
		t.Fatalf("getStoryContext: %v", err)
	}
	for _, gone := range []string{"dialogue-rhythm", "flashback-voice", "Skills you can read"} {
		if strings.Contains(out.Brief, gone) {
			t.Errorf("%q survived include_skills=false; got:\n%s", gone, out.Brief)
		}
	}
	if slices.Contains(out.IncludedSections, "skills") || !slices.Contains(out.EmptySections, "skills") {
		t.Errorf("skills survived the toggle in the report: included=%v empty=%v",
			out.IncludedSections, out.EmptySections)
	}
}

// The memories toggle must not reach the skills, and vice versa. This is the
// wire-level half of storycontext's TestSkillsAreIndependentlyToggleable — it
// is what justifies a separate ContextKeySkills instead of riding
// ContextKeyMemories the way the curated documents do.
func TestTheTwoTogglesDoNotReachEachOther(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, fakeCurated{profile: "no em dashes"}, nil)
	d.Context = d.Context.WithSkillSource(fakeBriefSkills{items: briefSkills()})
	d.Settings = languageSettings(t, "en")

	off := false

	_, memOff, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID, IncludeMemories: &off})
	if err != nil {
		t.Fatalf("getStoryContext (memories off): %v", err)
	}
	if strings.Contains(memOff.Brief, "no em dashes") {
		t.Fatal("test setup: the memories toggle did not take the curated memory")
	}
	if !strings.Contains(memOff.Brief, "dialogue-rhythm") {
		t.Errorf("turning memories off also took the skills; got:\n%s", memOff.Brief)
	}

	_, skillsOff, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID, IncludeSkills: &off})
	if err != nil {
		t.Fatalf("getStoryContext (skills off): %v", err)
	}
	if !strings.Contains(skillsOff.Brief, "no em dashes") {
		t.Errorf("turning skills off also took the curated memory; got:\n%s", skillsOff.Brief)
	}
}

// The built-in agent already carries the same list in its own system prompt
// (agent.systemPrompt / skillsBlock), so repeating it here costs context and
// teaches it nothing — exactly the reason the curated documents are suppressed
// for SourceAgent above. And the report has to be computed AFTER that
// suppression: a report claiming "skills" over a brief that carries none is
// the same defect #97 shipped a fix round for.
func TestGetStoryContextForTheAgentOmitsTheSkillsAndTheReportAgrees(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, nil, nil)
	d.Context = d.Context.WithSkillSource(fakeBriefSkills{items: briefSkills()})
	d.Settings = languageSettings(t, "en")
	d.Source = SourceAgent

	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID})
	if err != nil {
		t.Fatalf("getStoryContext: %v", err)
	}
	for _, gone := range []string{"dialogue-rhythm", "flashback-voice", "Skills you can read"} {
		if strings.Contains(out.Brief, gone) {
			t.Errorf("agent's brief still carries %q, already in its system prompt; got:\n%s", gone, out.Brief)
		}
	}
	if slices.Contains(out.IncludedSections, "skills") {
		t.Errorf("agent's report claims skills the suppressed brief does not carry: included=%v", out.IncludedSections)
	}
	if !slices.Contains(out.EmptySections, "skills") {
		t.Errorf("agent's report should list skills as empty: empty=%v", out.EmptySections)
	}
}
