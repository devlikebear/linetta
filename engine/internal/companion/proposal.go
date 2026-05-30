// Package companion implements the conversational writing-companion backend:
// session persistence (tars pkg/session), context-injected streaming, and
// parsing of structured plot-edit proposals embedded in model output.
package companion

import (
	"encoding/json"
	"fmt"
	"strings"
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
	"set_outline": true,
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

func validateProposal(p Proposal) error {
	if len(p.Ops) == 0 {
		return fmt.Errorf("proposal has no ops")
	}
	refs := map[string]bool{}
	for _, op := range p.Ops {
		if op.Type == "create_thread" && op.Ref != "" {
			refs[op.Ref] = true
		}
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
			if hasRef && !refs[op.ThreadRef] {
				return fmt.Errorf("op[%d] add_beat: thread_ref %q not declared by any create_thread.ref", i, op.ThreadRef)
			}
			if op.NodeID == "" || strings.TrimSpace(op.Label) == "" {
				return fmt.Errorf("op[%d] add_beat: node_id and label required", i)
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
		buf = append(buf, ln)
	}
	return out
}
