package companion

import (
	"encoding/json"
	"fmt"
	"strings"
)

// choicesFence is the fenced-block language tag the model uses to offer the
// writer a set of pick-one options (rendered as a button list in the UI).
const choicesFence = "linetta-choices"

// Choices is the parsed contents of a linetta-choices block. The writer picks
// one option (sent verbatim as their next reply) or, when AllowCustom is true,
// types their own answer instead.
type Choices struct {
	Prompt      string   `json:"prompt,omitempty"`
	Options     []string `json:"options"`
	AllowCustom bool     `json:"allow_custom,omitempty"`
}

// ParseChoices scans full model output for a linetta-choices fenced block.
// Returns (choices, blockPresent, error), mirroring ParseProposal:
//   - no block:        (Choices{}, false, nil)
//   - one valid block: (parsed, true, nil)
//   - invalid/>=2:     (best-effort, true, err)
//
// Only the first block is considered when several are present.
func ParseChoices(full string) (Choices, bool, error) {
	blocks := extractFencedBlocks(full, choicesFence)
	if len(blocks) == 0 {
		return Choices{}, false, nil
	}
	c, err := decodeChoices(blocks[0])
	if err != nil {
		return c, true, err
	}
	if len(blocks) > 1 {
		return c, true, fmt.Errorf("multiple linetta-choices blocks (%d); only one allowed", len(blocks))
	}
	if err := validateChoices(c); err != nil {
		return c, true, err
	}
	return c, true, nil
}

func decodeChoices(body string) (Choices, error) {
	var c Choices
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &c); err != nil {
		return Choices{}, fmt.Errorf("invalid choices JSON: %w", err)
	}
	return c, nil
}

func validateChoices(c Choices) error {
	clean := make([]string, 0, len(c.Options))
	for _, opt := range c.Options {
		if strings.TrimSpace(opt) != "" {
			clean = append(clean, opt)
		}
	}
	if len(clean) < 2 {
		return fmt.Errorf("choices needs at least 2 options, got %d", len(clean))
	}
	return nil
}
