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

// openFence and closeFence are the LF-only fence forms Render always
// produces (see Render's doc comment for why it never emits CRLF).
// splitFrontmatter accepts CRLF on input too; see its own doc comment.
//
// closeFence is searched for starting right after openFence, and the FIRST
// match wins — not the last — so a body that itself contains a line reading
// "---" is treated as body bytes rather than closing the frontmatter early.
// That is the one thing most worth getting right here: Parse and Render
// must round-trip such a body untouched.
const (
	openFence  = "---\n"
	closeFence = "\n---\n"
)

// bom is the UTF-8 encoding of U+FEFF, the byte-order mark some Windows
// editors (Notepad, in particular) write at the start of a file by default.
// Parse strips it before anything else so such a file's frontmatter is still
// detected rather than reported as absent.
const bom = "\uFEFF"

// maxFrontmatterRunes bounds the size of the raw text between the fences,
// checked before it is handed to yaml.Unmarshal. yaml.v3 has no built-in
// alias-expansion limit, and Parse is reachable from an MCP tool argument
// (a later task), so an unbounded block could be used to exhaust memory
// during Parse itself, before any other guard runs. The block holds four
// short fields (name, description, author, enabled); a few kilobytes is
// enormously generous. This does NOT bound Body — that is Task 2's Guard,
// with its own limit (MaxBodyRunes), enforced elsewhere.
const maxFrontmatterRunes = 4096

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
//
// The pattern also requires the first and last characters to be alphanumeric,
// which forbids a leading or trailing hyphen and, as a consequence, an
// all-hyphen name too. None of those are unsafe as a path segment — this is
// a deliberate cosmetic decision, not a safety one: Name becomes a directory
// name a writer sees in a file browser (Task 3), and "-foo", "foo-", and
// "---" are confusing entries to find there. Interior hyphens are otherwise
// unrestricted (e.g. "a--b" is fine) since only the ends were the reviewer's
// concern.
var validNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// reservedDeviceNames are Windows reserved device names: writing a file or
// directory with one of these names fails, or behaves strangely, on every
// Windows box this app ships to — regardless of case or of any extension
// appended to it. Name becomes a directory name (Task 3), so these must be
// refused here, once, rather than fixed later as a breaking change to skills
// that already exist on disk by then.
var reservedDeviceNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// isReservedDeviceName reports whether name collides with a Windows reserved
// device name, case-insensitively (the slug rule already forces lowercase in
// practice, but this check does not rely on that).
func isReservedDeviceName(name string) bool {
	return reservedDeviceNames[strings.ToLower(name)]
}

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
//
// It also refuses a Windows reserved device name (see reservedDeviceNames)
// even though such a name is otherwise a well-formed slug.
func ValidName(name string) bool {
	if name == "" || len([]rune(name)) > MaxNameRunes {
		return false
	}
	if !validNamePattern.MatchString(name) {
		return false
	}
	return !isReservedDeviceName(name)
}

// splitFrontmatter separates raw into its YAML block and body bytes.
//
// It strips a leading UTF-8 byte-order mark first, then recognizes fence
// lines terminated by either "\n" or "\r\n" — a file with CRLF line endings
// or a BOM (both realistic on Windows, which Linetta ships to, written by
// tools it does not control, such as Notepad) must not be reported as
// having no frontmatter at all. Only one line-ending style is recognized per
// file: whichever the opening fence uses is also required of the closing
// fence. Render (see its doc comment) always emits LF, regardless of what
// was parsed — so a CRLF file's frontmatter round-trips to LF. This is a
// deliberate choice, not an oversight: the fences and YAML are structure
// Linetta owns and rewrites anyway, so there is no reason to preserve their
// original line ending once parsed.
//
// The body is everything after the closing fence, kept byte-for-byte: no
// trimming, no reformatting, and — this is the part that must not regress —
// no rewriting of ITS line endings either. A CRLF body survives Parse and
// Render untouched, even though the frontmatter around it does not, because
// the body is content Linetta does not own; only the split points (BOM,
// fence prefix/suffix lengths) are computed from the normalized view above,
// and the body slice itself is taken from the original bytes.
func splitFrontmatter(raw string) (yamlBlock, body string, err error) {
	raw = strings.TrimPrefix(raw, bom)

	nl := "\n"
	prefix := openFence // "---\n"
	if !strings.HasPrefix(raw, prefix) {
		crlfPrefix := "---\r\n"
		if !strings.HasPrefix(raw, crlfPrefix) {
			return "", "", ErrNoFrontmatter
		}
		nl = "\r\n"
		prefix = crlfPrefix
	}
	rest := raw[len(prefix):]
	closer := nl + "---" + nl

	// An empty frontmatter block: the closing fence immediately follows the
	// opening one, so there is no leading line-ending inside rest for
	// closer's pattern to match against.
	if strings.HasPrefix(rest, "---"+nl) {
		return "", rest[len("---"+nl):], nil
	}

	idx := strings.Index(rest, closer)
	if idx < 0 {
		return "", "", ErrBadFrontmatter
	}
	return rest[:idx], rest[idx+len(closer):], nil
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

	// Bound the block BEFORE it reaches yaml.Unmarshal — see
	// maxFrontmatterRunes' doc comment for why this has to happen here and
	// not only in a later guard.
	if n := len([]rune(yamlBlock)); n > maxFrontmatterRunes {
		return Skill{}, fmt.Errorf("%w: frontmatter block is %d runes, over the %d-rune limit", ErrBadFrontmatter, n, maxFrontmatterRunes)
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
		if isReservedDeviceName(name) {
			return Skill{}, fmt.Errorf("%w: %q is a Windows reserved device name; pick a different name", ErrBadName, name)
		}
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
// embedded "---" line, or with its own CRLF line endings, survives Render
// too. The fences and YAML Render generates around Body are always
// LF-terminated, even if the source Skill was Parsed from a CRLF file with
// a BOM: that structure is Linetta's own, rewritten every time regardless,
// so there is nothing gained by preserving its original line ending, and
// LF keeps the writer output uniform across platforms.
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
