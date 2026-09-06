package agentskills

import (
	"errors"
	"strings"
	"testing"
)

// wellFormed is the on-disk shape Parse and Render must round-trip, taken
// verbatim from the task brief.
const wellFormed = `---
name: dialogue-rhythm
description: How to get this writer's dialogue rhythm — short beats, no dashes
author: agent
enabled: true
---

Body markdown, headings allowed.
`

func TestWellFormedDocumentRoundTrips(t *testing.T) {
	first, err := Parse(wellFormed)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	rendered := Render(first)

	second, err := Parse(rendered)
	if err != nil {
		t.Fatalf("Parse(Render(first)): %v", err)
	}

	// UpdatedAt and BodyRunes are not part of the file: Render must not
	// emit them, and Parse must leave them zero on both passes.
	if first.UpdatedAt != 0 || first.BodyRunes != 0 {
		t.Fatalf("Parse must leave UpdatedAt/BodyRunes zero, got %+v", first)
	}
	if second.UpdatedAt != 0 || second.BodyRunes != 0 {
		t.Fatalf("Parse must leave UpdatedAt/BodyRunes zero, got %+v", second)
	}

	if first != second {
		t.Fatalf("round trip mismatch:\nfirst:  %+v\nsecond: %+v", first, second)
	}
	if first.Name != "dialogue-rhythm" {
		t.Errorf("Name = %q", first.Name)
	}
	if first.Author != AuthorAgent {
		t.Errorf("Author = %q, want %q", first.Author, AuthorAgent)
	}
	if !first.Enabled {
		t.Errorf("Enabled = false, want true")
	}
}

func TestParseMissingFrontmatterReturnsErrNoFrontmatter(t *testing.T) {
	_, err := Parse("Body markdown, no frontmatter fence at all.\n")
	if !errors.Is(err, ErrNoFrontmatter) {
		t.Fatalf("Parse() error = %v, want ErrNoFrontmatter", err)
	}
}

func TestParseUnterminatedFrontmatterReturnsErrBadFrontmatter(t *testing.T) {
	raw := "---\nname: dialogue-rhythm\ndescription: no closing fence\n"
	_, err := Parse(raw)
	if !errors.Is(err, ErrBadFrontmatter) {
		t.Fatalf("Parse() error = %v, want ErrBadFrontmatter", err)
	}
}

func TestParseMissingNameReturnsErrNoName(t *testing.T) {
	raw := "---\ndescription: has a description but no name\n---\n\nBody.\n"
	_, err := Parse(raw)
	if !errors.Is(err, ErrNoName) {
		t.Fatalf("Parse() error = %v, want ErrNoName", err)
	}
}

func TestParseMissingDescriptionReturnsErrNoDescription(t *testing.T) {
	raw := "---\nname: dialogue-rhythm\n---\n\nBody.\n"
	_, err := Parse(raw)
	if !errors.Is(err, ErrNoDescription) {
		t.Fatalf("Parse() error = %v, want ErrNoDescription", err)
	}
}

// TestNameMustBeASlug covers name validation, both directly through
// ValidName and through Parse's ErrBadName.
//
// The Korean case is deliberate and not a language preference: Name becomes
// a directory name on disk and a path-safety boundary (Task 3), so it must
// stay ASCII-only slug characters. The writer's own language belongs in
// Description, which carries arbitrary text — this test only pins down that
// Name does not.
func TestNameMustBeASlug(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"dialogue-rhythm", true},
		{"Dialogue Rhythm", false},       // spaces and uppercase
		{"../escape", false},             // path traversal characters
		{"대사", false},                    // non-ASCII: deliberate, see doc comment above
		{"", false},                      // empty
		{strings.Repeat("a", 65), false}, // over MaxNameRunes
		{strings.Repeat("a", 64), true},  // at MaxNameRunes
	}
	for _, c := range cases {
		if got := ValidName(c.name); got != c.ok {
			t.Errorf("ValidName(%q) = %v, want %v", c.name, got, c.ok)
		}
	}

	// And through Parse: a bad name must surface as ErrBadName, not some
	// other error, so a calling agent can tell what to change.
	for _, c := range cases {
		if c.ok {
			continue
		}
		raw := "---\nname: " + yamlQuote(c.name) + "\ndescription: some description\n---\n\nBody.\n"
		_, err := Parse(raw)
		if !errors.Is(err, ErrBadName) {
			t.Errorf("Parse() with name %q: error = %v, want ErrBadName", c.name, err)
		}
	}
}

// yamlQuote wraps a value in double quotes so YAML doesn't choke on an empty
// scalar or on Korean characters used in the name-validation test above.
func yamlQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func TestAuthorDefaultsToWriterWhenAbsent(t *testing.T) {
	raw := "---\nname: dialogue-rhythm\ndescription: some description\n---\n\nBody.\n"
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Author != AuthorWriter {
		t.Errorf("Author = %q, want %q", s.Author, AuthorWriter)
	}
}

func TestAuthorUnknownValueIsRefused(t *testing.T) {
	raw := "---\nname: dialogue-rhythm\ndescription: some description\nauthor: robot\n---\n\nBody.\n"
	_, err := Parse(raw)
	if err == nil {
		t.Fatalf("Parse() with unknown author: got nil error, want a refusal")
	}
}

func TestEnabledDefaultsToTrueWhenAbsent(t *testing.T) {
	raw := "---\nname: dialogue-rhythm\ndescription: some description\n---\n\nBody.\n"
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !s.Enabled {
		t.Errorf("Enabled = false, want true (default)")
	}
}

func TestEnabledFalseIsHonored(t *testing.T) {
	raw := "---\nname: dialogue-rhythm\ndescription: some description\nenabled: false\n---\n\nBody.\n"
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Enabled {
		t.Errorf("Enabled = true, want false")
	}
}

// TestBodyKeepsMarkdownHeadingsVerbatim is the difference from agentmemory's
// Screen, which refuses a markdown heading in a memory line. A skill IS
// markdown; headings are normal body content and must survive untouched.
func TestBodyKeepsMarkdownHeadingsVerbatim(t *testing.T) {
	raw := "---\nname: dialogue-rhythm\ndescription: some description\n---\n\n# Heading One\n\n## Heading Two\n\nSome body text.\n"
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(s.Body, "# Heading One") || !strings.Contains(s.Body, "## Heading Two") {
		t.Fatalf("Body dropped or mangled headings: %q", s.Body)
	}
}

// TestBodyContainingFrontmatterLikeLineSurvivesRoundTrip is the one thing
// most worth care per the brief: split on the FIRST terminator after the
// opening fence, so a body line that itself reads "---" is treated as body
// bytes rather than accidentally closing the frontmatter block early.
func TestBodyContainingFrontmatterLikeLineSurvivesRoundTrip(t *testing.T) {
	raw := "---\nname: dialogue-rhythm\ndescription: some description\n---\n\nBefore.\n\n---\n\nAfter the embedded rule line.\n"
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(s.Body, "---") {
		t.Fatalf("Body lost its embedded --- line: %q", s.Body)
	}
	if !strings.Contains(s.Body, "After the embedded rule line.") {
		t.Fatalf("Body lost content after the embedded --- line: %q", s.Body)
	}

	rendered := Render(s)
	s2, err := Parse(rendered)
	if err != nil {
		t.Fatalf("Parse(Render(s)): %v", err)
	}
	if s.Body != s2.Body {
		t.Fatalf("body round trip mismatch:\nfirst:  %q\nsecond: %q", s.Body, s2.Body)
	}
}

func TestParseScopeRejectsUnknownScope(t *testing.T) {
	if _, err := ParseScope("bogus"); err == nil {
		t.Fatalf("ParseScope(\"bogus\") = nil error, want a refusal")
	}
	writer, err := ParseScope("writer")
	if err != nil || writer != ScopeWriter {
		t.Fatalf("ParseScope(\"writer\") = %q, %v, want ScopeWriter, nil", writer, err)
	}
	work, err := ParseScope("work")
	if err != nil || work != ScopeWork {
		t.Fatalf("ParseScope(\"work\") = %q, %v, want ScopeWork, nil", work, err)
	}
}
