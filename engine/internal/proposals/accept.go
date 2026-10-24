package proposals

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// AcceptResult is what Accept returns: one entry per requested item id, in
// the order given, reflecting what actually happened to it. An id that did
// not resolve to a pending item in this project is reported the same way
// (Status failed, Result.Error set) without anything being written for it.
type AcceptResult struct {
	Items []Item `json:"items"`
}

// kindForOp names what Story World area an op touches, for the
// proposals.changed notification's "kinds" field — the panels listening
// for it (Story World, FactBook) use this to decide whether to refetch
// without having to inspect every accepted op themselves.
func kindForOp(opType string) string {
	switch opType {
	case "create_entity", "update_entity":
		return "entity"
	case "create_relationship":
		return "relationship"
	case "create_fact_card":
		return "fact"
	case "create_thread", "update_thread":
		return "thread"
	case "add_beat", "update_beat":
		return "beat"
	default:
		return ""
	}
}

// Accept applies the chosen items, plus nothing else. An item whose
// dependency is neither already accepted nor in this call is refused with
// the dependency named — never applied with a dangling ref. Edits are
// validated the same way Submit validates a new item (the allowed op list,
// storyops.ValidateProposal, and every referenced id checked against this
// project) before being applied in the edited item's place.
//
// Items are applied in the proposal's own ordinal order — not the order
// itemIDs was given in — because a later item's ref may depend on an
// earlier one created earlier in the SAME call. Items from different
// proposals are independent and do not affect each other's dependency
// resolution.
//
// Unlike Submit (one all-or-nothing transaction) and storyops' own
// structural batches, Accept commits one item at a time with no enclosing
// transaction: storyops.ApplyOpsWithRefs reaches entity/thread/beat/fact
// repos that share this process's single SQLite connection
// (store.Open sets MaxOpenConns(1)), so holding a proposals-table
// transaction open across that call would deadlock the connection against
// itself. "하나가 실패해도 나머지는 진행한다" (3.B.3) already asks for
// per-item, not per-batch, commit semantics, so this is the right shape
// even without that constraint.
func (s *Service) Accept(ctx context.Context, now int64, projectID string, itemIDs []string, edits map[string]storyops.Op) (AcceptResult, error) {
	projectID = strings.TrimSpace(projectID)
	requested, err := s.getItemsByIDs(ctx, projectID, itemIDs)
	if err != nil {
		return AcceptResult{}, err
	}

	result := AcceptResult{Items: make([]Item, 0, len(itemIDs))}
	kinds := map[string]bool{"proposals": true}

	// Group the requested items by proposal so each proposal's ref seed and
	// "already accepted" set is computed once and applied in that
	// proposal's own ordinal order.
	byProposal := map[string][]string{} // proposal id -> requested item ids (any order)
	for _, id := range itemIDs {
		it, ok := requested[id]
		if !ok {
			result.Items = append(result.Items, Item{
				ID: id, ProjectID: projectID, Status: StatusFailed,
				Result: ItemResult{Error: "proposal item not found in this project"},
			})
			continue
		}
		byProposal[it.ProposalID] = append(byProposal[it.ProposalID], id)
	}

	outcomes := map[string]Item{} // item id -> its final Item, across every proposal processed

	for proposalID, wantedIDs := range byProposal {
		all, err := s.listItemsByProposal(ctx, s.store.DB(), proposalID)
		if err != nil {
			return AcceptResult{}, err
		}
		wanted := map[string]bool{}
		for _, id := range wantedIDs {
			wanted[id] = true
		}

		lookup := s.lookupFor(ctx, projectID)
		committed := map[string]bool{} // item ids whose op is already applied (DB or this call)
		itemByID := map[string]Item{}  // every item in this proposal, by id
		seeded := storyops.Refs{Entities: map[string]string{}, Threads: map[string]string{}}
		for _, it := range all {
			itemByID[it.ID] = it
			if it.Status == StatusAccepted {
				committed[it.ID] = true
				seedRefFrom(seeded, it)
			}
		}

		changed := false
		for _, it := range all { // ordinal order — listItemsByProposal already sorts by ordinal
			if !wanted[it.ID] || it.Status != StatusPending {
				if wanted[it.ID] && it.Status != StatusPending {
					// Already decided (accepted/rejected/failed earlier) —
					// report its current state rather than silently
					// skipping, so a caller retrying a batch request can
					// tell the item is settled.
					outcomes[it.ID] = it
				}
				continue
			}

			op := it.Op
			if edited, ok := edits[it.ID]; ok {
				validated, err := s.validateEditedOp(lookup, seeded, edited)
				if err != nil {
					it.Status, it.Result, it.StatusAt = StatusFailed, ItemResult{Error: err.Error()}, now
					_ = s.updateItemStatus(ctx, s.store.DB(), it.ID, it.Status, it.Result, now)
					outcomes[it.ID] = it
					changed = true
					continue
				}
				// A writer's edit payload has no reason to carry the
				// internal ref token Submit assigned for created-id
				// tracking; keep the original one so CreatedID below (and
				// any item accepted later that depends on this one) still
				// resolves, unless the edit declared its own.
				if validated.Ref == "" && it.Op.Ref != "" &&
					(validated.Type == "create_entity" || validated.Type == "create_thread") {
					validated.Ref = it.Op.Ref
				}
				op = validated
			}

			if missing := firstMissingDependency(it.DependsOn, committed); missing != "" {
				label := dependencyLabel(itemByID, missing)
				it.Status = StatusFailed
				it.Result = ItemResult{Error: fmt.Sprintf("depends on %q, which has not been accepted", label)}
				it.StatusAt = now
				_ = s.updateItemStatus(ctx, s.store.DB(), it.ID, it.Status, it.Result, now)
				outcomes[it.ID] = it
				changed = true
				continue
			}

			applied, newRefs := s.storyops.ApplyOpsWithRefs(ctx, projectID, "",
				storyops.Proposal{Ops: []storyops.Op{op}}, seeded, func() int64 { return now })
			if applied.IsError() {
				it.Status = StatusFailed
				msg := "apply failed"
				if len(applied.Failures) > 0 {
					msg = applied.Failures[0].Error
				}
				it.Result = ItemResult{Error: msg}
			} else {
				it.Status = StatusAccepted
				it.Result = ItemResult{CreatedID: createdIDFor(op, applied, newRefs)}
				committed[it.ID] = true
				seeded = newRefs
				kinds[kindForOp(op.Type)] = true
			}
			it.StatusAt = now
			_ = s.updateItemStatus(ctx, s.store.DB(), it.ID, it.Status, it.Result, now)
			outcomes[it.ID] = it
			changed = true
		}

		if changed {
			if err := s.updateProposalRefBindings(ctx, s.store.DB(), proposalID, mergeRefs(seeded)); err != nil {
				return AcceptResult{}, err
			}
		}
	}

	for _, id := range itemIDs {
		if it, ok := outcomes[id]; ok {
			result.Items = append(result.Items, it)
		}
	}
	s.fireNotify("proposals.changed", proposalsChangedPayload{ProjectID: projectID, Kinds: sortedKinds(kinds)})
	return result, nil
}

// Reject marks the named pending items rejected. It never cascades: a
// pending item that depends on a rejected one is left exactly as it is —
// the writer sees "depends on a rejected item" the next time they list the
// inbox (proposals.list, 3.D.1), rather than losing the item to an
// automatic chain reaction they did not ask for.
func (s *Service) Reject(ctx context.Context, now int64, projectID string, itemIDs []string) error {
	projectID = strings.TrimSpace(projectID)
	items, err := s.getItemsByIDs(ctx, projectID, itemIDs)
	if err != nil {
		return err
	}
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	any := false
	for _, id := range itemIDs {
		it, ok := items[id]
		if !ok || it.Status != StatusPending {
			continue
		}
		if err := s.updateItemStatus(ctx, tx, id, StatusRejected, ItemResult{}, now); err != nil {
			return err
		}
		any = true
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if any {
		s.fireNotify("proposals.changed", proposalsChangedPayload{ProjectID: projectID, Kinds: []string{"proposals"}})
	}
	return nil
}

// proposalsChangedPayload is what fireNotify sends for "proposals.changed" —
// the proposal inbox's own change event (3.B.3's decision), separate from
// mcp.changed because an accept/reject click comes from the writer, not an
// MCP client.
type proposalsChangedPayload struct {
	ProjectID string   `json:"project_id"`
	Kinds     []string `json:"kinds"`
}

func sortedKinds(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		if k == "" {
			continue
		}
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// seedRefFrom folds an already-accepted item's declared ref (if any) into
// seeded, using its stored CreatedID — rebuilding storyops.Refs from the
// item table rather than trusting a cached copy keeps it correct even if
// an earlier Accept call's own cache update failed partway.
func seedRefFrom(seeded storyops.Refs, it Item) {
	if it.Op.Ref == "" || it.Result.CreatedID == "" {
		return
	}
	switch it.Op.Type {
	case "create_entity":
		seeded.Entities[it.Op.Ref] = it.Result.CreatedID
	case "create_thread":
		seeded.Threads[it.Op.Ref] = it.Result.CreatedID
	}
}

func mergeRefs(r storyops.Refs) map[string]string {
	out := map[string]string{}
	for k, v := range r.Entities {
		out["entity:"+k] = v
	}
	for k, v := range r.Threads {
		out["thread:"+k] = v
	}
	return out
}

func createdIDFor(op storyops.Op, applied storyops.ApplyOpsResult, refs storyops.Refs) string {
	if op.Ref == "" {
		return ""
	}
	switch op.Type {
	case "create_entity":
		return refs.Entities[op.Ref]
	case "create_thread":
		return refs.Threads[op.Ref]
	case "create_fact_card":
		return applied.Created["fact:"+op.Ref]
	default:
		return ""
	}
}

func firstMissingDependency(dependsOn []string, committed map[string]bool) string {
	for _, dep := range dependsOn {
		if !committed[dep] {
			return dep
		}
	}
	return ""
}

func dependencyLabel(byID map[string]Item, id string) string {
	it, ok := byID[id]
	if !ok {
		return id
	}
	if it.Op.Name != "" {
		return it.Op.Name
	}
	if it.Op.Label != "" {
		return it.Op.Label
	}
	return id
}

// validateEditedOp re-validates a writer's edit of a pending item's op the
// same way Submit validates a new one: the allowed-op list,
// storyops.ValidateProposal's shape check, and every id/ref the op touches
// checked against this project (direct ids) or the proposal's already-known
// refs (seeded — both earlier-accepted items and items accepted earlier in
// the SAME Accept call).
func (s *Service) validateEditedOp(lookup refLookup, seeded storyops.Refs, op storyops.Op) (storyops.Op, error) {
	if !allowedOpTypes[op.Type] {
		return op, fmt.Errorf("op %q cannot be proposed; use linetta_apply_story_ops or linetta_write_scene instead", op.Type)
	}
	single := storyops.Proposal{Ops: []storyops.Op{op}}
	if err := storyops.ValidateProposal(single); err != nil {
		return op, err
	}
	op = single.Ops[0]

	resolveEntity := func(id, ref, label string) error {
		switch {
		case id != "":
			if !lookup.HasEntity(id) {
				return fmt.Errorf("%s %q not found in this work", label, id)
			}
		case ref != "":
			if _, ok := seeded.Entities[ref]; ok {
				return nil
			}
			if !lookup.HasEntity(ref) {
				return fmt.Errorf("%s %q is not a known entity in this proposal or this work", label, ref)
			}
		}
		return nil
	}
	resolveThread := func(id, ref, label string) error {
		switch {
		case id != "":
			if !lookup.HasThread(id) {
				return fmt.Errorf("%s %q not found in this work", label, id)
			}
		case ref != "":
			if _, ok := seeded.Threads[ref]; ok {
				return nil
			}
			if !lookup.HasThread(ref) {
				return fmt.Errorf("%s %q is not a known thread in this proposal or this work", label, ref)
			}
		}
		return nil
	}

	switch op.Type {
	case "update_entity":
		if err := resolveEntity(op.EntityID, "", "entity_id"); err != nil {
			return op, err
		}
	case "create_relationship":
		if err := resolveEntity(op.From, op.FromRef, "from/from_ref"); err != nil {
			return op, err
		}
		if err := resolveEntity(op.To, op.ToRef, "to/to_ref"); err != nil {
			return op, err
		}
	case "add_beat":
		if err := resolveThread(op.ThreadID, op.ThreadRef, "thread_id/thread_ref"); err != nil {
			return op, err
		}
		if op.NodeID != "" && !lookup.IsLeafNode(op.NodeID) {
			return op, fmt.Errorf("node_id %q is not a leaf scene in this work", op.NodeID)
		}
	case "update_beat":
		if !lookup.HasBeat(op.BeatID) {
			return op, fmt.Errorf("beat_id %q not found in this work", op.BeatID)
		}
	case "update_thread":
		if err := resolveThread(op.ThreadID, "", "thread_id"); err != nil {
			return op, err
		}
	case "create_fact_card":
		if op.NodeID != "" && !lookup.IsLeafNode(op.NodeID) {
			return op, fmt.Errorf("node_id %q is not a leaf scene in this work", op.NodeID)
		}
	}
	return op, nil
}
