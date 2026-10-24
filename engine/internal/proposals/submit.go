package proposals

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/devlikebear/linetta/engine/internal/evidence"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// Submit validates a batch and, if nothing is wrong with it, stores it as a
// new pending proposal. Nothing is applied — that only happens through
// Accept.
//
// purpose=forecast is refused by name: Phase 4 is what gives it meaning
// (branch-forecast op bundles, create_scene/create_outline_node allowed
// only there), and this phase does not implement it.
//
// Every problem found is returned together, the same retry shape
// continuity.Validate uses: if there is even one problem, nothing is
// stored, so a model fixing what it can see never has to discover a second
// round of problems that were sitting right next to the first.
func (s *Service) Submit(ctx context.Context, now int64, in SubmitInput, source string) (SubmitResult, []Problem, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = SourceExternal
	}
	projectID := strings.TrimSpace(in.ProjectID)

	var problems []Problem
	add := func(idx int, format string, args ...any) {
		problems = append(problems, Problem{Index: idx, Message: fmt.Sprintf(format, args...)})
	}

	if projectID == "" {
		add(GlobalIndex, "project_id is required")
	}
	purpose := strings.TrimSpace(in.Purpose)
	if purpose == "" {
		purpose = PurposeGeneral
	}
	switch purpose {
	case PurposeGeneral, PurposeImportExtraction:
		// ok
	case PurposeForecast:
		add(GlobalIndex, "purpose=forecast is not supported yet; use general or import_extraction")
	default:
		add(GlobalIndex, "purpose %q is not valid; use general or import_extraction", purpose)
	}
	if n := len(in.Items); n == 0 {
		add(GlobalIndex, "items is required; propose at least one")
	} else if n > MaxItemsPerBatch {
		add(GlobalIndex, "a batch has %d items; linetta_propose_story_ops takes at most %d per call — "+
			"split it across turns", n, MaxItemsPerBatch)
	}
	if projectID == "" || len(in.Items) == 0 {
		// Nothing further can be meaningfully checked without a project or
		// any items.
		return SubmitResult{}, problems, nil
	}

	lookup := s.lookupFor(ctx, projectID)

	// itemIDs are generated up front so an op's own Ref (the one it
	// declares, for create_entity/create_thread) and this item's stored id
	// can be the same token when the model did not supply one — see the
	// auto-ref assignment below. depends_on then only ever names real item
	// ids, never a model-chosen ref string.
	itemIDs := make([]string, len(in.Items))
	for i := range itemIDs {
		itemIDs[i] = uuid.NewString()
	}

	// Pass 1: op allow-list + shape validation + normalization, and collect
	// which ref each create_entity/create_thread item declares (assigning
	// one from the item's own id when the model left Ref empty, so an item
	// with no explicit ref can still be depended on by another item in the
	// same batch, and so Accept always has a ref to seed storyops with).
	ops := make([]storyops.Op, len(in.Items))
	declaredRefs := map[string]int{} // ref -> item index
	for i, item := range in.Items {
		op := item.Op
		if !allowedOpTypes[op.Type] {
			add(i, "item %d: op %q cannot be proposed; use linetta_apply_story_ops for a direct small "+
				"change or linetta_write_scene for manuscript text. Proposable ops: %s",
				i+1, op.Type, strings.Join(AllowedOpTypes(), ", "))
			ops[i] = op
			continue
		}
		single := storyops.Proposal{Ops: []storyops.Op{op}}
		if err := storyops.ValidateProposal(single); err != nil {
			add(i, "item %d: %v", i+1, err)
			ops[i] = op
			continue
		}
		op = single.Ops[0]
		if op.Type == "create_entity" || op.Type == "create_thread" {
			if strings.TrimSpace(op.Ref) == "" {
				op.Ref = itemIDs[i]
			}
			declaredRefs[op.Ref] = i
		}
		ops[i] = op
	}

	// Pass 2: referenced ids belong to this project, *_ref usage resolves
	// to a ref declared somewhere in this batch (or a real existing id —
	// the model is allowed to put a real id in a ref field, just not a
	// name: unlike storyops.resolveEntityID, an undeclared, non-id ref is a
	// problem here, not a best-effort name search, because a proposal
	// that silently resolved to the wrong same-named entity would apply
	// against the writer's record without the writer ever having chosen
	// it), from_node_ids/evidence point at real leaf scenes, and evidence
	// quotes verify.
	dependsOnIdx := make([][]int, len(in.Items))
	for i, item := range in.Items {
		op := ops[i]
		if !allowedOpTypes[op.Type] {
			continue // already reported in pass 1
		}
		num := i + 1

		resolveEntityRef := func(id, ref, label string) {
			switch {
			case id != "":
				if !lookup.HasEntity(id) {
					add(i, "item %d: %s %q not found in this work; check it with linetta_list_characters",
						num, label, id)
				}
			case ref != "":
				if dep, ok := declaredRefs[ref]; ok {
					dependsOnIdx[i] = append(dependsOnIdx[i], dep)
				} else if lookup.HasEntity(ref) {
					// a real id placed in the ref field — fine, no dependency.
				} else {
					add(i, "item %d: %s %q is not declared by any create_entity item in this batch and is "+
						"not an existing entity id", num, label, ref)
				}
			}
		}
		resolveThreadRef := func(id, ref, label string) {
			switch {
			case id != "":
				if !lookup.HasThread(id) {
					add(i, "item %d: %s %q not found in this work; check it with linetta_get_plot", num, label, id)
				}
			case ref != "":
				if dep, ok := declaredRefs[ref]; ok {
					dependsOnIdx[i] = append(dependsOnIdx[i], dep)
				} else if lookup.HasThread(ref) {
					// a real id placed in the ref field.
				} else {
					add(i, "item %d: %s %q is not declared by any create_thread item in this batch and is "+
						"not an existing thread id", num, label, ref)
				}
			}
		}

		switch op.Type {
		case "update_entity":
			resolveEntityRef(op.EntityID, "", "entity_id")
		case "create_relationship":
			resolveEntityRef(op.From, op.FromRef, "from/from_ref")
			resolveEntityRef(op.To, op.ToRef, "to/to_ref")
		case "add_beat":
			resolveThreadRef(op.ThreadID, op.ThreadRef, "thread_id/thread_ref")
			if op.NodeID != "" && !lookup.IsLeafNode(op.NodeID) {
				add(i, "item %d: node_id %q is not a leaf scene in this work", num, op.NodeID)
			}
		case "update_beat":
			if !lookup.HasBeat(op.BeatID) {
				add(i, "item %d: beat_id %q not found in this work; check it with linetta_get_plot", num, op.BeatID)
			}
		case "update_thread":
			resolveThreadRef(op.ThreadID, "", "thread_id")
		case "create_fact_card":
			if op.NodeID != "" && !lookup.IsLeafNode(op.NodeID) {
				add(i, "item %d: node_id %q is not a leaf scene in this work", num, op.NodeID)
			}
		}

		for _, nodeID := range item.FromNodeIDs {
			if !lookup.IsLeafNode(nodeID) {
				add(i, "item %d: from_node_ids has %q, which is not a leaf scene in this work", num, nodeID)
			}
		}
		if n := len(item.Evidence); n > MaxEvidencePerItem {
			add(i, "item %d has %d evidence entries; the limit is %d", num, n, MaxEvidencePerItem)
		}
		for ei, ev := range item.Evidence {
			paragraphs, ok := lookup.SceneParagraphs(ev.NodeID)
			if !ok {
				add(i, "item %d evidence %d: scene %q is not a leaf scene in this work", num, ei+1, ev.NodeID)
				continue
			}
			if err := evidence.VerifyQuote(paragraphs, ev.Paragraph, ev.Paragraph, ev.Quote); err != nil {
				add(i, "item %d evidence %d: %s", num, ei+1, evidenceMessage(ev, err))
			}
		}
	}

	if len(problems) > 0 {
		return SubmitResult{}, problems, nil
	}

	// Nothing wrong: resolve depends_on into real item ids, run dedupe
	// (informational — never blocks a submission), and store.
	items := make([]Item, len(in.Items))
	for i, item := range in.Items {
		dependsOn := make([]string, 0, len(dependsOnIdx[i]))
		for _, dep := range dependsOnIdx[i] {
			dependsOn = append(dependsOn, itemIDs[dep])
		}
		items[i] = Item{
			ID:          itemIDs[i],
			Ordinal:     i,
			Op:          ops[i],
			DependsOn:   dependsOn,
			Evidence:    item.Evidence,
			FromNodeIDs: trimAll(item.FromNodeIDs),
			Status:      StatusPending,
			Result:      ItemResult{},
			StatusAt:    now,
		}
	}

	if err := s.attachDuplicates(ctx, projectID, items); err != nil {
		return SubmitResult{}, nil, err
	}

	proposal := Proposal{
		ID:          uuid.NewString(),
		ProjectID:   projectID,
		Purpose:     purpose,
		Source:      source,
		Summary:     strings.TrimSpace(in.Summary),
		RefBindings: map[string]string{},
		CreatedAt:   now,
	}
	for i := range items {
		items[i].ProposalID = proposal.ID
		items[i].ProjectID = projectID
	}

	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return SubmitResult{}, nil, err
	}
	defer tx.Rollback()
	if err := s.insertProposal(ctx, tx, proposal); err != nil {
		return SubmitResult{}, nil, err
	}
	for _, it := range items {
		if err := s.insertItem(ctx, tx, it); err != nil {
			return SubmitResult{}, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SubmitResult{}, nil, err
	}

	possible := 0
	for _, it := range items {
		if len(it.Duplicates) > 0 {
			possible++
		}
	}
	return SubmitResult{ProposalID: proposal.ID, Items: items, PossibleDuplicates: possible}, nil, nil
}

// evidenceMessage phrases an evidence.Error the way an item's own evidence
// entry is addressed, distinct from continuity's "observation %d ref %d"
// phrasing since an evidence entry here is not a Ref.
func evidenceMessage(ev EvidenceInput, err error) string {
	var ee *evidence.Error
	if e, ok := err.(*evidence.Error); ok {
		ee = e
	}
	if ee == nil {
		return err.Error()
	}
	switch ee.Kind {
	case evidence.FromTooSmall, evidence.OutOfRange:
		return fmt.Sprintf("paragraph %d is out of range; scene %q has %d paragraphs", ee.To, ev.NodeID, ee.N)
	case evidence.QuoteNotFound:
		return fmt.Sprintf("quote not found in scene %q paragraph %d; copy it exactly or drop the evidence",
			ev.NodeID, ee.To)
	default:
		return err.Error()
	}
}
