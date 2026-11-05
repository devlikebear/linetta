package settings

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCharCountPresets_setPersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	ctx := context.Background()

	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if cfg, _ := s.Get(ctx); len(cfg.CharCountPresets) != 0 {
		t.Fatalf("fresh install presets = %v, want none", cfg.CharCountPresets)
	}

	presets := []CharCountPreset{
		{ID: "a", Name: "  내 공모전  ", CharCountBasis: "without_spaces", EpisodeCharTarget: 4000},
		{ID: "b", Name: "연재처", CharCountBasis: "with_spaces", EpisodeCharTarget: 5500},
	}
	got, err := s.Set(ctx, Patch{CharCountPresets: &presets})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []CharCountPreset{
		{ID: "a", Name: "내 공모전", CharCountBasis: "without_spaces", EpisodeCharTarget: 4000},
		{ID: "b", Name: "연재처", CharCountBasis: "with_spaces", EpisodeCharTarget: 5500},
	}
	if !reflect.DeepEqual(got.CharCountPresets, want) {
		t.Fatalf("Set returned %v, want %v", got.CharCountPresets, want)
	}

	s2, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	cfg, _ := s2.Get(ctx)
	if !reflect.DeepEqual(cfg.CharCountPresets, want) {
		t.Fatalf("presets after reload = %v, want %v — missing from persist() or load()", cfg.CharCountPresets, want)
	}

	// An unrelated patch keeps the list; an empty list clears it.
	if _, err := s2.Set(ctx, Patch{Theme: strPtr("dark")}); err != nil {
		t.Fatalf("Set theme: %v", err)
	}
	if cfg, _ := s2.Get(ctx); len(cfg.CharCountPresets) != 2 {
		t.Fatalf("unrelated patch changed presets to %v", cfg.CharCountPresets)
	}
	empty := []CharCountPreset{}
	if got, err := s2.Set(ctx, Patch{CharCountPresets: &empty}); err != nil || len(got.CharCountPresets) != 0 {
		t.Fatalf("clearing presets = %v, %v", got.CharCountPresets, err)
	}
}

func TestCharCountPresets_setRefusesInvalidList(t *testing.T) {
	t.Setenv("LINETTA_HOME", t.TempDir())
	ctx := context.Background()
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ok := CharCountPreset{ID: "ok", Name: "좋은 것", CharCountBasis: "with_spaces", EpisodeCharTarget: 5000}
	tooMany := make([]CharCountPreset, MaxCharCountPresets+1)
	for i := range tooMany {
		tooMany[i] = ok
		tooMany[i].ID = strings.Repeat("x", i+1)
	}
	cases := map[string][]CharCountPreset{
		"no id":         {{Name: "이름", CharCountBasis: "with_spaces", EpisodeCharTarget: 5000}},
		"blank name":    {{ID: "a", Name: "   ", CharCountBasis: "with_spaces", EpisodeCharTarget: 5000}},
		"long name":     {{ID: "a", Name: strings.Repeat("가", MaxCharCountPresetNameRunes+1), CharCountBasis: "with_spaces", EpisodeCharTarget: 5000}},
		"unknown basis": {{ID: "a", Name: "이름", CharCountBasis: "bytes", EpisodeCharTarget: 5000}},
		"zero target":   {{ID: "a", Name: "이름", CharCountBasis: "with_spaces", EpisodeCharTarget: 0}},
		"huge target":   {{ID: "a", Name: "이름", CharCountBasis: "with_spaces", EpisodeCharTarget: MaxCharCountPresetTarget + 1}},
		"duplicate ids": {ok, ok},
		"too many":      tooMany,
	}
	for name, list := range cases {
		if _, err := s.Set(ctx, Patch{CharCountPresets: &list}); err == nil {
			t.Errorf("%s: Set accepted %v", name, list)
		}
	}
	if cfg, _ := s.Get(ctx); len(cfg.CharCountPresets) != 0 {
		t.Fatalf("a refused patch stored %v", cfg.CharCountPresets)
	}
}

func TestCharCountPresets_loadDropsOnlyBadEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	body := `{"language":"ko","char_count_presets":[
		{"id":"good","name":"남는 것","char_count_basis":"without_spaces","episode_char_target":3000},
		{"id":"bad-basis","name":"버릴 것","char_count_basis":"bytes","episode_char_target":3000},
		{"id":"good","name":"중복","char_count_basis":"with_spaces","episode_char_target":3000},
		{"id":"bad-target","name":"버릴 것","char_count_basis":"with_spaces","episode_char_target":-1}
	]}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cfg, _ := s.Get(context.Background())
	want := []CharCountPreset{{ID: "good", Name: "남는 것", CharCountBasis: "without_spaces", EpisodeCharTarget: 3000}}
	if !reflect.DeepEqual(cfg.CharCountPresets, want) {
		t.Fatalf("loaded presets = %v, want %v", cfg.CharCountPresets, want)
	}
}
