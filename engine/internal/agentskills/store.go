package agentskills

import (
	"errors"
	"fmt"
	"io"
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
//
// It is the only name the store itself reserves, because it is the only path
// under a scope directory that the store manages rather than the writer. The
// other name the store owns — skillFile — sits one level deeper, inside a
// skill's own directory, so no skill NAME can collide with it.
const worksSegment = "works"

// maxSkillFileBytes bounds what readSkillFile will pull into memory before
// anything has decided the file is a skill at all. Without it one hand-placed
// gigabyte at <home>/skills/x/SKILL.md makes List allocate the whole thing
// just to have Guard reject it afterwards.
//
// A SKILL.md that can pass Guard holds at most MaxBodyRunes (8000) runes of
// body plus maxFrontmatterRunes (4096) runes of frontmatter; at UTF-8's worst
// case of four bytes per rune that is a little under 48 KiB, so 64 KiB is the
// smallest round cap that cannot refuse a file the rest of the package would
// have accepted. Over it, the file is ErrTooLong — a diagnostic in List, an
// error in Read — which is what an over-long body is anyway.
const maxSkillFileBytes = 64 << 10

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

	// ErrPathOccupied is something other than the skill occupying the skill's
	// path: a plain file where the skill's directory belongs, or a SKILL.md
	// that is itself a directory. It is deliberately distinct from a write
	// failure — nothing here is corrupt, the writer just has something else
	// sitting in the way, and the fix is to move it.
	//
	// It is NOT "a skill by that name already exists" — Write is an upsert and
	// overwriting is normal. The name says "path occupied" rather than
	// "exists" so that the tool layer's create action can own the plain
	// already-exists meaning without the two being confused for each other.
	ErrPathOccupied = errors.New("agentskills: another file is already at that path")
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

// isReservedSegment reports whether name would resolve to a directory the
// store manages rather than to a skill of the writer's.
//
// The comparison is case-INSENSITIVE, and that is the whole point. On the
// case-insensitive filesystems this app ships to (macOS, Windows) "WORKS"
// opens the very same directory as "works", so an exact compare would let
// Delete(ScopeWriter, "", "WORKS") RemoveAll the root holding every
// work-scoped skill in the home. An exact compare is safe ONLY as long as
// ValidName forbids uppercase — that is trusting a check in another file to
// hold a path boundary here, which is exactly what this file promises not to
// do (see the block comment above). Reproduced before the fix: with the
// ValidName call removed from checkName, "WORKS" deleted <home>/skills/works
// and everything under it.
func isReservedSegment(name string) bool {
	return strings.EqualFold(name, worksSegment)
}

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
	if scope == ScopeWriter && isReservedSegment(name) {
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
// a SKILL.md that does not parse, one that is too big to read, one the guard
// refuses (it must never reach a prompt, but the writer must still see that
// it is there), a directory whose SKILL.md is missing or is itself a
// directory, and a name in the frontmatter that disagrees with the folder it
// sits in.
//
// That last one is dropped because List addresses skills by folder name and
// the caller would otherwise get back a skill under a name that does not
// locate it. Being precise about the direction, since the earlier version of
// this comment was not: on a case-SENSITIVE filesystem "MixedCase" holding
// `name: mixedcase` is unreachable by either name, and dropping it is the
// only honest answer. On a case-INSENSITIVE one (macOS, Windows)
// Read/Delete("mixedcase") do happen to reach it — that is the filesystem
// folding the name, not a promise this store makes, and the same call fails
// on Linux. So: everything List RETURNS can be Read and Deleted by its Name;
// the converse does not hold, and no caller should assume it does.
//
// Hidden entries (a leading dot: .git, .DS_Store) are skipped in silence.
// They are not the writer's mistake, and reporting them would bury the
// diagnostics that are.
//
// Entries are classified with os.Stat, which FOLLOWS symlinks, and that is
// intended: a writer who symlinks a shared or version-controlled skills
// folder in here should have it work. A dangling or looping link stats with
// an error and is skipped rather than reported, since it is not a skill that
// broke. Write follows a symlinked skill directory too — MkdirAll on an
// existing link is a no-op and atomicfile.Write then writes through it — so
// editing a linked skill through this store lands the new SKILL.md at the
// link's target, which is what a writer who made the link would want. The
// one place the store must never follow a link is Delete — see there.
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
		if scope == ScopeWriter && isReservedSegment(name) {
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
		// The guard runs HERE rather than inside readSkillFile, so that Read
		// can still hand a guard-failing skill back for repair (see Read).
		// A skill the guard refuses is never listed — it must not reach a
		// prompt — but it is always reported, which is the whole point: the
		// writer has to be told which skill is broken and why before they can
		// fix it.
		if err := Guard(s); err != nil {
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

// readSkillFile loads and parses one SKILL.md, filling the two fields that
// are metadata rather than document content: UpdatedAt comes from the file's
// modification time (it is not, and should not be, written into the file —
// a writer editing SKILL.md in their own editor updates it for free) and
// BodyRunes is derived.
//
// It does NOT run Guard. Storage reads what is on disk; screening is the
// write path's job and the prompt's job (see Read and Guard's placement).
//
// It returns ErrNotFound for an absent file and ErrPathOccupied for a SKILL.md that
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
		return Skill{}, fmt.Errorf("%w: %s is a directory, not a file", ErrPathOccupied, file)
	}

	raw, err := readCapped(file)
	if err != nil {
		return Skill{}, err
	}
	s, err := Parse(string(raw))
	if err != nil {
		return Skill{}, err
	}
	s.UpdatedAt = info.ModTime().UnixMilli()
	s.BodyRunes = len([]rune(s.Body))
	return s, nil
}

// readCapped reads a SKILL.md without letting one hand-placed huge file
// decide how much memory this process allocates.
//
// The read itself is bounded rather than the stat that precedes it: a stat
// followed by an unbounded ReadFile is a race, and the file here is one the
// writer or another agent can be editing at the same moment. One byte over
// the cap is enough to know the file is too big, so the reader asks for
// maxSkillFileBytes+1 and refuses if it got them all.
func readCapped(file string) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	raw, err := io.ReadAll(io.LimitReader(f, maxSkillFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxSkillFileBytes {
		return nil, fmt.Errorf("%w: %s is over the %d-byte file limit", ErrTooLong, file, maxSkillFileBytes)
	}
	return raw, nil
}

// ---- Read -----------------------------------------------------------------

// Read returns one skill by name. A skill that is not there is ErrNotFound; a
// SKILL.md that is a directory is ErrPathOccupied; a file that does not parse
// is that failure's own error. Unlike List, this addresses exactly one skill,
// so there is nothing for a diagnostic to protect — the caller asked about
// this file and wants to hear what is wrong with it.
//
// Read deliberately does NOT run Guard, and this is a reversal of the first
// version of this file. Guarding here made a guard-failing skill visible in
// Settings (as a List diagnostic) but impossible to fetch, so the writer was
// told "this skill is broken" with no way to open it — and an 8050-rune body
// cannot be trimmed by someone who cannot read it. A broken skill has to be
// both visible AND fixable, and Read is the fixing half.
//
// The verdict is not swallowed either: the caller has the Skill, and Guard is
// exported and pure, so `Guard(s)` on the returned value IS the accessor.
// That is the choice made here rather than a new field on Skill, because
// Skill is the SKILL.md document (skill.go) and a guard verdict is a fact
// about a moment, not a line of the document — putting it in the struct would
// have it rendered, parsed and stored right along with the rest.
//
// Nothing over-long or invisible-character-laden reaches a model as a result:
// Write guards before it stores, List guards before it lists, and prompt
// assembly (Task 6) guards every skill again on its way into the prompt.
// Storage is not the screening layer.
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

// ReadRaw returns a SKILL.md's text exactly as it is on disk, with the
// modification time Read would have reported, and does not parse it.
//
// It is the last half of the rule Read's doc comment states: a broken skill
// must be visible AND fixable. Read already refuses to GUARD, so an
// over-long body can be opened and trimmed — but it still PARSES, and a
// SKILL.md whose frontmatter the writer broke by hand (a missing fence, a
// stray tab in the YAML) does not parse, so Read cannot hand back the one
// thing that would let anyone repair it: the text. Settings listed such a
// file as a diagnostic and then had no way to open it.
//
// Parsing stays where it is. Every caller that wants a Skill wants it
// parsed, and a Read that returned half-filled structs for unparseable
// files would push that check onto all of them. This is the narrower door
// for the one caller that wants the bytes: it resolves the path through
// skillPaths (so the same name and containment checks apply) and reads
// through readCapped (so the same file-size ceiling applies), and that is
// all it does.
//
// A file that is not there is ErrNotFound, matching Read.
func (st *Store) ReadRaw(scope Scope, projectID, name string) (string, int64, error) {
	_, file, err := st.skillPaths(scope, projectID, name)
	if err != nil {
		return "", 0, err
	}
	info, err := os.Stat(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", 0, fmt.Errorf("%w: %q", ErrNotFound, name)
		}
		return "", 0, err
	}
	if info.IsDir() {
		return "", 0, fmt.Errorf("%w: %s is a directory, not a file", ErrPathOccupied, file)
	}
	raw, err := readCapped(file)
	if err != nil {
		return "", 0, err
	}
	return string(raw), info.ModTime().UnixMilli(), nil
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
//
// Symlinks: if the skill's directory is itself a symlink, Write follows it —
// os.MkdirAll on an existing link is a no-op, and atomicfile.Write's rename
// then lands the new SKILL.md at the link's target, outside home. This is
// intentional and consistent with List and Read (see List's doc comment): a
// writer who deliberately links a shared or version-controlled skills folder
// into <home>/skills should be able to edit it through this store too, not
// just read it. Delete is the sole exception, and for a reason specific to
// it — see Delete's doc comment.
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
		return Skill{}, fmt.Errorf("%w: %s is not a directory", ErrPathOccupied, dir)
	}

	if _, err := os.Stat(file); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return Skill{}, err
		}
		// A skill that does not exist yet is the only kind the cap can refuse.
		// count excludes s.Name — the directory THIS write is about to fill —
		// so a half-made directory sitting at that exact name (no SKILL.md
		// yet, whether from a Write that died between MkdirAll and
		// atomicfile.Write, or a folder made by hand) is judged as what it
		// is: the slot this write would occupy, not one more slot on top of
		// it. See count's doc comment.
		n, err := st.count(s.Scope, s.ProjectID, s.Name)
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

// count is how many OTHER slots a scope already holds — "other" than
// exclude, which is always the name the caller is about to write.
//
// It counts exactly the entries List treats as a skill directory — every
// non-hidden subdirectory that is not the works root — and does NOT require a
// SKILL.md inside. That is deliberate, and it is the second version of this
// function: requiring the SKILL.md let forty half-made folders sit alongside
// forty real skills, doubling a cap whose whole job is to bound how much of
// this the writer has to keep track of. A directory here is a slot whether or
// not its document parses, whether or not the document is even there yet: it
// is a real folder taking a real name, List already reports it, and excluding
// it would let the cap drift upward every time something on disk broke.
//
// exclude is the third version's fix, and the reason it exists: a directory
// with no SKILL.md counts as a slot by the rule above, but when that
// directory is the very one Write is about to fill, counting it double-counts
// the same slot — once as "already there", once as "about to be created" —
// and refuses the write that would turn a half-made directory into a real
// skill. There is then no way to clear that slot at all: Delete refuses it
// too (a bare SKILL.md stat finds nothing), so the state is permanent until
// this exclusion existed. What matters for the cap is room for every OTHER
// skill plus this one, so this one is left out of the count and checked
// against the cap by the caller instead (n >= MaxSkillsPerScope). A name that
// does not already occupy a directory costs nothing to exclude — it simply
// is not in entries — so forty half-made directories under forty OTHER names
// still refuse a forty-first real skill, exactly as before.
func (st *Store) count(scope Scope, projectID, exclude string) (int, error) {
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
		if scope == ScopeWriter && isReservedSegment(name) {
			continue
		}
		if name == exclude {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.IsDir() {
			continue
		}
		n++
	}
	return n, nil
}

// ---- Delete ---------------------------------------------------------------

// Delete removes a skill and everything in its directory — the SKILL.md and
// any sibling files it grew.
//
// It refuses a name it cannot find at all with ErrNotFound rather than
// reporting a silent success, because the caller is often an agent: one that
// deletes the wrong name and is told "done" will believe it tidied up when it
// did not.
//
// A directory that exists but has no SKILL.md — a half-made skill, whether
// from a Write that died between MkdirAll and atomicfile.Write, or a folder a
// writer made by hand — is NOT "not found": something is there, it occupies
// exactly one of the scope's forty slots (see count), and it must be
// removable, because nothing else in this package can clear it (Write only
// refuses that name a fresh cap slot now — see count's exclude parameter — it
// does not remove the directory, and there is no separate "delete a folder"
// call). So Delete removes it and reports success: the caller asked for the
// name to be gone, and after this call it is. The alternative — a distinct
// error naming the half-made state — would still leave the caller with no
// path to clear it, which is the one outcome this fix rules out.
//
// The SKILL.md is confirmed to exist before deleting a real skill, so a
// RemoveAll on a directory that already has a document can only ever recurse
// into a directory that really is a skill — an os.RemoveAll driven by a
// model's argument needs both that check and the path-boundary one above it.
// For the half-made case, the check instead confirms dir itself is a
// directory (not nothing, not a stray file) before RemoveAll runs.
//
// Symlinks: List, Read and Write follow them on purpose (a writer may link a
// shared skills folder in and expect edits through this store to land there
// too), but Delete must not, and os.RemoveAll does not — it unlinks the
// symlink itself and never recurses through it, so deleting a linked skill
// removes the link and leaves the writer's real folder intact. This is the
// one symlink behaviour where being wrong destroys data that is not ours, so
// TestDeleteDoesNotFollowASymlink pins it. The same os.Stat-then-RemoveAll
// shape covers the half-made case too: Stat follows the link only to check
// it is a directory, RemoveAll itself never does.
func (st *Store) Delete(scope Scope, projectID, name string) error {
	dir, file, err := st.skillPaths(scope, projectID, name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(file); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		// No SKILL.md. Distinguish "nothing here at all" from "a half-made
		// directory sits here" — see the doc comment above.
		info, statErr := os.Stat(dir)
		if statErr != nil || !info.IsDir() {
			return fmt.Errorf("%w: %q", ErrNotFound, name)
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("agentskills: delete %s: %w", dir, err)
	}
	return nil
}
