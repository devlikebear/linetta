package forecast

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
)

// canonTables is every table a forecast save/list/select/clear/delete must
// never touch: canon itself, plus Phase 2/3's own review-queue tables
// (continuity_checks, continuity_observations, story_proposals,
// story_proposal_items) — phase-4-forecast.md's 4.A.3, extended per the
// roadmap's decision to also guard the earlier phases' tables. This is
// InkOS's forecast-fixture.ts, ported: a whole-library before/after
// snapshot rather than asserting against each call's own return value.
var canonTables = []string{
	"projects", "nodes", "entities", "relationships", "fact_cards",
	"threads", "beats", "project_focus", "scene_intents",
	"continuity_checks", "continuity_observations",
	"story_proposals", "story_proposal_items",
}

// dumpCanon hashes every row of every table in canonTables, so the test
// does not need to know each table's columns up front — any future column
// added to any of these tables is covered automatically.
func dumpCanon(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	out := make(map[string]string, len(canonTables))
	for _, table := range canonTables {
		rows, err := db.Query("SELECT * FROM " + table)
		if err != nil {
			t.Fatalf("dump %s: %v", table, err)
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatalf("dump %s columns: %v", table, err)
		}
		var lines []string
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				t.Fatalf("dump %s scan: %v", table, err)
			}
			parts := make([]string, len(cols))
			for i, v := range vals {
				parts[i] = fmt.Sprintf("%v", v)
			}
			lines = append(lines, strings.Join(parts, "\x1f"))
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatalf("dump %s rows: %v", table, err)
		}
		sort.Strings(lines)
		h := sha256.New()
		for _, l := range lines {
			h.Write([]byte(l))
			h.Write([]byte{0})
		}
		out[table] = hex.EncodeToString(h.Sum(nil))
	}
	return out
}

func assertCanonUnchanged(t *testing.T, before, after map[string]string) {
	t.Helper()
	for _, table := range canonTables {
		if before[table] != after[table] {
			t.Errorf("table %q changed — forecasts must never touch canon or Phase 2/3's review tables", table)
		}
	}
}

func TestCanonUnchangedThroughSaveListSelectClearDelete(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)
	env.writeScene(t, env.sceneID, "정사 보호 테스트 문장.", 1)

	e, err := env.entities.Create(ctx, 1_000, entity.NewInput{ProjectID: env.project.ID, Kind: "character", Name: "하린"})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	branches := twoBranches()
	branches[0].Decisions = []CharDecision{{EntityID: e.ID, Decision: "맞선다"}}

	before := dumpCanon(t, env.st.DB())

	f, problems, err := svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "다음 회차 갈래?", Horizon: 3, Branches: branches,
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("problems = %+v, want none", problems)
	}
	afterSave := dumpCanon(t, env.st.DB())
	assertCanonUnchanged(t, before, afterSave)

	if _, err := svc.List(ctx, env.project.ID); err != nil {
		t.Fatalf("List: %v", err)
	}
	afterList := dumpCanon(t, env.st.DB())
	assertCanonUnchanged(t, before, afterList)

	if _, err := svc.Select(ctx, 10_001, env.project.ID, f.ID, "b1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	afterSelect := dumpCanon(t, env.st.DB())
	assertCanonUnchanged(t, before, afterSelect)

	if err := svc.ClearSelection(ctx, env.project.ID); err != nil {
		t.Fatalf("ClearSelection: %v", err)
	}
	afterClear := dumpCanon(t, env.st.DB())
	assertCanonUnchanged(t, before, afterClear)

	if err := svc.Delete(ctx, env.project.ID, f.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	afterDelete := dumpCanon(t, env.st.DB())
	assertCanonUnchanged(t, before, afterDelete)
}
