package companion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/storyops"
	tarstools "github.com/devlikebear/tars/pkg/tools"
)

type webToolSource interface {
	WebSearchProvider() string
	WebSearchAPIKey() string
}

// ApplyOps result types moved to internal/storyops with the applier.
type ApplyOpsResult = storyops.ApplyOpsResult

type ApplyOpsFailure = storyops.ApplyOpsFailure

type AppliedNodeChange = storyops.AppliedNodeChange

// ApplyOps applies a validated proposal op list directly to project state.
// The applier lives in internal/storyops; the companion delegates so chat
// applies, RPC applies, and (later) MCP applies share one path.
func (s *Service) ApplyOps(ctx context.Context, projectID, nodeID string, p Proposal, now func() int64) ApplyOpsResult {
	return s.story.ApplyOps(ctx, projectID, nodeID, p, now)
}

// plainTextToTiptapDoc kept as an alias for this package's tests; the
// implementation moved to storyops with set_scene_text.
var plainTextToTiptapDoc = storyops.PlainTextToTiptapDoc

func (s *Service) buildToolRegistry(projectID, nodeID string, now func() int64, runIDAndUserText ...string) *tarstools.Registry {
	userText := ""
	if len(runIDAndUserText) > 1 {
		userText = runIDAndUserText[1]
	}
	return s.buildToolRegistryWithIntent(projectID, nodeID, turnHistoryScope("", nodeID), now, classifyCompanionIntent(userText), "", runIDAndUserText...)
}

func (s *Service) buildToolRegistryWithIntent(projectID, nodeID, scope string, now func() int64, intent companionIntent, language string, runIDAndUserText ...string) *tarstools.Registry {
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
		reg.Register(s.buildApplyOpsTool(projectID, nodeID, scope, activeRunID, userText, intent, language, now))
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

func (s *Service) buildApplyOpsTool(projectID, nodeID, scope, runID, userText string, intent companionIntent, language string, now func() int64) tarstools.Tool {
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
			// A stop that arrives before the first write leaves the project
			// untouched; past this point the apply runs to completion so the work
			// is never left half-changed.
			if err := ctx.Err(); err != nil {
				result := ApplyOpsResult{
					Summary:  strings.TrimSpace(p.Summary),
					Failures: []ApplyOpsFailure{{Index: -1, Error: "the request was stopped before applying; nothing was changed"}},
				}
				return tarstools.JSONTextResult(result, true), nil
			}
			// A batch that reshapes the outline goes to the writer first: they see
			// the counts and the tree, then apply or discard it.
			if runID != "" && s.notify != nil && needsOutlineApproval(p) {
				preview := s.buildOutlinePreview(ctx, projectID, p)
				_ = s.notify.Notify("companion.preview", previewPayload{
					RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope,
					Intent: string(intent.Kind), Preview: preview,
				})
				return tarstools.JSONTextResult(ApplyOpsResult{
					Summary:         strings.TrimSpace(p.Summary),
					PendingApproval: true,
				}, false), nil
			}
			if runID != "" && s.notify != nil {
				_ = s.notify.Notify("companion.thinking", thinkingPayload{
					RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: string(intent.Kind),
					Text: applyingStatusText(language), Phase: phaseApplying, Total: len(p.Ops),
				})
			}
			result := s.ApplyOps(context.WithoutCancel(ctx), projectID, nodeID, p, now)
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
					UndoBatchID:  result.UndoBatchID,
				})
			}
			return tarstools.JSONTextResult(result, result.IsError()), nil
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
