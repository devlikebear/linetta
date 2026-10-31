package proposals

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// AcceptResult is what Accept returns: one entry per requested item id, in
// the order given, reflecting what actually happened to it. An id that did
// not resolve to a pending item in this project is reported the same way
// (Status failed, Result.Error set) without anything being written for it.
type AcceptResult struct {
	Items []Item `json:"items"`
	// UndoBatchIDs lists every storyops undo batch this call created —
	// purpose=forecast's structural batches only (the create_outline_node/
	// create_scene items applied together in one ApplyOpsWithRefs call).
	// Usually at most one entry: a single accept call almost always
	// targets one proposal's structural items. The panel's "되돌리기"
	// button on a forecast's outline-reflection proposal reads this to
	// call proposals.undo.
	UndoBatchIDs []string `json:"undo_batch_ids,omitempty"`
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
	case "create_outline_node", "create_scene":
		return "outline"
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
// The dependency gate is computed fresh from the op actually being
// applied — the edit, when there is one — not from the item's stored
// depends_on: depends_on is Submit's own reading of the ORIGINAL op, and
// an edit that drops a ref (e.g. switching a create_relationship from a
// from_ref to a real existing id) must not keep waiting on a dependency
// the edited op no longer has. A sibling's ref ownership is read off the
// stored items themselves (their own op.ref never changes, even when an
// edit elsewhere converts the TYPE a ref was declared under — see
// resultIDFor below), so this still gives the identical answer
// depends_on's original computation would for an unedited item.
//
// A previously FAILED item can be retried in a later call (edited or not)
// — a terminal "no second chance" would contradict the inbox's own retry
// affordance (3.D.2's [편집] stays live on a failed row). Only accepted
// and rejected are terminal.
//
// Items are applied in the proposal's own ordinal order — not the order
// itemIDs was given in — with ref-declaring items first (declarationsFirst):
// a relationship may name an entity created in the SAME call, and Submit
// never required the entity to be listed first. Items from different
// proposals are independent and do not affect each other's dependency
// resolution.
//
// A create_entity whose exact name is already in the work cannot become a
// second row: entities is UNIQUE(project_id, name), and an import sweep
// proposes the same cast in batch after batch. With the same kind it is the
// same record, so the item links to it, adding only what that record lacks
// (intoExistingEntity), and whatever depends on the item resolves to it. A
// different kind is refused with ErrorCodeNameTaken. Spelling variants and
// aliases are never linked here: those stay dedupe.go's flag for the writer.
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
//
// A failure persisting an item's own status/result (e.g. a disk error)
// aborts the whole call rather than being swallowed: Accept must never
// tell the caller an item is "accepted" when the row behind it still
// reads "pending", because a later retry would then call
// ApplyOpsWithRefs a second time for the same item and create its entity
// twice.
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
	var undoBatchIDs []string

	for proposalID, wantedIDs := range byProposal {
		all, err := s.listItemsByProposal(ctx, s.store.DB(), proposalID)
		if err != nil {
			return AcceptResult{}, err
		}
		proposal, err := s.getProposal(ctx, s.store.DB(), proposalID)
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
		seeded := storyops.Refs{Entities: map[string]string{}, Threads: map[string]string{}, Nodes: map[string]string{}}
		for _, it := range all {
			itemByID[it.ID] = it
			if it.Status == StatusAccepted {
				committed[it.ID] = true
				seedRefFrom(seeded, it)
			}
		}
		// refOwner maps a declared ref to the item that declared it —
		// read from the STORED items' own op.ref, which never changes
		// even when an edit converts that item into a different op type
		// (the alias-conversion path: create_entity -> update_entity).
		// This is what lets the dependency gate below be recomputed from
		// whatever op is actually about to be applied, instead of
		// trusting the possibly-stale depends_on an earlier Submit call
		// computed for the UNedited op.
		refOwner := refOwnerMap(all)

		changed := false

		// purpose=forecast's structural items (create_outline_node,
		// create_scene) batch together into ONE ApplyOpsWithRefs call, so
		// storyops' "whole structural batch is all-or-nothing, with one
		// undo" guarantee covers them as a unit — see this package's own
		// allowedOpTypes/forecastOpTypes comment above. Every other op
		// type (add_beat) still applies one item at a time
		// in the loop below, exactly as every other purpose always has.
		handled := map[string]bool{} // item ids this pre-pass has already given an outcome
		var structuralBatchID string
		if proposal.Purpose == PurposeForecast {
			type pendingStructural struct {
				it Item
				op storyops.Op
			}
			var structural []pendingStructural
			for _, it := range all {
				if !wanted[it.ID] {
					continue
				}
				if it.Status != StatusPending && it.Status != StatusFailed {
					continue // settled — the main loop below reports it
				}
				op := it.Op
				if edited, ok := edits[it.ID]; ok {
					validated, err := s.validateEditedOp(lookup, seeded, edited)
					if err != nil {
						it.Status, it.Result, it.StatusAt = StatusFailed, ItemResult{Error: err.Error()}, now
						if werr := s.updateItemStatus(ctx, s.store.DB(), it.ID, it.Status, it.Result, now); werr != nil {
							return AcceptResult{}, fmt.Errorf("persist item %s status: %w", it.ID, werr)
						}
						outcomes[it.ID] = it
						changed = true
						handled[it.ID] = true
						continue
					}
					if validated.Ref == "" && it.Op.Ref != "" && isRefDeclaringOp(it.Op.Type) {
						validated.Ref = it.Op.Ref
					}
					op = validated
				}
				if op.Type != "create_outline_node" && op.Type != "create_scene" {
					continue // add_beat — handled by the main loop below
				}
				structural = append(structural, pendingStructural{it: it, op: op})
			}

			if len(structural) > 0 {
				ops := make([]storyops.Op, len(structural))
				for idx, p := range structural {
					ops[idx] = p.op
				}
				applied, newRefs := s.storyops.ApplyOpsWithRefs(ctx, projectID, "",
					storyops.Proposal{Ops: ops}, seeded, func() int64 { return now })
				failureAt := map[int]string{}
				for _, f := range applied.Failures {
					failureAt[f.Index] = f.Error
				}
				if applied.IsError() {
					generic := "apply failed"
					if len(applied.Failures) > 0 {
						generic = applied.Failures[0].Error
					}
					for idx, p := range structural {
						it := p.it
						msg := generic
						if m, ok := failureAt[idx]; ok {
							msg = m
						}
						it.Status = StatusFailed
						it.Result = ItemResult{Error: msg}
						it.StatusAt = now
						if werr := s.updateItemStatus(ctx, s.store.DB(), it.ID, it.Status, it.Result, now); werr != nil {
							return AcceptResult{}, fmt.Errorf("persist item %s status: %w", it.ID, werr)
						}
						outcomes[it.ID] = it
						handled[it.ID] = true
					}
					changed = true
				} else {
					seeded = newRefs
					structuralBatchID = applied.UndoBatchID
					if structuralBatchID != "" {
						undoBatchIDs = append(undoBatchIDs, structuralBatchID)
					}
					for _, p := range structural {
						it, op := p.it, p.op
						resultID := resultIDFor(it, op, applied, newRefs)
						it.Status = StatusAccepted
						it.Result = ItemResult{CreatedID: resultID}
						it.UndoBatchID = structuralBatchID
						it.StatusAt = now
						committed[it.ID] = true
						if it.Op.Ref != "" && resultID != "" {
							seeded.Nodes[it.Op.Ref] = resultID
						}
						if werr := s.updateItemStatus(ctx, s.store.DB(), it.ID, it.Status, it.Result, now); werr != nil {
							return AcceptResult{}, fmt.Errorf("persist item %s status: %w", it.ID, werr)
						}
						if structuralBatchID != "" {
							if werr := s.updateItemUndoBatch(ctx, s.store.DB(), it.ID, structuralBatchID); werr != nil {
								return AcceptResult{}, fmt.Errorf("persist item %s undo batch: %w", it.ID, werr)
							}
						}
						// Keep itemByID current for this SAME call's main
						// loop below: an add_beat item processed right
						// after this pre-pass depends on this item's
						// UndoBatchID to tag itself, and itemByID was built
						// from `all` before this pre-pass ran.
						itemByID[it.ID] = it
						outcomes[it.ID] = it
						handled[it.ID] = true
					}
					kinds["outline"] = true
					changed = true
				}
			}
		}

		for _, it := range declarationsFirst(all) {
			if handled[it.ID] {
				continue // already given an outcome by the structural pre-pass above
			}
			// pending: not yet decided. failed: decided once, but not
			// terminal — a later call (edited or not) may retry it.
			// accepted/rejected are the only terminal statuses.
			retryable := it.Status == StatusPending || it.Status == StatusFailed
			if !wanted[it.ID] || !retryable {
				if wanted[it.ID] && !retryable {
					// Already settled (accepted/rejected) — report its
					// current state rather than silently skipping, so a
					// caller retrying a batch request can tell the item
					// is done.
					outcomes[it.ID] = it
				}
				continue
			}

			op := it.Op
			if edited, ok := edits[it.ID]; ok {
				validated, err := s.validateEditedOp(lookup, seeded, edited)
				if err != nil {
					it.Status, it.Result, it.StatusAt = StatusFailed, ItemResult{Error: err.Error()}, now
					if werr := s.updateItemStatus(ctx, s.store.DB(), it.ID, it.Status, it.Result, now); werr != nil {
						return AcceptResult{}, fmt.Errorf("persist item %s status: %w", it.ID, werr)
					}
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

			linked := false
			if op.Type == "create_entity" {
				existing, taken, err := s.entityNamed(ctx, projectID, op.Name)
				if err != nil {
					return AcceptResult{}, err
				}
				switch {
				case !taken:
				case existing.Kind == op.Kind:
					op, linked = intoExistingEntity(op, existing), true
				default:
					it.Status = StatusFailed
					it.Result = ItemResult{
						Code: ErrorCodeNameTaken,
						Error: fmt.Sprintf("%q is already a %s in this work and a name can only be used once; "+
							"edit this item's name or reject it", op.Name, existing.Kind),
					}
					it.StatusAt = now
					if werr := s.updateItemStatus(ctx, s.store.DB(), it.ID, it.Status, it.Result, now); werr != nil {
						return AcceptResult{}, fmt.Errorf("persist item %s status: %w", it.ID, werr)
					}
					outcomes[it.ID] = it
					changed = true
					continue
				}
			}

			if missing := effectiveMissingDependency(op, it.ID, refOwner, committed); missing != "" {
				label := dependencyLabel(itemByID, missing)
				it.Status = StatusFailed
				it.Result = ItemResult{
					Code:  ErrorCodeDependencyMissing,
					Error: fmt.Sprintf("depends on %q, which has not been accepted", label),
				}
				it.StatusAt = now
				if werr := s.updateItemStatus(ctx, s.store.DB(), it.ID, it.Status, it.Result, now); werr != nil {
					return AcceptResult{}, fmt.Errorf("persist item %s status: %w", it.ID, werr)
				}
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
				resultID := resultIDFor(it, op, applied, newRefs)
				it.Result = ItemResult{CreatedID: resultID, LinkedExisting: linked}
				committed[it.ID] = true
				seeded = newRefs
				// Fold this item's own ref (declared under its ORIGINAL
				// op type, e.g. create_entity, regardless of what op was
				// actually applied) into seeded, so a sibling in this
				// same call that still references it — and
				// updateProposalRefBindings below, for a later call —
				// both resolve it correctly even when an edit changed
				// what this item's op applies (f4: the alias-conversion
				// path, where ApplyOpsWithRefs' own newRefs has nothing
				// under this ref because the applied op was update_entity,
				// not create_entity).
				if it.Op.Ref != "" && resultID != "" {
					switch it.Op.Type {
					case "create_entity":
						seeded.Entities[it.Op.Ref] = resultID
					case "create_thread":
						seeded.Threads[it.Op.Ref] = resultID
					}
				}
				kinds[kindForOp(op.Type)] = true
				// An add_beat item that depends on a structural item
				// (a scene/node a forecast batch created) rides that
				// item's OWN undo batch — whether that structural item
				// was accepted in THIS call (itemByID was just refreshed
				// by the pre-pass above) or an earlier one (itemByID's
				// initial load from `all` already carries its stored
				// undo_batch_id). Undoing the outline also has to delete
				// this beat, or it survives its own scene's disappearance
				// pointing at nothing — see migration 0029's
				// story_proposal_items.undo_batch_id comment.
				//
				// Read from op.NodeRef — the ref the ACTUALLY APPLIED op
				// still carries, via the same refOwner map
				// effectiveMissingDependency's gate above just used — not
				// from it.DependsOn, which is Submit's reading of the
				// ORIGINAL op and goes stale the moment an edit drops the
				// ref (review finding f1, 2026-10-25): an edit that
				// rewrites this add_beat to point at a direct node_id
				// instead (dropping node_ref entirely, same pattern
				// TestAccept_editDroppingARefDependencyIsNotBlockedByThe-
				// StaleDependency exercises for create_relationship) no
				// longer names any sibling at all, so it.DependsOn's old
				// entry would tag this item with an unrelated structural
				// batch's undo id — later undoing THAT batch would delete
				// THIS beat as a side effect, even though the two have
				// nothing to do with each other.
				if op.Type == "add_beat" && op.NodeRef != "" {
					if owner, ok := itemByID[refOwner[op.NodeRef]]; ok && owner.UndoBatchID != "" {
						it.UndoBatchID = owner.UndoBatchID
					}
				}
			}
			it.StatusAt = now
			if werr := s.updateItemStatus(ctx, s.store.DB(), it.ID, it.Status, it.Result, now); werr != nil {
				return AcceptResult{}, fmt.Errorf("persist item %s status: %w", it.ID, werr)
			}
			if it.UndoBatchID != "" {
				if werr := s.updateItemUndoBatch(ctx, s.store.DB(), it.ID, it.UndoBatchID); werr != nil {
					return AcceptResult{}, fmt.Errorf("persist item %s undo batch: %w", it.ID, werr)
				}
			}
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
	result.UndoBatchIDs = undoBatchIDs
	s.fireNotify("proposals.changed", proposalsChangedPayload{ProjectID: projectID, Kinds: sortedKinds(kinds)})
	return result, nil
}

// Reject marks the named undecided items rejected — failed ones included,
// or a row Accept could not apply would stay in the inbox for good — and
// returns how many it rejected. It never cascades: a pending item that
// depends on a rejected one is left exactly as it is — the writer sees
// "depends on a rejected item" the next time they list the inbox
// (proposals.list, 3.D.1), rather than losing the item to an automatic chain
// reaction they did not ask for.
func (s *Service) Reject(ctx context.Context, now int64, projectID string, itemIDs []string) (int, error) {
	projectID = strings.TrimSpace(projectID)
	items, err := s.getItemsByIDs(ctx, projectID, itemIDs)
	if err != nil {
		return 0, err
	}
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rejected := 0
	for _, id := range itemIDs {
		it, ok := items[id]
		if !ok || !IsOpen(it) {
			continue
		}
		if err := s.updateItemStatus(ctx, tx, id, StatusRejected, ItemResult{}, now); err != nil {
			return 0, err
		}
		items[id] = Item{Status: StatusRejected}
		rejected++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if rejected > 0 {
		s.fireNotify("proposals.changed", proposalsChangedPayload{ProjectID: projectID, Kinds: []string{"proposals"}})
	}
	return rejected, nil
}

// IsOpen reports whether an item still waits on a decision: pending, or
// failed and so retryable. accepted and rejected are terminal, and reverting
// belongs to Undo.
func IsOpen(it Item) bool {
	return it.Status == StatusPending || it.Status == StatusFailed
}

// declarationsFirst orders a proposal's items for applying: everything that
// declares a ref, then everything else, each in ordinal order
// (listItemsByProposal already sorts by it).
func declarationsFirst(items []Item) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if isRefDeclaringOp(it.Op.Type) {
			out = append(out, it)
		}
	}
	for _, it := range items {
		if !isRefDeclaringOp(it.Op.Type) {
			out = append(out, it)
		}
	}
	return out
}

func (s *Service) entityNamed(ctx context.Context, projectID, name string) (entity.Entity, bool, error) {
	if s.entities == nil {
		return entity.Entity{}, false, nil
	}
	return s.entities.FindByName(ctx, projectID, strings.TrimSpace(name))
}

// intoExistingEntity turns a create_entity into an update of the record that
// already holds its name, carrying only what adds to it: the writer's own
// role, summary and attributes are never overwritten by a candidate, and
// aliases only ever grow. The result declares no ref of its own — resultIDFor
// reads the target back from EntityID, the same path an alias conversion
// takes, so a sibling naming this item resolves to the existing record.
func intoExistingEntity(op storyops.Op, existing entity.Entity) storyops.Op {
	merged := storyops.Op{Type: "update_entity", EntityID: existing.ID, Aliases: op.Aliases}
	if strings.TrimSpace(existing.Role) == "" {
		merged.Role = op.Role
	}
	if strings.TrimSpace(existing.Summary) == "" {
		merged.Summary = op.Summary
	}
	for key, value := range op.Attributes {
		if _, set := existing.Attributes[key]; set {
			continue
		}
		if merged.Attributes == nil {
			merged.Attributes = map[string]string{}
		}
		merged.Attributes[key] = value
	}
	return merged
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
	case "create_scene", "create_outline_node":
		seeded.Nodes[it.Op.Ref] = it.Result.CreatedID
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
	for k, v := range r.Nodes {
		out["node:"+k] = v
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
	case "create_scene", "create_outline_node":
		return refs.Nodes[op.Ref]
	case "add_beat":
		return applied.Created["beat:"+op.Ref]
	default:
		return ""
	}
}

// resultIDFor is createdIDFor plus the alias-conversion fallback (f4): when
// an item ORIGINALLY declared a ref as a create_entity/create_thread (so a
// sibling may still reference that ref), but the op actually applied this
// time is an edit that changed its type to update_entity/update_thread
// (the alias-conversion path has no other way to say "this ref now means
// an existing id"), createdIDFor finds nothing under the new op type —
// fall back to the id the edit targeted, so both a sibling in this same
// call and a later Accept call rebuilding seeded refs from storage
// (seedRefFrom, reading this same Result.CreatedID back) resolve it
// instead of silently losing the ref.
func resultIDFor(it Item, op storyops.Op, applied storyops.ApplyOpsResult, refs storyops.Refs) string {
	if id := createdIDFor(op, applied, refs); id != "" {
		return id
	}
	if it.Op.Ref == "" {
		return ""
	}
	switch {
	case it.Op.Type == "create_entity" && op.Type == "update_entity":
		return op.EntityID
	case it.Op.Type == "create_thread" && op.Type == "update_thread":
		return op.ThreadID
	default:
		return ""
	}
}

// refOwnerMap maps a declared ref (a create_entity/create_thread/
// create_scene/create_outline_node item's own op.ref, as stored — never
// rewritten by an edit) to the id of the item that declared it.
//
// create_scene/create_outline_node are included alongside create_entity/
// create_thread so that purpose=forecast's add_beat items get the same
// dependency-gate protection non-forecast items already have (review
// finding f3, 2026-10-25): without this, accepting an add_beat whose
// node_ref names a structural sibling that was never requested in this
// call (reachable from ProposalInbox — checking an item auto-selects its
// dependency, but unchecking the dependency afterward does not un-check
// it) skipped effectiveMissingDependency's "depends on X, which has not
// been accepted" refusal entirely and fell through to a raw storyops
// "scene ref/id not found" failure instead.
func refOwnerMap(items []Item) map[string]string {
	out := map[string]string{}
	for _, it := range items {
		if it.Op.Ref == "" {
			continue
		}
		switch it.Op.Type {
		case "create_entity", "create_thread", "create_scene", "create_outline_node":
			out[it.Op.Ref] = it.ID
		}
	}
	return out
}

// effectiveMissingDependency reports the first sibling item (by id) that
// op — the op actually about to be applied, which may be an edit — still
// references by an undeclared ref and that has not been committed yet.
// Computed fresh from op rather than from the item's stored depends_on, so
// an edit that drops a ref-based dependency (e.g. switching a
// create_relationship from a from_ref to a real existing id) is not kept
// waiting on a dependency it no longer has (f1); for an unedited op this
// gives the identical answer depends_on's own computation at Submit time
// did, since refOwner is built from the same ref declarations.
func effectiveMissingDependency(op storyops.Op, selfID string, refOwner map[string]string, committed map[string]bool) string {
	check := func(ref string) string {
		if ref == "" {
			return ""
		}
		owner, ok := refOwner[ref]
		if !ok || owner == selfID || committed[owner] {
			return ""
		}
		return owner
	}
	switch op.Type {
	case "create_relationship":
		if m := check(op.FromRef); m != "" {
			return m
		}
		if m := check(op.ToRef); m != "" {
			return m
		}
	case "add_beat":
		if m := check(op.ThreadRef); m != "" {
			return m
		}
		if m := check(op.NodeRef); m != "" {
			return m
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
	// allowedOpTypes ∪ forecastOpTypes, not just allowedOpTypes: an edit on
	// a purpose=forecast item (create_outline_node/create_scene) must stay
	// as acceptable as the unedited op Submit already stored — Accept has
	// no purpose value of its own to pick the narrower list with, and a
	// writer's edit changing a forecast item's TYPE across the two lists
	// entirely is not a case worth refusing specially here.
	if !allowedOpTypes[op.Type] && !forecastOpTypes[op.Type] {
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
