package vaultbundle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/store"
)

const workID = "work-1"

var fixedNow = time.UnixMilli(1_800_000_000_000)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func openLibrary(t *testing.T) *sql.DB {
	t.Helper()
	return openStore(t).DB()
}

func execAll(t *testing.T, db *sql.DB, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

// seedWork gives a work at least one row in every table a bundle carries, so
// the round trip below cannot pass by carrying nothing.
func seedWork(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	statements := []string{
		`INSERT INTO projects (id, title, genres, length_target, default_pov, style_notes, last_opened_node_id, word_count, created_at, updated_at, outline, synopsis)
		 VALUES ('P', '작품', '["판타지"]', 'long', '3rd', '문체', 'P-n2', 42, 1, 2, '개요', '줄거리')`,
		`INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title, content_doc, word_count, created_at, updated_at)
		 VALUES ('P-n1', 'P', NULL, 0, 'episode', '1화', '시작', NULL, 0, 1, 1)`,
		`INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title, content_doc, word_count, content_version, created_at, updated_at)
		 VALUES ('P-n2', 'P', 'P-n1', 0, 'scene', '씬 1', '', '{"type":"doc","content":[{"type":"paragraph"}]}', 42, 3, 1, 2)`,
		`INSERT INTO entities (id, project_id, kind, name, aliases, created_at, updated_at) VALUES ('P-e1', 'P', 'character', '가나', '["나"]', 1, 1)`,
		`INSERT INTO entities (id, project_id, kind, name, created_at, updated_at) VALUES ('P-e2', 'P', 'character', '다라', 1, 1)`,
		`INSERT INTO entity_visuals (entity_id, hair, updated_at) VALUES ('P-e1', '검은 머리', 1)`,
		`INSERT INTO entity_reference_images (id, entity_id, mime, byte_size, data, created_at) VALUES ('P-r1', 'P-e1', 'image/png', 3, x'00ff10', 1)`,
		`INSERT INTO project_art_style (project_id, style, updated_at) VALUES ('P', '수채화', 1)`,
		`INSERT INTO project_style_rules (project_id, avoid_phrases, max_sentence_chars, updated_at) VALUES ('P', '["그러나"]', 80, 1)`,
		`INSERT INTO project_style_drafts (project_id, body, created_at) VALUES ('P', '초안', 1)`,
		`INSERT INTO story_ledger (id, project_id, kind, body, status, source, created_at, updated_at) VALUES ('P-l1', 'P', 'promise', '복선', 'open', 'writer', 1, 1)`,
		`INSERT INTO part_arcs (node_id, project_id, role, goal, tension, updated_at) VALUES ('P-n2', 'P', '승', '목표', 5, 1)`,
		`INSERT INTO project_focus (project_id, body, anchor_node_id, set_at) VALUES ('P', '집중', 'P-n2', 1)`,
		`INSERT INTO illustrations (id, project_id, source_node_id, prompt, provider, model, mime, byte_size, data, created_at)
		 VALUES ('P-i1', 'P', 'P-n2', '그림', 'p', 'm', 'image/png', 4, x'89504e47', 1)`,
		`INSERT INTO illustrations (id, project_id, prompt, provider, model, mime, byte_size, data, created_at)
		 VALUES ('P-i2', 'P', '같은 그림', 'p', 'm', 'image/png', 4, x'89504e47', 2)`,
		`INSERT INTO node_representative_illustrations (node_id, project_id, illustration_id, updated_at) VALUES ('P-n1', 'P', 'P-i1', 1)`,
		`INSERT INTO illustration_requests (id, project_id, target_node_id, purpose, prompt, aspect, source, status, illustration_id, created_at, decided_at)
		 VALUES ('P-q1', 'P', 'P-n2', 'insert', '요청', '1:1', 'agent', 'done', 'P-i1', 1, 1)`,
		`INSERT INTO mentions (id, node_id, entity_id, position, surface) VALUES ('P-m1', 'P-n2', 'P-e1', 3, '가나')`,
		`INSERT INTO relationships (id, project_id, from_id, to_id, label, pair_id) VALUES ('P-l1', 'P', 'P-e1', 'P-e2', '친구', 'pair')`,
		`INSERT INTO threads (id, project_id, name, closed_at) VALUES ('P-t1', 'P', '복선', NULL)`,
		`INSERT INTO threads (id, project_id, name, closed_at) VALUES ('P-t2', 'P', '다른 복선', 5)`,
		`INSERT INTO beats (id, thread_id, node_id, ordinal, label) VALUES ('P-b1', 'P-t1', 'P-n2', 0, '시작')`,
		`INSERT INTO notes (id, node_id, anchor, body, created_at) VALUES ('P-o1', 'P-n2', 1, '메모', 1)`,
		`INSERT INTO scene_intents (node_id, project_id, goal, updated_at) VALUES ('P-n2', 'P', '목표', 1)`,
		`INSERT INTO node_snapshots (id, node_id, content_doc, reason, created_at) VALUES ('P-s1', 'P-n2', '{}', 'manual', 1)`,
		`INSERT INTO fact_cards (id, project_id, node_id, claim, result, status, created_at, updated_at) VALUES ('P-c1', 'P', 'P-n2', '주장', '결과', 'ok', 1, 1)`,
		`INSERT INTO fact_sources (id, card_id, url, accessed_at) VALUES ('P-u1', 'P-c1', 'https://example.test', 1)`,
		`INSERT INTO writing_stats (project_id, day, chars_added) VALUES ('P', '2026-10-31', 10)`,
		`INSERT INTO agent_memory (scope, project_id, body, updated_at) VALUES ('work_notes', 'P', '작업 노트', 1)`,
		`INSERT INTO continuity_checks (id, project_id, node_id, content_version, source, checked_at) VALUES ('P-k1', 'P', 'P-n2', 3, 'agent', 1)`,
		`INSERT INTO continuity_observations (id, check_id, project_id, node_id, category, kind, severity, summary, paragraph_from, paragraph_to, quote, status_at, created_at)
		 VALUES ('P-v1', 'P-k1', 'P', 'P-n2', 'fact', 'gap', 'low', '요약', 0, 1, '인용', 1, 1)`,
		`INSERT INTO forecasts (id, project_id, divergence, horizon, branches_json, fingerprint, input_hashes_json, source, created_at)
		 VALUES ('P-f1', 'P', '질문', 3, '[]', 'fp', '{}', 'agent', 1)`,
		`INSERT INTO story_proposals (id, project_id, purpose, source, created_at, forecast_id) VALUES ('P-g1', 'P', 'general', 'agent', 1, 'P-f1')`,
		`INSERT INTO story_proposal_items (id, proposal_id, project_id, ordinal, op_json, status_at) VALUES ('P-g2', 'P-g1', 'P', 0, '{}', 1)`,
	}
	for _, statement := range statements {
		replaced := strings.NewReplacer("'P-", "'"+id+"-", "'P'", "'"+id+"'").Replace(statement)
		execAll(t, db, replaced)
	}
}

// addDeviceRows gives the work rows a bundle must leave alone.
func addDeviceRows(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	execAll(t, db,
		fmt.Sprintf(`INSERT INTO ai_runs (id, project_id, node_id, provider, prompt, context_json, status, started_at) VALUES ('%[1]s-run', '%[1]s', '%[1]s-n2', 'p', '비밀 프롬프트', '{}', 'done', 1)`, id),
		fmt.Sprintf(`INSERT INTO companion_messages (id, project_id, node_id, role, content, created_at) VALUES ('%[1]s-msg', '%[1]s', '%[1]s-n2', 'user', '동반자 대화', 1)`, id),
		fmt.Sprintf(`INSERT INTO publish_works (project_id, remote_url, slug) VALUES ('%[1]s', 'https://works.test', 'my-work')`, id),
		fmt.Sprintf(`INSERT INTO publish_entities (entity_id, project_id, created_at) VALUES ('%[1]s-e1', '%[1]s', 1)`, id),
	)
}

func export(t *testing.T, db *sql.DB, home, id string) *Bundle {
	t.Helper()
	bundle, err := Export(context.Background(), db, home, id, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func apply(t *testing.T, db *sql.DB, bundle *Bundle) error {
	t.Helper()
	ctx := context.Background()
	library, err := SchemaVersion(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(bundle, library); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := Apply(ctx, tx, bundle); err != nil {
		return err
	}
	return tx.Commit()
}

// dump reads every row of a table as sorted strings, BLOBs and all.
func dump(t *testing.T, db *sql.DB, table Table, id string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT * FROM `+table.Name+` WHERE `+table.Scope, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, _ := rows.Columns()
	var out []string
	for rows.Next() {
		raw := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range raw {
			pointers[i] = &raw[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		parts := make([]string, len(columns))
		for i, column := range columns {
			parts[i] = fmt.Sprintf("%s=%T:%v", column, raw[i], raw[i])
		}
		out = append(out, strings.Join(parts, "|"))
	}
	sort.Strings(out)
	return out
}

func noForeignKeyViolations(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key violation after apply")
	}
}

func TestEveryTableIsClassified(t *testing.T) {
	rows, err := openLibrary(t).Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := checkClassified(names); err != nil {
		t.Fatal(err)
	}
}

func TestUnclassifiedTableFails(t *testing.T) {
	var names []string
	for _, table := range allTables() {
		names = append(names, table.Name)
	}
	for name := range excluded {
		names = append(names, name)
	}
	if err := checkClassified(append(names, "new_work_table")); err == nil || !strings.Contains(err.Error(), "new_work_table") {
		t.Fatalf("a new table must fail classification, got %v", err)
	}
	if err := checkClassified(names[1:]); err == nil || !strings.Contains(err.Error(), names[0]) {
		t.Fatalf("a classified table that is gone must fail, got %v", err)
	}
}

func TestEveryBlobColumnBecomesAPart(t *testing.T) {
	db := openLibrary(t)
	rows, err := db.Query(`SELECT m.name, p.name FROM sqlite_master m, pragma_table_info(m.name) p
		WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%' AND m.name NOT LIKE 'manuscript_fts%' AND upper(p.type) = 'BLOB'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]string{}
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		seen[table] = column
	}
	if !reflect.DeepEqual(seen, imageColumns) {
		t.Fatalf("BLOB columns %v differ from imageColumns %v: a blob read as text would be damaged", seen, imageColumns)
	}
}

func TestRoundTripKeepsEveryRow(t *testing.T) {
	source, target := openLibrary(t), openLibrary(t)
	seedWork(t, source, workID)
	bundle := export(t, source, t.TempDir(), workID)

	for _, table := range allTables() {
		if bundle.Manifest.Tables[table.Name] == 0 {
			t.Errorf("fixture has no row in %s, so this test proves nothing about it", table.Name)
		}
	}
	if err := apply(t, target, bundle); err != nil {
		t.Fatal(err)
	}
	noForeignKeyViolations(t, target)
	for _, table := range allTables() {
		got, want := dump(t, target, table, workID), dump(t, source, table, workID)
		if table.Name == "projects" {
			got, want = withoutColumn(got, "last_opened_node_id"), withoutColumn(want, "last_opened_node_id")
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %v\nwant %v", table.Name, got, want)
		}
	}
}

func withoutColumn(rows []string, column string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		fields := strings.Split(row, "|")
		kept := fields[:0:0]
		for _, field := range fields {
			if !strings.HasPrefix(field, column+"=") {
				kept = append(kept, field)
			}
		}
		out[i] = strings.Join(kept, "|")
	}
	return out
}

func TestImagesStayBinaryAndAreSharedAsOnePart(t *testing.T) {
	source, target := openLibrary(t), openLibrary(t)
	seedWork(t, source, workID)
	bundle := export(t, source, t.TempDir(), workID)
	images := 0
	for _, part := range bundle.Manifest.Parts {
		if strings.HasPrefix(part.Name, imagePrefix) {
			images++
		}
	}
	if images != 2 {
		t.Fatalf("two illustrations with the same bytes and one reference image make 2 parts, got %d", images)
	}
	if strings.Contains(string(bundle.Parts[tablePrefix+"illustrations"]), "PNG") {
		t.Fatal("image bytes must not sit in the table part")
	}
	if err := apply(t, target, bundle); err != nil {
		t.Fatal(err)
	}
	var kind string
	var data []byte
	if err := target.QueryRow(`SELECT typeof(data), data FROM illustrations WHERE id = ?`, workID+"-i1").Scan(&kind, &data); err != nil {
		t.Fatal(err)
	}
	if kind != "blob" || string(data) != "\x89PNG" {
		t.Fatalf("got %s %x", kind, data)
	}
}

func TestSameWorkHashesTheSameAcrossDevices(t *testing.T) {
	source, target := openLibrary(t), openLibrary(t)
	home := t.TempDir()
	seedWork(t, source, workID)
	first := export(t, source, home, workID)
	if err := apply(t, target, first); err != nil {
		t.Fatal(err)
	}
	again := export(t, target, home, workID)
	if first.Manifest.ContentHash() != again.Manifest.ContentHash() {
		for name, data := range first.Parts {
			if string(data) != string(again.Parts[name]) {
				t.Errorf("part %s differs:\n%s\n%s", name, data, again.Parts[name])
			}
		}
		t.Fatal("a pulled work must not look locally changed")
	}
}

// Two devices insert the same rows in different orders. Without a canonical
// order the second one would see a change it never made.
func TestRowOrderDoesNotChangeTheHash(t *testing.T) {
	source, target := openLibrary(t), openLibrary(t)
	home := t.TempDir()
	seedWork(t, source, workID)
	first := export(t, source, home, workID)

	reversed := &Bundle{Manifest: first.Manifest, Parts: map[string][]byte{}}
	for name, data := range first.Parts {
		reversed.Parts[name] = data
	}
	for _, table := range allTables() {
		lines := strings.Split(string(first.Parts[tablePrefix+table.Name]), "\n")
		slices.Reverse(lines)
		reversed.Parts[tablePrefix+table.Name] = []byte(strings.Join(lines, "\n"))
	}
	// threads has only the project index, so its rows come back in insert
	// order. Without a table like this the test would pass with no sort at all.
	if first.Manifest.Tables["threads"] < 2 {
		t.Fatal("threads needs two rows to reverse")
	}
	reversed.seal()
	if err := apply(t, target, reversed); err != nil {
		t.Fatal(err)
	}
	if got := export(t, target, home, workID).Manifest.ContentHash(); got != first.Manifest.ContentHash() {
		t.Fatal("the same rows inserted in another order changed the hash")
	}
}

func TestOpeningAWorkIsNotAChange(t *testing.T) {
	st := openStore(t)
	db := st.DB()
	seedWork(t, db, workID)
	before := export(t, db, t.TempDir(), workID).Manifest.ContentHash()
	if err := node.NewRepo(st).SetLastOpened(context.Background(), workID, workID+"-n1", fixedNow.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	var updated int64
	if err := db.QueryRow(`SELECT updated_at FROM projects WHERE id = ?`, workID).Scan(&updated); err != nil || updated != fixedNow.UnixMilli() {
		t.Fatalf("this test relies on opening a work bumping updated_at: %d %v", updated, err)
	}
	if after := export(t, db, t.TempDir(), workID).Manifest.ContentHash(); after != before {
		t.Fatal("opening a work changed its hash")
	}
}

func TestEditingAWorkIsAChangeOfOnlyThatPart(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	before := export(t, db, t.TempDir(), workID)
	execAll(t, db, fmt.Sprintf(`UPDATE nodes SET title = '고침' WHERE id = '%s-n2'`, workID))
	after := export(t, db, t.TempDir(), workID)
	if before.Manifest.ContentHash() == after.Manifest.ContentHash() {
		t.Fatal("an edit must change the hash")
	}
	var changed []string
	for name, data := range after.Parts {
		if string(before.Parts[name]) != string(data) {
			changed = append(changed, name)
		}
	}
	if !reflect.DeepEqual(changed, []string{tablePrefix + "nodes"}) {
		t.Fatalf("changed parts %v", changed)
	}
}

func TestOtherWorksAndDeviceRowsAreLeftOut(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	seedWork(t, db, "other")
	addDeviceRows(t, db, workID)
	bundle := export(t, db, t.TempDir(), workID)
	for name, data := range bundle.Parts {
		text := string(data)
		for _, secret := range []string{"other-", "비밀 프롬프트", "동반자 대화", "works.test", "P-n2"} {
			if strings.Contains(text, secret) {
				t.Errorf("part %s holds %q", name, secret)
			}
		}
	}
	if strings.Contains(string(bundle.Parts[tablePrefix+"agent_memory"]), "writer_profile") {
		t.Error("the global writer profile must stay in the library")
	}
}

func TestGlobalWriterProfileIsNotTouched(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	execAll(t, db, `INSERT INTO agent_memory (scope, project_id, body, updated_at) VALUES ('writer_profile', NULL, '작가 프로필', 1)`)
	bundle := export(t, db, t.TempDir(), workID)
	if strings.Contains(string(bundle.Parts[tablePrefix+"agent_memory"]), "작가 프로필") {
		t.Fatal("writer profile leaked into the bundle")
	}
	if err := apply(t, db, bundle); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := db.QueryRow(`SELECT body FROM agent_memory WHERE scope = 'writer_profile'`).Scan(&body); err != nil || body != "작가 프로필" {
		t.Fatalf("writer profile after apply: %q %v", body, err)
	}
}

func TestApplyOverAnEditedWorkRestoresItAndKeepsDeviceRows(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	seedWork(t, db, "other")
	addDeviceRows(t, db, workID)
	bundle := export(t, db, t.TempDir(), workID)
	want := map[string][]string{}
	for _, table := range allTables() {
		want[table.Name] = dump(t, db, table, workID)
	}

	id := workID
	execAll(t, db,
		// The two characters trade names, which UNIQUE(project_id, name) only
		// allows through a third name.
		fmt.Sprintf(`UPDATE entities SET name = '임시' WHERE id = '%s-e1'`, id),
		fmt.Sprintf(`UPDATE entities SET name = '가나' WHERE id = '%s-e2'`, id),
		fmt.Sprintf(`UPDATE entities SET name = '다라' WHERE id = '%s-e1'`, id),
		fmt.Sprintf(`INSERT INTO entities (id, project_id, kind, name, created_at, updated_at) VALUES ('%[1]s-e9', '%[1]s', 'place', '새 장소', 9, 9)`, id),
		fmt.Sprintf(`INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, created_at, updated_at) VALUES ('%[1]s-n9', '%[1]s', NULL, 1, 'episode', '2화', 9, 9)`, id),
		fmt.Sprintf(`UPDATE nodes SET parent_id = '%[1]s-n9', title = '옮김' WHERE id = '%[1]s-n2'`, id),
		fmt.Sprintf(`DELETE FROM notes WHERE id = '%s-o1'`, id),
		fmt.Sprintf(`UPDATE projects SET title = '바뀐 제목', last_opened_node_id = '%[1]s-n1' WHERE id = '%[1]s'`, id),
	)
	otherBefore := dump(t, db, Table{"nodes", byProject}, "other")

	if err := apply(t, db, bundle); err != nil {
		t.Fatal(err)
	}
	noForeignKeyViolations(t, db)
	for _, table := range allTables() {
		got := dump(t, db, table, workID)
		if table.Name == "projects" {
			got, want[table.Name] = withoutColumn(got, "last_opened_node_id"), withoutColumn(want[table.Name], "last_opened_node_id")
		}
		if !reflect.DeepEqual(got, want[table.Name]) {
			t.Errorf("%s:\n got %v\nwant %v", table.Name, got, want[table.Name])
		}
	}
	if !reflect.DeepEqual(dump(t, db, Table{"nodes", byProject}, "other"), otherBefore) {
		t.Error("another work changed")
	}

	var place string
	if err := db.QueryRow(`SELECT last_opened_node_id FROM projects WHERE id = ?`, id).Scan(&place); err != nil || place != id+"-n1" {
		t.Errorf("the writer's place moved: %q %v", place, err)
	}
	var linked sql.NullString
	if err := db.QueryRow(`SELECT node_id FROM ai_runs WHERE id = ?`, id+"-run").Scan(&linked); err != nil || linked.String != id+"-n2" {
		t.Errorf("ai run lost its scene: %v %v", linked, err)
	}
	if err := db.QueryRow(`SELECT node_id FROM companion_messages WHERE id = ?`, id+"-msg").Scan(&linked); err != nil || linked.String != id+"-n2" {
		t.Errorf("companion message lost its scene: %v %v", linked, err)
	}
	var remote string
	if err := db.QueryRow(`SELECT remote_url FROM publish_works WHERE project_id = ?`, id).Scan(&remote); err != nil || remote == "" {
		t.Errorf("publish target lost: %v", err)
	}
	var published int
	if err := db.QueryRow(`SELECT COUNT(*) FROM publish_entities WHERE entity_id = ?`, id+"-e1").Scan(&published); err != nil || published != 1 {
		t.Errorf("publish entity lost: %d %v", published, err)
	}
}

func TestFailedApplyLeavesTheWorkAlone(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	bundle := export(t, db, t.TempDir(), workID)
	bundle.Parts[tablePrefix+"notes"] = []byte(`{"id":"broken","node_id":"no-such-node","anchor":1,"body":"x","created_at":1}`)
	bundle.seal()
	execAll(t, db, fmt.Sprintf(`UPDATE nodes SET title = '살아남을 글' WHERE id = '%s-n2'`, workID))

	if err := apply(t, db, bundle); err == nil {
		t.Fatal("a row pointing nowhere must fail the pull")
	}
	var title string
	if err := db.QueryRow(`SELECT title FROM nodes WHERE id = ?`, workID+"-n2").Scan(&title); err != nil || title != "살아남을 글" {
		t.Fatalf("got %q %v", title, err)
	}
	var notes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notes WHERE id = ?`, workID+"-o1").Scan(&notes); err != nil || notes != 1 {
		t.Fatalf("notes %d %v", notes, err)
	}
}

func TestRowOfAnotherWorkIsNotTakenOver(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, "other")
	source := openLibrary(t)
	seedWork(t, source, workID)
	bundle := export(t, source, t.TempDir(), workID)
	// Every row follows the renamed id, so only the ownership check can stop it.
	for name, data := range bundle.Parts {
		if strings.HasPrefix(name, tablePrefix) {
			bundle.Parts[name] = []byte(strings.ReplaceAll(string(data), workID+"-e1", "other-e1"))
		}
	}
	bundle.seal()
	if err := apply(t, db, bundle); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("an id owned by another work must be refused, got %v", err)
	}
	var project string
	if err := db.QueryRow(`SELECT project_id FROM entities WHERE id = 'other-e1'`).Scan(&project); err != nil || project != "other" {
		t.Fatalf("got %q %v", project, err)
	}
}

func TestNewerBundlesAreRefused(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	library, err := SchemaVersion(context.Background(), db)
	if err != nil || library == 0 {
		t.Fatalf("schema %d %v", library, err)
	}
	bundle := export(t, db, t.TempDir(), workID)
	if bundle.Manifest.SchemaVersion != library {
		t.Fatalf("manifest schema %d", bundle.Manifest.SchemaVersion)
	}
	newerSchema := *bundle
	newerSchema.Manifest.SchemaVersion = library + 1
	if err := Validate(&newerSchema, library); !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("got %v", err)
	}
	newerFormat := *bundle
	newerFormat.Manifest.Format = FormatVersion + 1
	if err := Validate(&newerFormat, library); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("got %v", err)
	}
}

func TestOlderBundleDropsColumnsThatNoLongerExist(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	bundle := export(t, db, t.TempDir(), workID)
	bundle.Parts[tablePrefix+"threads"] = []byte(strings.Replace(string(bundle.Parts[tablePrefix+"threads"]), `{"closed_at"`, `{"dropped_later":1,"closed_at"`, 1))
	bundle.seal()
	if err := apply(t, db, bundle); err == nil {
		t.Fatal("an unknown column at the same schema means the bundle is wrong")
	}
	bundle.Manifest.SchemaVersion--
	if err := apply(t, db, bundle); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptPartsAreRefusedBeforeAnythingIsTouched(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	library, _ := SchemaVersion(context.Background(), db)

	changed := export(t, db, t.TempDir(), workID)
	changed.Parts[tablePrefix+"nodes"] = []byte("{}")
	if err := Validate(changed, library); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("changed part: %v", err)
	}
	missing := export(t, db, t.TempDir(), workID)
	delete(missing.Parts, tablePrefix+"nodes")
	if err := Validate(missing, library); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("missing part: %v", err)
	}
	unnamed := export(t, db, t.TempDir(), workID)
	unnamed.Manifest.ProjectID = ""
	if err := Validate(unnamed, library); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("no project: %v", err)
	}
}

func TestNodeCycleIsCorrupt(t *testing.T) {
	_, err := parentsFirst([]map[string]any{{"id": "a", "parent_id": "b"}, {"id": "b", "parent_id": "a"}})
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
}

func writeFile(t *testing.T, file, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkFilesTravelAndOthersStay(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	home, away := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(home, "skills", "works", workID, "tone", "SKILL.md"), "문체 스킬")
	writeFile(t, filepath.Join(home, workID, "memory", "experiences.jsonl"), "기억")
	writeFile(t, filepath.Join(home, "skills", "global", "SKILL.md"), "전역 스킬")
	writeFile(t, filepath.Join(home, "skills", "works", "other", "x", "SKILL.md"), "다른 작품 스킬")
	if err := os.Symlink(filepath.Join(home, "skills", "global", "SKILL.md"), filepath.Join(home, workID, "memory", "link")); err != nil {
		t.Fatal(err)
	}
	bundle := export(t, db, home, workID)
	if got := bundle.Parts[skillPrefix+"tone/SKILL.md"]; string(got) != "문체 스킬" {
		t.Fatalf("skill part %q", got)
	}
	if got := bundle.Parts[memoryPrefix+"experiences.jsonl"]; string(got) != "기억" {
		t.Fatalf("memory part %q", got)
	}
	for name := range bundle.Parts {
		if strings.Contains(name, "global") || strings.Contains(name, "other") || strings.HasSuffix(name, "link") {
			t.Errorf("part %s should not travel", name)
		}
	}

	writeFile(t, filepath.Join(away, "skills", "works", workID, "old", "SKILL.md"), "지워질 스킬")
	if err := ApplyFiles(away, bundle); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(away, "skills", "works", workID, "tone", "SKILL.md")); err != nil || string(got) != "문체 스킬" {
		t.Fatalf("%q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(away, workID, "memory", "experiences.jsonl")); err != nil || string(got) != "기억" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(away, "skills", "works", workID, "old")); !os.IsNotExist(err) {
		t.Fatalf("the old skill must be replaced: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(away, "skills", "works"))
	if len(entries) != 1 {
		t.Fatalf("staging left behind: %v", entries)
	}
}

func TestApplyFilesRemovesDirectoriesTheBundleDoesNotHave(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	home := t.TempDir()
	dir := filepath.Join(home, workID, "memory")
	writeFile(t, filepath.Join(dir, "experiences.jsonl"), "지워질 기억")
	bundle := export(t, db, t.TempDir(), workID)
	if err := ApplyFiles(home, bundle); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("got %v", err)
	}
}

func TestUnsafePathsAreRefused(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	library, _ := SchemaVersion(context.Background(), db)
	for _, name := range []string{"../../escape", "a/../../escape", "/etc/passwd", "a/./b", ""} {
		bundle := export(t, db, t.TempDir(), workID)
		bundle.add(skillPrefix+name, []byte("x"))
		bundle.seal()
		if err := Validate(bundle, library); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%q: got %v", name, err)
		}
	}
}

func TestManifestSurvivesJSON(t *testing.T) {
	db := openLibrary(t)
	seedWork(t, db, workID)
	bundle := export(t, db, t.TempDir(), workID)
	encoded, err := json.Marshal(bundle.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Manifest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ContentHash() != bundle.Manifest.ContentHash() || decoded.ProjectUpdatedAt != bundle.Manifest.ProjectUpdatedAt {
		t.Fatalf("got %+v", decoded)
	}
}
