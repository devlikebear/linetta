// Package companion implements the conversational writing-companion backend:
// session persistence (tars pkg/session), context-injected streaming, and
// parsing of structured plot-edit proposals embedded in model output.
package companion

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// Op and Proposal moved to internal/storyops in the MCP-first pivot (#47);
// the aliases keep the chat-parsing layer and RPC handlers unchanged until
// this package is removed.
type Op = storyops.Op

type Proposal = storyops.Proposal

// proposalFence is the fenced-block language tag the model must use to emit a
// structured plot-edit proposal.
const proposalFence = "linetta-proposal"

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
	if err := storyops.ValidateProposal(p); err != nil {
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
