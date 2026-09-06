package agentskills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/atomicfile"
)

// Skills live on disk, not in the database, and that is the whole point: the
// writer can open <LINETTA_HOME>/skills in a file browser, edit a SKILL.md by
// hand, keep it in git, or point their own Claude Code at the folder and have
// it find the shape it expects. A directory per skill — not a flat
// <name>.md — because agentskills.io skills carry sibling files (references,
// scripts, examples) and a skill that is a folder can grow them without a
// later migration.
//
//	<home>/skills/<name>/SKILL.md                     ScopeWriter
//	<home>/skills/works/<project id>/<name>/SKILL.md  ScopeWork
//
// The literal "works" segment is what keeps the two scopes from colliding: a
// project id can be anything a UUID generator produces, and without that
// segment a work id would share a namespace with writer-scope skill names.
// It costs one reserved name in the writer scope (see errReservedName).

// skillFile is the document's name inside a skill's directory. It is
// upper-case because agentskills.io says so, and because a writer pointing
// another agent at this folder needs the name that agent looks for.
const skillFile = "SKILL.md"

// worksSegment separates work-scoped skills from writer-scoped ones. It is
// therefore not available as a writer-scope skill name: a skill called
// "works" would put its SKILL.md inside the directory that holds every work's
// skills, and deleting it would take all of them with it.
const worksSegment = "works"

// Sentinel causes, alongside the ones in skill.go and guard.go.
var (
	// ErrNotFound is a skill that is not on disk. Delete returns it rather
	// than succeeding silently: an agent that deletes the wrong name has to
	// be told, or it will believe it cleaned up when it did not.
	ErrNotFound = errors.New("agentskills: skill not found")

	// ErrTooManySkills is the MaxSkillsPerScope cap, counted per scope. It
	// only ever blocks a NEW skill; rewriting one that already exists is
	// always allowed, so a writer at the cap can still fix what they have.
	ErrTooManySkills = errors.New("agentskills: too many skills in this scope")

	// ErrExists is something other than the skill occupying the skill's
	// path: a plain file where the skill's directory belongs, or a SKILL.md
	// that is itself a directory. It is deliberately distinct from a write
	// failure — nothing here is corrupt, the writer just has something else
	// sitting in the way, and the fix is to move it.
	ErrExists = errors.New("agentskills: another file is already at that path")
)

// Diagnostic is one thing on disk that should have been a skill and is not.
//
// It exists because of the single most important rule in this store: a file
// that fails to parse is reported, never returned as an error and never
// silently dropped. A writer who broke a SKILL.md by hand has to SEE it in
// Settings to fix it, and one broken file must not hide the twelve that are
// fine. List's error return is for something that stops the whole listing —
// an unreadable directory — not for one bad file.
type Diagnostic struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Store reads and writes skills under a Linetta home directory.
type Store struct{ home string }

// NewStore builds a Store over home, which is normally paths.Home(). The
// directory need not exist: List treats a missing one as no skills, and Write
// creates what it needs.
func NewStore(home string) *Store { return &Store{home: home} }

// ---- path safety ----------------------------------------------------------

// Every name and project id below reaches these functions as a tool argument
// written by a language model. ValidName (skill.go) already refuses "..",
// separators and Windows device names, and it is called here too — but this
// file does not TRUST it. The two checks guard different mistakes: ValidName
// is about what a name may look like and could be relaxed one day for a
// reason that has nothing to do with paths, while insideDir is about where
// the resulting path actually lands. A single-layer defence here fails
// catastrophically (an arbitrary file read, or an os.RemoveAll outside the
// home); two independent layers fail closed. Please do not remove either as
// "redundant".

// projectIDPattern is what a work id may look like as a path segment. Real
// ids are UUIDs; this is deliberately a little wider (so an id scheme can
// change without stranding folders on disk) and still far narrower than
// "anything without a slash".
var projectIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

var errReservedName = fmt.Errorf("agentskills: %q is reserved; work-scoped skills live under it", worksSegment)

// safeSegment is the path-shape check, independent of any naming rule: the
// value must be exactly one path component that cannot climb, cannot be
// absolute, and cannot smuggle a terminator past a C-level API. The
// backslash case matters even on Unix, because the same name is written to
// disk on Windows, where it IS a separator.
func safeSegment(v string) error {
	switch {
	case v == "":
		return errors.New("agentskills: empty path segment")
	case strings.ContainsRune(v, 0):
		return errors.New("agentskills: path segment contains a NUL byte")
	case v == "." || v == "..":
		return fmt.Errorf("agentskills: %q is not a name", v)
	case strings.ContainsRune(v, '/'), strings.ContainsRune(v, '\\'):
		return fmt.Errorf("agentskills: %q may not contain a path separator", v)
	case filepath.IsAbs(v), filepath.VolumeName(v) != "":
		return fmt.Errorf("agentskills: %q may not be a path", v)
	}
	return nil
}

// insideDir joins base and segment and then proves the result is a direct
// child of base. It re-derives the answer from the joined path rather than
// from the input, so it holds even if a future filepath.Join or a platform
// quirk turns an input this file thought was safe into something that is not.
func insideDir(base, segment string) (string, error) {
	if err := safeSegment(segment); err != nil {
		return "", err
	}
	cleanBase := filepath.Clean(base)
	p := filepath.Clean(filepath.Join(cleanBase, segment))
	if filepath.Dir(p) != cleanBase || filepath.Base(p) != segment {
		return "", fmt.Errorf("agentskills: %q would escape %s", segment, cleanBase)
	}
	return p, nil
}

// checkName is the full gate a skill name passes before it becomes a path.
func checkName(scope Scope, name string) error {
	if err := safeSegment(name); err != nil {
		return err
	}
	if !ValidName(name) {
		return fmt.Errorf("%w: %q", ErrBadName, name)
	}
	if scope == ScopeWriter && name == worksSegment {
		return errReservedName
	}
	return nil
}

// ---- layout ---------------------------------------------------------------

// Dir is the directory holding one scope's skills. It is exported because the
// writer is meant to be told where their skills are — Settings shows this
// path so they can open the folder, or point another agent at it.
func (st *Store) Dir(scope Scope, projectID string) (string, error) {
	root := filepath.Join(st.home, "skills")
	id := strings.TrimSpace(projectID)
	switch scope {
	case ScopeWriter:
		if id != "" {
			return "", fmt.Errorf("agentskills: writer skills are global; they take no work id (got %q)", id)
		}
		return root, nil
	case ScopeWork:
		if id == "" {
			return "", errors.New("agentskills: work skills need the work they belong to")
		}
		if !projectIDPattern.MatchString(id) {
			return "", fmt.Errorf("agentskills: %q is not a usable work id", id)
		}
		worksDir, err := insideDir(root, worksSegment)
		if err != nil {
			return "", err
		}
		return insideDir(worksDir, id)
	}
	return "", fmt.Errorf("agentskills: unknown scope %q; use %q or %q", scope, ScopeWriter, ScopeWork)
}

// skillPaths resolves one skill to its directory and its SKILL.md.
func (st *Store) skillPaths(scope Scope, projectID, name string) (dir, file string, err error) {
	if err := checkName(scope, name); err != nil {
		return "", "", err
	}
	scopeDir, err := st.Dir(scope, projectID)
	if err != nil {
		return "", "", err
	}
	dir, err = insideDir(scopeDir, name)
	if err != nil {
		return "", "", err
	}
	file, err = insideDir(dir, skillFile)
	if err != nil {
		return "", "", err
	}
	return dir, file, nil
}

// ---- List -----------------------------------------------------------------

// List reads every skill in a scope, reporting what it could not read instead
// of failing.
//
// A missing scope directory is an empty list and a nil error: a writer who has
// never made a skill is the normal case, the same way agentmemory.Repo.Load
// treats an absent row. A directory that EXISTS but cannot be read is a
// different thing — a permissions problem the writer needs to hear about,
// where returning "no skills" would quietly lie about what is on disk — so
// that is an error.
//
// Everything else that goes wrong with a single entry becomes a Diagnostic:
// a SKILL.md that does not parse, one the guard refuses (it must never reach
// a prompt, but the writer must still see that it is there), a directory
// whose SKILL.md is missing or is itself a directory, and a name in the
// frontmatter that disagrees with the folder it sits in — that last one
// cannot be addressed by Read or Delete at all, so listing it would hand the
// caller a skill it could not then act on.
//
// Hidden entries (a leading dot: .git, .DS_Store) are skipped in silence.
// They are not the writer's mistake, and reporting them would bury the
// diagnostics that are.
func (st *Store) List(scope Scope, projectID string) ([]Skill, []Diagnostic, error) {
	dir, err := st.Dir(scope, projectID)
	if err != nil {
		return nil, nil, err
	}

	skills := make([]Skill, 0)
	diags := make([]Diagnostic, 0)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return skills, diags, nil
		}
		return nil, nil, fmt.Errorf("agentskills: read %s: %w", dir, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if scope == ScopeWriter && name == worksSegment {
			continue // the work scopes' own root, not a skill
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path) // Stat, not entry.Type(): a symlinked skill folder is fine
		if err != nil || !info.IsDir() {
			continue // a stray file next to the skills is not a broken skill
		}

		file := filepath.Join(path, skillFile)
		s, err := readSkillFile(file)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				diags = append(diags, Diagnostic{Path: path, Message: "no " + skillFile + " in this directory"})
				continue
			}
			diags = append(diags, Diagnostic{Path: file, Message: err.Error()})
			continue
		}
		if s.Name != name {
			diags = append(diags, Diagnostic{
				Path:    file,
				Message: fmt.Sprintf("frontmatter name %q does not match its directory %q; rename one to match the other", s.Name, name),
			})
			continue
		}
		s.Scope = scope
		s.ProjectID = strings.TrimSpace(projectID)
		skills = append(skills, s)
	}

	// os.ReadDir already sorts by filename and a skill's directory name is
	// its name, so this is normally a no-op — but the order List promises is
	// by skill name, and that should not depend on a detail of ReadDir.
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, diags, nil
}

// readSkillFile loads and screens one SKILL.md, filling the two fields that
// are metadata rather than document content: UpdatedAt comes from the file's
// modification time (it is not, and should not be, written into the file —
// a writer editing SKILL.md in their own editor updates it for free) and
// BodyRunes is derived.
//
// It returns ErrNotFound for an absent file and ErrExists for a SKILL.md that
// is a directory, so callers can tell those apart from a parse failure.
func readSkillFile(file string) (Skill, error) {
	info, err := os.Stat(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Skill{}, ErrNotFound
		}
		return Skill{}, err
	}
	if info.IsDir() {
		return Skill{}, fmt.Errorf("%w: %s is a directory, not a file", ErrExists, file)
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		return Skill{}, err
	}
	s, err := Parse(string(raw))
	if err != nil {
		return Skill{}, err
	}
	// The guard runs on read, not only on write: a skill body is bound for a
	// model's prompt, and a file the writer (or another agent editing this
	// folder) put there by hand never went through Write. In List this
	// becomes a Diagnostic rather than an error, which is exactly the
	// visibility a screened-out skill needs.
	if err := Guard(s); err != nil {
		return Skill{}, err
	}
	s.UpdatedAt = info.ModTime().UnixMilli()
	s.BodyRunes = len([]rune(s.Body))
	return s, nil
}

// ---- Read -----------------------------------------------------------------

// Read returns one skill by name. A skill that is not there is ErrNotFound; a
// SKILL.md that is a directory is ErrExists; a file that does not parse, or
// that the guard refuses, is that failure's own error. Unlike List, this
// addresses exactly one skill, so there is nothing for a diagnostic to
// protect — the caller asked about this file and wants to hear what is wrong
// with it.
func (st *Store) Read(scope Scope, projectID, name string) (Skill, error) {
	_, file, err := st.skillPaths(scope, projectID, name)
	if err != nil {
		return Skill{}, err
	}
	s, err := readSkillFile(file)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Skill{}, fmt.Errorf("%w: %q", ErrNotFound, name)
		}
		return Skill{}, err
	}
	s.Scope = scope
	s.ProjectID = strings.TrimSpace(projectID)
	return s, nil
}

// ---- Write ----------------------------------------------------------------

// Write stores a skill, creating or replacing it, and returns what was
// stored with UpdatedAt and BodyRunes filled.
//
// Order matters here. Guard runs FIRST, before any path is resolved or any
// directory is created, so an oversized or invisible-character body cannot
// leave a stray folder behind on its way to being refused. The cap is checked
// next, and only against a NEW name: a writer at forty skills can still
// rewrite the forty they have.
//
// The file lands through atomicfile.Write (a temp file in the same directory
// plus a rename), so a crash or a full disk mid-write cannot leave half a
// SKILL.md that then reads as a broken skill. Modes are 0600 for the file and
// 0700 for the directories: this is the writer's own material under
// LINETTA_HOME, following codexauth's credential handling rather than the
// 0644 folder-sync path, which writes into a folder the writer chose to share.
func (st *Store) Write(s Skill, now int64) (Skill, error) {
	// An author is optional in the struct but not in the file: Render would
	// emit `author: ""` and Parse would then refuse to read back what we just
	// wrote. Default it before the guard so the stored file always round-trips.
	if s.Author == "" {
		s.Author = AuthorWriter
	}
	switch s.Author {
	case AuthorWriter, AuthorAgent:
	default:
		return Skill{}, fmt.Errorf("agentskills: unknown author %q; use %q or %q", s.Author, AuthorWriter, AuthorAgent)
	}
	if strings.TrimSpace(s.Description) == "" {
		return Skill{}, ErrNoDescription
	}
	if err := Guard(s); err != nil {
		return Skill{}, err
	}

	dir, file, err := st.skillPaths(s.Scope, s.ProjectID, s.Name)
	if err != nil {
		return Skill{}, err
	}

	// A plain file sitting where the skill's directory belongs is checked for
	// by name, before anything stats through it, rather than by decoding an
	// errno that differs across the platforms this ships to. It is not a
	// corrupt store — the writer just has something in the way, and the fix
	// is to move it.
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		return Skill{}, fmt.Errorf("%w: %s is not a directory", ErrExists, dir)
	}

	if _, err := os.Stat(file); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return Skill{}, err
		}
		// A skill that does not exist yet is the only kind the cap can refuse.
		n, err := st.count(s.Scope, s.ProjectID)
		if err != nil {
			return Skill{}, err
		}
		if n >= MaxSkillsPerScope {
			return Skill{}, fmt.Errorf("%w: %d of %d used; delete one first", ErrTooManySkills, n, MaxSkillsPerScope)
		}
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Skill{}, fmt.Errorf("agentskills: create %s: %w", dir, err)
	}
	if err := atomicfile.Write(file, []byte(Render(s)), 0o600); err != nil {
		return Skill{}, fmt.Errorf("agentskills: write %s: %w", file, err)
	}

	s.UpdatedAt = now
	s.BodyRunes = len([]rune(s.Body))
	s.ProjectID = strings.TrimSpace(s.ProjectID)
	return s, nil
}

// count is how many skills a scope already holds. A skill that does not parse
// still occupies a slot — it is a real directory with a real SKILL.md, and
// pretending otherwise would let the cap drift upward every time a file broke.
func (st *Store) count(scope Scope, projectID string) (int, error) {
	dir, err := st.Dir(scope, projectID)
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("agentskills: read %s: %w", dir, err)
	}
	n := 0
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if scope == ScopeWriter && name == worksSegment {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name, skillFile)); err == nil {
			n++
		}
	}
	return n, nil
}

// ---- Delete ---------------------------------------------------------------

// Delete removes a skill and everything in its directory — the SKILL.md and
// any sibling files it grew.
//
// It refuses a name it cannot find with ErrNotFound rather than reporting a
// silent success, because the caller is often an agent: one that deletes the
// wrong name and is told "done" will believe it tidied up when it did not.
//
// The SKILL.md is confirmed to exist before the directory is removed, so this
// can only ever recurse into a directory that really is a skill — an
// os.RemoveAll driven by a model's argument needs both that check and the
// path-boundary one above it.
func (st *Store) Delete(scope Scope, projectID, name string) error {
	dir, file, err := st.skillPaths(scope, projectID, name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(file); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %q", ErrNotFound, name)
		}
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("agentskills: delete %s: %w", dir, err)
	}
	return nil
}
