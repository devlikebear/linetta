// Package mdmeta owns the structured Linetta metadata embedded in exported
// markdown. The visible markdown stays readable, while this frontmatter carries
// stable entity and relationship references for import round-trips.
package mdmeta

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Version 2 adds project details, the node list, threads/beats, margin notes,
// and fact cards (#83). Version-1 files (entities + relationships only) still
// parse: every new section is optional and Normalize fills nothing in.
const Version = 2

type FrontMatter struct {
	Linetta Metadata `yaml:"linetta,omitempty" json:"linetta,omitempty"`
}

type Metadata struct {
	Version       int            `yaml:"version,omitempty" json:"version,omitempty"`
	OutlinePreset string         `yaml:"outline_preset,omitempty" json:"outline_preset,omitempty"`
	Project       *ProjectMeta   `yaml:"project,omitempty" json:"project,omitempty"`
	Nodes         []Node         `yaml:"nodes,omitempty" json:"nodes,omitempty"`
	Entities      []Entity       `yaml:"entities,omitempty" json:"entities,omitempty"`
	Relationships []Relationship `yaml:"relationships,omitempty" json:"relationships,omitempty"`
	Threads       []Thread       `yaml:"threads,omitempty" json:"threads,omitempty"`
	Notes         []Note         `yaml:"notes,omitempty" json:"notes,omitempty"`
	FactCards     []FactCard     `yaml:"fact_cards,omitempty" json:"fact_cards,omitempty"`
}

// ProjectMeta carries the project fields that live outside the manuscript
// body: the New Project form, the synopsis, and the plot outline.
type ProjectMeta struct {
	Genres            []string `yaml:"genres,omitempty" json:"genres,omitempty"`
	LengthTarget      string   `yaml:"length_target,omitempty" json:"length_target,omitempty"`
	DefaultPOV        string   `yaml:"default_pov,omitempty" json:"default_pov,omitempty"`
	StyleNotes        string   `yaml:"style_notes,omitempty" json:"style_notes,omitempty"`
	Synopsis          string   `yaml:"synopsis,omitempty" json:"synopsis,omitempty"`
	Outline           string   `yaml:"outline,omitempty" json:"outline,omitempty"`
	EpisodeCharTarget int      `yaml:"episode_char_target,omitempty" json:"episode_char_target,omitempty"`
}

// Node is one manuscript node in export document order (the same pre-order
// walk that writes the headings). Import aligns this list positionally with
// the headings it rebuilt, which is how beats/notes/fact cards find their
// node again after every id is regenerated.
type Node struct {
	ID      string `yaml:"id,omitempty" json:"id,omitempty"`
	Label   string `yaml:"label,omitempty" json:"label,omitempty"`
	Title   string `yaml:"title,omitempty" json:"title,omitempty"`
	Status  string `yaml:"status,omitempty" json:"status,omitempty"`
	Summary string `yaml:"summary,omitempty" json:"summary,omitempty"`
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

// Thread is a storyline with its beats in ordinal order.
type Thread struct {
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`
	Color    string `yaml:"color,omitempty" json:"color,omitempty"`
	Summary  string `yaml:"summary,omitempty" json:"summary,omitempty"`
	ClosedAt int64  `yaml:"closed_at,omitempty" json:"closed_at,omitempty"`
	Beats    []Beat `yaml:"beats,omitempty" json:"beats,omitempty"`
}

type Beat struct {
	NodeID      string `yaml:"node_id,omitempty" json:"node_id,omitempty"`
	Label       string `yaml:"label,omitempty" json:"label,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Intensity   int    `yaml:"intensity,omitempty" json:"intensity,omitempty"`
}

// Note is a margin note anchored to a character offset in one node.
type Note struct {
	NodeID    string `yaml:"node_id,omitempty" json:"node_id,omitempty"`
	Anchor    int    `yaml:"anchor,omitempty" json:"anchor,omitempty"`
	Body      string `yaml:"body,omitempty" json:"body,omitempty"`
	CreatedAt int64  `yaml:"created_at,omitempty" json:"created_at,omitempty"`
}

// FactCard is one fact-book research card with its sources.
type FactCard struct {
	NodeID   string       `yaml:"node_id,omitempty" json:"node_id,omitempty"`
	Claim    string       `yaml:"claim,omitempty" json:"claim,omitempty"`
	Result   string       `yaml:"result,omitempty" json:"result,omitempty"`
	Status   string       `yaml:"status,omitempty" json:"status,omitempty"`
	Category string       `yaml:"category,omitempty" json:"category,omitempty"`
	Sources  []FactSource `yaml:"sources,omitempty" json:"sources,omitempty"`
}

type FactSource struct {
	URL        string `yaml:"url,omitempty" json:"url,omitempty"`
	Title      string `yaml:"title,omitempty" json:"title,omitempty"`
	Snippet    string `yaml:"snippet,omitempty" json:"snippet,omitempty"`
	AccessedAt int64  `yaml:"accessed_at,omitempty" json:"accessed_at,omitempty"`
}

func (m Metadata) Empty() bool {
	return m.OutlinePreset == "" &&
		m.Project == nil &&
		len(m.Nodes) == 0 &&
		len(m.Entities) == 0 &&
		len(m.Relationships) == 0 &&
		len(m.Threads) == 0 &&
		len(m.Notes) == 0 &&
		len(m.FactCards) == 0
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
