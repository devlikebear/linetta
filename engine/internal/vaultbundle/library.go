package vaultbundle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/atomicfile"
)

// LibraryFormat is the layout of a library item, separate from a work's.
const LibraryFormat = 1

const (
	librarySettingsPart = "settings"
	libraryProfilePart  = "profile"
	librarySkillPrefix  = "skills/"
	worksSkillDir       = "works"
)

// LibrarySettings is the writer's choice of what to carry. Omit is a list of
// category ids; UpdatedAt, in unix millis, decides between two devices.
type LibrarySettings struct {
	Omit      []string `json:"omit"`
	UpdatedAt int64    `json:"updated_at"`
}

// Omits says whether the selection leaves a category out.
func (l LibrarySettings) Omits(category string) bool {
	return slices.Contains(l.Omit, category)
}

// SameWorkCategories says whether two selections make the same work bundles.
func SameWorkCategories(a, b []string) bool {
	return slices.Equal(omittedWorkCategories(a), omittedWorkCategories(b))
}

// LibraryMeta is what a library item says besides its parts. The times stay out
// of the hash: two devices with the same text are in step, whenever they wrote
// it.
type LibraryMeta struct {
	Format           int              `json:"format"`
	Settings         LibrarySettings  `json:"settings"`
	ProfileUpdatedAt int64            `json:"profile_updated_at,omitempty"`
	FileTimes        map[string]int64 `json:"file_times,omitempty"`
}

// LibraryBundle holds everything that lives outside any work. Its parts are
// "settings", "profile" and "skills/<name>/<file>", moved the same way a work's
// are. Provider keys, tokens and agreements have no part here and never will:
// they belong to one device.
type LibraryBundle struct {
	Bundle
	Meta LibraryMeta
}

// ContentHash identifies the contents, on every device alike.
func (b *LibraryBundle) ContentHash() string { return b.Manifest.ContentHash() }

// Empty says the library holds nothing a writer would miss, so a device has no
// reason to create an item for it.
func (b *LibraryBundle) Empty() bool {
	return len(b.Parts) == 1 && len(b.Meta.Settings.Omit) == 0
}

// PartHashes maps each part to its hash, for comparing later.
func (b *LibraryBundle) PartHashes() map[string]string {
	hashes := make(map[string]string, len(b.Manifest.Parts))
	for _, part := range b.Manifest.Parts {
		hashes[part.Name] = part.SHA256
	}
	return hashes
}

// GlobalSkillsDir is where writer-scope skills live. Work skills are one level
// down, in worksSkillDir, and travel with their work.
func GlobalSkillsDir(home string) string {
	return filepath.Join(home, "skills")
}

// ExportLibrary reads the library outside any work, leaving out what the
// selection does. A skill that is a symlink stays here: it points at something
// the writer keeps elsewhere.
func ExportLibrary(ctx context.Context, db *sql.DB, home string, selection LibrarySettings, now time.Time) (*LibraryBundle, error) {
	bundle := &LibraryBundle{
		Bundle: Bundle{
			Manifest: Manifest{Format: FormatVersion, CreatedAt: now.UnixMilli()},
			Parts:    map[string][]byte{},
		},
		Meta: LibraryMeta{Format: LibraryFormat, Settings: selection, FileTimes: map[string]int64{}},
	}
	omit := slices.Clone(selection.Omit)
	slices.Sort(omit)
	if omit == nil {
		omit = []string{}
	}
	bundle.Meta.Settings.Omit = omit
	canonical, err := json.Marshal(omit)
	if err != nil {
		return nil, err
	}
	bundle.add(librarySettingsPart, canonical)
	if !selection.Omits(CategoryWriterProfile) {
		doc, err := agentmemory.NewRepo(db).Load(ctx, agentmemory.ScopeWriterProfile, "")
		if err != nil {
			return nil, err
		}
		if doc.Body != "" {
			bundle.add(libraryProfilePart, []byte(doc.Body))
			bundle.Meta.ProfileUpdatedAt = doc.UpdatedAt
		}
	}
	if !selection.Omits(CategoryGlobalSkills) {
		if err := exportGlobalSkills(bundle, GlobalSkillsDir(home)); err != nil {
			return nil, err
		}
	}
	bundle.seal()
	return bundle, nil
}

func exportGlobalSkills(bundle *LibraryBundle, root string) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == worksSkillDir || strings.HasPrefix(name, ".") {
			continue
		}
		dir := filepath.Join(root, name)
		err := filepath.WalkDir(dir, func(file string, item fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.HasPrefix(item.Name(), ".") {
				if item.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !item.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(root, file)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			info, err := item.Info()
			if err != nil {
				return err
			}
			part := librarySkillPrefix + filepath.ToSlash(rel)
			bundle.add(part, data)
			bundle.Meta.FileTimes[part] = info.ModTime().UnixMilli()
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidateLibrary checks a library bundle before anything is touched.
func ValidateLibrary(bundle *LibraryBundle) error {
	if bundle.Meta.Format > LibraryFormat {
		return ErrNewerFormat
	}
	if _, ok := bundle.Parts[librarySettingsPart]; !ok {
		return fmt.Errorf("%w: library has no settings", ErrCorrupt)
	}
	for _, part := range bundle.Manifest.Parts {
		data, ok := bundle.Parts[part.Name]
		if !ok {
			return fmt.Errorf("%w: part %s is missing", ErrCorrupt, part.Name)
		}
		if len(data) != part.Size || hashHex(data) != part.SHA256 {
			return fmt.Errorf("%w: part %s does not match", ErrCorrupt, part.Name)
		}
		if err := checkLibraryPart(part.Name); err != nil {
			return err
		}
	}
	return nil
}

// checkLibraryPart keeps a part inside the writer's own skills. It refuses the
// works directory by name: a library item must not reach into a work.
func checkLibraryPart(name string) error {
	if name == librarySettingsPart || name == libraryProfilePart {
		return nil
	}
	rel, ok := strings.CutPrefix(name, librarySkillPrefix)
	if !ok || !filepath.IsLocal(filepath.FromSlash(rel)) || path.Clean(rel) != rel {
		return fmt.Errorf("%w: unsafe path %q", ErrCorrupt, name)
	}
	skill, _, nested := strings.Cut(rel, "/")
	if !nested || skill == worksSkillDir || strings.HasPrefix(skill, ".") {
		return fmt.Errorf("%w: unsafe path %q", ErrCorrupt, name)
	}
	return nil
}

// LibraryApplied says what taking a library bundle did. ProfileRefused means the
// other device's writer profile could not be stored here and this one stands.
type LibraryApplied struct {
	Taken          []string
	Removed        []string
	ProfileRefused bool
}

// ApplyLibrary merges a library bundle into this library. base holds each part's
// hash as of the last time the two met. A part only one side changed goes that
// way, including a delete, and when both changed it the newer one wins without
// asking: this is the writer's own material, and the loser stays in the skill's
// history here and among the restore points there. Categories either side left
// out are not touched.
func ApplyLibrary(ctx context.Context, db *sql.DB, home string, remote *LibraryBundle, base map[string]string, local LibrarySettings, now time.Time) (LibraryApplied, error) {
	var out LibraryApplied
	skip := func(category string) bool {
		return local.Omits(category) || remote.Meta.Settings.Omits(category)
	}
	current, err := ExportLibrary(ctx, db, home, LibrarySettings{}, now)
	if err != nil {
		return out, err
	}
	localHashes, remoteHashes := current.PartHashes(), remote.PartHashes()
	names := slices.Sorted(maps.Keys(remoteHashes))
	for name := range localHashes {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	apply := libraryApplier{ctx: ctx, db: db, home: home, remote: remote, now: now}
	for _, name := range names {
		if name == librarySettingsPart {
			continue
		}
		category := CategoryGlobalSkills
		if name == libraryProfilePart {
			category = CategoryWriterProfile
		}
		if skip(category) {
			continue
		}
		verdict := mergePart(localHashes[name], remoteHashes[name], base[name], current.partTime(name), remote.partTime(name))
		if verdict == keepLocal {
			continue
		}
		if err := apply.part(name, verdict, &out); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (b *LibraryBundle) partTime(name string) int64 {
	if name == libraryProfilePart {
		return b.Meta.ProfileUpdatedAt
	}
	return b.Meta.FileTimes[name]
}

type mergeVerdict int

const (
	keepLocal mergeVerdict = iota
	takeRemote
	removeLocal
)

// mergePart decides one part. An empty hash means that side does not have it.
func mergePart(local, remote, base string, localTime, remoteTime int64) mergeVerdict {
	switch {
	case local == remote:
		return keepLocal
	case remote == base:
		return keepLocal // only this device changed it, or deleted it
	case local == base:
		if remote == "" {
			return removeLocal
		}
		return takeRemote
	case remote == "":
		return keepLocal // deleted there, changed here: the edit stands
	case local == "":
		return takeRemote // the same, the other way round
	case remoteTime > localTime, remoteTime == localTime && remote > local:
		return takeRemote
	}
	return keepLocal
}

type libraryApplier struct {
	ctx    context.Context
	db     *sql.DB
	home   string
	remote *LibraryBundle
	now    time.Time
}

func (a libraryApplier) part(name string, verdict mergeVerdict, out *LibraryApplied) error {
	if name == libraryProfilePart {
		return a.profile(verdict, out)
	}
	skill, rel, _ := strings.Cut(strings.TrimPrefix(name, librarySkillPrefix), "/")
	dir := filepath.Join(GlobalSkillsDir(a.home), skill)
	if info, err := os.Lstat(dir); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return nil // a link is the writer's own arrangement; exporting skips it too
	}
	file := filepath.Join(dir, filepath.FromSlash(rel))
	if verdict == removeLocal {
		a.remember(skill, rel, agentskills.ReasonDeleted)
		if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		_ = os.Remove(dir) // only succeeds when that was the skill's last file
		out.Removed = append(out.Removed, name)
		return nil
	}
	a.remember(skill, rel, agentskills.ReasonEdited)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	if err := atomicfile.Write(file, a.remote.Parts[name], 0o600); err != nil {
		return err
	}
	// The other device's time, or this file would read as the newer edit.
	if when, ok := a.remote.Meta.FileTimes[name]; ok {
		stamp := time.UnixMilli(when)
		if err := os.Chtimes(file, stamp, stamp); err != nil {
			return err
		}
	}
	out.Taken = append(out.Taken, name)
	return nil
}

// remember keeps a SKILL.md in the skill's history before it is replaced. A file
// that does not parse has no version to keep, and is no reason to stop.
func (a libraryApplier) remember(skill, rel, reason string) {
	if rel != "SKILL.md" {
		return
	}
	current, err := agentskills.NewStore(a.home).Read(agentskills.ScopeWriter, "", skill)
	if errors.Is(err, agentskills.ErrNotFound) {
		return
	}
	if err != nil {
		log.Printf("vault: keep skill %s before replacing it: %v", skill, err)
		return
	}
	if err := agentskills.NewHistory(a.db).Record(a.ctx, current, reason, a.now.UnixMilli()); err != nil {
		log.Printf("vault: keep skill %s before replacing it: %v", skill, err)
	}
}

// profile stores the other device's writer profile, or refuses it whole: a
// memory cut to fit would quietly lose its end. The budget is checked here
// rather than left to Save, whose shrink-only allowance is for digging out of an
// oversized document, not for taking one in.
func (a libraryApplier) profile(verdict mergeVerdict, out *LibraryApplied) error {
	body, when := "", a.now.UnixMilli()
	if verdict == takeRemote {
		body, when = string(a.remote.Parts[libraryProfilePart]), a.remote.Meta.ProfileUpdatedAt
	}
	if utf8.RuneCountInString(body) > agentmemory.ScopeWriterProfile.Budget() {
		out.ProfileRefused = true
		return nil
	}
	if _, err := agentmemory.NewRepo(a.db).Save(a.ctx, agentmemory.ScopeWriterProfile, "", body, when); err != nil {
		if errors.Is(err, agentmemory.ErrInvisible) || errors.Is(err, agentmemory.ErrControl) || errors.Is(err, agentmemory.ErrDelimiter) {
			out.ProfileRefused = true
			return nil
		}
		return err
	}
	if verdict == takeRemote {
		out.Taken = append(out.Taken, libraryProfilePart)
	} else {
		out.Removed = append(out.Removed, libraryProfilePart)
	}
	return nil
}

// Sizes adds up what each category holds across the library, as it would travel.
// The key "" is everything that always travels.
func Sizes(ctx context.Context, db *sql.DB, home string, now time.Time) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM projects`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sizes := map[string]int64{}
	for _, id := range ids {
		bundle, err := Export(ctx, db, home, id, now, nil)
		if err != nil {
			return nil, err
		}
		for _, part := range bundle.Manifest.Parts {
			sizes[PartCategory(part.Name)] += int64(part.Size)
		}
	}
	library, err := ExportLibrary(ctx, db, home, LibrarySettings{}, now)
	if err != nil {
		return nil, err
	}
	for _, part := range library.Manifest.Parts {
		switch {
		case part.Name == libraryProfilePart:
			sizes[CategoryWriterProfile] += int64(part.Size)
		case strings.HasPrefix(part.Name, librarySkillPrefix):
			sizes[CategoryGlobalSkills] += int64(part.Size)
		}
	}
	return sizes, nil
}
