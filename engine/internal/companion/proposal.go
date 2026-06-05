// Package companion implements the conversational writing-companion backend:
// session persistence (tars pkg/session), context-injected streaming, and
// parsing of structured plot-edit proposals embedded in model output.
package companion

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/fact"
)

// proposalFence is the fenced-block language tag the model must use to emit a
// structured plot-edit proposal.
const proposalFence = "linetta-proposal"

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
	Text     string `json:"text,omitempty"`
	Category string `json:"category,omitempty"`

	// create_entity / update_entity
	Kind     string `json:"kind,omitempty"`
	Role     string `json:"role,omitempty"`
	EntityID string `json:"entity_id,omitempty"`

	// create_scene
	AfterNodeID   string `json:"after_node_id,omitempty"`
	AfterNodeRef  string `json:"after_node_ref,omitempty"`
	Title         string `json:"title,omitempty"`
	NodeRef       string `json:"node_ref,omitempty"`
	ParentNodeID  string `json:"parent_node_id,omitempty"`
	ParentNodeRef string `json:"parent_node_ref,omitempty"`

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

// Proposal is the parsed contents of a linetta-proposal block.
type Proposal struct {
	Summary string `json:"summary"`
	Ops     []Op   `json:"ops"`
}

// knownOps lists the plot-core op types accepted in Phase 1.
var knownOps = map[string]bool{
	"create_thread": true, "update_thread": true,
	"add_beat": true, "update_beat": true, "delete_beat": true,
	"set_outline":   true,
	"remember":      true,
	"create_entity": true, "update_entity": true, "create_relationship": true,
	"create_scene":        true,
	"create_outline_node": true,
	"create_fact_card":    true,
}

// ParseProposal scans full model output for a linetta-proposal fenced block.
// Returns (proposal, blockPresent, error):
//   - no block:        (Proposal{}, false, nil)
//   - one valid block: (parsed, true, nil)
//   - invalid/>=2:     (best-effort, true, err)
func ParseProposal(full string) (Proposal, bool, error) {
	blocks := extractFencedBlocks(full, proposalFence)
	if len(blocks) == 0 {
		return Proposal{}, false, nil
	}
	if len(blocks) > 1 {
		p, _ := decodeProposal(blocks[0])
		return p, true, fmt.Errorf("multiple linetta-proposal blocks (%d); only one allowed", len(blocks))
	}
	p, err := decodeProposal(blocks[0])
	if err != nil {
		return p, true, err
	}
	if err := validateProposal(p); err != nil {
		return p, true, err
	}
	return p, true, nil
}

func decodeProposal(body string) (Proposal, error) {
	var p Proposal
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &p); err != nil {
		return Proposal{}, fmt.Errorf("invalid proposal JSON: %w", err)
	}
	return p, nil
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
	case "concept", "idea", "theme", "개념", "주제":
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

func validateProposal(p Proposal) error {
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

// extractFencedBlocks returns the bodies of all ```<lang> ... ``` blocks whose
// info-string equals lang.
func extractFencedBlocks(s, lang string) []string {
	var out []string
	lines := strings.Split(s, "\n")
	inBlock := false
	var buf []string
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if !inBlock {
			if trimmed == "```"+lang {
				inBlock = true
				buf = nil
			}
			continue
		}
		if trimmed == "```" {
			out = append(out, strings.Join(buf, "\n"))
			inBlock = false
			continue
		}
		buf = append(buf, strings.TrimRight(ln, "\r"))
	}
	return out
}
