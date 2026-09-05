//go:build !mobile

package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/devlikebear/linetta/engine/internal/companion"
)

// transcriptIntent stamps every row this package writes, so the panel's own
// conversation can be told apart from the 1.0 companion's rows in the table
// they share.
//
// The value must not collide with anything the removed companion wrote. That
// agent stamped `intent` with its RequestIntent.Kind — "chat", "read_only",
// "generic_mutation", "scene_write", "scene_rewrite" — and left it empty on a
// compacted summary row. "agent" is none of those, and it matches the name
// mcphost already uses for the same actor in the activity log
// (mcphost.SourceAgent).
const transcriptIntent = "agent"

// transcript stores the panel's conversation. It reuses companion_messages
// rather than adding a table: the columns already match, and the 1.0 archive
// export (export.companion_history) then carries the new conversations with
// no change of its own.
//
// Reusing the table means sharing it with the removed 1.0 companion's rows,
// which are still there for any writer upgrading from it. Every read and
// every delete here is scoped to transcriptIntent for that reason: the panel
// must not put words the writer never said to THIS agent into the model's
// context, and "clear conversation" must not destroy history whose only
// remaining rescue path is export.companion_history.
type transcript struct {
	repo  *companion.HistoryRepo
	clock func() int64
}

func (t *transcript) now() int64 {
	if t.clock != nil {
		return t.clock()
	}
	return time.Now().UnixMilli()
}

func (t *transcript) append(ctx context.Context, m companion.HistoryMessage) error {
	m.CreatedAt = t.now()
	m.Intent = transcriptIntent
	return t.repo.Append(ctx, m)
}

func (t *transcript) appendUser(ctx context.Context, projectID, nodeID, runID, content string) error {
	return t.append(ctx, companion.HistoryMessage{
		ProjectID: projectID, NodeID: nodeID, RunID: runID,
		Role: "user", Content: content, Status: companion.HistoryStatusDone,
	})
}

func (t *transcript) appendAssistant(ctx context.Context, projectID, nodeID, runID, content, status string) error {
	return t.append(ctx, companion.HistoryMessage{
		ProjectID: projectID, NodeID: nodeID, RunID: runID,
		Role: "assistant", Content: content, Status: status,
	})
}

// toolEvent is what the panel renders as a chip under the reply: which tool
// ran, whether it worked, and — for a write — what to undo.
type toolEvent struct {
	Name    string   `json:"name"`
	Summary string   `json:"summary,omitempty"`
	OK      bool     `json:"ok"`
	BatchID string   `json:"batch_id,omitempty"`
	NodeIDs []string `json:"node_ids,omitempty"`
}

func (t *transcript) appendToolEvent(ctx context.Context, projectID, nodeID, runID string, ev toolEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return t.append(ctx, companion.HistoryMessage{
		ProjectID: projectID, NodeID: nodeID, RunID: runID,
		Role: "tool", Content: string(body), Status: companion.HistoryStatusDone,
	})
}

func (t *transcript) load(ctx context.Context, projectID string, limit int) ([]companion.HistoryMessage, error) {
	return t.repo.List(ctx, companion.HistoryQuery{
		ProjectID: projectID,
		Scope:     companion.HistoryViewProject,
		Intent:    transcriptIntent,
		Limit:     limit,
	})
}

// clear drops the conversation. Two things deliberately survive it:
//
//   - The activity log, a different table entirely. It records what was done
//     to the manuscript, which the writer needs whether or not they wanted to
//     keep the chat.
//   - The removed 1.0 companion's rows in this same table. An upgrading
//     writer's pre-1.0 history is not this panel's to delete, and
//     export.companion_history — still registered, still offered in settings
//     as "기록 보존" — is the affordance that exists to rescue it.
func (t *transcript) clear(ctx context.Context, projectID string) error {
	return t.repo.Clear(ctx, companion.HistoryQuery{
		ProjectID: projectID,
		Scope:     companion.HistoryViewProject,
		Intent:    transcriptIntent,
	})
}

// markRun stamps every row of a run with how the turn ended, so the panel can
// show a cancelled turn as cancelled and offer a retry on a failed one.
//
// The context is stripped of cancellation on purpose: this is called on the
// way out of a turn that was very often cancelled, and a cancelled context
// would refuse the very write that records the cancellation.
func (t *transcript) markRun(ctx context.Context, runID, status string) error {
	return t.repo.MarkRunStatus(context.WithoutCancel(ctx), runID, status)
}
