// Package agentskills owns the SKILL.md documents an agent writes for itself
// and the writer can read, edit and revert.
//
// It must never import tars/pkg/llm. mcphost and storycontext import this
// package, and scripts/validate-story-core-deps.sh forbids a model client in
// their dependency graph — which is also why the background review that calls
// a model lives in internal/agent and reaches this package through an
// interface.
package agentskills

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Scope names where a skill lives: global to the writer, or scoped to one
// work.
type Scope string

const (
	ScopeWriter Scope = "writer"
	ScopeWork   Scope = "work"
)

// ParseScope converts a value off the wire. An unknown scope is an error
// rather than a zero value, so a typo from a model is told, not ignored —
// the same rule agentmemory.ParseScope follows for its own two scopes.
func ParseScope(v string) (Scope, error) {
	switch s := Scope(strings.TrimSpace(v)); s {
	case ScopeWriter, ScopeWork:
		return s, nil
	default:
		return "", fmt.Errorf("agentskills: unknown scope %q; use %q or %q", v, ScopeWriter, ScopeWork)
	}
}

// Budgets, in RUNES. MaxBodyRunes and MaxDescriptionRunes are enforced by the
// guard added in a later task, not by Parse here; MaxSkillsPerScope is
// enforced by the store. They are declared alongside the document shape they
// bound.
const (
	MaxBodyRunes        = 8000
	MaxDescriptionRunes = 200
	MaxSkillsPerScope   = 40
	MaxNameRunes        = 64
)

// Author says who last wrote a skill: the writer, editing by hand, or the
// agent that authored it during a session.
type Author string

const (
	AuthorWriter Author = "writer"
	AuthorAgent  Author = "agent"
)

// Skill is one SKILL.md document: agentskills.io-format frontmatter plus a
// markdown body.
type Skill struct {
	Name        string `json:"name"` // slug: lowercase letters, digits, hyphens
	Scope       Scope  `json:"scope"`
	ProjectID   string `json:"project_id,omitempty"`
	Description string `json:"description"`
	Author      Author `json:"author"`
	Enabled     bool   `json:"enabled"`
	Body        string `json:"body"`
	UpdatedAt   int64  `json:"updated_at"`
	BodyRunes   int    `json:"body_runes"`
}

// Sentinel causes. The caller turns each into a tool error the agent reads,
// so each one has to say what to change.
var (
	ErrNoFrontmatter  = errors.New("agentskills: no frontmatter")
	ErrBadFrontmatter = errors.New("agentskills: malformed frontmatter")
	ErrNoName         = errors.New("agentskills: missing name")
	ErrBadName        = errors.New("agentskills: name must be a lowercase slug (letters, digits, hyphens)")
	ErrNoDescription  = errors.New("agentskills: missing description")
)

// openFence and closeFence bound the YAML frontmatter block. closeFence is
// searched for starting right after openFence, and the FIRST match wins —
// not the last — so a body that itself contains a line reading "---" is
// treated as body bytes rather than closing the frontmatter early. That is
// the one thing most worth getting right here: Parse and Render must
// round-trip such a body untouched.
const (
	openFence  = "---\n"
	closeFence = "\n---\n"
)

// frontmatter is the YAML shape of the block between the fences. Name and
// Enabled are pointers so Parse can tell "the key is absent" from "the key
// is present with its zero value" — a missing name is ErrNoName, but a name
// key present with an empty (or otherwise invalid) value is ErrBadName; a
// missing enabled defaults to true, which a plain bool couldn't represent
// since its own zero value is false.
type frontmatter struct {
	Name        *string `yaml:"name"`
	Description string  `yaml:"description"`
	Author      string  `yaml:"author,omitempty"`
	Enabled     *bool   `yaml:"enabled,omitempty"`
}

// validNamePattern is the slug rule: lowercase ASCII letters, digits and
// hyphens only. Nothing else — not a space, not "..", not a non-ASCII
// script — is a name character.
var validNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidName reports whether name is an acceptable Skill.Name.
//
// This is deliberately ASCII-only, and deliberately rejects a well-formed
// Korean (or any non-ASCII) name, for a reason that has nothing to do with
// language: Name becomes a directory name on disk and a path-safety boundary
// (see the store landing in a later task), so it has to stay inside the
// characters that are safe as a path segment on every filesystem this app
// ships on. The writer's own language belongs in Description, which is free
// text with no such constraint. Please don't "fix" this to accept Unicode
// names — see TestNameMustBeASlug for the case this guards.
func ValidName(name string) bool {
	if name == "" || len([]rune(name)) > MaxNameRunes {
		return false
	}
	return validNamePattern.MatchString(name)
}

// splitFrontmatter separates raw into its YAML block and body bytes.
//
// The body is everything after the closing fence, kept byte-for-byte: no
// trimming, no reformatting. That is what lets a body containing its own
// "---" line, or trailing/leading blank lines, survive a Parse/Render round
// trip unchanged.
func splitFrontmatter(raw string) (yamlBlock, body string, err error) {
	if !strings.HasPrefix(raw, openFence) {
		return "", "", ErrNoFrontmatter
	}
	rest := raw[len(openFence):]

	// An empty frontmatter block: the closing fence immediately follows the
	// opening one, so there is no leading "\n" inside rest for closeFence's
	// pattern to match against.
	if strings.HasPrefix(rest, "---\n") {
		return "", rest[len("---\n"):], nil
	}

	idx := strings.Index(rest, closeFence)
	if idx < 0 {
		return "", "", ErrBadFrontmatter
	}
	return rest[:idx], rest[idx+len(closeFence):], nil
}

// Parse reads a SKILL.md document: agentskills.io-format YAML frontmatter
// followed by a markdown body.
//
// UpdatedAt and BodyRunes are never in the file — they are metadata the
// store fills in around the document — so Parse always leaves them zero.
func Parse(raw string) (Skill, error) {
	yamlBlock, body, err := splitFrontmatter(raw)
	if err != nil {
		return Skill{}, err
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return Skill{}, fmt.Errorf("%w: %v", ErrBadFrontmatter, err)
	}

	if fm.Name == nil {
		return Skill{}, ErrNoName
	}
	name := *fm.Name
	if !ValidName(name) {
		return Skill{}, fmt.Errorf("%w: %q", ErrBadName, name)
	}
	if strings.TrimSpace(fm.Description) == "" {
		return Skill{}, ErrNoDescription
	}

	author := AuthorWriter
	if fm.Author != "" {
		switch a := Author(fm.Author); a {
		case AuthorWriter, AuthorAgent:
			author = a
		default:
			return Skill{}, fmt.Errorf("%w: unknown author %q; use %q or %q",
				ErrBadFrontmatter, fm.Author, AuthorWriter, AuthorAgent)
		}
	}

	enabled := true
	if fm.Enabled != nil {
		enabled = *fm.Enabled
	}

	return Skill{
		Name:        name,
		Description: fm.Description,
		Author:      author,
		Enabled:     enabled,
		Body:        body,
	}, nil
}

// Render is Parse's inverse: it writes s back out as SKILL.md text.
//
// It never emits UpdatedAt or BodyRunes — they are not part of the file —
// and it writes Body back verbatim, so a body that survived Parse with an
// embedded "---" line survives Render too.
func Render(s Skill) string {
	enabled := s.Enabled
	name := s.Name
	fm := frontmatter{
		Name:        &name,
		Description: s.Description,
		Author:      string(s.Author),
		Enabled:     &enabled,
	}
	// yaml.Marshal on this fixed, plain-scalar struct cannot fail.
	yamlBytes, _ := yaml.Marshal(fm)

	var b strings.Builder
	b.WriteString(openFence)
	b.Write(yamlBytes)
	b.WriteString("---\n")
	b.WriteString(s.Body)
	return b.String()
}
