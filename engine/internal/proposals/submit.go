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
// purpose=forecast (story-assist Phase 4) turns a forecast's selected
// branch into outline structure: it requires ForecastID to name a forecast
// in this project that has a branch selected, and it REPLACES the usual
// allowed-op list with forecastOpTypes (create_outline_node, create_scene,
// add_beat) — not every general-purpose op widened to include those two,
// because reflecting a branch into the outline is never also a place to
// smuggle in a character or a fact card.
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
	opAllowList := allowedOpTypes
	forecastID := ""
	switch purpose {
	case PurposeGeneral, PurposeImportExtraction:
		// ok
	case PurposeForecast:
		opAllowList = forecastOpTypes
		forecastID = strings.TrimSpace(in.ForecastID)
		if forecastID == "" {
			add(GlobalIndex, "forecast_id is required when purpose=forecast")
		} else if projectID != "" {
			if s.forecasts == nil {
				add(GlobalIndex, "forecast_id %q not found", forecastID)
			} else if f, ok, err := s.forecasts.Get(ctx, forecastID); err != nil {
				return SubmitResult{}, nil, err
			} else if !ok || f.ProjectID != projectID {
				add(GlobalIndex, "forecast_id %q not found in this work", forecastID)
			} else if f.SelectedBranch == "" {
				add(GlobalIndex, "forecast %q has no selected branch yet; pick one in the forecast sheet first", forecastID)
			}
		}
	default:
		add(GlobalIndex, "purpose %q is not valid; use general, import_extraction or forecast", purpose)
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
	// which ref each ref-declaring item declares (create_entity/
	// create_thread, and — purpose=forecast only — create_scene/
	// create_outline_node, which storyops applies under op.Ref exactly the
	// same way). Assigns one from the item's own id when the model left
	// Ref empty, so an item with no explicit ref can still be depended on
	// by another item in the same batch, and so Accept always has a ref to
	// seed storyops with.
	allowedList := AllowedOpTypes()
	if purpose == PurposeForecast {
		allowedList = AllowedOpTypesForForecast()
	}
	ops := make([]storyops.Op, len(in.Items))
	declaredRefs := map[string]int{} // ref -> item index
	for i, item := range in.Items {
		op := item.Op
		if !opAllowList[op.Type] {
			add(i, "item %d: op %q cannot be proposed for purpose=%s; use linetta_apply_story_ops for a "+
				"direct small change or linetta_write_scene for manuscript text. Proposable ops: %s",
				i+1, op.Type, purpose, strings.Join(allowedList, ", "))
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
		// add_beat gets an auto ref too, same as the others, purely so
		// Accept's createdIDFor can read its created beat id back —
		// nothing else in the op catalogue ever depends ON a beat's ref,
		// so it is not added to declaredRefs below.
		if (isRefDeclaringOp(op.Type) || op.Type == "add_beat") && strings.TrimSpace(op.Ref) == "" {
			op.Ref = itemIDs[i]
		}
		if isRefDeclaringOp(op.Type) {
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
		if !opAllowList[op.Type] {
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
		// resolveNodeRef is create_scene/create_outline_node's own
		// after_node_*/parent_node_* (purpose=forecast only): any node,
		// container or leaf, declared by a create_scene/create_outline_node
		// item earlier in this batch, or a real existing node id. storyops'
		// own apply-time check is what catches "parent must be a
		// container" — Submit only confirms the id/ref resolves to
		// something in this work, the same division IsLeafNode draws for
		// every other op below.
		resolveNodeRef := func(id, ref, label string) {
			switch {
			case id != "":
				if !lookup.HasNode(id) {
					add(i, "item %d: %s %q not found in this work", num, label, id)
				}
			case ref != "":
				if dep, ok := declaredRefs[ref]; ok {
					if ops[dep].Type != "create_scene" && ops[dep].Type != "create_outline_node" {
						add(i, "item %d: %s %q is declared by item %d, which does not create a node",
							num, label, ref, dep+1)
						break
					}
					dependsOnIdx[i] = append(dependsOnIdx[i], dep)
				} else if lookup.HasNode(ref) {
					// a real id placed in the ref field.
				} else {
					add(i, "item %d: %s %q is not declared by any create_scene/create_outline_node item "+
						"in this batch and is not an existing node id", num, label, ref)
				}
			}
		}
		// resolveLeafNodeRef is add_beat's node_id/node_ref: unlike
		// resolveNodeRef above, the target must be (or become) a scene,
		// never a container — a beat anchors to something the writer
		// reads, not a folder.
		resolveLeafNodeRef := func(id, ref, label string) {
			switch {
			case id != "":
				if !lookup.IsLeafNode(id) {
					add(i, "item %d: %s %q is not a leaf scene in this work", num, label, id)
				}
			case ref != "":
				if dep, ok := declaredRefs[ref]; ok {
					if ops[dep].Type == "create_outline_node" && ops[dep].Kind == "container" {
						add(i, "item %d: %s %q refers to item %d, which creates a container, not a scene",
							num, label, ref, dep+1)
						break
					}
					if ops[dep].Type != "create_scene" && ops[dep].Type != "create_outline_node" {
						add(i, "item %d: %s %q is declared by item %d, which does not create a scene",
							num, label, ref, dep+1)
						break
					}
					dependsOnIdx[i] = append(dependsOnIdx[i], dep)
				} else if lookup.IsLeafNode(ref) {
					// a real leaf id placed in the ref field.
				} else {
					add(i, "item %d: %s %q is not declared by any create_scene/create_outline_node item "+
						"in this batch and is not an existing leaf scene", num, label, ref)
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
			resolveLeafNodeRef(op.NodeID, op.NodeRef, "node_id/node_ref")
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
		case "create_scene":
			resolveNodeRef(op.AfterNodeID, op.AfterNodeRef, "after_node_id/after_node_ref")
			if strings.TrimSpace(op.AfterNodeID) == "" && strings.TrimSpace(op.AfterNodeRef) == "" {
				// Accept applies a forecast proposal's structural ops with
				// no "current scene" context (nodeID=""), unlike
				// linetta_apply_story_ops' own call — so there is no
				// fallback to fall back TO. Submit requires an anchor up
				// front rather than letting Accept fail later with a
				// confusing "after_node_id required".
				add(i, "item %d: create_scene requires after_node_id or after_node_ref for purpose=forecast", num)
			}
		case "create_outline_node":
			resolveNodeRef(op.ParentNodeID, op.ParentNodeRef, "parent_node_id/parent_node_ref")
			resolveNodeRef(op.AfterNodeID, op.AfterNodeRef, "after_node_id/after_node_ref")
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
		ForecastID:  forecastID,
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
