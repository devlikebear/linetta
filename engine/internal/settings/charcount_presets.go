package settings

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/charcount"
)

// CharCountPreset is a length rule the writer saved to apply to a work in one
// step (#239): the basis a serial site or contest counts in, and the length of
// one episode under it. The built-in presets live in the app; these are the
// writer's own, kept across works.
type CharCountPreset struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	CharCountBasis    string `json:"char_count_basis"`
	EpisodeCharTarget int    `json:"episode_char_target"`
}

const (
	// MaxCharCountPresets bounds the list so a runaway caller cannot grow
	// settings.json without limit.
	MaxCharCountPresets = 50
	// MaxCharCountPresetNameRunes keeps a name short enough for the picker.
	MaxCharCountPresetNameRunes = 40
	// MaxCharCountPresetTarget is far past any episode a site accepts; it only
	// catches a typo with extra digits.
	MaxCharCountPresetTarget = 1_000_000
)

// normalizeCharCountPreset trims p and reports why it cannot be kept, if it
// cannot.
func normalizeCharCountPreset(p CharCountPreset) (CharCountPreset, error) {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	switch {
	case p.ID == "":
		return p, fmt.Errorf("settings: char count preset has no id")
	case p.Name == "":
		return p, fmt.Errorf("settings: char count preset %q has no name", p.ID)
	case utf8.RuneCountInString(p.Name) > MaxCharCountPresetNameRunes:
		return p, fmt.Errorf("settings: char count preset name longer than %d characters", MaxCharCountPresetNameRunes)
	case !charcount.Valid(p.CharCountBasis):
		return p, fmt.Errorf("settings: char count preset %q has unknown basis %q", p.ID, p.CharCountBasis)
	case p.EpisodeCharTarget <= 0 || p.EpisodeCharTarget > MaxCharCountPresetTarget:
		return p, fmt.Errorf("settings: char count preset %q episode_char_target %d out of range (1-%d)", p.ID, p.EpisodeCharTarget, MaxCharCountPresetTarget)
	}
	return p, nil
}

// validateCharCountPresets checks a whole list a patch would store.
func validateCharCountPresets(in []CharCountPreset) ([]CharCountPreset, error) {
	if len(in) > MaxCharCountPresets {
		return nil, fmt.Errorf("settings: more than %d char count presets", MaxCharCountPresets)
	}
	out := make([]CharCountPreset, 0, len(in))
	seen := map[string]bool{}
	for _, p := range in {
		p, err := normalizeCharCountPreset(p)
		if err != nil {
			return nil, err
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("settings: duplicate char count preset id %q", p.ID)
		}
		seen[p.ID] = true
		out = append(out, p)
	}
	return out, nil
}

// loadCharCountPresets keeps the presets on disk that are still valid. A hand
// edited or newer file must not cost the writer the rest of their list, so a
// bad entry is dropped rather than failing the load.
func loadCharCountPresets(in []CharCountPreset) []CharCountPreset {
	var out []CharCountPreset
	seen := map[string]bool{}
	for _, p := range in {
		if len(out) == MaxCharCountPresets {
			break
		}
		p, err := normalizeCharCountPreset(p)
		if err != nil || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		out = append(out, p)
	}
	return out
}
