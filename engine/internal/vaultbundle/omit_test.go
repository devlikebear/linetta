package vaultbundle

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
)

func exportOmit(t *testing.T, db *sql.DB, home, id string, omit []string) *Bundle {
	t.Helper()
	bundle, err := Export(context.Background(), db, home, id, fixedNow, omit)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func partNames(bundle *Bundle) []string {
	names := make([]string, 0, len(bundle.Parts))
	for name := range bundle.Parts {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func TestOmittedCategoryLeavesTheBundle(t *testing.T) {
	cases := []struct {
		category string
		gone     []string
	}{
		{CategoryImages, []string{"tables/illustrations", "tables/node_representative_illustrations", "tables/illustration_requests", "tables/entity_reference_images", "images/"}},
		{CategoryHistory, []string{"tables/node_snapshots"}},
		{CategoryWorkAgent, []string{"tables/agent_memory", "tables/agent_memory_archive", "files/skills/", "files/memory/"}},
	}
	for _, tc := range cases {
		t.Run(tc.category, func(t *testing.T) {
			db := openLibrary(t)
			home := t.TempDir()
			seedWork(t, db, workID)
			writeFile(t, filepath.Join(SkillDir(home, workID), "a", "SKILL.md"), "스킬")
			writeFile(t, filepath.Join(MemoryDir(home, workID), "m.md"), "메모리")

			bundle := exportOmit(t, db, home, workID, []string{tc.category})

			for _, name := range partNames(bundle) {
				for _, gone := range tc.gone {
					if strings.HasPrefix(name, gone) {
						t.Errorf("part %s should have been left out", name)
					}
				}
			}
			if !slices.Equal(bundle.Manifest.Omitted, []string{tc.category}) {
				t.Errorf("Omitted = %v", bundle.Manifest.Omitted)
			}
			for _, kept := range []string{"tables/projects", "tables/nodes", "tables/entities"} {
				if _, ok := bundle.Parts[kept]; !ok {
					t.Errorf("required part %s is gone", kept)
				}
			}
		})
	}
}

func TestOmittingChangesTheHashAndNothingOmittedDoesNot(t *testing.T) {
	db := openLibrary(t)
	home := t.TempDir()
	seedWork(t, db, workID)
	full := exportOmit(t, db, home, workID, nil).Manifest.ContentHash()
	if got := exportOmit(t, db, home, workID, []string{}).Manifest.ContentHash(); got != full {
		t.Error("an empty selection hashes differently from none")
	}
	if got := exportOmit(t, db, home, workID, []string{CategoryImages}).Manifest.ContentHash(); got == full {
		t.Error("leaving out the images did not change the hash")
	}
}

func TestLibraryCategoriesDoNotTouchAWorkBundle(t *testing.T) {
	db := openLibrary(t)
	home := t.TempDir()
	seedWork(t, db, workID)
	bundle := exportOmit(t, db, home, workID, []string{CategoryGlobalSkills, CategoryWriterProfile})
	if len(bundle.Manifest.Omitted) != 0 {
		t.Errorf("Omitted = %v", bundle.Manifest.Omitted)
	}
	if bundle.Manifest.ContentHash() != exportOmit(t, db, home, workID, nil).Manifest.ContentHash() {
		t.Error("a library category changed the work's hash")
	}
}

func TestOmittedRoundTripIntoAnEmptyLibrary(t *testing.T) {
	for _, category := range []string{CategoryImages, CategoryHistory, CategoryWorkAgent} {
		t.Run(category, func(t *testing.T) {
			src := openLibrary(t)
			seedWork(t, src, workID)
			bundle := exportOmit(t, src, t.TempDir(), workID, []string{category})

			dst := openLibrary(t)
			if err := apply(t, dst, bundle); err != nil {
				t.Fatal(err)
			}
			noForeignKeyViolations(t, dst)
			for _, table := range allTables() {
				got := dump(t, dst, table, workID)
				if table.Category == category {
					if len(got) != 0 {
						t.Errorf("%s came along: %v", table.Name, got)
					}
					continue
				}
				if len(got) == 0 {
					t.Errorf("%s did not arrive", table.Name)
				}
			}
		})
	}
}

func TestApplyKeepsWhatTheBundleLeftOut(t *testing.T) {
	src := openLibrary(t)
	seedWork(t, src, workID)
	execAll(t, src, `UPDATE projects SET title = '고친 제목' WHERE id = '`+workID+`'`)
	home := t.TempDir()
	bundle := exportOmit(t, src, home, workID, []string{CategoryImages, CategoryHistory, CategoryWorkAgent})

	dst := openLibrary(t)
	dstHome := t.TempDir()
	seedWork(t, dst, workID)
	writeFile(t, filepath.Join(SkillDir(dstHome, workID), "a", "SKILL.md"), "스킬")
	writeFile(t, filepath.Join(MemoryDir(dstHome, workID), "m.md"), "메모리")
	before := map[string][]string{}
	for _, table := range allTables() {
		if table.Category != "" {
			before[table.Name] = dump(t, dst, table, workID)
		}
	}

	if err := apply(t, dst, bundle); err != nil {
		t.Fatal(err)
	}
	if err := ApplyFiles(dstHome, bundle); err != nil {
		t.Fatal(err)
	}

	for name, rows := range before {
		table := tableNamed(t, name)
		if got := dump(t, dst, table, workID); !slices.Equal(got, rows) {
			t.Errorf("%s changed:\n got %v\nwant %v", name, got, rows)
		}
	}
	for _, file := range []string{filepath.Join(SkillDir(dstHome, workID), "a", "SKILL.md"), filepath.Join(MemoryDir(dstHome, workID), "m.md")} {
		if _, err := os.Stat(file); err != nil {
			t.Errorf("work file is gone: %v", err)
		}
	}
	var title string
	if err := dst.QueryRow(`SELECT title FROM projects WHERE id = ?`, workID).Scan(&title); err != nil || title != "고친 제목" {
		t.Errorf("title = %q, %v", title, err)
	}
}

func TestOmittedRowsOnlyGoWithTheirParent(t *testing.T) {
	src := openLibrary(t)
	seedWork(t, src, workID)
	// The scene the snapshot and the cover point at is gone in the bundle.
	execAll(t, src,
		`DELETE FROM node_snapshots`,
		`DELETE FROM nodes WHERE id = '`+workID+`-n2'`,
		`DELETE FROM entities WHERE id = '`+workID+`-e1'`,
	)
	bundle := exportOmit(t, src, t.TempDir(), workID, []string{CategoryImages, CategoryHistory})

	dst := openLibrary(t)
	seedWork(t, dst, workID)
	execAll(t, dst, `INSERT INTO node_snapshots (id, node_id, content_doc, reason, created_at) VALUES ('kept', '`+workID+`-n1', '{}', 'manual', 1)`)
	if err := apply(t, dst, bundle); err != nil {
		t.Fatal(err)
	}
	noForeignKeyViolations(t, dst)

	count := func(query string) int {
		t.Helper()
		var n int
		if err := dst.QueryRow(query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count(`SELECT COUNT(*) FROM node_snapshots WHERE id = 'kept'`) != 1 {
		t.Error("a snapshot of a scene that stayed was removed")
	}
	if count(`SELECT COUNT(*) FROM node_snapshots WHERE id = '`+workID+`-s1'`) != 0 {
		t.Error("the snapshot of a removed scene stayed")
	}
	if count(`SELECT COUNT(*) FROM entity_reference_images`) != 0 {
		t.Error("the reference image of a removed character stayed")
	}
	if count(`SELECT COUNT(*) FROM illustrations`) != 2 {
		t.Error("a picture left the gallery with its source scene")
	}
}

func TestFormatOneBundleIsReadInFull(t *testing.T) {
	src := openLibrary(t)
	seedWork(t, src, workID)
	bundle := exportOmit(t, src, t.TempDir(), workID, nil)

	raw, err := json.Marshal(bundle.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["omitted"]; ok {
		t.Fatal("a full bundle writes an omitted list")
	}
	fields["format"] = 1
	raw, _ = json.Marshal(fields)
	var old Manifest
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	bundle.Manifest = old
	if err := Validate(bundle, 1<<30); err != nil {
		t.Fatal(err)
	}

	dst := openLibrary(t)
	if err := apply(t, dst, bundle); err != nil {
		t.Fatal(err)
	}
	for _, table := range allTables() {
		if len(dump(t, dst, table, workID)) == 0 {
			t.Errorf("%s did not arrive from a format 1 bundle", table.Name)
		}
	}
}

func TestTablesAreClassifiedByCategory(t *testing.T) {
	wanted := map[string]string{
		"illustrations":                     CategoryImages,
		"node_representative_illustrations": CategoryImages,
		"illustration_requests":             CategoryImages,
		"entity_reference_images":           CategoryImages,
		"node_snapshots":                    CategoryHistory,
		"agent_memory":                      CategoryWorkAgent,
		"agent_memory_archive":              CategoryWorkAgent,
	}
	for _, table := range allTables() {
		if table.Category != wanted[table.Name] {
			t.Errorf("%s is in %q, want %q", table.Name, table.Category, wanted[table.Name])
		}
	}
}

func TestPartCategory(t *testing.T) {
	for name, want := range map[string]string{
		"tables/nodes":                "",
		"tables/illustrations":        CategoryImages,
		"images/abc":                  CategoryImages,
		"tables/node_snapshots":       CategoryHistory,
		"tables/agent_memory":         CategoryWorkAgent,
		"tables/agent_memory_archive": CategoryWorkAgent,
		"files/skills/a/b.md":         CategoryWorkAgent,
		"files/memory/m.md":           CategoryWorkAgent,
	} {
		if got := PartCategory(name); got != want {
			t.Errorf("PartCategory(%q) = %q, want %q", name, got, want)
		}
	}
}

func tableNamed(t *testing.T, name string) Table {
	t.Helper()
	for _, table := range allTables() {
		if table.Name == name {
			return table
		}
	}
	t.Fatalf("no table %s", name)
	return Table{}
}

func archiveHits(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	hits, err := agentmemory.NewRepo(db).SearchArchived(context.Background(), workID, query, 10)
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, hit := range hits {
		bodies = append(bodies, hit.Body)
	}
	return bodies
}

// The index is not in the bundle. Triggers keep it, so a pull has to leave it
// answering for exactly the rows that landed.
func TestArchivedNotesTravelAndStaySearchable(t *testing.T) {
	src := openLibrary(t)
	seedWork(t, src, workID)
	execAll(t, src, `INSERT INTO agent_memory_archive (id, project_id, body, source_message_id, archived_at)
		VALUES ('new-line', '`+workID+`', '끝난 결정은 등대지기 쪽으로', '', 2)`)
	bundle := exportOmit(t, src, t.TempDir(), workID, nil)

	dst := openLibrary(t)
	seedWork(t, dst, workID)
	execAll(t, dst, `INSERT INTO agent_memory_archive (id, project_id, body, source_message_id, archived_at)
		VALUES ('stale-line', '`+workID+`', '여기만 있던 옛 줄 항해사', '', 3)`)
	if err := apply(t, dst, bundle); err != nil {
		t.Fatal(err)
	}

	if got := archiveHits(t, dst, "등대지기"); len(got) != 1 {
		t.Errorf("the arrived line is not found: %v", got)
	}
	if got := archiveHits(t, dst, "항해사"); len(got) != 0 {
		t.Errorf("the index still answers for a line the pull replaced: %v", got)
	}
}

func TestOmittingWorkAgentLeavesArchivedNotesAndTheirIndex(t *testing.T) {
	src := openLibrary(t)
	seedWork(t, src, workID)
	execAll(t, src, `DELETE FROM agent_memory_archive`, `UPDATE projects SET title = '고친 제목' WHERE id = '`+workID+`'`)
	bundle := exportOmit(t, src, t.TempDir(), workID, []string{CategoryWorkAgent})
	if _, ok := bundle.Parts["tables/agent_memory_archive"]; ok {
		t.Fatal("the archive went up with work_agent off")
	}

	dst := openLibrary(t)
	seedWork(t, dst, workID)
	execAll(t, dst, `INSERT INTO agent_memory_archive (id, project_id, body, source_message_id, archived_at)
		VALUES ('kept-line', '`+workID+`', '이 기기에 남을 등대지기 줄', '', 2)`)
	if err := apply(t, dst, bundle); err != nil {
		t.Fatal(err)
	}

	if got := archiveHits(t, dst, "등대지기"); len(got) != 1 {
		t.Errorf("a bundle without the category touched the index: %v", got)
	}
}
