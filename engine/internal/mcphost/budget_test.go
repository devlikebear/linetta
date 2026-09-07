//go:build !mobile

package mcphost

import (
	"slices"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

// Every group name has to be a tool that exists. A group naming a tool the
// server never registers would discount the Settings pane's budget by bytes
// nothing was ever going to send, and ToolNames would filter a name that is
// not there — both silent.
func TestToolGroupNamesAreAllRealTools(t *testing.T) {
	all := append(append([]string{}, ReadToolNames...), WriteToolNames...)
	for _, group := range [][]string{MemoryToolNames, SkillToolNames} {
		for _, n := range group {
			if !slices.Contains(all, n) {
				t.Errorf("group names %q, which is in neither ReadToolNames nor WriteToolNames", n)
			}
		}
	}
	// And the two groups must not overlap: a tool counted twice would make
	// the pane's arithmetic wrong in a way nothing else notices.
	for _, n := range MemoryToolNames {
		if slices.Contains(SkillToolNames, n) {
			t.Errorf("%q is in both the memory and the skills group", n)
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
			{Memory: true, Skills: true},
			{Memory: false, Skills: true},
			{Memory: true, Skills: false},
			{Memory: false, Skills: false},
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
			for _, n := range append(append([]string{}, MemoryToolNames...), SkillToolNames...) {
				if slices.Contains(off, n) {
					t.Errorf("%s: %q is still served with its group switched off", mode, n)
				}
			}
			// Everything else survives: switching a group off must not cost
			// the writer a manuscript tool.
			on := registeredToolNamesWithGroups(t, mode, AllToolGroups())
			for _, n := range on {
				if slices.Contains(MemoryToolNames, n) || slices.Contains(SkillToolNames, n) {
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
	// And a ToolDeps with no settings store at all falls back to the same
	// thing, which is what keeps a build with no store open (and every test
	// that registers a zero ToolDeps) on the documented tool set.
	if g := (ToolDeps{}).toolGroups(); g != AllToolGroups() {
		t.Errorf("a nil settings store yields %+v, want every group on", g)
	}
}

// The budget is measured from the tools the server actually serves, so the
// number the pane shows is the number the agent gets — and each group's cost
// stays reported whether or not it is currently on, because that is the price
// the writer is deciding about.
func TestMeasureToolBudgetTracksTheSwitches(t *testing.T) {
	full := MeasureToolBudget(true, true)
	if full.Tools != len(ReadToolNames)+len(WriteToolNames) {
		t.Fatalf("default budget = %d tools, want %d (measurement failed?)",
			full.Tools, len(ReadToolNames)+len(WriteToolNames))
	}
	if full.Bytes <= 0 || full.Memory.Bytes <= 0 || full.Skills.Bytes <= 0 {
		t.Fatalf("budget carries no bytes: %+v", full)
	}
	if full.Memory.Tools != len(MemoryToolNames) || full.Skills.Tools != len(SkillToolNames) {
		t.Errorf("group tool counts = memory %d skills %d, want %d and %d",
			full.Memory.Tools, full.Skills.Tools, len(MemoryToolNames), len(SkillToolNames))
	}

	none := MeasureToolBudget(false, false)
	if none.Tools != full.Tools-full.Memory.Tools-full.Skills.Tools {
		t.Errorf("with both groups off the budget is %d tools; %d minus the two groups is %d",
			none.Tools, full.Tools, full.Tools-full.Memory.Tools-full.Skills.Tools)
	}
	if none.Bytes != full.Bytes-full.Memory.Bytes-full.Skills.Bytes {
		t.Errorf("with both groups off the budget is %d bytes; %d minus the two groups is %d",
			none.Bytes, full.Bytes, full.Bytes-full.Memory.Bytes-full.Skills.Bytes)
	}
	if none.Memory.Bytes != full.Memory.Bytes || none.Skills.Bytes != full.Skills.Bytes {
		t.Error("a switched-off group stopped reporting what it costs, so the pane can no longer " +
			"tell the writer what switching it back on would buy")
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
	return s
}
