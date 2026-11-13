package settings

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// The four settings below were once accepted by Set and kept in memory, but
// missing from persistWith and load(), so the writer's choice was gone at the
// next launch.
func TestAgentPassesAndAvoidPhrases_surviveRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	ctx := context.Background()

	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.AgentEpisodeReviewEnabled() || s.AgentSettlementEnabled() || s.AgentImpactReviewEnabled() || s.SummarizerProviderEnabled() {
		t.Fatalf("a fresh install has a manuscript-resending pass switched on")
	}
	if got := s.StyleAvoidPhrases(); len(got) != 0 {
		t.Fatalf("fresh install avoid phrases = %q, want none", got)
	}

	phrases := []string{"그녀는 미소 지었다", "심장이 쿵"}
	if _, err := s.Set(ctx, Patch{
		AgentEpisodeReviewEnabled: boolPtr(true),
		AgentSettlementEnabled:    boolPtr(true),
		AgentImpactReviewEnabled:  boolPtr(true),
		SummarizerProviderEnabled: boolPtr(true),
		StyleAvoidPhrases:         &phrases,
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	s2, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	if !s2.AgentEpisodeReviewEnabled() {
		t.Error("agent_episode_review_enabled lost on restart")
	}
	if !s2.AgentSettlementEnabled() {
		t.Error("agent_settlement_enabled lost on restart")
	}
	if !s2.AgentImpactReviewEnabled() {
		t.Error("agent_impact_review_enabled lost on restart")
	}
	if !s2.SummarizerProviderEnabled() {
		t.Error("summarizer_provider_enabled lost on restart")
	}
	if got := s2.StyleAvoidPhrases(); !slices.Equal(got, phrases) {
		t.Errorf("style_avoid_phrases after restart = %q, want %q", got, phrases)
	}

	// Switching back off must survive too, not just switching on.
	empty := []string{}
	if _, err := s2.Set(ctx, Patch{
		AgentEpisodeReviewEnabled: boolPtr(false),
		AgentSettlementEnabled:    boolPtr(false),
		AgentImpactReviewEnabled:  boolPtr(false),
		StyleAvoidPhrases:         &empty,
	}); err != nil {
		t.Fatalf("Set off: %v", err)
	}
	s3, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	if s3.AgentEpisodeReviewEnabled() || s3.AgentSettlementEnabled() || s3.AgentImpactReviewEnabled() {
		t.Error("a pass switched off came back on after restart")
	}
	if got := s3.StyleAvoidPhrases(); len(got) != 0 {
		t.Errorf("cleared avoid phrases came back after restart: %q", got)
	}
}

// notPersisted lists the Config fields that are deliberately never written to
// settings.json. Every other json field must survive persistWith → load().
// Adding a field here needs a reason; forgetting the field in persistWith or
// load() fails TestConfig_everyFieldSurvivesRestart instead.
var notPersisted = map[string]string{
	"BackupDir":                "computed from LINETTA_HOME at load time",
	"WebSearchAPIKey":          "secret; lives in the SecretStore",
	"WebSearchAPIKeySet":       "derived presence flag for settings.get",
	"MCPTokenSet":              "derived presence flag for settings.get",
	"ToolBudget":               "measured at settings.get time",
	"LegacyPlaintextProviders": "derived at load time (#113)",
	"LegacyPlaintextWebSearch": "derived at load time (#113)",
	"LegacyPlaintextReason":    "derived at load time (#113)",
	"LegacyPlaintextPath":      "derived at load time (#113)",
}

// validSeeds are non-default values for the fields load() validates, so an
// arbitrary value would be rejected and look like a missing field. Every other
// field gets a generic non-default value from nonDefaultValue.
func validSeeds() map[string]any {
	return map[string]any{
		"Language":           "en",
		"Provider":           "openai",
		"Providers":          map[string]ProviderConfig{"openai": {Model: "gpt-test"}},
		"Theme":              "dark",
		"Palette":            validPalettes()[len(validPalettes())-1],
		"EditorFontSize":     17,
		"EditorLineHeight":   2.0,
		"CopyProfile":        validCopyProfiles()[len(validCopyProfiles())-1],
		"WebSearchProvider":  "tavily",
		"MCPMode":            MCPModeReadOnly,
		"MCPPort":            DefaultMCPPort + 1,
		"AgentMaxToolCalls":  MinAgentMaxToolCalls + 1,
		"AgentHistoryBudget": MinAgentHistoryBudget + 1,
		"PublishAccount":     &PublishAccount{Handle: "writer"},
		"CharCountPresets": []CharCountPreset{{
			ID: "persist-check", Name: "persist-check", CharCountBasis: "without_spaces", EpisodeCharTarget: 4321,
		}},
		"ImageProvider":  ValidImageProviders()[0],
		"ImageProviders": map[string]ImageProviderConfig{ValidImageProviders()[0]: {Model: "img-test"}},
	}
}

func nonDefaultValue(t *testing.T, name string, def reflect.Value) reflect.Value {
	t.Helper()
	typ := def.Type()
	switch typ.Kind() {
	case reflect.Bool:
		// The opposite of the default, so a default-on switch is checked for
		// a saved false and a default-off one for a saved true.
		return reflect.ValueOf(!def.Bool())
	case reflect.String:
		return reflect.ValueOf("persist-check").Convert(typ)
	case reflect.Int, reflect.Int64:
		return reflect.ValueOf(7).Convert(typ)
	case reflect.Slice:
		if typ.Elem().Kind() == reflect.String {
			return reflect.ValueOf([]string{"persist-check"}).Convert(typ)
		}
	}
	t.Fatalf("Config.%s (%s): add a valid non-default value to validSeeds()", name, typ)
	return reflect.Value{}
}

func TestConfig_everyFieldSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	seeds := validSeeds()
	want := defaults(dir)
	wv := reflect.ValueOf(&want).Elem()
	dv := reflect.ValueOf(defaults(dir))
	ct := wv.Type()
	var checked []string
	for i := range ct.NumField() {
		f := ct.Field(i)
		if f.Tag.Get("json") == "" || f.Tag.Get("json") == "-" {
			continue
		}
		if _, skip := notPersisted[f.Name]; skip {
			continue
		}
		var v reflect.Value
		if seed, ok := seeds[f.Name]; ok {
			v = reflect.ValueOf(seed).Convert(f.Type)
		} else {
			v = nonDefaultValue(t, f.Name, dv.Field(i))
		}
		if reflect.DeepEqual(v.Interface(), dv.Field(i).Interface()) {
			t.Fatalf("Config.%s seed equals the default; the check could not tell it was lost", f.Name)
		}
		wv.Field(i).Set(v)
		checked = append(checked, f.Name)
	}

	if err := s.persistWith(want, legacyPlaintextKeys{}); err != nil {
		t.Fatalf("persistWith: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var onDisk map[string]json.RawMessage
	if err := json.Unmarshal(body, &onDisk); err != nil {
		t.Fatalf("decode settings.json: %v", err)
	}

	s2, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	s2.mu.RLock()
	got := s2.cfg
	s2.mu.RUnlock()
	gv := reflect.ValueOf(got)

	for _, name := range checked {
		f, _ := ct.FieldByName(name)
		key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if _, ok := onDisk[key]; !ok {
			t.Errorf("Config.%s (%q) is not written by persistWith: add it to the persistable literal, or to notPersisted with a reason", name, key)
			continue
		}
		if g, w := gv.FieldByName(name).Interface(), wv.FieldByName(name).Interface(); !reflect.DeepEqual(g, w) {
			t.Errorf("Config.%s (%q) after restart = %#v, want %#v: load() does not read it back", name, key, g, w)
		}
	}
}

// Longform Phase 6: the moment settlement is switched on is kept, so the scenes
// written before it never become a queue — across restarts, and for a writer
// who switched it on before the moment was recorded.
func TestSettlementSince_isSetWhenSwitchedOnAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	ctx := context.Background()

	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	if since, on := s.AgentSettlementSince(); on || since != 0 {
		t.Fatalf("fresh install = since %d on %v", since, on)
	}
	before := time.Now().UnixMilli()
	if _, err := s.Set(ctx, Patch{AgentSettlementEnabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	since, on := s.AgentSettlementSince()
	if !on || since < before {
		t.Fatalf("after switching on = since %d on %v, want on since %d or later", since, on, before)
	}
	// Setting it on again is not switching it on.
	if _, err := s.Set(ctx, Patch{AgentSettlementEnabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.AgentSettlementSince(); again != since {
		t.Errorf("since moved from %d to %d on a patch that changed nothing", since, again)
	}
	s2, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	if got, on := s2.AgentSettlementSince(); !on || got != since {
		t.Errorf("after restart = since %d on %v, want %d", got, on, since)
	}

	// A settings.json from before the key: on, with no moment.
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"agent_settlement_enabled":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s3, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	backfilled, on := s3.AgentSettlementSince()
	if !on || backfilled == 0 {
		t.Fatalf("old file = since %d on %v, want the load moment", backfilled, on)
	}
	s4, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s4.AgentSettlementSince(); got != backfilled {
		t.Errorf("backfilled moment was not kept: %d then %d", backfilled, got)
	}
}
