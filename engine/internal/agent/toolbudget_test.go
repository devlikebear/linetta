//go:build !mobile

package agent

import (
	"context"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// promptToolName matches every Linetta tool the system prompt names. The
// prompt writes them bare, as `linetta_get_story_context`, so the pattern is
// the naming convention itself rather than a list this test would have to be
// remembered and updated.
var promptToolName = regexp.MustCompile(`linetta_[a-z_]+`)

// budgetService wires a Service the way engineapp/agent_enabled.go does: the
// REAL mcphost registration, reading the two tool-budget switches off a real
// settings store, and the same store behind Deps.MemoryToolsEnabled and
// Deps.SkillToolsEnabled.
//
// stubTools is what every other test in this package registers, and it is why
// the switches shipped broken: a stub that installs one `echo` tool can never
// disagree with the prompt about which Linetta tools exist, so a test built on
// it passes whether or not the turn's tool list follows the switches at all.
// This is the only wiring in which that question has an answer.
func budgetService(t *testing.T, c llm.Client, rec *recorder) (*Service, *settings.Store, *int) {
	t.Helper()
	cfg, err := settings.NewForHomeWithSecretStore(t.TempDir(), settings.NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings.NewForHomeWithSecretStore: %v", err)
	}
	tools := mcphost.ToolDeps{Settings: cfg}
	registrations := 0
	db := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers: fakeProviders{client: c},
		History:   companion.NewHistoryRepo(db.DB()),
		Scope:     fakeScope{titles: map[string]string{"p1": "제목"}},
		Register: func(s *mcp.Server, groups mcphost.ToolGroups) {
			registrations++
			// Always full mode, and the turn's own groups — exactly as
			// setupAgent binds it. The registrar is handed the budget rather
			// than reading the store for itself, which is the only way the
			// server it builds can be guaranteed to match the label the
			// session is cached under.
			tools.Register(s, settings.MCPModeFull, groups)
		},
		Notify:   rec.notify,
		Language: func() string { return "ko" },
		Skills: &countingSkills{items: []agentskills.Skill{
			{Name: "dialogue-rhythm", Scope: agentskills.ScopeWriter, Description: "rhythm", Enabled: true},
		}},
		MemoryToolsEnabled: cfg.MemoryToolsEnabled,
		SkillToolsEnabled:  cfg.SkillToolsEnabled,
		Clock:              func() int64 { return 1700000000000 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc, cfg, &registrations
}

func setToolSwitches(t *testing.T, cfg *settings.Store, memory, skills bool) {
	t.Helper()
	if _, err := cfg.Set(context.Background(), settings.Patch{
		MemoryToolsEnabled: &memory,
		SkillToolsEnabled:  &skills,
	}); err != nil {
		t.Fatalf("settings.Set(memory=%v, skills=%v): %v", memory, skills, err)
	}
}

// oneTurn runs a turn and waits for it to end.
func oneTurn(t *testing.T, svc *Service, rec *recorder) {
	t.Helper()
	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the turn to end", func() bool { return rec.hasTerminalFor(runID) })
}

func offeredTools(schemas []llm.ToolSchema) map[string]bool {
	out := map[string]bool{}
	for _, s := range schemas {
		out[s.Function.Name] = true
	}
	return out
}

// The invariant the whole feature rests on: whatever a turn's system prompt
// tells the agent to call, that same turn's ChatOptions.Tools contains — and
// the switches the writer threw are honoured by BOTH halves of the turn, on
// the turn right after the flip rather than after an app restart.
//
// Both directions of the flip are checked because they fail differently and
// only one of them is merely wasteful. Flipping a group OFF and still
// shipping its tools costs the writer the bytes the switch promised to save.
// Flipping one ON and not shipping its tools produces a prompt that instructs
// the agent to use a tool it is not holding, which is the failure the feature
// exists to prevent.
func TestRun_thePromptAndTheOfferedToolsDescribeTheSameTurn(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{textReply("ok")}}
	svc, cfg, _ := budgetService(t, c, rec)

	cases := []struct {
		name           string
		memory, skills bool
	}{
		// Deliberately one session, in this order: every case after the
		// first is a flip against a tool session the previous case already
		// built and cached, which is the state the shipped code got wrong.
		{"both on, as they ship", true, true},
		{"both off", false, false},
		{"skills switched back on mid-session", false, true},
		{"memory switched back on mid-session", true, true},
		{"skills off again mid-session", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setToolSwitches(t, cfg, tc.memory, tc.skills)
			oneTurn(t, svc, rec)

			msgs := c.messages()
			if len(msgs) == 0 {
				t.Fatal("no messages reached the model")
			}
			system := msgs[0].Content
			offered := offeredTools(c.options().Tools)

			// Every group's tools are present exactly when its switch is on.
			for name, want := range map[string]bool{
				"linetta_edit_memory": tc.memory,
				"linetta_read_skill":  tc.skills,
				"linetta_edit_skill":  tc.skills,
			} {
				if offered[name] != want {
					t.Errorf("the turn offered %s = %v, want %v — the tool list does not "+
						"follow the switch until the app restarts", name, offered[name], want)
				}
			}

			// The self-review guard reads this very list — selfreview.go's
			// offers(st.schemas, "linetta_edit_skill") — so with the skills
			// tools off the pass cannot start, and the case where it fired
			// against a stale session and wrote skill files the writer had
			// just switched off goes away with the stale session itself.
			if got := offers(c.options().Tools, "linetta_edit_skill"); got != tc.skills {
				t.Errorf("the self-review guard sees linetta_edit_skill = %v, want %v", got, tc.skills)
			}

			// And nothing the prompt names is missing from that list.
			for _, name := range promptToolName.FindAllString(system, -1) {
				if !offered[name] {
					t.Errorf("the system prompt tells the agent to call %s, but the same "+
						"turn offered %d tools and not that one; prompt:\n%s",
						name, len(offered), system)
				}
			}
		})
	}
}

// One reading per turn, not one per consumer. Even with the session cache
// keyed correctly, a prompt that went back to the switches for its own answer
// would reopen the same hole on a narrower window: the writer's click can land
// between the two reads, and the turn then ships a prompt describing a tool
// set the server it is already connected to does not have.
//
// The switch here answers differently on every call — off the first time, on
// after that — so a second reading cannot agree with the first. A turn that
// reads once passes whichever answer it got; a turn that reads twice builds a
// server without the skills tools and then a prompt that names them.
func TestRun_theSwitchesAreReadOnceForTheWholeTurn(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{textReply("ok")}}
	svc, _, _ := budgetService(t, c, rec)

	// Atomic because the second read this test is hunting for would happen on
	// the turn's own goroutine: a plain counter would report the regression as
	// a data race under -race instead of as the assertion below.
	var reads atomic.Int64
	svc.deps.SkillToolsEnabled = func() bool { return reads.Add(1) > 1 }
	oneTurn(t, svc, rec)

	if got := reads.Load(); got != 1 {
		t.Errorf("the turn read the skills switch %d times, want exactly 1", got)
	}
	system := c.messages()[0].Content
	offered := offeredTools(c.options().Tools)
	for _, name := range promptToolName.FindAllString(system, -1) {
		if !offered[name] {
			t.Errorf("the system prompt names %s, which this turn was not offered — "+
				"the switch was read a second time and answered differently", name)
		}
	}
}

// The cache is the reason the bug existed, and it is still worth having: a
// writer who never opens the Settings pane must not pay to stand up a second
// MCP server on every message. Rebuilding unconditionally would fix the
// contradiction and quietly undo that.
func TestRun_theToolSessionIsRebuiltOnlyWhenTheBudgetChanges(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{textReply("ok")}}
	svc, cfg, registrations := budgetService(t, c, rec)

	oneTurn(t, svc, rec)
	oneTurn(t, svc, rec)
	oneTurn(t, svc, rec)
	if *registrations != 1 {
		t.Errorf("three turns with no switch flipped built %d tool servers, want 1", *registrations)
	}

	first := svc.tools
	setToolSwitches(t, cfg, false, false)
	oneTurn(t, svc, rec)
	if *registrations != 2 {
		t.Errorf("the turn after the flip built %d tool servers in total, want 2", *registrations)
	}

	// The superseded session is not merely dropped. A rebuild that left the
	// old client/server pair open would leak one MCP server per flip for the
	// life of the engine.
	first.mu.Lock()
	closed := first.closed
	first.mu.Unlock()
	if !closed {
		t.Error("the superseded tool session was never closed")
	}

	oneTurn(t, svc, rec)
	if *registrations != 2 {
		t.Errorf("a turn after the rebuild built another server (%d total), want 2 — "+
			"the new session is not being cached", *registrations)
	}
}

// One reading per turn is not enough on its own if a SECOND consumer goes back
// to the store. Until this round the registrar did exactly that: Run resolved
// the switches, and mcphost.Register then read them again to decide which
// tools to install. A settings.Set landing in that window built a session
// whose TOOLS came from the second reading and whose LABEL — the value
// Service.session caches on — came from the first.
//
// The label being the cache key is what turns a one-turn slip into a
// permanent one. Once the writer's switches are back where they started, every
// later turn resolves the same label, finds the cached session, and is handed
// the tool set built during the window — for the life of the process.
//
// So this runs three turns with the switches steady at both-on, and flips them
// off and back exactly ONCE, between the two readings of the first turn. No
// goroutine, no sleep: the flip is performed by the switch accessor itself,
// after it has answered, which is precisely the interleaving the race under
// -race was only sampling.
func TestRun_aFlipBetweenTheTwoReadingsIsNotCachedForever(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{textReply("ok")}}
	svc, cfg, _ := budgetService(t, c, rec)
	setToolSwitches(t, cfg, true, true)

	// The window. resolveToolGroups reads memory first and skills second, so
	// hanging the flip off the skills accessor puts it after the whole
	// snapshot has been taken and before anything else can read the store.
	// It answers with the value the writer actually set; the store is what
	// moves underneath.
	flipped := false
	svc.deps.SkillToolsEnabled = func() bool {
		answer := cfg.SkillToolsEnabled()
		if !flipped {
			flipped = true
			setToolSwitches(t, cfg, false, false)
		}
		return answer
	}

	for turn := 1; turn <= 3; turn++ {
		oneTurn(t, svc, rec)
		if turn == 1 {
			if !flipped {
				t.Fatal("test setup: the flip never happened, so nothing was interleaved")
			}
			// The writer's switches, back where the writer left them. From
			// here on every turn resolves both-on — and a turn that is handed
			// the cached session from the window gets 16 tools with a 19-tool
			// prompt.
			setToolSwitches(t, cfg, true, true)
		}

		system := c.messages()[0].Content
		offered := offeredTools(c.options().Tools)

		// Both switches were on for the whole of every one of these turns.
		for _, name := range []string{"linetta_edit_memory", "linetta_read_skill", "linetta_edit_skill"} {
			if !offered[name] {
				t.Errorf("turn %d was not offered %s, though both switches were on for the "+
					"whole turn: the tools came from a second reading of the store, taken "+
					"inside the window the flip opened", turn, name)
			}
		}
		// The invariant itself, on the turn's own two halves.
		for _, name := range promptToolName.FindAllString(system, -1) {
			if !offered[name] {
				t.Errorf("turn %d: the system prompt tells the agent to call %s, but the same "+
					"turn offered %d tools and not that one", turn, name, len(offered))
			}
		}
	}
}

// A session released twice reports it. Nothing in the tree does that today —
// every acquire has exactly one release, and `closed` would stop a second one
// from closing anything — but a refcount that silently sits at -1 is a session
// a later retire can close while another holder is still calling tools on it.
func TestToolSession_aSecondReleaseIsReportedAndChangesNothing(t *testing.T) {
	ts, err := connectTools(context.Background(), stubTools(nil), allToolGroups())
	if err != nil {
		t.Fatalf("connectTools: %v", err)
	}
	ts.acquire()
	if err := ts.retire(); err != nil {
		t.Fatalf("retire: %v", err)
	}
	ts.mu.Lock()
	closedWhileHeld := ts.closed
	ts.mu.Unlock()
	if closedWhileHeld {
		t.Fatal("retire closed a session someone was still holding")
	}
	if err := ts.release(); err != nil {
		t.Fatalf("the last release: %v", err)
	}

	if err := ts.release(); err == nil {
		t.Error("a second release for the same reference was accepted in silence")
	}
	ts.mu.Lock()
	refs, closed := ts.refs, ts.closed
	ts.mu.Unlock()
	if refs < 0 {
		t.Errorf("the refcount is %d: a later acquire/release pair would drop it to -1 "+
			"again and let a retire close the session under its holder", refs)
	}
	if !closed {
		t.Error("the extra release reopened the session's closed flag")
	}
}
