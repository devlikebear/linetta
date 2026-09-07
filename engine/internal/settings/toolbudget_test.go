package settings

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tool budget's two switches, through all five places a settings field
// can be silently lost — defaults, the accessor, persist()'s allowlist,
// load()'s presence guard, and Set. This is the sibling of
// TestAgentSelfReviewEnabled_defaultsOnAndSurvivesADeliberateFalse and it is
// written the same way for the same reason: both directions are tested,
// because switching a default-true field OFF and reloading passes even when
// the field is missing from persist()'s allowlist (an unlisted field is still
// written — as false, which is what was wanted). Only switching it back ON
// catches that.
func TestToolBudgetSwitches_defaultOnAndSurviveADeliberateFalse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	ctx := context.Background()

	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, _ := s.Get(ctx)
	if !got.MemoryToolsEnabled || !got.SkillToolsEnabled {
		t.Fatalf("the tool groups do not default to on; a fresh install would ship an agent "+
			"with no memory and no skills: %+v", got)
	}
	if !s.MemoryToolsEnabled() || !s.SkillToolsEnabled() {
		t.Error("the accessors the tool layer reads disagree with the config")
	}

	if _, err := s.Set(ctx, Patch{
		MemoryToolsEnabled: boolPtr(false),
		SkillToolsEnabled:  boolPtr(false),
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if s.MemoryToolsEnabled() || s.SkillToolsEnabled() {
		t.Error("switching the tool groups off did not take effect in memory")
	}

	s2, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	if s2.MemoryToolsEnabled() || s2.SkillToolsEnabled() {
		t.Error("a deliberate false did not survive a reload — either persist() never wrote it, " +
			"or load() overwrote it with the default")
	}

	if _, err := s2.Set(ctx, Patch{
		MemoryToolsEnabled: boolPtr(true),
		SkillToolsEnabled:  boolPtr(true),
	}); err != nil {
		t.Fatalf("Set back on: %v", err)
	}
	s3, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	if !s3.MemoryToolsEnabled() || !s3.SkillToolsEnabled() {
		t.Error("switching the tool groups back on did not survive a reload — " +
			"memory_tools_enabled / skill_tools_enabled are missing from persist()'s allowlist")
	}

	// One switch off must not drag the other with it: they are two decisions,
	// and a writer who kept skills and dropped memory has to get that.
	if _, err := s3.Set(ctx, Patch{MemoryToolsEnabled: boolPtr(false)}); err != nil {
		t.Fatalf("Set memory off: %v", err)
	}
	if s3.MemoryToolsEnabled() || !s3.SkillToolsEnabled() {
		t.Errorf("the two switches are not independent: memory=%v skills=%v",
			s3.MemoryToolsEnabled(), s3.SkillToolsEnabled())
	}

	// A settings.json written by a build that predates the keys must still get
	// the defaults, not the zero value.
	legacy := t.TempDir()
	t.Setenv("LINETTA_HOME", legacy)
	if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte(`{"language":"en"}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s4, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("legacy New: %v", err)
	}
	if !s4.MemoryToolsEnabled() || !s4.SkillToolsEnabled() {
		t.Error("a settings.json written before these keys existed reads as a deliberate false, " +
			"so upgrading would silently take the memory and skills tools away")
	}
}

// The measured budget reaches settings.get, and it describes the switches as
// they stand rather than a fixed snapshot. Without a producer wired there is
// no block at all — a mobile build must not report numbers it cannot measure.
func TestToolBudget_reportedOnlyWhenMeasured(t *testing.T) {
	ctx := context.Background()
	s := newStoreOnTemp(t)

	got, err := s.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ToolBudget != nil {
		t.Errorf("a store with no measurement wired reported a budget anyway: %+v", got.ToolBudget)
	}

	var sawMemory, sawSkills bool
	s.WithToolBudget(func(memoryTools, skillTools bool) ToolBudget {
		sawMemory, sawSkills = memoryTools, skillTools
		b := ToolBudget{
			Tools:  16,
			Bytes:  21694,
			Memory: ToolBudgetGroup{Tools: 1, Bytes: 1357},
			Skills: ToolBudgetGroup{Tools: 2, Bytes: 4636},
		}
		if memoryTools {
			b.Tools += b.Memory.Tools
			b.Bytes += b.Memory.Bytes
		}
		if skillTools {
			b.Tools += b.Skills.Tools
			b.Bytes += b.Skills.Bytes
		}
		return b
	})

	got, err = s.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ToolBudget == nil {
		t.Fatal("a wired measurement did not reach settings.get")
	}
	if !sawMemory || !sawSkills {
		t.Errorf("the measurement was not asked about the switches as they stand: memory=%v skills=%v",
			sawMemory, sawSkills)
	}
	if got.ToolBudget.Tools != 19 {
		t.Errorf("default budget = %d tools, want 19", got.ToolBudget.Tools)
	}

	if _, err := s.Set(ctx, Patch{SkillToolsEnabled: boolPtr(false)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err = s.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ToolBudget.Tools != 17 {
		t.Errorf("budget with skills off = %d tools, want 17 — the pane's count does not track the switch",
			got.ToolBudget.Tools)
	}
	// The price of what was given up stays visible: a writer cannot decide
	// whether to switch a group back on if the pane only reports the state
	// they are already in.
	if got.ToolBudget.Skills.Tools != 2 {
		t.Errorf("a switched-off group reported %d tools; the pane can no longer say what it costs",
			got.ToolBudget.Skills.Tools)
	}
}

// The budget is derived, so it must never be written to settings.json — a
// measurement on disk outlives the tool set it measured, which is the same
// rule the legacy-plaintext fields follow.
func TestToolBudget_neverPersisted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	ctx := context.Background()
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.WithToolBudget(func(bool, bool) ToolBudget {
		return ToolBudget{Tools: 19, Bytes: 27687}
	})
	if _, err := s.Set(ctx, Patch{MemoryToolsEnabled: boolPtr(false)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	if strings.Contains(string(body), "tool_budget") {
		t.Errorf("settings.json carries the derived tool budget:\n%s", body)
	}
	if !strings.Contains(string(body), "memory_tools_enabled") {
		t.Errorf("settings.json is missing memory_tools_enabled:\n%s", body)
	}
}
