//go:build !mobile

package mcphost

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

// Every group name has to be a tool that exists. A group naming a tool the
// server never registers would discount the Settings pane's budget by bytes
// nothing was ever going to send, and ToolNames would filter a name that is
// not there — both silent.
func TestToolGroupNamesAreAllRealTools(t *testing.T) {
	all := append(append([]string{}, ReadToolNames...), WriteToolNames...)
	groups := [][]string{MemoryToolNames, SkillToolNames, ReviewToolNames}
	for _, group := range groups {
		for _, n := range group {
			if !slices.Contains(all, n) {
				t.Errorf("group names %q, which is in neither ReadToolNames nor WriteToolNames", n)
			}
		}
	}
	// And no two groups may overlap: a tool counted twice would make the
	// pane's arithmetic wrong in a way nothing else notices.
	for i, a := range groups {
		for j, b := range groups {
			if i >= j {
				continue
			}
			for _, n := range a {
				if slices.Contains(b, n) {
					t.Errorf("%q is in two tool-budget groups at once", n)
				}
			}
		}
	}
}

// ToolNames is what the Settings pane counts, and this is what stops that
// count drifting from what an agent is actually handed: for every mode and
// every combination of the switches, the list must be exactly what a real
// tools/list returns over an in-memory transport.
func TestToolNamesMatchesWhatTheServerActuallyServes(t *testing.T) {
	for _, mode := range []string{settings.MCPModeReadOnly, settings.MCPModeFull} {
		for _, groups := range []ToolGroups{
			{Memory: true, Skills: true, Review: true},
			{Memory: false, Skills: true, Review: true},
			{Memory: true, Skills: false, Review: true},
			{Memory: true, Skills: true, Review: false},
			{Memory: false, Skills: false, Review: false},
		} {
			t.Run(nameFor(mode, groups), func(t *testing.T) {
				// Sorted on both sides: tools/list returns names in the
				// server's own order (alphabetical), and what is being
				// asserted is the SET a client is offered, not the order
				// AddTool happened to run in.
				want := slices.Sorted(slices.Values(ToolNames(mode, groups)))
				got := slices.Sorted(slices.Values(registeredToolNamesWithGroups(t, mode, groups)))
				if !slices.Equal(want, got) {
					t.Errorf("ToolNames says\n  %v\nbut the server serves\n  %v", want, got)
				}
			})
		}
	}
}

// The whole point of the feature, asserted over a real tools/list rather than
// against the Go list above: with a group off, its tools are not merely
// refused, they are absent — in read_only as well as full, because
// linetta_read_skill is a READ tool and a read-only server is exactly where
// forgetting that would go unnoticed.
func TestAGroupThatIsOffIsAbsentFromToolsListInBothModes(t *testing.T) {
	for _, mode := range []string{settings.MCPModeReadOnly, settings.MCPModeFull} {
		t.Run(mode, func(t *testing.T) {
			off := registeredToolNamesWithGroups(t, mode, ToolGroups{})
			optional := append(append(append([]string{}, MemoryToolNames...), SkillToolNames...), ReviewToolNames...)
			for _, n := range optional {
				if slices.Contains(off, n) {
					t.Errorf("%s: %q is still served with its group switched off", mode, n)
				}
			}
			// Everything else survives: switching a group off must not cost
			// the writer a manuscript tool.
			on := registeredToolNamesWithGroups(t, mode, AllToolGroups())
			for _, n := range on {
				if slices.Contains(optional, n) {
					continue
				}
				if !slices.Contains(off, n) {
					t.Errorf("%s: %q disappeared, and it is in neither group", mode, n)
				}
			}
		})
	}
}

// The default is the tool set 1.2 shipped: nothing changes for a writer who
// never opens the pane.
func TestDefaultsServeTheWholeToolSet(t *testing.T) {
	got := registeredToolNamesWithGroups(t, settings.MCPModeFull, AllToolGroups())
	if len(got) != len(ReadToolNames)+len(WriteToolNames) {
		t.Errorf("a default full server serves %d tools, want %d",
			len(got), len(ReadToolNames)+len(WriteToolNames))
	}
	// And no settings store at all falls back to the same thing, which is
	// what keeps a build with no store open (and every test that registers a
	// zero ToolDeps) on the documented tool set.
	if g := ToolGroupsFrom(nil); g != AllToolGroups() {
		t.Errorf("a nil settings store yields %+v, want every group on", g)
	}
}

// The budget is measured from the tools the server actually serves, so the
// number the pane shows is the number the agent gets — and each group's cost
// stays reported whether or not it is currently on, because that is the price
// the writer is deciding about.
func TestMeasureToolBudgetTracksTheSwitches(t *testing.T) {
	full, ok := MeasureToolBudget(true, true, true)
	if !ok {
		t.Fatal("the tool set could not be measured at all")
	}
	if full.Tools != len(ReadToolNames)+len(WriteToolNames) {
		t.Fatalf("default budget = %d tools, want %d (measurement failed?)",
			full.Tools, len(ReadToolNames)+len(WriteToolNames))
	}
	if full.Bytes <= 0 || full.Memory.Bytes <= 0 || full.Skills.Bytes <= 0 || full.Review.Bytes <= 0 {
		t.Fatalf("budget carries no bytes: %+v", full)
	}
	if full.Memory.Tools != len(MemoryToolNames) || full.Skills.Tools != len(SkillToolNames) || full.Review.Tools != len(ReviewToolNames) {
		t.Errorf("group tool counts = memory %d skills %d review %d, want %d, %d and %d",
			full.Memory.Tools, full.Skills.Tools, full.Review.Tools,
			len(MemoryToolNames), len(SkillToolNames), len(ReviewToolNames))
	}

	none, _ := MeasureToolBudget(false, false, false)
	groupTools := full.Memory.Tools + full.Skills.Tools + full.Review.Tools
	groupBytes := full.Memory.Bytes + full.Skills.Bytes + full.Review.Bytes
	if none.Tools != full.Tools-groupTools {
		t.Errorf("with every group off the budget is %d tools; %d minus the groups is %d",
			none.Tools, full.Tools, full.Tools-groupTools)
	}
	if none.Bytes != full.Bytes-groupBytes {
		t.Errorf("with every group off the budget is %d bytes; %d minus the groups is %d",
			none.Bytes, full.Bytes, full.Bytes-groupBytes)
	}
	if none.Memory.Bytes != full.Memory.Bytes || none.Skills.Bytes != full.Skills.Bytes || none.Review.Bytes != full.Review.Bytes {
		t.Error("a switched-off group stopped reporting what it costs, so the pane can no longer " +
			"tell the writer what switching it back on would buy")
	}
}

// Three tools carried most of the tool budget's weight: linetta_apply_story_ops,
// linetta_edit_skill and linetta_read_skill together cost 8,282B, mostly
// description prose that restated their own JSON schema (#132). This pins
// what #132 measured after trimming them — a ceiling well below the old
// total, not the old total itself, so a description creeping back up trips
// it long before it gets back to 8,282B.
func TestThreeTrimmedToolsStayUnderTheirSharedCeiling(t *testing.T) {
	sizes := toolSchemaBytes()
	if len(sizes) == 0 {
		t.Fatal("could not measure tool schema bytes")
	}

	three := []string{"linetta_apply_story_ops", "linetta_edit_skill", "linetta_read_skill"}
	sum := 0
	for _, n := range three {
		b, ok := sizes[n]
		if !ok {
			t.Fatalf("%s missing from the measured tool set", n)
		}
		sum += b
	}
	// #132 measured 7,120B after trimming (down from 8,282B). 7,400B leaves
	// slack for incidental SDK schema-rendering drift without hiding a real
	// description regrowth.
	if sum > 7400 {
		t.Errorf("linetta_apply_story_ops + linetta_edit_skill + linetta_read_skill = %dB, want <= 7400B "+
			"(#132 trimmed them from 8282B to 7120B)", sum)
	}

	total, ledgerTools := 0, 0
	for name, b := range sizes {
		// Keep the original ceiling intact; #154's two new tools have their own allowance.
		if name == "linetta_get_character_visuals" || name == "linetta_build_illustration_prompt" {
			continue
		}
		if slices.Contains(VisualWriteToolNames, name) {
			continue
		}
		// Illustration Phase 4's three tools carry their own allowance below.
		if name == "linetta_list_illustrations" || slices.Contains(IllustrationWriteToolNames, name) {
			continue
		}
		// #162's style check and #163's proposal carry their own allowances below.
		if name == "linetta_analyze_style" || name == "linetta_propose_style_profile" {
			continue
		}
		// Phase 3's publish tools carry their own allowance below.
		if name == "linetta_publish_status" || name == "linetta_publish_episode" || name == "linetta_register_work" || name == "linetta_publish_cover" {
			continue
		}
		// story-assist Phase 2's report tool, Phase 3's propose tool, and
		// Phase 4's save-forecast tool carry their own allowances below.
		if name == "linetta_report_continuity" || name == "linetta_propose_story_ops" || name == "linetta_save_forecast" {
			continue
		}
		if name == "linetta_list_ledger" || name == "linetta_edit_ledger" || name == "linetta_set_part_arc" {
			ledgerTools += b
			continue
		}
		total += b
	}
	// #162 measured 1,815B for linetta_analyze_style. #163 added the
	// measurements to its output as an untyped field, which is why that cost
	// only 72B (1,887B) rather than the ~900B a typed schema would have.
	if style := sizes["linetta_analyze_style"]; style > 1950 {
		t.Errorf("linetta_analyze_style costs %dB, want <= 1950B", style)
	}
	// #163 measured 868B for the proposal tool.
	if propose := sizes["linetta_propose_style_profile"]; propose > 950 {
		t.Errorf("linetta_propose_style_profile costs %dB, want <= 950B", propose)
	}
	// Phase 3 measured 1,958B for the publish pair (1,231B status, 727B
	// publish), after cutting the output schema to what an agent needs.
	// linetta_register_work joined them at 690B (publish now 694B): 2,615B.
	// linetta_publish_cover and the status tool's cover field brought the
	// four to 3,527B.
	if pub := sizes["linetta_publish_status"] + sizes["linetta_publish_episode"] + sizes["linetta_register_work"] + sizes["linetta_publish_cover"]; pub > 3600 {
		t.Errorf("publish tools cost %dB, want <= 3600B", pub)
	}
	// story-assist Phase 2 measured 3,436B for linetta_report_continuity —
	// the nested observations[].refs[] schema is most of it, and that
	// structure is the whole point of engine-side validation.
	if report := sizes["linetta_report_continuity"]; report > 3600 {
		t.Errorf("linetta_report_continuity costs %dB, want <= 3600B", report)
	}
	// story-assist Phase 3 measured 7,849B for linetta_propose_story_ops —
	// the items[].op schema is the whole storyops.Op struct (every op's
	// fields, shared with linetta_apply_story_ops), which is the cost of
	// one schema describing every allowed op rather than eight narrower
	// ones.
	if propose := sizes["linetta_propose_story_ops"]; propose > 8100 {
		t.Errorf("linetta_propose_story_ops costs %dB, want <= 8100B", propose)
	}
	// story-assist Phase 4's save-forecast tool measured 3,701B: branches[]
	// nests beats[], decisions[], changes{} and risks[] — one schema
	// describing a whole branch, repeated up to 5 times per call.
	if save := sizes["linetta_save_forecast"]; save > 3900 {
		t.Errorf("linetta_save_forecast costs %dB, want <= 3900B", save)
	}
	writes := 0
	for _, name := range VisualWriteToolNames {
		writes += sizes[name]
	}
	if writes > 7000 {
		t.Errorf("visual writes cost %dB, want <= 7000B", writes)
	}
	visuals := sizes["linetta_get_character_visuals"] + sizes["linetta_build_illustration_prompt"]
	// #154 measured 3,468B for the pair, input and output schemas included.
	if visuals > 3500 {
		t.Errorf("visual tools cost %dB, want <= 3500B", visuals)
	}
	// Illustration Phase 4 measured 3,205B for its three tools (list 1,129B,
	// request 1,369B, set representative 707B). The request's
	// prior_illustration_ids (earlier pictures sent along, in and out)
	// brought it to 3,468B.
	illustrations := sizes["linetta_list_illustrations"]
	for _, name := range IllustrationWriteToolNames {
		illustrations += sizes[name]
	}
	if illustrations > 3550 {
		t.Errorf("illustration tools cost %dB, want <= 3550B", illustrations)
	}
	// #132 measured 26,525B for the full 19-tool set (down from 27,687B).
	// story-assist Phase 1 added linetta_get_story_context's IncludeDirection
	// field (the switch behind storycontext.ContextKeyDirection, matching
	// IncludeSkills), measured at 26,976B. story-assist Phase 2 added
	// linetta_read_scene's numbered field, measured at 27,279B. story-assist
	// Phase 3 (3.A.2) added storyops.Op.Aliases, which linetta_apply_story_ops
	// shares, measured at 27,446B — the ceiling moves up with it;
	// linetta_report_continuity carries its own allowance above and is
	// excluded from this total. The long-form work added the plan field that
	// linetta_apply_story_ops and linetta_write_summary share, measured at
	// 27,618B; the ledger tools carry their own allowance, 4,500B.
	if ledgerTools > 4500 {
		t.Errorf("the ledger tools cost %dB, want <= 4500B", ledgerTools)
	}
	if total > 27650 {
		t.Errorf("the original tool budget (excluding linetta_report_continuity) is %dB, want <= 27650B "+
			"(#132 trimmed it from 27687B to 26525B; story-assist raised it to 27100B, 27400B, then 27500B)", total)
	}
}

// Guard against the three descriptions themselves growing back: #132 set a
// byte target for each Description string (not the surrounding schema) so
// that reviewing a future PR against this test catches a clause creeping
// back in one sentence at a time.
func TestTrimmedDescriptionsFitTheirByteTargets(t *testing.T) {
	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: ServerVersion}, nil)
	ToolDeps{}.Register(srv, settings.MCPModeFull, AllToolGroups())

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer func() { _ = ss.Close() }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "linetta-budget-test", Version: ServerVersion}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer func() { _ = cs.Close() }()

	targets := map[string]int{
		"linetta_apply_story_ops": 300,
		"linetta_edit_skill":      600,
		"linetta_read_skill":      280,
	}
	seen := map[string]bool{}
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		limit, ok := targets[tool.Name]
		if !ok {
			continue
		}
		seen[tool.Name] = true
		if n := len(tool.Description); n > limit {
			t.Errorf("%s description is %dB, want <= %dB: %q", tool.Name, n, limit, tool.Description)
		}
	}
	for name := range targets {
		if !seen[name] {
			t.Errorf("%s was not served, could not check its description", name)
		}
	}
}

func nameFor(mode string, g ToolGroups) string {
	s := mode
	if g.Memory {
		s += "+memory"
	}
	if g.Skills {
		s += "+skills"
	}
	if g.Review {
		s += "+review"
	}
	return s
}
