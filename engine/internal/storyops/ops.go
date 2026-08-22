// Package storyops owns the structured story-mutation vocabulary and its
// applier: validated op batches over outline nodes, scenes, threads, beats,
// entities, relationships, facts, and memories, with all-or-nothing rollback
// and a one-step undo. It performs no LLM calls and must not import LLM
// client or agent-loop code.
//
// Extracted from internal/companion as part of the MCP-first pivot (#47):
// the companion delegates here today, and the MCP write tools build on the
// same applier so every external mutation shares one snapshot/undo path.
package storyops

import (
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/fact"
)

// Op is one proposed plot-core mutation. Only fields relevant to Type are set.
type Op struct {
	Type string `json:"op"`

	// create_thread
	Ref     string `json:"ref,omitempty"`
	Name    string `json:"name,omitempty"`
	Color   string `json:"color,omitempty"`
	Summary string `json:"summary,omitempty"`

	// update_thread / add_beat target
	ThreadID  string `json:"thread_id,omitempty"`
	ThreadRef string `json:"thread_ref,omitempty"`

	// add_beat / update_beat
	NodeID      string `json:"node_id,omitempty"`
	BeatID      string `json:"beat_id,omitempty"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Intensity   int    `json:"intensity,omitempty"`

	// set_outline
	Outline string `json:"outline,omitempty"`

	// remember
	Text       string `json:"text,omitempty"`
	AllowEmpty bool   `json:"allow_empty,omitempty"`
	Category   string `json:"category,omitempty"`

	// create_entity / update_entity
	Kind       string            `json:"kind,omitempty"`
	Role       string            `json:"role,omitempty"`
	EntityID   string            `json:"entity_id,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`

	// create_scene
	AfterNodeID   string `json:"after_node_id,omitempty"`
	AfterNodeRef  string `json:"after_node_ref,omitempty"`
	Title         string `json:"title,omitempty"`
	NodeRef       string `json:"node_ref,omitempty"`
	ParentNodeID  string `json:"parent_node_id,omitempty"`
	ParentNodeRef string `json:"parent_node_ref,omitempty"`
	Direction     string `json:"direction,omitempty"`

	// create_relationship
	From         string `json:"from,omitempty"`
	FromRef      string `json:"from_ref,omitempty"`
	To           string `json:"to,omitempty"`
	ToRef        string `json:"to_ref,omitempty"`
	Notes        string `json:"notes,omitempty"`
	InverseLabel string `json:"inverse_label,omitempty"`

	// create_fact_card
	Claim   string             `json:"claim,omitempty"`
	Result  string             `json:"result,omitempty"`
	Status  string             `json:"status,omitempty"`
	Sources []fact.SourceInput `json:"sources,omitempty"`
}

// Proposal is a validated batch of ops with a human-readable summary.
type Proposal struct {
	Summary string `json:"summary"`
	Ops     []Op   `json:"ops"`
}

// knownOps lists the accepted op types.
var knownOps = map[string]bool{
	"create_thread": true, "update_thread": true,
	"add_beat": true, "update_beat": true, "delete_beat": true,
	"set_outline":    true,
	"set_scene_text": true,
	"remember":       true,
	"create_entity":  true, "update_entity": true, "create_relationship": true,
	"create_scene":        true,
	"create_outline_node": true,
	"rename_outline_node": true,
	"delete_outline_node": true,
	"move_outline_node":   true,
	"create_fact_card":    true,
}

// normalizeEntityKind maps a raw create_entity kind to one of the canonical
// values (character|place|item|concept). It is lenient because the model does
// not always emit the exact token: an empty kind defaults to "character" (the
// dominant entity type), and common English/Korean synonyms are accepted.
// Returns (canonical, true) on success, or ("", false) for an unknown value.
func normalizeEntityKind(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "character", "char", "person", "people", "인물", "캐릭터", "등장인물":
		return "character", true
	case "place", "location", "장소", "공간", "위치":
		return "place", true
	case "item", "object", "thing", "사물", "아이템", "물건":
		return "item", true
	case "concept", "idea", "theme", "skill", "magic", "ability",
		"spell", "rule", "system", "개념", "주제", "스킬", "마법", "능력",
		"주문", "규칙", "세계관":
		return "concept", true
	default:
		return "", false
	}
}

func normalizeOutlineNodeKind(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "leaf", "scene", "씬", "장면":
		return "leaf", true
	case "container", "chapter", "part", "장", "챕터", "부", "파트", "막":
		return "container", true
	default:
		return "", false
	}
}

// ValidateProposal checks every op for required fields and normalizes lenient
// values (entity kinds, node kinds, move directions, fact statuses) in place.
func ValidateProposal(p Proposal) error {
	if len(p.Ops) == 0 {
		return fmt.Errorf("proposal has no ops")
	}
	for i, op := range p.Ops {
		if !knownOps[op.Type] {
			return fmt.Errorf("op[%d]: unknown op %q", i, op.Type)
		}
		switch op.Type {
		case "create_thread":
			if strings.TrimSpace(op.Name) == "" {
				return fmt.Errorf("op[%d] create_thread: name required", i)
			}
		case "update_thread":
			if op.ThreadID == "" {
				return fmt.Errorf("op[%d] update_thread: thread_id required", i)
			}
		case "add_beat":
			hasID := op.ThreadID != ""
			hasRef := op.ThreadRef != ""
			if hasID == hasRef {
				return fmt.Errorf("op[%d] add_beat: exactly one of thread_id/thread_ref required", i)
			}
			// An undeclared thread_ref is allowed here: the model often places a
			// real thread id in thread_ref. Resolution (declared ref → real id →
			// name) and any clear error happen at apply time.
			if strings.TrimSpace(op.Label) == "" {
				return fmt.Errorf("op[%d] add_beat: label required", i)
			}
			if op.NodeID != "" && op.NodeRef != "" {
				return fmt.Errorf("op[%d] add_beat: node_id and node_ref are mutually exclusive", i)
			}
		case "create_scene":
			if strings.TrimSpace(op.Label) == "" {
				return fmt.Errorf("op[%d] create_scene: label required", i)
			}
			if op.AfterNodeID != "" && op.AfterNodeRef != "" {
				return fmt.Errorf("op[%d] create_scene: after_node_id and after_node_ref are mutually exclusive", i)
			}
		case "create_outline_node":
			if strings.TrimSpace(op.Label) == "" {
				return fmt.Errorf("op[%d] create_outline_node: label required", i)
			}
			kind, ok := normalizeOutlineNodeKind(op.Kind)
			if !ok {
				return fmt.Errorf("op[%d] create_outline_node: kind must be container|leaf", i)
			}
			p.Ops[i].Kind = kind
			if op.ParentNodeID != "" && op.ParentNodeRef != "" {
				return fmt.Errorf("op[%d] create_outline_node: parent_node_id and parent_node_ref are mutually exclusive", i)
			}
			if op.AfterNodeID != "" && op.AfterNodeRef != "" {
				return fmt.Errorf("op[%d] create_outline_node: after_node_id and after_node_ref are mutually exclusive", i)
			}
			hasParent := op.ParentNodeID != "" || op.ParentNodeRef != ""
			hasAfter := op.AfterNodeID != "" || op.AfterNodeRef != ""
			if hasParent && hasAfter {
				return fmt.Errorf("op[%d] create_outline_node: parent_node_* and after_node_* are mutually exclusive", i)
			}
		case "rename_outline_node":
			if op.NodeID != "" && op.NodeRef != "" {
				return fmt.Errorf("op[%d] rename_outline_node: node_id and node_ref are mutually exclusive", i)
			}
			if op.NodeID == "" && op.NodeRef == "" {
				return fmt.Errorf("op[%d] rename_outline_node: node_id or node_ref required", i)
			}
			if strings.TrimSpace(op.Label) == "" && strings.TrimSpace(op.Title) == "" {
				return fmt.Errorf("op[%d] rename_outline_node: label or title required", i)
			}
		case "delete_outline_node":
			if op.NodeID != "" && op.NodeRef != "" {
				return fmt.Errorf("op[%d] delete_outline_node: node_id and node_ref are mutually exclusive", i)
			}
			if op.NodeID == "" && op.NodeRef == "" {
				return fmt.Errorf("op[%d] delete_outline_node: node_id or node_ref required", i)
			}
		case "move_outline_node":
			if op.NodeID != "" && op.NodeRef != "" {
				return fmt.Errorf("op[%d] move_outline_node: node_id and node_ref are mutually exclusive", i)
			}
			if op.NodeID == "" && op.NodeRef == "" {
				return fmt.Errorf("op[%d] move_outline_node: node_id or node_ref required", i)
			}
			direction := strings.ToLower(strings.TrimSpace(op.Direction))
			if direction != "up" && direction != "down" {
				return fmt.Errorf("op[%d] move_outline_node: direction must be up|down", i)
			}
			p.Ops[i].Direction = direction
		case "update_beat":
			if op.BeatID == "" {
				return fmt.Errorf("op[%d] update_beat: beat_id required", i)
			}
		case "delete_beat":
			if op.BeatID == "" {
				return fmt.Errorf("op[%d] delete_beat: beat_id required", i)
			}
		case "set_outline":
			// outline may be empty (clears); no required field
		case "set_scene_text":
			if op.NodeID != "" && op.NodeRef != "" {
				return fmt.Errorf("op[%d] set_scene_text: node_id and node_ref are mutually exclusive", i)
			}
			if strings.TrimSpace(op.Text) == "" && !op.AllowEmpty {
				return fmt.Errorf("op[%d] set_scene_text: text required unless allow_empty is true", i)
			}
		case "remember":
			if strings.TrimSpace(op.Text) == "" {
				return fmt.Errorf("op[%d] remember: text required", i)
			}
		case "create_entity":
			if strings.TrimSpace(op.Name) == "" {
				return fmt.Errorf("op[%d] create_entity: name required", i)
			}
			kind, ok := normalizeEntityKind(op.Kind)
			if !ok {
				return fmt.Errorf("op[%d] create_entity: kind must be character|place|item|concept", i)
			}
			p.Ops[i].Kind = kind
		case "update_entity":
			if op.EntityID == "" {
				return fmt.Errorf("op[%d] update_entity: entity_id required", i)
			}
			if strings.TrimSpace(op.Kind) != "" {
				kind, ok := normalizeEntityKind(op.Kind)
				if !ok {
					return fmt.Errorf("op[%d] update_entity: kind must be character|place|item|concept", i)
				}
				p.Ops[i].Kind = kind
			}
		case "create_relationship":
			if strings.TrimSpace(op.Label) == "" {
				return fmt.Errorf("op[%d] create_relationship: label required", i)
			}
			hasFrom, hasFromRef := op.From != "", op.FromRef != ""
			hasTo, hasToRef := op.To != "", op.ToRef != ""
			if hasFrom == hasFromRef {
				return fmt.Errorf("op[%d] create_relationship: exactly one of from/from_ref required", i)
			}
			if hasTo == hasToRef {
				return fmt.Errorf("op[%d] create_relationship: exactly one of to/to_ref required", i)
			}
			// Undeclared from_ref/to_ref are allowed: the model often places a
			// real entity id (or name) in the ref field. Resolution and any
			// clear error happen at apply time.
		case "create_fact_card":
			if strings.TrimSpace(op.Claim) == "" {
				return fmt.Errorf("op[%d] create_fact_card: claim required", i)
			}
			if strings.TrimSpace(op.Result) == "" {
				return fmt.Errorf("op[%d] create_fact_card: result required", i)
			}
			status := strings.TrimSpace(op.Status)
			if status == "" {
				status = fact.StatusUncertain
				p.Ops[i].Status = status
			}
			if !fact.ValidStatus(status) {
				return fmt.Errorf("op[%d] create_fact_card: status must be verified|uncertain|intentional_fiction|stale", i)
			}
			hasSource := false
			for _, src := range op.Sources {
				if strings.TrimSpace(src.URL) != "" {
					hasSource = true
					break
				}
			}
			if !hasSource {
				return fmt.Errorf("op[%d] create_fact_card: at least one source URL required", i)
			}
			if op.NodeID != "" && op.NodeRef != "" {
				return fmt.Errorf("op[%d] create_fact_card: node_id and node_ref are mutually exclusive", i)
			}
		}
	}
	return nil
}
