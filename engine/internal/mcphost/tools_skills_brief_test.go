//go:build !mobile

package mcphost

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/settings"
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

	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID}, AllToolGroups())
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

	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID}, AllToolGroups())
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
	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID, IncludeSkills: &off}, AllToolGroups())
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

	_, memOff, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID, IncludeMemories: &off}, AllToolGroups())
	if err != nil {
		t.Fatalf("getStoryContext (memories off): %v", err)
	}
	if strings.Contains(memOff.Brief, "no em dashes") {
		t.Fatal("test setup: the memories toggle did not take the curated memory")
	}
	if !strings.Contains(memOff.Brief, "dialogue-rhythm") {
		t.Errorf("turning memories off also took the skills; got:\n%s", memOff.Brief)
	}

	_, skillsOff, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID, IncludeSkills: &off}, AllToolGroups())
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

	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID}, AllToolGroups())
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

/* ---------------------------------------------------------------------------
 * #99: the writer's skill-tools switch, on the brief.
 * ------------------------------------------------------------------------ */

// The brief is the ONLY channel that tells an external client skills exist,
// and the sentence it uses to do that names linetta_read_skill. With the
// skills group switched off that tool is not in the client's tools/list at
// all, so a brief still carrying the block would be pointing at a tool the
// same server just refused to register — the exact failure this feature is
// supposed to prevent, one level out from the system prompt.
func TestGetStoryContextDropsTheSkillsBlockWhenTheSkillToolsAreOff(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, fakeCurated{profile: "PROFILE-BODY: no em dashes"}, nil)
	d.Context = d.Context.WithSkillSource(fakeBriefSkills{items: briefSkills()})
	d.Settings = languageSettings(t, "en")
	setToolGroups(t, d.Settings, ToolGroups{Memory: true, Skills: false})

	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID}, ToolGroupsFrom(d.Settings))
	if err != nil {
		t.Fatalf("getStoryContext: %v", err)
	}
	for _, gone := range []string{
		"Skills you can read",
		"linetta_read_skill",
		"dialogue-rhythm",
		"flashback-voice",
	} {
		if strings.Contains(out.Brief, gone) {
			t.Errorf("the brief still carries %q with the skills tools switched off; got:\n%s", gone, out.Brief)
		}
	}
	if slices.Contains(out.IncludedSections, "skills") {
		t.Errorf("the report claims the brief carries skills: included=%v", out.IncludedSections)
	}
	// The memory documents are NOT treated this way and must survive: they
	// are content the writer still edits, not a pointer to a tool. See
	// getStoryContext's own comment for the ruling.
	if !strings.Contains(out.Brief, "PROFILE-BODY: no em dashes") {
		t.Errorf("the curated memory went with the skills block; got:\n%s", out.Brief)
	}
}

// The writer's switch is upstream of the client's per-request flag, and the
// composition runs one way only: with the group off, include_skills=true
// cannot put the block back. A client must not be able to ask its way past a
// decision the writer made.
func TestIncludeSkillsTrueCannotOverrideTheWritersSwitch(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, nil, nil)
	d.Context = d.Context.WithSkillSource(fakeBriefSkills{items: briefSkills()})
	d.Settings = languageSettings(t, "en")
	setToolGroups(t, d.Settings, ToolGroups{Memory: true, Skills: false})

	on := true
	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID, IncludeSkills: &on}, ToolGroupsFrom(d.Settings))
	if err != nil {
		t.Fatalf("getStoryContext: %v", err)
	}
	if strings.Contains(out.Brief, "Skills you can read") {
		t.Errorf("include_skills=true put the block back over the writer's switch; got:\n%s", out.Brief)
	}
}

// And with the group ON, nothing about the client's flag changes: the two
// controls compose, they do not replace each other.
func TestTheWritersSwitchLeavesIncludeSkillsAloneWhenTheGroupIsOn(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, nil, nil)
	d.Context = d.Context.WithSkillSource(fakeBriefSkills{items: briefSkills()})
	d.Settings = languageSettings(t, "en")
	setToolGroups(t, d.Settings, AllToolGroups())

	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID}, ToolGroupsFrom(d.Settings))
	if err != nil {
		t.Fatalf("getStoryContext: %v", err)
	}
	if !strings.Contains(out.Brief, "Skills you can read") {
		t.Errorf("the default brief lost its skills block; got:\n%s", out.Brief)
	}
	off := false
	_, out, err = d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID, IncludeSkills: &off}, ToolGroupsFrom(d.Settings))
	if err != nil {
		t.Fatalf("getStoryContext: %v", err)
	}
	if strings.Contains(out.Brief, "Skills you can read") {
		t.Errorf("include_skills=false stopped working; got:\n%s", out.Brief)
	}
}

// The brief is bound to the tool budget the SERVER was built with, not to
// whatever the store says when the call arrives. Those two are the same value
// almost always — which is why a live read looked harmless — but not while a
// switch is thrown against a server already standing. The MCP host builds one
// server per HTTP session, so a client that connected before the flip holds
// that server for the length of its session.
//
// The dangerous direction is the group going ON: the server registered no
// linetta_read_skill, and a brief that re-read the store would hand that
// client a list of skills and tell it to open one with a tool its own
// tools/list does not contain. This test puts the server and the store in
// exactly that disagreement and asks the server itself, over the wire, for
// both answers.
func TestTheBriefNamesNoToolTheSameServerDidNotRegister(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, nil, nil)
	d.Context = d.Context.WithSkillSource(fakeBriefSkills{items: briefSkills()})
	d.Settings = languageSettings(t, "en")

	// The server is built while the skills group is off...
	built := ToolGroups{Memory: true, Skills: false}
	cs := connectedServer(t, d, settings.MCPModeFull, built)
	// ...and the writer switches it back on a moment later. Nothing rebuilds
	// this server: the client is mid-session.
	setToolGroups(t, d.Settings, AllToolGroups())

	served := map[string]bool{}
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		served[tool.Name] = true
	}
	if served["linetta_read_skill"] {
		t.Fatal("test setup: the server was built with the skills group off and served it anyway")
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "linetta_get_story_context",
		Arguments: map[string]any{"node_id": nodeID},
	})
	if err != nil {
		t.Fatalf("call linetta_get_story_context: %v", err)
	}
	brief := firstText(res)
	for _, name := range promptToolName.FindAllString(brief, -1) {
		if !served[name] {
			t.Errorf("the brief tells the client to call %s, which this same server never "+
				"registered — the brief read the store instead of the budget its server was "+
				"built with; brief:\n%s", name, brief)
		}
	}
}

// promptToolName matches every Linetta tool a brief names, by the naming
// convention rather than by a list this test would have to be reminded to
// update.
var promptToolName = regexp.MustCompile(`linetta_[a-z_]+`)

// connectedServer builds a server exactly the way the host does — resolve the
// budget, hand it to Register — and dials it in memory.
func connectedServer(t *testing.T, d ToolDeps, mode string, groups ToolGroups) *mcp.ClientSession {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: ServerVersion}, nil)
	d.Register(srv, mode, groups)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).
		Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func setToolGroups(t *testing.T, st *settings.Store, groups ToolGroups) {
	t.Helper()
	if _, err := st.Set(context.Background(), settings.Patch{
		MemoryToolsEnabled: &groups.Memory,
		SkillToolsEnabled:  &groups.Skills,
	}); err != nil {
		t.Fatalf("set tool groups: %v", err)
	}
}
