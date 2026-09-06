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
	// Assert the sentinel, not just "an error": Parse maps an unknown author
	// to ErrBadFrontmatter (there is no dedicated sentinel for it), and a
	// bare err != nil check would still pass if that mapping silently
	// changed to some other error.
	if !errors.Is(err, ErrBadFrontmatter) {
		t.Fatalf("Parse() with unknown author: error = %v, want ErrBadFrontmatter", err)
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

// TestNameRejectsWindowsReservedDeviceNames covers the second Important
// review finding: Name becomes a directory name (Task 3), and these names
// are reserved on Windows regardless of case or extension. Fixing this
// later, once real skills exist on disk, would be a breaking change — hence
// covering it now.
func TestNameRejectsWindowsReservedDeviceNames(t *testing.T) {
	cases := []string{"con", "CON", "Con", "nul", "aux", "prn", "com1", "COM9", "lpt1", "lpt9"}
	for _, name := range cases {
		if ValidName(name) {
			t.Errorf("ValidName(%q) = true, want false (Windows reserved device name)", name)
		}
	}

	// And through Parse: the error must still be ErrBadName (there is no
	// dedicated sentinel for this), with a message that says what is wrong
	// rather than just "bad name".
	raw := "---\nname: con\ndescription: some description\n---\n\nBody.\n"
	_, err := Parse(raw)
	if !errors.Is(err, ErrBadName) {
		t.Fatalf("Parse() with reserved name %q: error = %v, want ErrBadName", "con", err)
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("Parse() error = %q, want it to explain the name is reserved", err.Error())
	}

	// A name that merely contains a reserved word is not itself reserved.
	if !ValidName("console") {
		t.Errorf("ValidName(%q) = false, want true (not itself a reserved name)", "console")
	}
}

// TestNameRejectsLeadingTrailingOrAllHyphens pins down the cosmetic decision
// documented on validNamePattern: a leading or trailing hyphen (and, as a
// consequence, an all-hyphen name) is refused, even though none of them are
// unsafe as a path segment. Interior hyphens remain unrestricted.
func TestNameRejectsLeadingTrailingOrAllHyphens(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"-foo", false},
		{"foo-", false},
		{"-foo-", false},
		{"-", false},
		{"---", false},
		{"a--b", true}, // interior consecutive hyphens are fine
		{"a-b-c", true},
	}
	for _, c := range cases {
		if got := ValidName(c.name); got != c.ok {
			t.Errorf("ValidName(%q) = %v, want %v", c.name, got, c.ok)
		}
	}
}

// TestParseStripsLeadingBOM covers the first Important review finding: a
// UTF-8 byte-order mark (which Notepad writes by default) must not defeat
// frontmatter detection.
func TestParseStripsLeadingBOM(t *testing.T) {
	raw := "\uFEFF" + wellFormed
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() with leading BOM: %v", err)
	}
	if s.Name != "dialogue-rhythm" {
		t.Errorf("Name = %q", s.Name)
	}
}

// TestParseAcceptsCRLFFrontmatter covers the other half of the first
// Important review finding: a CRLF file (realistic on Windows, which
// Linetta ships to, written by tools it does not control) must be readable,
// not reported as having no frontmatter at all.
func TestParseAcceptsCRLFFrontmatter(t *testing.T) {
	raw := "---\r\nname: dialogue-rhythm\r\ndescription: some description\r\nauthor: agent\r\nenabled: true\r\n---\r\n\r\nBody line one.\r\nBody line two.\r\n"
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() with CRLF frontmatter: %v", err)
	}
	if s.Name != "dialogue-rhythm" || s.Author != AuthorAgent || !s.Enabled {
		t.Fatalf("Parse() with CRLF frontmatter produced %+v", s)
	}
	// The body's own CRLF line endings are content Linetta does not own —
	// they must survive untouched, not get silently normalized to LF.
	if !strings.Contains(s.Body, "Body line one.\r\nBody line two.\r\n") {
		t.Fatalf("Body CRLF line endings were not preserved: %q", s.Body)
	}
}

// TestRenderAlwaysEmitsLFFrontmatterButPreservesBodyLineEndings pins down
// the deliberate choice documented on splitFrontmatter/Render: Render always
// writes LF-terminated fences and YAML, regardless of what Parse read, but
// Body's own line endings (content Linetta does not own) survive Parse and
// Render untouched.
func TestRenderAlwaysEmitsLFFrontmatterButPreservesBodyLineEndings(t *testing.T) {
	raw := "---\r\nname: dialogue-rhythm\r\ndescription: some description\r\n---\r\n\r\nCRLF body.\r\n"
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	rendered := Render(s)
	if !strings.HasPrefix(rendered, "---\nname: dialogue-rhythm\n") {
		t.Fatalf("Render() did not emit an LF frontmatter fence: %q", rendered)
	}
	if !strings.Contains(rendered, "CRLF body.\r\n") {
		t.Fatalf("Render() lost the body's own CRLF line ending: %q", rendered)
	}

	// And it must still round-trip: the CRLF body Render just emitted is
	// still followed by an LF closing fence, so Parse must find it again.
	s2, err := Parse(rendered)
	if err != nil {
		t.Fatalf("Parse(Render(s)): %v", err)
	}
	if s2.Body != s.Body {
		t.Fatalf("body mismatch after Render round trip:\nfirst:  %q\nsecond: %q", s.Body, s2.Body)
	}
}

// TestParseOversizedFrontmatterReturnsErrBadFrontmatter covers the Minor
// finding: the frontmatter block is bounded before it ever reaches
// yaml.Unmarshal, since yaml.v3 has no alias-expansion limit of its own.
func TestParseOversizedFrontmatterReturnsErrBadFrontmatter(t *testing.T) {
	padding := strings.Repeat("a", maxFrontmatterRunes+1)
	raw := "---\nname: dialogue-rhythm\ndescription: some description\n# " + padding + "\n---\n\nBody.\n"
	_, err := Parse(raw)
	if !errors.Is(err, ErrBadFrontmatter) {
		t.Fatalf("Parse() with oversized frontmatter: error = %v, want ErrBadFrontmatter", err)
	}

	// A frontmatter block right at the limit must still parse.
	padding = strings.Repeat("a", maxFrontmatterRunes-100)
	raw = "---\nname: dialogue-rhythm\ndescription: some description\n# " + padding + "\n---\n\nBody.\n"
	if _, err := Parse(raw); err != nil {
		t.Fatalf("Parse() with frontmatter under the limit: %v", err)
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
