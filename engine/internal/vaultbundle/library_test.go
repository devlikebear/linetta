package vaultbundle

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/agentskills"
)

const skillBody = "---\nname: %s\ndescription: 설명\nauthor: writer\n---\n"

type lib struct {
	db   *sql.DB
	home string
}

func newLib(t *testing.T) lib { return lib{db: openLibrary(t), home: t.TempDir()} }

func skillFile(home, name string) string {
	return filepath.Join(GlobalSkillsDir(home), name, "SKILL.md")
}

func (l lib) skill(t *testing.T, name, body string, when time.Time) {
	t.Helper()
	writeFile(t, skillFile(l.home, name), strings.ReplaceAll(skillBody, "%s", name)+body)
	if err := os.Chtimes(skillFile(l.home, name), when, when); err != nil {
		t.Fatal(err)
	}
}

func (l lib) profile(t *testing.T, body string, when int64) {
	t.Helper()
	if _, err := agentmemory.NewRepo(l.db).Save(context.Background(), agentmemory.ScopeWriterProfile, "", body, when); err != nil {
		t.Fatal(err)
	}
}

func (l lib) export(t *testing.T, selection LibrarySettings) *LibraryBundle {
	t.Helper()
	bundle, err := ExportLibrary(context.Background(), l.db, l.home, selection, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func (l lib) apply(t *testing.T, remote *LibraryBundle, base map[string]string, selection LibrarySettings) LibraryApplied {
	t.Helper()
	if err := ValidateLibrary(remote); err != nil {
		t.Fatal(err)
	}
	out, err := ApplyLibrary(context.Background(), l.db, l.home, remote, base, selection, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (l lib) read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(skillFile(l.home, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (l lib) history(t *testing.T, name string) []agentskills.Version {
	t.Helper()
	versions, err := agentskills.NewHistory(l.db).List(context.Background(), agentskills.ScopeWriter, "", name, 0)
	if err != nil {
		t.Fatal(err)
	}
	return versions
}

func (l lib) profileBody(t *testing.T) agentmemory.Document {
	t.Helper()
	doc, err := agentmemory.NewRepo(l.db).Load(context.Background(), agentmemory.ScopeWriterProfile, "")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

var (
	earlier = time.UnixMilli(1_700_000_000_000)
	later   = time.UnixMilli(1_700_000_500_000)
)

func TestLibraryCarriesSkillsAndProfileToAnEmptyLibrary(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.skill(t, "style", "짧게 쓴다", earlier)
	writeFile(t, filepath.Join(GlobalSkillsDir(a.home), "style", "refs", "note.md"), "참고")
	a.profile(t, "존댓말을 쓴다", 123)

	out := b.apply(t, a.export(t, LibrarySettings{}), nil, LibrarySettings{})

	if len(out.Taken) != 3 {
		t.Fatalf("taken %v", out.Taken)
	}
	if got := b.read(t, "style"); !strings.Contains(got, "짧게 쓴다") {
		t.Errorf("skill = %q", got)
	}
	if data, err := os.ReadFile(filepath.Join(GlobalSkillsDir(b.home), "style", "refs", "note.md")); err != nil || string(data) != "참고" {
		t.Errorf("sibling file = %q, %v", data, err)
	}
	doc := b.profileBody(t)
	if doc.Body != "존댓말을 쓴다" || doc.UpdatedAt != 123 {
		t.Errorf("profile = %+v", doc)
	}
	if a.export(t, LibrarySettings{}).ContentHash() != b.export(t, LibrarySettings{}).ContentHash() {
		t.Error("two libraries in step hash differently")
	}
	info, err := os.Stat(skillFile(b.home, "style"))
	if err != nil || !info.ModTime().Equal(earlier) {
		t.Errorf("time = %v, %v", info.ModTime(), err)
	}
}

func TestLibraryNewerSideWinsAndTheLoserStaysInHistory(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.skill(t, "style", "새 글", later)
	b.skill(t, "style", "옛 글", earlier)

	b.apply(t, a.export(t, LibrarySettings{}), nil, LibrarySettings{})

	if !strings.Contains(b.read(t, "style"), "새 글") {
		t.Error("the newer side lost")
	}
	versions := b.history(t, "style")
	if len(versions) != 1 || !strings.Contains(versions[0].Skill.Body, "옛 글") {
		t.Errorf("history = %+v", versions)
	}

	a2, b2 := newLib(t), newLib(t)
	a2.skill(t, "style", "옛 글", earlier)
	b2.skill(t, "style", "새 글", later)
	out := b2.apply(t, a2.export(t, LibrarySettings{}), nil, LibrarySettings{})
	if len(out.Taken) != 0 || !strings.Contains(b2.read(t, "style"), "새 글") {
		t.Errorf("the older side won: %+v", out)
	}
}

func TestLibraryOnlyTheChangedSideMoves(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.skill(t, "style", "같은 글", earlier)
	b.skill(t, "style", "같은 글", earlier)
	base := a.export(t, LibrarySettings{}).PartHashes()

	// An edit there with an older clock still beats a file nobody touched here.
	a.skill(t, "style", "고친 글", earlier.Add(-time.Hour))
	b.apply(t, a.export(t, LibrarySettings{}), base, LibrarySettings{})
	if !strings.Contains(b.read(t, "style"), "고친 글") {
		t.Error("an edit on the other device did not arrive")
	}
}

func TestLibraryDeleteTravelsBothWays(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.skill(t, "style", "글", earlier)
	b.skill(t, "style", "글", earlier)
	a.skill(t, "second", "글", earlier)
	b.skill(t, "second", "글", earlier)
	base := a.export(t, LibrarySettings{}).PartHashes()

	// Deleted there: goes here, with the last version kept.
	if err := os.RemoveAll(filepath.Join(GlobalSkillsDir(a.home), "style")); err != nil {
		t.Fatal(err)
	}
	out := b.apply(t, a.export(t, LibrarySettings{}), base, LibrarySettings{})
	if len(out.Removed) != 1 {
		t.Fatalf("removed %v", out.Removed)
	}
	if _, err := os.Stat(filepath.Join(GlobalSkillsDir(b.home), "style")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the skill folder stayed: %v", err)
	}
	if versions := b.history(t, "style"); len(versions) != 1 || versions[0].Reason != agentskills.ReasonDeleted {
		t.Errorf("history = %+v", versions)
	}

	// Deleted here and untouched there: stays gone, and the next upload says so.
	if err := os.RemoveAll(filepath.Join(GlobalSkillsDir(b.home), "second")); err != nil {
		t.Fatal(err)
	}
	out = b.apply(t, a.export(t, LibrarySettings{}), a.export(t, LibrarySettings{}).PartHashes(), LibrarySettings{})
	if len(out.Taken) != 0 {
		t.Fatalf("a deleted skill came back: %v", out.Taken)
	}
}

func TestLibraryEditBeatsDelete(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.skill(t, "style", "글", earlier)
	b.skill(t, "style", "글", earlier)
	base := a.export(t, LibrarySettings{}).PartHashes()

	if err := os.RemoveAll(filepath.Join(GlobalSkillsDir(a.home), "style")); err != nil {
		t.Fatal(err)
	}
	b.skill(t, "style", "고친 글", later)
	b.apply(t, a.export(t, LibrarySettings{}), base, LibrarySettings{})
	if !strings.Contains(b.read(t, "style"), "고친 글") {
		t.Error("the edit was lost to a delete")
	}
}

func TestLibraryWorkSkillsStayOut(t *testing.T) {
	a := newLib(t)
	a.skill(t, "style", "글", earlier)
	writeFile(t, filepath.Join(SkillDir(a.home, "work-1"), "작품스킬", "SKILL.md"), "작품 스킬")
	writeFile(t, filepath.Join(GlobalSkillsDir(a.home), ".hidden", "SKILL.md"), "숨김")
	writeFile(t, filepath.Join(GlobalSkillsDir(a.home), "style", ".DS_Store"), "x")

	for name := range a.export(t, LibrarySettings{}).Parts {
		if strings.Contains(name, "works") || strings.Contains(name, "hidden") || strings.Contains(name, "DS_Store") {
			t.Errorf("part %s should not travel", name)
		}
	}
}

func TestLibrarySymlinkedSkillStaysHere(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.skill(t, "linked", "다른 곳의 글", earlier)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "SKILL.md"), "내 원본")
	if err := os.MkdirAll(GlobalSkillsDir(b.home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(GlobalSkillsDir(b.home), "linked")); err != nil {
		t.Fatal(err)
	}

	for name := range b.export(t, LibrarySettings{}).Parts {
		if strings.HasPrefix(name, "skills/") {
			t.Errorf("a linked skill was exported: %s", name)
		}
	}
	b.apply(t, a.export(t, LibrarySettings{}), nil, LibrarySettings{})
	data, err := os.ReadFile(filepath.Join(outside, "SKILL.md"))
	if err != nil || string(data) != "내 원본" {
		t.Errorf("wrote through the link: %q, %v", data, err)
	}
}

func TestLibraryOmittedCategoriesStayOutAndAreLeftAlone(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.skill(t, "style", "거기", earlier)
	a.profile(t, "거기 프로필", 100)
	b.skill(t, "other", "여기", earlier)
	b.profile(t, "여기 프로필", 50)
	selection := LibrarySettings{Omit: []string{CategoryWriterProfile, CategoryGlobalSkills}}

	bundle := a.export(t, selection)
	if names := partNames(&bundle.Bundle); !slices.Equal(names, []string{"settings"}) {
		t.Fatalf("parts = %v", names)
	}
	out := b.apply(t, bundle, nil, LibrarySettings{})
	if len(out.Taken)+len(out.Removed) != 0 {
		t.Fatalf("touched %+v", out)
	}
	if b.profileBody(t).Body != "여기 프로필" || !strings.Contains(b.read(t, "other"), "여기") {
		t.Error("a category the bundle left out was changed")
	}

	// The same when only this device has it off.
	out = b.apply(t, a.export(t, LibrarySettings{}), nil, selection)
	if len(out.Taken)+len(out.Removed) != 0 {
		t.Fatalf("touched %+v", out)
	}
}

func TestLibraryRefusesAnOversizedProfileWhole(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.profile(t, "짧다", 100)
	bundle := a.export(t, LibrarySettings{})
	long := strings.Repeat("가", agentmemory.ScopeWriterProfile.Budget()+1)
	bundle.Parts[libraryProfilePart] = []byte(long)
	bundle.seal()
	b.profile(t, "여기 프로필", 50)

	out := b.apply(t, bundle, nil, LibrarySettings{})

	if !out.ProfileRefused {
		t.Fatal("not refused")
	}
	if got := b.profileBody(t).Body; got != "여기 프로필" {
		t.Errorf("profile = %q", got)
	}
}

func TestLibraryNewerProfileWinsAndDeleteTravels(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.profile(t, "옛 프로필", 100)
	b.profile(t, "새 프로필", 200)
	b.apply(t, a.export(t, LibrarySettings{}), nil, LibrarySettings{})
	if got := b.profileBody(t).Body; got != "새 프로필" {
		t.Fatalf("profile = %q", got)
	}

	a.profile(t, "새 프로필", 300)
	base := a.export(t, LibrarySettings{}).PartHashes()
	a.profile(t, "", 400)
	b.apply(t, a.export(t, LibrarySettings{}), base, LibrarySettings{})
	if got := b.profileBody(t).Body; got != "" {
		t.Errorf("profile = %q", got)
	}
}

func TestLibraryHoldsNoSecrets(t *testing.T) {
	a := newLib(t)
	a.skill(t, "style", "글", earlier)
	a.profile(t, "프로필", 1)
	for _, name := range []string{"settings.json", "secrets.json", "publish-token", "vault-key"} {
		writeFile(t, filepath.Join(a.home, name), "sk-secret")
	}
	for name, data := range a.export(t, LibrarySettings{}).Parts {
		if strings.Contains(string(data), "sk-secret") {
			t.Errorf("part %s holds a secret", name)
		}
	}
}

func TestLibraryHashIgnoresTimesAndFollowsTheSelection(t *testing.T) {
	a := newLib(t)
	a.skill(t, "style", "글", earlier)
	first := a.export(t, LibrarySettings{UpdatedAt: 1}).ContentHash()
	a.skill(t, "style", "글", later)
	if a.export(t, LibrarySettings{UpdatedAt: 2}).ContentHash() != first {
		t.Error("touching a file or the selection's clock changed the hash")
	}
	if a.export(t, LibrarySettings{Omit: []string{CategoryImages}}).ContentHash() == first {
		t.Error("changing the selection did not change the hash")
	}
}

func TestEmptyLibraryIsEmpty(t *testing.T) {
	a := newLib(t)
	if !a.export(t, LibrarySettings{}).Empty() {
		t.Error("nothing to carry, but not empty")
	}
	if a.export(t, LibrarySettings{Omit: []string{CategoryImages}}).Empty() {
		t.Error("a selection is something to carry")
	}
	a.profile(t, "프로필", 1)
	if a.export(t, LibrarySettings{}).Empty() {
		t.Error("a profile is something to carry")
	}
}

func TestUnsafeLibraryPartsAreRefused(t *testing.T) {
	for _, name := range []string{"skills/works/P/x/SKILL.md", "skills/../../etc/x", "skills/SKILL.md", "skills/.hidden/SKILL.md", "memory/x", "skills/a//b"} {
		a := newLib(t)
		bundle := a.export(t, LibrarySettings{})
		bundle.Parts[name] = []byte("x")
		bundle.seal()
		if err := ValidateLibrary(bundle); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestNewerLibraryIsRefused(t *testing.T) {
	bundle := newLib(t).export(t, LibrarySettings{})
	bundle.Meta.Format = LibraryFormat + 1
	if err := ValidateLibrary(bundle); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("got %v", err)
	}
}

func TestSizesAddUpByCategory(t *testing.T) {
	l := newLib(t)
	seedWork(t, l.db, workID)
	l.skill(t, "style", "글", earlier)
	l.profile(t, "프로필", 1)
	writeFile(t, filepath.Join(MemoryDir(l.home, workID), "m.md"), "메모리")

	sizes, err := Sizes(context.Background(), l.db, l.home, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range append(Categories(), "") {
		if sizes[category] <= 0 {
			t.Errorf("%q = %d", category, sizes[category])
		}
	}
}
