package companion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	tarstools "github.com/devlikebear/tars/pkg/tools"
)

type webToolSource interface {
	WebSearchProvider() string
	WebSearchAPIKey() string
}

type ApplyOpsResult struct {
	Summary      string              `json:"summary,omitempty"`
	Applied      int                 `json:"applied"`
	Created      map[string]string   `json:"created,omitempty"`
	ChangedNodes []AppliedNodeChange `json:"changed_nodes,omitempty"`
	Failures     []ApplyOpsFailure   `json:"failures,omitempty"`
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

func (r ApplyOpsResult) isError() bool {
	return len(r.Failures) > 0
}

func (s *Service) buildToolRegistry(projectID, nodeID string, now func() int64, runIDAndUserText ...string) *tarstools.Registry {
	userText := ""
	if len(runIDAndUserText) > 1 {
		userText = runIDAndUserText[1]
	}
	return s.buildToolRegistryWithIntent(projectID, nodeID, turnHistoryScope("", nodeID), now, classifyCompanionIntent(userText), runIDAndUserText...)
}

func (s *Service) buildToolRegistryWithIntent(projectID, nodeID, scope string, now func() int64, intent companionIntent, runIDAndUserText ...string) *tarstools.Registry {
	reg := tarstools.NewRegistryWithScope(tarstools.RegistryScopeUser)
	reg.Register(tarstools.NewWebFetchTool(true))
	reg.Register(s.buildWebSearchTool())
	activeRunID := ""
	if len(runIDAndUserText) > 0 {
		activeRunID = runIDAndUserText[0]
	}
	userText := ""
	if len(runIDAndUserText) > 1 {
		userText = runIDAndUserText[1]
	}
	// Read-only turns (diagnosis, review, critique) never get the mutation
	// tool at all, so the model cannot silently persist anything. Anything
	// worth keeping is offered as a linetta-proposal block instead.
	if !intent.IsReadOnly() {
		reg.Register(s.buildApplyOpsTool(projectID, nodeID, scope, activeRunID, userText, intent, now))
	}
	return reg
}

func (s *Service) buildWebSearchTool() tarstools.Tool {
	provider := "brave"
	apiKey := ""
	if src, ok := s.src.(webToolSource); ok {
		if v := strings.TrimSpace(src.WebSearchProvider()); v != "" {
			provider = strings.ToLower(v)
		}
		apiKey = strings.TrimSpace(src.WebSearchAPIKey())
	}
	opts := tarstools.WebSearchOptions{
		Enabled:  true,
		Provider: provider,
	}
	switch provider {
	case "perplexity":
		opts.PerplexityAPIKey = apiKey
	default:
		opts.BraveAPIKey = apiKey
	}
	return tarstools.NewWebSearchToolWithOptions(opts)
}

func (s *Service) buildApplyOpsTool(projectID, nodeID, scope, runID, userText string, intent companionIntent, now func() int64) tarstools.Tool {
	return tarstools.Tool{
		Name:        "linetta_apply_ops",
		Description: "Directly apply Linetta story mutations to the current project. Use set_scene_text to rewrite the current scene body, create_outline_node/create_scene for new left outline tree items, thread/beat ops for plot beats, entity/relationship ops for characters, places, items, skills, magic, abilities, and create_fact_card for source-backed Fact Book cards.",
		Parameters:  applyOpsSchema(),
		Execute: func(ctx context.Context, params json.RawMessage) (tarstools.Result, error) {
			p, err := decodeApplyOpsParams(params)
			if err != nil {
				result := ApplyOpsResult{Failures: []ApplyOpsFailure{{Index: -1, Error: "invalid JSON: " + err.Error()}}}
				return tarstools.JSONTextResult(result, true), nil
			}
			if err := validateApplyOpsIntent(p, companionApplyOpsIntent(userText, nodeID, intent)); err != nil {
				result := ApplyOpsResult{
					Summary:  strings.TrimSpace(p.Summary),
					Failures: []ApplyOpsFailure{{Index: -1, Error: err.Error()}},
				}
				return tarstools.JSONTextResult(result, true), nil
			}
			result := s.ApplyOps(ctx, projectID, nodeID, p, now)
			if runID != "" && result.Applied > 0 && s.notify != nil {
				_ = s.notify.Notify("companion.applied", appliedPayload{
					RunID:        runID,
					ProjectID:    projectID,
					NodeID:       nodeID,
					Scope:        scope,
					Intent:       string(intent.Kind),
					Summary:      result.Summary,
					Applied:      result.Applied,
					ChangedNodes: result.ChangedNodes,
				})
			}
			return tarstools.JSONTextResult(result, result.isError()), nil
		},
	}
}

func applyOpsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "properties":{
    "summary":{"type":"string","description":"Short Korean summary of the actual project changes to apply."},
    "ops_json":{"type":"string","description":"JSON array string of Linetta mutation objects to apply now. Use set_scene_text{text,node_id?,node_ref?,allow_empty?} to replace the actual body of the current or specified scene. Use create_outline_node/create_scene only for new visible left outline items; use rename_outline_node/delete_outline_node/move_outline_node for existing outline cleanup. Use set_outline only for project synopsis/overview text, create_thread/add_beat for storylines and beats, entity/relationship ops for characters, places, items, skills, magic, abilities, and create_fact_card only when at least one source URL is available. create_entity/update_entity may include attributes for effect, cost, trigger, limits, owner, origin, or weakness."}
  },
  "required":["summary","ops_json"],
  "additionalProperties":false
}`)
}

type applyOpsParams struct {
	Summary string `json:"summary"`
	OpsJSON string `json:"ops_json"`
	Ops     []Op   `json:"ops,omitempty"`
}

func decodeApplyOpsParams(params json.RawMessage) (Proposal, error) {
	var in applyOpsParams
	if err := unmarshalApplyOpsJSONValue(params, &in); err != nil {
		return Proposal{}, err
	}
	ops := in.Ops
	if strings.TrimSpace(in.OpsJSON) != "" {
		if err := unmarshalApplyOpsJSONValue([]byte(in.OpsJSON), &ops); err != nil {
			return Proposal{}, fmt.Errorf("invalid ops_json: %w", err)
		}
	}
	return Proposal{Summary: in.Summary, Ops: ops}, nil
}

func unmarshalApplyOpsJSONValue(data []byte, dst any) error {
	err := json.Unmarshal(data, dst)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "after top-level value") {
		return err
	}
	reader := bytes.NewReader(data)
	dec := json.NewDecoder(reader)
	if fallbackErr := dec.Decode(dst); fallbackErr != nil {
		return err
	}
	if trailing := strings.TrimSpace(remainingDecoderBytes(dec, reader)); trailing != "" && !isExtraJSONCloseDelimiterTrail(trailing) {
		return err
	}
	return nil
}

func remainingDecoderBytes(dec *json.Decoder, reader *bytes.Reader) string {
	var out []byte
	if buffered := dec.Buffered(); buffered != nil {
		chunk, _ := io.ReadAll(buffered)
		out = append(out, chunk...)
	}
	if reader.Len() > 0 {
		chunk, _ := io.ReadAll(reader)
		out = append(out, chunk...)
	}
	return string(out)
}

func isExtraJSONCloseDelimiterTrail(s string) bool {
	for _, r := range s {
		if r != '}' && r != ']' {
			return false
		}
	}
	return true
}

type applyOpsIntent struct {
	ReadOnly            bool
	RequireOutlineTree  bool
	RequireSceneText    bool
	AllowEmptySceneText bool
	TargetNodeID        string
}

func companionApplyOpsIntent(text, currentNodeID string, intent companionIntent) applyOpsIntent {
	if intent.Kind == "" {
		intent = classifyCompanionIntent(text)
	}
	if intent.IsReadOnly() {
		return applyOpsIntent{ReadOnly: true}
	}
	if intent.Kind == companionIntentChat {
		return applyOpsIntent{}
	}
	s := strings.ToLower(strings.TrimSpace(text))
	return applyOpsIntent{
		RequireOutlineTree:  !intent.RequiresSceneText() && containsAny(s, companionOutlineTreeTerms) && containsAny(s, companionMutationTerms),
		RequireSceneText:    intent.RequiresSceneText(),
		AllowEmptySceneText: intent.AllowEmptySceneText,
		TargetNodeID:        firstNonEmpty(intent.TargetNodeID, currentNodeID),
	}
}

var companionOutlineTreeTerms = []string{
	"아웃라인", "목차", "챕터", "회차", "세부 씬", "몇 편", "1부", "2부",
	"3부", "4부", "파트", "얼개", "막 구성", "부 구성", "부별", "장별",
}

func validateApplyOpsIntent(p Proposal, intent applyOpsIntent) error {
	// Second line of defence for read-only turns: the tool is not registered
	// for them, but the proposal fallback and any other caller go through here.
	if intent.ReadOnly && len(p.Ops) > 0 {
		return fmt.Errorf("read-only requests (diagnosis/review/critique) must not change the project; report the findings and offer a linetta-proposal block the writer can apply")
	}
	if intent.RequireSceneText {
		foundSceneText := false
		for _, op := range p.Ops {
			if op.Type != "set_scene_text" {
				continue
			}
			foundSceneText = true
			if op.AllowEmpty && !intent.AllowEmptySceneText {
				return fmt.Errorf("scene text requests must not use empty set_scene_text unless the user explicitly asked to clear the scene")
			}
			if strings.TrimSpace(op.Text) == "" && !intent.AllowEmptySceneText {
				return fmt.Errorf("scene text requests must write non-empty text with set_scene_text")
			}
			if intent.TargetNodeID != "" && op.NodeID != "" && op.NodeID != intent.TargetNodeID {
				return fmt.Errorf("set_scene_text target must match the requested current scene")
			}
		}
		if !foundSceneText {
			return fmt.Errorf("scene body writing/editing requests must apply set_scene_text; remember/create_thread/add_beat/set_outline do not change the manuscript body")
		}
	}
	if intent.RequireOutlineTree {
		for _, op := range p.Ops {
			if op.Type == "create_outline_node" || op.Type == "create_scene" ||
				op.Type == "rename_outline_node" || op.Type == "delete_outline_node" || op.Type == "move_outline_node" {
				return nil
			}
		}
		return fmt.Errorf("outline requests must update the visible left outline tree with create_outline_node or create_scene; create_thread/add_beat only update plot beats")
	}
	return nil
}

// ApplyOps applies a validated proposal op list directly to project state.
func (s *Service) ApplyOps(ctx context.Context, projectID, nodeID string, p Proposal, now func() int64) ApplyOpsResult {
	result := ApplyOpsResult{
		Summary: strings.TrimSpace(p.Summary),
		Created: map[string]string{},
	}
	if err := validateProposal(p); err != nil {
		result.Failures = append(result.Failures, ApplyOpsFailure{Index: -1, Error: err.Error()})
		return result
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
		doc, err := plainTextToTiptapDoc(op.Text)
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
			TextPreview:    trimRunesLocal(strings.TrimSpace(plainTextFromDoc(after.ContentDoc)), 120),
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
		return s.Remember(projectID, op.Text, op.Category)
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

func plainTextToTiptapDoc(text string) (string, error) {
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
			}
		} else {
			if existing, ok, err := s.findMatchingOutlineNode(ctx, projectID, nil, kind, op.Label); err != nil {
				return err
			} else if ok {
				n = existing
			} else {
				n, err = s.nodes.CreateRoot(ctx, projectID, kind, op.Label, op.Title, now())
			}
		}
	}
	if err != nil {
		return err
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
