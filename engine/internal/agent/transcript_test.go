//go:build !mobile

package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// newTranscript opens a real on-disk store so the test exercises the actual
// companion_messages schema, including its foreign keys to projects and
// nodes. Those FKs mean a bare "p1"/"n1" insert fails, so the helper seeds
// the minimal parent rows the transcript's project and node ids need — the
// brief's transcript_test.go did not include this, and it is required against
// the real schema (see engine/internal/store/migrations/0001_init.sql and
// 0013_companion_messages.sql).
func newTranscript(t *testing.T) *transcript {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if _, err := st.DB().ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES ('p1', 'Test', '["SF"]', 'novel', 'first', 0, 0)`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, `
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title, content_doc, created_at, updated_at)
VALUES ('n1', 'p1', NULL, 0, 'leaf', 'scene 1', 'opening', '{}', 0, 0)`); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	return &transcript{
		repo:  companion.NewHistoryRepo(st.DB()),
		clock: func() int64 { return 1700000000000 },
	}
}

// The transcript reuses companion_messages so the 1.0 archive export picks up
// the new conversations without being touched.
func TestTranscript_roundTripsATurn(t *testing.T) {
	tr := newTranscript(t)
	ctx := context.Background()
	if err := tr.appendUser(ctx, "p1", "n1", "run-1", "write the opening"); err != nil {
		t.Fatalf("appendUser: %v", err)
	}
	if err := tr.appendAssistant(ctx, "p1", "n1", "run-1", "here it is", companion.HistoryStatusDone); err != nil {
		t.Fatalf("appendAssistant: %v", err)
	}
	got, err := tr.load(ctx, "p1", 50)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[0].Role != "user" || got[1].Role != "assistant" {
		t.Errorf("roles = %q,%q", got[0].Role, got[1].Role)
	}
	if got[1].RunID != "run-1" {
		t.Errorf("RunID = %q", got[1].RunID)
	}
}

// A tool event is a row the panel renders as a chip. Its content is JSON so
// the panel can show the name, whether it worked, and an undo button.
func TestTranscript_toolEventIsStructuredJSON(t *testing.T) {
	tr := newTranscript(t)
	ctx := context.Background()
	if err := tr.appendToolEvent(ctx, "p1", "n1", "run-1", toolEvent{
		Name: "linetta_write_scene", Summary: "wrote 1장", OK: true,
		BatchID: "batch-1", NodeIDs: []string{"n1"},
	}); err != nil {
		t.Fatalf("appendToolEvent: %v", err)
	}
	got, err := tr.load(ctx, "p1", 50)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got[0].Role != "tool" {
		t.Fatalf("Role = %q, want tool", got[0].Role)
	}
	var ev toolEvent
	if err := json.Unmarshal([]byte(got[0].Content), &ev); err != nil {
		t.Fatalf("tool content is not JSON: %v (%s)", err, got[0].Content)
	}
	if ev.Name != "linetta_write_scene" || !ev.OK || ev.BatchID != "batch-1" {
		t.Errorf("event = %+v", ev)
	}
}

// agent.clear wipes the conversation. The activity log is a separate table and
// deliberately survives — it is the writer's record of what was done to the
// manuscript, not of what was said.
func TestTranscript_clearRemovesTheConversation(t *testing.T) {
	tr := newTranscript(t)
	ctx := context.Background()
	_ = tr.appendUser(ctx, "p1", "", "run-1", "hello")
	if err := tr.clear(ctx, "p1"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, err := tr.load(ctx, "p1", 50)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d rows after clear, want 0", len(got))
	}
}

// The transcript shares companion_messages with the REMOVED 1.0 companion,
// whose rows are still in the table for anyone upgrading. "Clear conversation"
// in the new panel must not destroy them: export.companion_history is still
// registered, settings still offers it as the "기록 보존" affordance, and
// diagnostics.companion_history_exists — which gates that affordance — would
// flip to false the moment the last legacy row went, taking the only recovery
// path with it.
//
// The two intents in this test are the real ones. The 1.0 companion stamped
// intent with its RequestIntent.Kind ("chat", "read_only",
// "generic_mutation", "scene_write", "scene_rewrite") or left it empty on a
// compacted summary row; transcriptIntent is none of those.
func TestTranscript_clearSparesTheRemovedCompanionsRows(t *testing.T) {
	tr := newTranscript(t)
	ctx := context.Background()

	legacy := companion.HistoryMessage{
		ProjectID: "p1", NodeID: "n1", RunID: "old-run",
		Role: "user", Scope: companion.HistoryScopeScene, Intent: "scene_write",
		Status: companion.HistoryStatusDone, Content: "2023년에 컴패니언에게 한 말",
		CreatedAt: 1600000000000,
	}
	if err := tr.repo.Append(ctx, legacy); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := tr.appendUser(ctx, "p1", "n1", "run-1", "새 패널에 한 말"); err != nil {
		t.Fatalf("appendUser: %v", err)
	}

	if err := tr.clear(ctx, "p1"); err != nil {
		t.Fatalf("clear: %v", err)
	}

	all, err := tr.repo.List(ctx, companion.HistoryQuery{
		ProjectID: "p1", Scope: companion.HistoryViewProject, Limit: 50,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("companion_messages holds %d rows after agent.clear, want the one legacy row: %+v",
			len(all), all)
	}
	if all[0].Content != legacy.Content {
		t.Errorf("the surviving row is %q, want the pre-1.0 companion's own row", all[0].Content)
	}

	// And the agent's own rows really are gone.
	got, err := tr.load(ctx, "p1", 50)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d agent rows after clear, want 0: %+v", len(got), got)
	}
}

// The other half of the same rule: what clear deletes and what the panel
// shows have to be the same set of rows. If load returned the 1.0
// companion's rows, "clear conversation" would leave them on screen — and
// they would also be replayed into the model's context as things the writer
// said to THIS agent, which they did not.
func TestTranscript_loadShowsOnlyTheAgentsOwnRows(t *testing.T) {
	tr := newTranscript(t)
	ctx := context.Background()

	if err := tr.repo.Append(ctx, companion.HistoryMessage{
		ProjectID: "p1", Role: "assistant", Intent: "chat",
		Status: companion.HistoryStatusDone, Content: "컴패니언의 옛 답",
		CreatedAt: 1600000000000,
	}); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := tr.appendUser(ctx, "p1", "", "run-1", "에이전트에게 한 말"); err != nil {
		t.Fatalf("appendUser: %v", err)
	}

	got, err := tr.load(ctx, "p1", 50)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("load returned %d rows, want only the agent's own: %+v", len(got), got)
	}
	if got[0].Intent != transcriptIntent {
		t.Errorf("row intent = %q, want %q", got[0].Intent, transcriptIntent)
	}
}
