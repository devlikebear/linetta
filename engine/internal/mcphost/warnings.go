package mcphost

import (
	"context"
	"fmt"
	"strings"
)

// The built-in agent is held back from writing a new episode with no plan, and
// catches up on earlier episodes' settlement before it writes (agent package,
// longform Phase 6). An external client's workflow is its own: its writes are
// never refused for either, and come back with a warning in the result
// instead, for it to act on or not.

// warningLabelMax caps how many waiting episodes a warning names.
const warningLabelMax = 5

// planWarning is the warning for a write into nodeID, read BEFORE the write:
// afterwards the scene is no longer empty and the question has no answer.
func (d ToolDeps) planWarning(ctx context.Context, nodeID string) string {
	if d.Plans == nil {
		return ""
	}
	empty, planned, err := d.Plans.PlanState(ctx, nodeID)
	if err != nil || !empty || planned {
		return ""
	}
	return "this episode had no plan: nothing said what it was for before it was written. Keep one with " +
		"linetta_write_summary as_plan=true so later episodes and the writer can check the prose against it."
}

// settlementWarning names the earlier episodes, before nodeID, whose changes
// the ledger does not hold yet. Empty while settlement is off.
func (d ToolDeps) settlementWarning(ctx context.Context, projectID, nodeID string) string {
	return d.pendingWarning(ctx, d.pendingBefore(ctx, projectID, nodeID))
}

func (d ToolDeps) pendingBefore(ctx context.Context, projectID, nodeID string) []string {
	if d.Settlements == nil {
		return nil
	}
	ids, err := d.Settlements.Pending(ctx, projectID, nodeID)
	if err != nil {
		logf("settlement warning: %v", err)
		return nil
	}
	return ids
}

func (d ToolDeps) pendingWarning(ctx context.Context, ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	labels := make([]string, 0, min(len(ids), warningLabelMax))
	for _, id := range ids[:min(len(ids), warningLabelMax)] {
		label := id
		if d.Nodes != nil {
			if n, err := d.Nodes.Get(ctx, id); err == nil && n.Label != "" {
				label = n.Label
			}
		}
		labels = append(labels, label)
	}
	more := ""
	if n := len(ids) - len(labels); n > 0 {
		more = fmt.Sprintf(" (+%d more)", n)
	}
	return fmt.Sprintf("%d earlier episode(s) are not settled into the ledger yet: %s%s. The ledger may not hold "+
		"what they changed; check their prose where it matters.", len(ids), strings.Join(labels, ", "), more)
}

func appendWarning(list []string, w string) []string {
	if w == "" {
		return list
	}
	return append(list, w)
}
