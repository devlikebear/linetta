// Package mdmeta owns the structured Linetta metadata embedded in exported
// markdown. The visible markdown stays readable, while this frontmatter carries
// stable entity and relationship references for import round-trips.
package mdmeta

import (
	"strings"

	"gopkg.in/yaml.v3"
)

const Version = 1

type FrontMatter struct {
	Linetta Metadata `yaml:"linetta,omitempty" json:"linetta,omitempty"`
}

type Metadata struct {
	Version       int            `yaml:"version,omitempty" json:"version,omitempty"`
	OutlinePreset string         `yaml:"outline_preset,omitempty" json:"outline_preset,omitempty"`
	Entities      []Entity       `yaml:"entities,omitempty" json:"entities,omitempty"`
	Relationships []Relationship `yaml:"relationships,omitempty" json:"relationships,omitempty"`
}

type Entity struct {
	ID         string            `yaml:"id,omitempty" json:"id,omitempty"`
	Kind       string            `yaml:"kind,omitempty" json:"kind,omitempty"`
	Name       string            `yaml:"name,omitempty" json:"name,omitempty"`
	Aliases    []string          `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Role       string            `yaml:"role,omitempty" json:"role,omitempty"`
	Summary    string            `yaml:"summary,omitempty" json:"summary,omitempty"`
	Attributes map[string]string `yaml:"attributes,omitempty" json:"attributes,omitempty"`
}

type Relationship struct {
	ID       string `yaml:"id,omitempty" json:"id,omitempty"`
	PairID   string `yaml:"pair_id,omitempty" json:"pair_id,omitempty"`
	FromID   string `yaml:"from_id,omitempty" json:"from_id,omitempty"`
	ToID     string `yaml:"to_id,omitempty" json:"to_id,omitempty"`
	FromName string `yaml:"from_name,omitempty" json:"from_name,omitempty"`
	ToName   string `yaml:"to_name,omitempty" json:"to_name,omitempty"`
	Label    string `yaml:"label,omitempty" json:"label,omitempty"`
	Notes    string `yaml:"notes,omitempty" json:"notes,omitempty"`
}

func (m Metadata) Empty() bool {
	return m.OutlinePreset == "" && len(m.Entities) == 0 && len(m.Relationships) == 0
}

func Normalize(m Metadata) Metadata {
	if m.Version == 0 {
		m.Version = Version
	}
	if m.Entities == nil {
		m.Entities = []Entity{}
	}
	if m.Relationships == nil {
		m.Relationships = []Relationship{}
	}
	return m
}

func RenderFrontMatter(m Metadata) (string, error) {
	m = Normalize(m)
	raw, err := yaml.Marshal(FrontMatter{Linetta: m})
	if err != nil {
		return "", err
	}
	return "---\n" + string(raw) + "---\n\n", nil
}

func ExtractFrontMatter(text string) (Metadata, string, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return Metadata{}, normalized, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return Metadata{}, normalized, nil
	}
	raw := strings.Join(lines[1:end], "\n")
	body := strings.Join(lines[end+1:], "\n")

	var wrapped FrontMatter
	if err := yaml.Unmarshal([]byte(raw), &wrapped); err != nil {
		return Metadata{}, normalized, err
	}
	if !wrapped.Linetta.Empty() || wrapped.Linetta.Version != 0 {
		return Normalize(wrapped.Linetta), body, nil
	}

	var legacy Metadata
	if err := yaml.Unmarshal([]byte(raw), &legacy); err != nil {
		return Metadata{}, normalized, err
	}
	if !legacy.Empty() || legacy.Version != 0 {
		return Normalize(legacy), body, nil
	}
	return Metadata{}, body, nil
}
