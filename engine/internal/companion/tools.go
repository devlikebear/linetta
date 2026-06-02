package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/thread"
	tarstools "github.com/devlikebear/tars/pkg/tools"
)

type webToolSource interface {
	WebSearchProvider() string
	WebSearchAPIKey() string
}

type ApplyOpsResult struct {
	Summary  string            `json:"summary,omitempty"`
	Applied  int               `json:"applied"`
	Created  map[string]string `json:"created,omitempty"`
	Failures []ApplyOpsFailure `json:"failures,omitempty"`
}

type ApplyOpsFailure struct {
	Index int    `json:"index"`
	Op    string `json:"op,omitempty"`
	Error string `json:"error"`
}

func (r ApplyOpsResult) isError() bool {
	return len(r.Failures) > 0
}

func (s *Service) buildToolRegistry(projectID, nodeID string, now func() int64, runID ...string) *tarstools.Registry {
	reg := tarstools.NewRegistryWithScope(tarstools.RegistryScopeUser)
	reg.Register(tarstools.NewWebFetchTool(true))
	reg.Register(s.buildWebSearchTool())
	activeRunID := ""
	if len(runID) > 0 {
		activeRunID = runID[0]
	}
	reg.Register(s.buildApplyOpsTool(projectID, nodeID, activeRunID, now))
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

func (s *Service) buildApplyOpsTool(projectID, nodeID, runID string, now func() int64) tarstools.Tool {
	return tarstools.Tool{
		Name:        "linetta_apply_ops",
		Description: "Apply Linetta story-structure mutations to the current project: outline, storylines, beats, entities, relationships, scenes, and memories.",
		Parameters:  applyOpsSchema(),
		Execute: func(ctx context.Context, params json.RawMessage) (tarstools.Result, error) {
			p, err := decodeApplyOpsParams(params)
			if err != nil {
				result := ApplyOpsResult{Failures: []ApplyOpsFailure{{Index: -1, Error: "invalid JSON: " + err.Error()}}}
				return tarstools.JSONTextResult(result, true), nil
			}
			result := s.ApplyOps(ctx, projectID, nodeID, p, now)
			if runID != "" && result.Applied > 0 && s.notify != nil {
				_ = s.notify.Notify("companion.applied", appliedPayload{
					RunID:   runID,
					Summary: result.Summary,
					Applied: result.Applied,
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
    "summary":{"type":"string","description":"Short Korean summary of the changes."},
    "ops_json":{"type":"string","description":"JSON array string of Linetta mutation objects. Each object has an op such as create_thread, update_thread, add_beat, update_beat, delete_beat, set_outline, remember, create_entity, update_entity, create_relationship, or create_scene."}
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
	if err := json.Unmarshal(params, &in); err != nil {
		return Proposal{}, err
	}
	ops := in.Ops
	if strings.TrimSpace(in.OpsJSON) != "" {
		if err := json.Unmarshal([]byte(in.OpsJSON), &ops); err != nil {
			return Proposal{}, fmt.Errorf("invalid ops_json: %w", err)
		}
	}
	return Proposal{Summary: in.Summary, Ops: ops}, nil
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

	for i, op := range p.Ops {
		if err := s.applyOneOp(ctx, projectID, nodeID, op, now, threadRefs, entityRefs, nodeRefs, result.Created); err != nil {
			result.Failures = append(result.Failures, ApplyOpsFailure{Index: i, Op: op.Type, Error: err.Error()})
			continue
		}
		result.Applied++
	}
	if len(result.Created) == 0 {
		result.Created = nil
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
) error {
	switch op.Type {
	case "set_outline":
		outline := op.Outline
		_, err := s.projects.Update(ctx, now(), project.UpdateInput{ID: projectID, Outline: &outline})
		return err
	case "create_thread":
		th, err := s.threads.Create(ctx, thread.NewInput{ProjectID: projectID, Name: op.Name, Color: op.Color})
		if err != nil {
			return err
		}
		if strings.TrimSpace(op.Summary) != "" {
			if err := s.threads.Update(ctx, thread.UpdateInput{ID: th.ID, Summary: op.Summary}); err != nil {
				return err
			}
		}
		if op.Ref != "" {
			threadRefs[op.Ref] = th.ID
			created["thread:"+op.Ref] = th.ID
		}
		return nil
	case "update_thread":
		in := thread.UpdateInput{ID: op.ThreadID, Name: op.Name, Color: op.Color, Summary: op.Summary}
		if strings.TrimSpace(op.Summary) == "" {
			cur, err := s.threads.Get(ctx, op.ThreadID)
			if err != nil {
				return err
			}
			in.Summary = cur.Summary
		}
		return s.threads.Update(ctx, in)
	case "add_beat":
		threadID, err := resolveID(op.ThreadID, op.ThreadRef, threadRefs, "thread")
		if err != nil {
			return err
		}
		beatNodeID, err := resolveOptionalNodeID(op.NodeID, op.NodeRef, currentNodeID, nodeRefs)
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
		if strings.TrimSpace(op.Summary) != "" {
			if err := s.entities.Update(ctx, now(), entity.UpdateInput{
				ID:      ent.ID,
				Kind:    ent.Kind,
				Name:    ent.Name,
				Role:    ent.Role,
				Summary: op.Summary,
			}); err != nil {
				return err
			}
		}
		if op.Ref != "" {
			entityRefs[op.Ref] = ent.ID
			created["entity:"+op.Ref] = ent.ID
		}
		return nil
	case "update_entity":
		cur, err := s.entities.Get(ctx, op.EntityID)
		if err != nil {
			return err
		}
		in := entity.UpdateInput{ID: op.EntityID, Kind: cur.Kind, Name: cur.Name, Role: cur.Role, Summary: cur.Summary}
		if strings.TrimSpace(op.Kind) != "" {
			in.Kind = op.Kind
		}
		if strings.TrimSpace(op.Name) != "" {
			in.Name = op.Name
		}
		if strings.TrimSpace(op.Role) != "" {
			in.Role = op.Role
		}
		if strings.TrimSpace(op.Summary) != "" {
			in.Summary = op.Summary
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
		afterNodeID, err := s.resolveSceneAnchor(ctx, projectID, currentNodeID, op.AfterNodeID)
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
		return nil
	default:
		return fmt.Errorf("unknown op %q", op.Type)
	}
}

// resolveEntityID resolves a create_relationship endpoint to a real entity id.
// It tolerates the model's common mistakes: an entity may be referenced by a
// proposal ref (in from_ref or mistakenly in from), by a real entity id, or by
// name (case-insensitive). Returns a clear error instead of letting an
// unresolved value hit a FOREIGN KEY constraint at insert time.
func (s *Service) resolveEntityID(ctx context.Context, projectID, id, ref string, refs map[string]string, label string) (string, error) {
	id = strings.TrimSpace(id)
	ref = strings.TrimSpace(ref)

	if ref != "" {
		if resolved := refs[ref]; resolved != "" {
			return resolved, nil
		}
		return "", fmt.Errorf("%s ref %q is not available yet", label, ref)
	}
	if id == "" {
		return "", fmt.Errorf("%s id or ref required", label)
	}
	// A proposal ref mistakenly placed in the id field.
	if resolved, ok := refs[id]; ok {
		return resolved, nil
	}
	// A real entity id.
	if _, err := s.entities.Get(ctx, id); err == nil {
		return id, nil
	}
	// An entity name (case-insensitive exact match within the project).
	if matches, err := s.entities.Search(ctx, projectID, id, 20); err == nil {
		for _, e := range matches {
			if strings.EqualFold(strings.TrimSpace(e.Name), id) {
				return e.ID, nil
			}
		}
	}
	return "", fmt.Errorf("%s %q not found (use an entity id, name, or from_ref)", label, id)
}

func resolveID(id, ref string, refs map[string]string, label string) (string, error) {
	if strings.TrimSpace(id) != "" {
		return id, nil
	}
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("%s id or ref required", label)
	}
	resolved := refs[ref]
	if resolved == "" {
		return "", fmt.Errorf("%s ref %q is not available yet", label, ref)
	}
	return resolved, nil
}

func resolveOptionalNodeID(id, ref, currentNodeID string, refs map[string]string) (*string, error) {
	if strings.TrimSpace(id) != "" {
		return &id, nil
	}
	if strings.TrimSpace(ref) != "" {
		resolved := refs[ref]
		if resolved == "" {
			return nil, fmt.Errorf("node ref %q is not available yet", ref)
		}
		return &resolved, nil
	}
	if strings.TrimSpace(currentNodeID) != "" {
		return &currentNodeID, nil
	}
	return nil, nil
}

func (s *Service) resolveSceneAnchor(ctx context.Context, projectID, currentNodeID, afterNodeID string) (string, error) {
	if strings.TrimSpace(afterNodeID) != "" {
		return afterNodeID, nil
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
