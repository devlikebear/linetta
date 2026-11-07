package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// How a settlement pass over a scene ended. Only done settles it: a pass that
// ran out of time or calls, or failed, leaves the scene waiting.
const (
	SettlementDone    = "done"
	SettlementTimeout = "timeout"
	SettlementFailed  = "failed"
)

// ValidSettlementOutcome reports whether s is one of the three outcomes.
func ValidSettlementOutcome(s string) bool {
	return s == SettlementDone || s == SettlementTimeout || s == SettlementFailed
}

// SettlementSwitch reports whether the writer has settlement on, and since
// when (unix ms). Scenes last written before that moment are not counted as
// waiting: switching it on must not turn a few hundred episodes into a queue.
type SettlementSwitch func() (since int64, enabled bool)

// Settlements is the record of which version of each scene was last settled
// into the ledger, and the list of scenes still waiting.
type Settlements struct {
	s      *store.Store
	nodes  *node.Repo
	enable SettlementSwitch
}

// NewSettlements returns the record. A nil enable means settlement is off and
// nothing is ever waiting.
func NewSettlements(s *store.Store, nodes *node.Repo, enable SettlementSwitch) *Settlements {
	return &Settlements{s: s, nodes: nodes, enable: enable}
}

// Enabled reports whether settlement is on.
func (r *Settlements) Enabled() bool {
	if r == nil || r.enable == nil {
		return false
	}
	_, on := r.enable()
	return on
}

// Version is nodeID's content version now: what a pass about to read the
// scene will have settled when it finishes.
func (r *Settlements) Version(ctx context.Context, nodeID string) (int, error) {
	n, err := r.nodes.Get(ctx, nodeID)
	if err != nil {
		return 0, err
	}
	return n.ContentVersion, nil
}

// Record keeps how the pass over one scene at version ended. A done already on
// record for the same version is not overwritten by a later pass that did not
// finish: the scene stays settled.
func (r *Settlements) Record(ctx context.Context, projectID, nodeID string, version int, outcome string, now int64) error {
	if !ValidSettlementOutcome(outcome) {
		return fmt.Errorf("ledger: unknown settlement outcome %q", outcome)
	}
	_, err := r.s.DB().ExecContext(ctx, `
INSERT INTO scene_settlements (node_id, project_id, content_version, outcome, settled_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(node_id) DO UPDATE SET
  project_id = excluded.project_id,
  content_version = excluded.content_version,
  outcome = excluded.outcome,
  settled_at = excluded.settled_at
WHERE NOT (scene_settlements.outcome = 'done'
           AND scene_settlements.content_version = excluded.content_version
           AND excluded.outcome <> 'done')`,
		nodeID, projectID, version, outcome, now)
	return err
}

// Settled is the last outcome on record for nodeID, and the version it was for.
func (r *Settlements) Settled(ctx context.Context, nodeID string) (outcome string, version int, ok bool, err error) {
	err = r.s.DB().QueryRowContext(ctx,
		`SELECT outcome, content_version FROM scene_settlements WHERE node_id = ?`, nodeID).Scan(&outcome, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	return outcome, version, err == nil, err
}

// Pending lists, in story order, the scenes before beforeNodeID that have
// prose, were written or changed since settlement was switched on, and whose
// current version was never settled done. An empty beforeNodeID, or one that
// is not in the work, asks about the whole work. Nothing is pending while
// settlement is off.
func (r *Settlements) Pending(ctx context.Context, projectID, beforeNodeID string) ([]string, error) {
	if r == nil || r.enable == nil {
		return nil, nil
	}
	since, on := r.enable()
	if !on {
		return nil, nil
	}
	all, err := r.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	t := newTree(all)
	end := len(t.leaves)
	if beforeNodeID != "" {
		if i, ok := t.index[beforeNodeID]; ok {
			end = i
		} else if first, ok := t.firstScene(beforeNodeID); ok {
			end = first
		}
	}
	done, err := r.doneVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range t.leaves[:end] {
		if n.WordCount == 0 || n.UpdatedAt < since {
			continue
		}
		if v, ok := done[n.ID]; ok && v == n.ContentVersion {
			continue
		}
		out = append(out, n.ID)
	}
	return out, nil
}

// doneVersions maps each scene settled done to the version it was settled at.
func (r *Settlements) doneVersions(ctx context.Context, projectID string) (map[string]int, error) {
	rows, err := r.s.DB().QueryContext(ctx,
		`SELECT node_id, content_version FROM scene_settlements WHERE project_id = ? AND outcome = 'done'`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var v int
		if err := rows.Scan(&id, &v); err != nil {
			return nil, err
		}
		out[id] = v
	}
	return out, rows.Err()
}

// firstScene returns the story index of the first scene under id.
func (t tree) firstScene(id string) (int, bool) {
	for i, l := range t.leaves {
		for at := l; at.ParentID != nil; {
			if *at.ParentID == id {
				return i, true
			}
			parent, ok := t.byID[*at.ParentID]
			if !ok {
				break
			}
			at = parent
		}
	}
	return 0, false
}
