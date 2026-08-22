package storyops

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/ptrutil"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// MemoryRecorder persists a remembered fact for a project. The companion's
// keyword memory implements it today; callers without memory (e.g. a
// standalone MCP applier) may leave it unset and the remember op fails with
// a clear message instead of a nil-pointer panic.
type MemoryRecorder interface {
	Remember(projectID, text, category string) error
}

// Service applies validated op batches to project state. All mutations run
// through the same repos the UI uses, so mention resync, manuscript
// reindexing, and word counts happen exactly as if the writer typed them.
type Service struct {
	projects      *project.Repo
	nodes         *node.Repo
	threads       *thread.Repo
	beats         *beat.Repo
	entities      *entity.Repo
	relationships *relationship.Repo
	facts         *fact.Repo
	snaps         *snapshot.Repo
	memory        MemoryRecorder

	undo undoState
}

// New wires the applier over the required repos. Facts, snapshots, and memory
// are optional; see the With* setters.
func New(
	projects *project.Repo, nodes *node.Repo, threads *thread.Repo,
	beats *beat.Repo, entities *entity.Repo, relationships *relationship.Repo,
) *Service {
	return &Service{
		projects: projects, nodes: nodes, threads: threads,
		beats: beats, entities: entities, relationships: relationships,
	}
}

// WithFacts enables create_fact_card.
func (s *Service) WithFacts(repo *fact.Repo) *Service {
	s.facts = repo
	return s
}

// WithSnapshots enables the companion-before snapshot on set_scene_text.
func (s *Service) WithSnapshots(snaps *snapshot.Repo) *Service {
	s.snaps = snaps
	return s
}

// WithMemory enables the remember op.
func (s *Service) WithMemory(m MemoryRecorder) *Service {
	s.memory = m
	return s
}

type ApplyOpsResult struct {
	Summary      string              `json:"summary,omitempty"`
	Applied      int                 `json:"applied"`
	Created      map[string]string   `json:"created,omitempty"`
	ChangedNodes []AppliedNodeChange `json:"changed_nodes,omitempty"`
	Failures     []ApplyOpsFailure   `json:"failures,omitempty"`
	// PendingApproval means the batch was large enough to show the writer first,
	// so nothing was applied and the ops are waiting in a preview.
	PendingApproval bool `json:"pending_approval,omitempty"`
	// RolledBack means an op failed partway and the outline was put back.
	RolledBack bool `json:"rolled_back,omitempty"`
	// UndoBatchID identifies the pre-change outline kept for a one-step undo.
	UndoBatchID string `json:"undo_batch_id,omitempty"`
}

type ApplyOpsFailure struct {
	Index int    `json:"index"`
	Op    string `json:"op,omitempty"`
	Error string `json:"error"`
}

type AppliedNodeChange struct {
	NodeID         string `json:"node_id"`
	Op             string `json:"op"`
	ContentVersion int    `json:"content_version"`
	CharCount      int    `json:"char_count"`
	TextPreview    string `json:"text_preview,omitempty"`
}

// IsError reports whether any op in the batch failed.
func (r ApplyOpsResult) IsError() bool {
	return len(r.Failures) > 0
}

// ApplyOps applies a validated proposal op list directly to project state.
func (s *Service) ApplyOps(ctx context.Context, projectID, nodeID string, p Proposal, now func() int64) ApplyOpsResult {
	result := ApplyOpsResult{
		Summary: strings.TrimSpace(p.Summary),
		Created: map[string]string{},
	}
	if err := ValidateProposal(p); err != nil {
		result.Failures = append(result.Failures, ApplyOpsFailure{Index: -1, Error: err.Error()})
		return result
	}

	// Structural batches are all-or-nothing: the outline is captured first so a
	// failure halfway through can be put back, and a clean run leaves the writer
	// one undo away from where they started.
	structural := CountOutlineChanges(p).Structural() > 0
	var before []node.Node
	if structural {
		before = s.snapshotOutline(ctx, projectID)
	}

	threadRefs := map[string]string{}
	entityRefs := map[string]string{}
	nodeRefs := map[string]string{}
	nodeInsertCursor := ""

	for i, op := range p.Ops {
		if err := s.applyOneOp(ctx, projectID, nodeID, op, now, threadRefs, entityRefs, nodeRefs, result.Created, &result.ChangedNodes, &nodeInsertCursor); err != nil {
			result.Failures = append(result.Failures, ApplyOpsFailure{Index: i, Op: op.Type, Error: err.Error()})
			continue
		}
		result.Applied++
	}
	if len(result.Created) == 0 {
		result.Created = nil
	}
	if len(result.ChangedNodes) == 0 {
		result.ChangedNodes = nil
	}
	if structural && len(before) > 0 {
		if result.IsError() {
			// Half a restructured outline is worse than none, so put the tree back
			// and report the failure against an unchanged project.
			if err := s.nodes.RestoreOutline(ctx, projectID, before, now()); err == nil {
				result.RolledBack = true
				result.Applied = 0
				result.Created = nil
				result.ChangedNodes = nil
			}
		} else if result.Applied > 0 {
			result.UndoBatchID = s.rememberUndoBatch(projectID, before)
		}
	}
	return result
}

func (s *Service) applyOneOp(
	ctx context.Context,
	projectID string,
	currentNodeID string,
	op Op,
	now func() int64,
	threadRefs map[string]string,
	entityRefs map[string]string,
	nodeRefs map[string]string,
	created map[string]string,
	changedNodes *[]AppliedNodeChange,
	nodeInsertCursor *string,
) error {
	switch op.Type {
	case "set_outline":
		outline := op.Outline
		_, err := s.projects.Update(ctx, now(), project.UpdateInput{ID: projectID, Outline: &outline})
		return err
	case "set_scene_text":
		targetNodeID, err := s.resolveOptionalNodeID(ctx, op.NodeID, op.NodeRef, currentNodeID, nodeRefs)
		if err != nil {
			return err
		}
		if targetNodeID == nil || strings.TrimSpace(*targetNodeID) == "" {
			return fmt.Errorf("set_scene_text requires a current node or node_id")
		}
		before, err := s.nodes.Get(ctx, *targetNodeID)
		if err != nil {
			return err
		}
		if s.snaps != nil {
			beforeDoc := ""
			if before.ContentDoc != nil {
				beforeDoc = *before.ContentDoc
			}
			if _, _, err := s.snaps.CreateIfChanged(ctx, *targetNodeID, beforeDoc, snapshot.ReasonCompanionBefore, now()); err != nil {
				return fmt.Errorf("companion-before snapshot: %w", err)
			}
		}
		doc, err := PlainTextToTiptapDoc(op.Text)
		if err != nil {
			return err
		}
		if err := s.nodes.UpdateContent(ctx, *targetNodeID, doc, now()); err != nil {
			return err
		}
		after, err := s.nodes.Get(ctx, *targetNodeID)
		if err != nil {
			return fmt.Errorf("verify set_scene_text: %w", err)
		}
		gotText := normalizeSceneTextForVerify(plainTextFromDoc(after.ContentDoc))
		wantText := normalizeSceneTextForVerify(op.Text)
		if gotText != wantText {
			return fmt.Errorf("verify set_scene_text: readback text mismatch")
		}
		if !op.AllowEmpty && strings.TrimSpace(gotText) == "" {
			return fmt.Errorf("verify set_scene_text: readback text is empty")
		}
		if after.ContentVersion <= before.ContentVersion {
			return fmt.Errorf("verify set_scene_text: content_version did not advance")
		}
		*changedNodes = append(*changedNodes, AppliedNodeChange{
			NodeID:         after.ID,
			Op:             "set_scene_text",
			ContentVersion: after.ContentVersion,
			CharCount:      after.WordCount,
			TextPreview:    trimRunes(strings.TrimSpace(plainTextFromDoc(after.ContentDoc)), 120),
		})
		return nil
	case "create_thread":
		th, err := s.threads.Create(ctx, thread.NewInput{ProjectID: projectID, Name: op.Name, Color: op.Color})
		if err != nil {
			return err
		}
		if strings.TrimSpace(op.Summary) != "" {
			if err := s.threads.Update(ctx, thread.UpdateInput{ID: th.ID, Summary: ptrutil.To(op.Summary)}); err != nil {
				return err
			}
		}
		if op.Ref != "" {
			threadRefs[op.Ref] = th.ID
			created["thread:"+op.Ref] = th.ID
		}
		return nil
	case "update_thread":
		in := thread.UpdateInput{ID: op.ThreadID}
		if strings.TrimSpace(op.Name) != "" {
			in.Name = ptrutil.To(op.Name)
		}
		if strings.TrimSpace(op.Color) != "" {
			in.Color = ptrutil.To(op.Color)
		}
		if strings.TrimSpace(op.Summary) != "" {
			in.Summary = ptrutil.To(op.Summary)
		}
		return s.threads.Update(ctx, in)
	case "add_beat":
		threadID, err := s.resolveThreadID(ctx, projectID, op.ThreadID, op.ThreadRef, threadRefs)
		if err != nil {
			return err
		}
		beatNodeID, err := s.resolveOptionalNodeID(ctx, op.NodeID, op.NodeRef, currentNodeID, nodeRefs)
		if err != nil {
			return err
		}
		_, err = s.beats.Create(ctx, beat.NewInput{
			ThreadID:    threadID,
			NodeID:      beatNodeID,
			Label:       op.Label,
			Description: op.Description,
			Intensity:   op.Intensity,
		})
		return err
	case "update_beat":
		in := beat.UpdateInput{ID: op.BeatID, Label: op.Label, Intensity: op.Intensity}
		if strings.TrimSpace(op.Description) != "" {
			in.Description = &op.Description
		}
		return s.beats.Update(ctx, in)
	case "delete_beat":
		return s.beats.Delete(ctx, op.BeatID)
	case "remember":
		if s.memory == nil {
			return fmt.Errorf("memory is not available")
		}
		return s.memory.Remember(projectID, op.Text, op.Category)
	case "create_fact_card":
		if s.facts == nil {
			return fmt.Errorf("fact book is not available")
		}
		cardNodeID, err := s.resolveOptionalNodeID(ctx, op.NodeID, op.NodeRef, currentNodeID, nodeRefs)
		if err != nil {
			return err
		}
		card, err := s.facts.Create(ctx, now(), fact.NewInput{
			ProjectID: projectID,
			NodeID:    cardNodeID,
			Claim:     op.Claim,
			Result:    op.Result,
			Status:    op.Status,
			Category:  op.Category,
			Sources:   op.Sources,
		})
		if err != nil {
			return err
		}
		if op.Ref != "" {
			created["fact:"+op.Ref] = card.ID
		}
		return nil
	case "create_entity":
		ent, err := s.entities.Create(ctx, now(), entity.NewInput{
			ProjectID: projectID,
			Kind:      op.Kind,
			Name:      op.Name,
			Role:      op.Role,
		})
		if err != nil {
			return err
		}
		attrs := cleanEntityAttributes(op.Attributes)
		if strings.TrimSpace(op.Summary) != "" || len(attrs) > 0 {
			in := entity.UpdateInput{ID: ent.ID, Attributes: optionalEntityAttributes(attrs)}
			if strings.TrimSpace(op.Summary) != "" {
				in.Summary = ptrutil.To(op.Summary)
			}
			if err := s.entities.Update(ctx, now(), in); err != nil {
				return err
			}
		}
		if op.Ref != "" {
			entityRefs[op.Ref] = ent.ID
			created["entity:"+op.Ref] = ent.ID
		}
		return nil
	case "update_entity":
		entityID, err := s.resolveEntityID(ctx, projectID, op.EntityID, "", entityRefs, "entity")
		if err != nil {
			return err
		}
		cur, err := s.entities.Get(ctx, entityID)
		if err != nil {
			return err
		}
		in := entity.UpdateInput{ID: entityID}
		if strings.TrimSpace(op.Kind) != "" {
			in.Kind = ptrutil.To(op.Kind)
		}
		if strings.TrimSpace(op.Name) != "" {
			in.Name = ptrutil.To(op.Name)
		}
		if strings.TrimSpace(op.Role) != "" {
			in.Role = ptrutil.To(op.Role)
		}
		if strings.TrimSpace(op.Summary) != "" {
			in.Summary = ptrutil.To(op.Summary)
		}
		if attrs := cleanEntityAttributes(op.Attributes); len(attrs) > 0 {
			merged := mergeEntityAttributes(cur.Attributes, attrs)
			in.Attributes = &merged
		}
		return s.entities.Update(ctx, now(), in)
	case "create_relationship":
		fromID, err := s.resolveEntityID(ctx, projectID, op.From, op.FromRef, entityRefs, "from entity")
		if err != nil {
			return err
		}
		toID, err := s.resolveEntityID(ctx, projectID, op.To, op.ToRef, entityRefs, "to entity")
		if err != nil {
			return err
		}
		if strings.TrimSpace(op.InverseLabel) != "" {
			_, err := s.relationships.CreatePair(ctx, relationship.NewPairInput{
				ProjectID:    projectID,
				FromID:       fromID,
				ToID:         toID,
				Label:        op.Label,
				InverseLabel: op.InverseLabel,
				Notes:        op.Notes,
			})
			return err
		}
		_, err = s.relationships.CreateOne(ctx, relationship.NewInput{
			ProjectID: projectID,
			FromID:    fromID,
			ToID:      toID,
			Label:     op.Label,
			Notes:     op.Notes,
		})
		return err
	case "create_scene":
		afterNodeID, err := s.resolveSceneAnchor(ctx, projectID, currentNodeID, op.AfterNodeID, op.AfterNodeRef, nodeRefs, *nodeInsertCursor)
		if err != nil {
			return err
		}
		n, err := s.nodes.CreateSibling(ctx, afterNodeID, node.KindLeaf, op.Label, op.Title, now())
		if err != nil {
			return err
		}
		if op.Ref != "" {
			nodeRefs[op.Ref] = n.ID
			created["node:"+op.Ref] = n.ID
		}
		*nodeInsertCursor = n.ID
		return nil
	case "create_outline_node":
		return s.applyCreateOutlineNode(ctx, projectID, currentNodeID, op, now, nodeRefs, created, nodeInsertCursor)
	case "rename_outline_node":
		nodeID, err := s.resolveRequiredNodeID(ctx, op.NodeID, op.NodeRef, nodeRefs)
		if err != nil {
			return err
		}
		cur, err := s.nodes.Get(ctx, nodeID)
		if err != nil {
			return err
		}
		label := cur.Label
		if strings.TrimSpace(op.Label) != "" {
			label = op.Label
		}
		title := cur.Title
		if strings.TrimSpace(op.Title) != "" {
			title = op.Title
		}
		return s.nodes.Rename(ctx, nodeID, label, title, now())
	case "delete_outline_node":
		nodeID, err := s.resolveRequiredNodeID(ctx, op.NodeID, op.NodeRef, nodeRefs)
		if err != nil {
			return err
		}
		return s.nodes.Delete(ctx, nodeID, now())
	case "move_outline_node":
		nodeID, err := s.resolveRequiredNodeID(ctx, op.NodeID, op.NodeRef, nodeRefs)
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(op.Direction)) == "up" {
			return s.nodes.MoveUp(ctx, nodeID, now())
		}
		return s.nodes.MoveDown(ctx, nodeID, now())
	default:
		return fmt.Errorf("unknown op %q", op.Type)
	}
}

func (s *Service) applyCreateOutlineNode(
	ctx context.Context,
	projectID string,
	currentNodeID string,
	op Op,
	now func() int64,
	nodeRefs map[string]string,
	created map[string]string,
	nodeInsertCursor *string,
) error {
	kind := strings.TrimSpace(op.Kind)
	if kind == "" {
		kind = node.KindLeaf
	}
	if !node.ValidKind(kind) {
		return fmt.Errorf("create_outline_node: kind must be container|leaf")
	}
	parentID, err := s.resolveOptionalNodeID(ctx, op.ParentNodeID, op.ParentNodeRef, "", nodeRefs)
	if err != nil {
		return err
	}
	var n node.Node
	if parentID != nil {
		parent, err := s.nodes.Get(ctx, *parentID)
		if err != nil {
			return err
		}
		if parent.Kind != node.KindContainer {
			return fmt.Errorf("create_outline_node: parent must be a container node")
		}
		if existing, ok, err := s.findMatchingOutlineNode(ctx, projectID, parentID, kind, op.Label); err != nil {
			return err
		} else if ok {
			n = existing
		} else {
			n, err = s.nodes.CreateChild(ctx, *parentID, kind, op.Label, op.Title, now())
			if err != nil {
				return err
			}
		}
	} else {
		hasAfter := strings.TrimSpace(op.AfterNodeID) != "" || strings.TrimSpace(op.AfterNodeRef) != "" || strings.TrimSpace(*nodeInsertCursor) != ""
		if hasAfter {
			afterNodeID, err := s.resolveSceneAnchor(ctx, projectID, currentNodeID, op.AfterNodeID, op.AfterNodeRef, nodeRefs, *nodeInsertCursor)
			if err != nil {
				return err
			}
			after, err := s.nodes.Get(ctx, afterNodeID)
			if err != nil {
				return err
			}
			if existing, ok, err := s.findMatchingOutlineNode(ctx, projectID, after.ParentID, kind, op.Label); err != nil {
				return err
			} else if ok {
				n = existing
			} else {
				n, err = s.nodes.CreateSibling(ctx, afterNodeID, kind, op.Label, op.Title, now())
				if err != nil {
					return err
				}
			}
		} else {
			if existing, ok, err := s.findMatchingOutlineNode(ctx, projectID, nil, kind, op.Label); err != nil {
				return err
			} else if ok {
				n = existing
			} else {
				n, err = s.nodes.CreateRoot(ctx, projectID, kind, op.Label, op.Title, now())
				if err != nil {
					return err
				}
			}
		}
	}
	if strings.TrimSpace(op.Title) != "" && strings.TrimSpace(n.Title) == "" {
		if err := s.nodes.Rename(ctx, n.ID, n.Label, op.Title, now()); err != nil {
			return err
		}
		n, err = s.nodes.Get(ctx, n.ID)
		if err != nil {
			return err
		}
	}
	if op.Ref != "" {
		nodeRefs[op.Ref] = n.ID
		created["node:"+op.Ref] = n.ID
	}
	if parentID == nil {
		*nodeInsertCursor = n.ID
	}
	return nil
}

func (s *Service) findMatchingOutlineNode(ctx context.Context, projectID string, parentID *string, kind, label string) (node.Node, bool, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return node.Node{}, false, nil
	}
	all, err := s.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return node.Node{}, false, err
	}
	for _, n := range all {
		if n.Kind != kind || strings.TrimSpace(n.Label) != label {
			continue
		}
		if parentID == nil {
			if n.ParentID == nil {
				return n, true, nil
			}
			continue
		}
		if n.ParentID != nil && *n.ParentID == *parentID {
			return n, true, nil
		}
	}
	return node.Node{}, false, nil
}

// resolveEntityID resolves a create_relationship endpoint to a real entity id.
// It tolerates the model's common mistakes: an entity may be referenced by a
// proposal ref (in from_ref or mistakenly in from), by a real entity id, or by
// name (case-insensitive). Returns a clear error instead of letting an
// unresolved value hit a FOREIGN KEY constraint at insert time.
func (s *Service) resolveEntityID(ctx context.Context, projectID, id, ref string, refs map[string]string, label string) (string, error) {
	// ref and id are treated interchangeably: the model conflates them (a real
	// entity id or name often lands in from_ref, and vice versa). Each is tried
	// as a declared proposal ref, then a real entity id, then a name.
	for _, candidate := range []string{strings.TrimSpace(ref), strings.TrimSpace(id)} {
		if candidate == "" {
			continue
		}
		if resolved, ok := refs[candidate]; ok {
			return resolved, nil
		}
		if _, err := s.entities.Get(ctx, candidate); err == nil {
			return candidate, nil
		}
		if matches, err := s.entities.Search(ctx, projectID, candidate, 20); err == nil {
			for _, e := range matches {
				if strings.EqualFold(strings.TrimSpace(e.Name), candidate) {
					return e.ID, nil
				}
			}
		}
	}
	return "", fmt.Errorf("%s could not be resolved to an entity (id, name, or ref)", label)
}

// resolveThreadID resolves an add_beat thread endpoint, tolerating a real thread
// id (or name) placed in thread_ref and vice versa.
func (s *Service) resolveThreadID(ctx context.Context, projectID, id, ref string, refs map[string]string) (string, error) {
	for _, candidate := range []string{strings.TrimSpace(ref), strings.TrimSpace(id)} {
		if candidate == "" {
			continue
		}
		if resolved, ok := refs[candidate]; ok {
			return resolved, nil
		}
		if _, err := s.threads.Get(ctx, candidate); err == nil {
			return candidate, nil
		}
		if list, err := s.threads.ListByProject(ctx, projectID, true); err == nil {
			for _, th := range list {
				if strings.EqualFold(strings.TrimSpace(th.Name), candidate) {
					return th.ID, nil
				}
			}
		}
	}
	return "", fmt.Errorf("thread could not be resolved (id, name, or ref)")
}

// resolveOptionalNodeID resolves an optional scene/node endpoint. A real node id
// placed in node_ref (or a declared scene ref) both resolve; absent both, it
// falls back to the current node.
func (s *Service) resolveOptionalNodeID(ctx context.Context, id, ref, currentNodeID string, refs map[string]string) (*string, error) {
	provided := strings.TrimSpace(ref)
	if provided == "" {
		provided = strings.TrimSpace(id)
	}
	if provided != "" {
		if resolved, ok := refs[provided]; ok {
			return &resolved, nil
		}
		if _, err := s.nodes.Get(ctx, provided); err == nil {
			p := provided
			return &p, nil
		}
		return nil, fmt.Errorf("scene ref/id %q not found", provided)
	}
	if strings.TrimSpace(currentNodeID) != "" {
		return &currentNodeID, nil
	}
	return nil, nil
}

func (s *Service) resolveRequiredNodeID(ctx context.Context, id, ref string, refs map[string]string) (string, error) {
	resolved, err := s.resolveOptionalNodeID(ctx, id, ref, "", refs)
	if err != nil {
		return "", err
	}
	if resolved == nil || strings.TrimSpace(*resolved) == "" {
		return "", fmt.Errorf("outline node id/ref required")
	}
	return *resolved, nil
}

func (s *Service) resolveSceneAnchor(ctx context.Context, projectID, currentNodeID, afterNodeID, afterNodeRef string, refs map[string]string, fallbackAfterNodeID string) (string, error) {
	provided := strings.TrimSpace(afterNodeRef)
	if provided == "" {
		provided = strings.TrimSpace(afterNodeID)
	}
	if provided != "" {
		if resolved, ok := refs[provided]; ok {
			return resolved, nil
		}
		if _, err := s.nodes.Get(ctx, provided); err == nil {
			return provided, nil
		}
		return "", fmt.Errorf("scene anchor ref/id %q not found", provided)
	}
	if strings.TrimSpace(fallbackAfterNodeID) != "" {
		return fallbackAfterNodeID, nil
	}
	if strings.TrimSpace(currentNodeID) != "" {
		return currentNodeID, nil
	}
	proj, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return "", err
	}
	if proj.LastOpenedNodeID == nil || strings.TrimSpace(*proj.LastOpenedNodeID) == "" {
		return "", fmt.Errorf("create_scene: after_node_id required")
	}
	return *proj.LastOpenedNodeID, nil
}

func cleanEntityAttributes(attrs map[string]string) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range attrs {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func optionalEntityAttributes(attrs map[string]string) *map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	return &attrs
}

func mergeEntityAttributes(base, next map[string]string) map[string]string {
	merged := map[string]string{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range next {
		merged[key] = value
	}
	return merged
}

type tiptapDoc struct {
	Type    string        `json:"type"`
	Content []tiptapBlock `json:"content"`
}

type tiptapBlock struct {
	Type    string         `json:"type"`
	Content []tiptapInline `json:"content,omitempty"`
}

type tiptapInline struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

var paragraphBreakRE = regexp.MustCompile(`\n{2,}`)

func normalizeSceneTextForVerify(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = paragraphBreakRE.ReplaceAllString(normalized, "\n\n")
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// PlainTextToTiptapDoc converts plain scene text (paragraphs separated by
// blank lines, hard breaks inside paragraphs) into the Tiptap document JSON
// the editor stores.
func PlainTextToTiptapDoc(text string) (string, error) {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	blocks := paragraphBreakRE.Split(normalized, -1)
	paragraphs := make([]tiptapBlock, 0, len(blocks))
	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		content := make([]tiptapInline, 0, len(lines)*2)
		for i, line := range lines {
			if i > 0 {
				content = append(content, tiptapInline{Type: "hardBreak"})
			}
			if line != "" {
				content = append(content, tiptapInline{Type: "text", Text: line})
			}
		}
		paragraph := tiptapBlock{Type: "paragraph"}
		if len(content) > 0 {
			paragraph.Content = content
		}
		paragraphs = append(paragraphs, paragraph)
	}
	if len(paragraphs) == 0 {
		paragraphs = append(paragraphs, tiptapBlock{Type: "paragraph"})
	}
	raw, err := json.Marshal(tiptapDoc{Type: "doc", Content: paragraphs})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// plainTextFromDoc and trimRunes are duplicated from companion/query.go; the
// companion copies die with that package in the pivot's removal phase.
func plainTextFromDoc(raw *string) string {
	if raw == nil || *raw == "" {
		return ""
	}
	var v interface{}
	if err := json.Unmarshal([]byte(*raw), &v); err != nil {
		return ""
	}
	var sb strings.Builder
	var walk func(x interface{})
	walk = func(x interface{}) {
		switch t := x.(type) {
		case map[string]interface{}:
			if t["type"] == "mention" {
				if attrs, ok := t["attrs"].(map[string]interface{}); ok {
					if label, ok := attrs["label"].(string); ok {
						sb.WriteString(label)
					}
				}
				return
			}
			if t["type"] == "text" {
				if s, ok := t["text"].(string); ok {
					sb.WriteString(s)
				}
			}
			if t["type"] == "hardBreak" {
				sb.WriteString("\n")
			}
			if c, ok := t["content"].([]interface{}); ok {
				for _, ch := range c {
					walk(ch)
				}
			}
			if k, _ := t["type"].(string); k == "paragraph" || k == "heading" {
				sb.WriteString("\n\n")
			}
		case []interface{}:
			for _, ch := range t {
				walk(ch)
			}
		}
	}
	walk(v)
	return strings.TrimSpace(sb.String())
}

func trimRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
