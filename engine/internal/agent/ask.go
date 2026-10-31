//go:build !mobile

package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/tars/pkg/llm"
)

// A card of questions for the writer, the agent's way of asking for decisions
// the writer can answer by picking instead of retyping options back.
//
// Asking ends the turn. The card lives on the tool row of the transcript and
// the answers arrive as the writer's next message, an ordinary agent.run. The
// alternative, a tool call that blocks until the writer answers, does not
// survive what a writer actually does with a plot question: Claude Code's CLI
// is killed after 15 quiet minutes, and an app restart ends every turn. A row
// waits for as long as they think about it.
//
// The tool exists only here, not in mcphost. An external MCP client has no
// panel to draw the card on, and has its own way of asking its user.

const askWriterTool = "linetta_ask_writer"

// askedReply is what a self-executing provider reads back. The loop never
// sends it to a model: it ends the turn on the spot.
const askedReply = "The card is on the writer's screen, and their answers will arrive as their next message. " +
	"End your turn now, without calling any more tools or saying anything else: the card speaks for itself."

const (
	maxAskQuestions  = 6
	minAskOptions    = 2
	maxAskOptions    = 6
	maxAskHeaderRune = 40
)

type askOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

type askQuestion struct {
	Header      string      `json:"header"`
	Question    string      `json:"question"`
	MultiSelect bool        `json:"multi_select,omitempty"`
	Options     []askOption `json:"options"`
}

// askCard is the tool's arguments and, once checked, what the panel draws.
type askCard struct {
	Questions []askQuestion `json:"questions"`
}

func askWriterSchema() llm.ToolSchema {
	return llm.ToolSchema{
		Type: "function",
		Function: llm.ToolFunctionSchema{
			Name: askWriterTool,
			Description: "Show the writer a card of questions they answer by picking options, each with room for their own answer. " +
				"Use it whenever you need the writer to decide something, instead of listing options in your reply. " +
				"Put every open decision in one call. This ends your turn: the answers arrive as their next message.",
			Parameters: json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["questions"],
  "properties": {
    "questions": {
      "type": "array",
      "minItems": 1,
      "maxItems": 6,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["header", "question", "options"],
        "properties": {
          "header": {"type": "string", "description": "A few words naming the decision. It labels the answer, so it must make sense alone."},
          "question": {"type": "string", "description": "The full question, with what the writer needs to decide it. Markdown is fine."},
          "multi_select": {"type": "boolean", "description": "True when the writer may pick more than one option."},
          "options": {
            "type": "array",
            "minItems": 2,
            "maxItems": 6,
            "description": "The choices. The card adds a free-answer field itself, so do not add an 'other' option.",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "required": ["label"],
              "properties": {
                "label": {"type": "string", "description": "The choice in a few words. It is what the answer quotes, so it must make sense alone."},
                "description": {"type": "string", "description": "What choosing it means for the work. Markdown is fine."},
                "recommended": {"type": "boolean", "description": "True on the option you recommend."}
              }
            }
          }
        }
      }
    }
  }
}`),
		},
	}
}

// parseAskCard reads the tool's arguments. Its errors are the model's to read
// and correct, so each says what to change.
func parseAskCard(arguments string) (*askCard, error) {
	var card askCard
	if err := json.Unmarshal([]byte(arguments), &card); err != nil {
		return nil, fmt.Errorf("arguments for %s are not valid: %v", askWriterTool, err)
	}
	if len(card.Questions) == 0 || len(card.Questions) > maxAskQuestions {
		return nil, fmt.Errorf("a card holds 1 to %d questions, got %d", maxAskQuestions, len(card.Questions))
	}
	for i := range card.Questions {
		q := &card.Questions[i]
		q.Header, q.Question = strings.TrimSpace(q.Header), strings.TrimSpace(q.Question)
		if q.Header == "" || q.Question == "" {
			return nil, fmt.Errorf("question %d needs a header and a question", i+1)
		}
		if utf8.RuneCountInString(q.Header) > maxAskHeaderRune {
			return nil, fmt.Errorf("question %d: the header is over %d characters", i+1, maxAskHeaderRune)
		}
		if len(q.Options) < minAskOptions || len(q.Options) > maxAskOptions {
			return nil, fmt.Errorf("question %d needs %d to %d options, got %d", i+1, minAskOptions, maxAskOptions, len(q.Options))
		}
		seen := make(map[string]bool, len(q.Options))
		for j := range q.Options {
			o := &q.Options[j]
			o.Label, o.Description = strings.TrimSpace(o.Label), strings.TrimSpace(o.Description)
			if o.Label == "" {
				return nil, fmt.Errorf("question %d: option %d has no label", i+1, j+1)
			}
			if seen[o.Label] {
				return nil, fmt.Errorf("question %d: two options share the label %q", i+1, o.Label)
			}
			seen[o.Label] = true
		}
	}
	return &card, nil
}

// askWriter checks a card. Nothing else happens here: runTool puts the card on
// the tool's event and transcript row, which is all that showing it takes.
func askWriter(arguments string) toolResult {
	card, err := parseAskCard(arguments)
	if err != nil {
		return toolResult{IsError: true, Text: err.Error()}
	}
	return toolResult{Text: askedReply, Ask: card}
}

// askedMessage is a card as the model reads it when the conversation is
// replayed, in place of the tool row replay otherwise skips. Without it a later
// turn sees "Timeline: Option A" answering a question it has no record of
// asking, and a writer who typed a reply instead is even harder to follow.
func askedMessage(toolRow string) string {
	var ev toolEvent
	if err := json.Unmarshal([]byte(toolRow), &ev); err != nil || ev.Ask == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("[I asked the writer these questions on a card.]\n")
	for i, q := range ev.Ask.Questions {
		fmt.Fprintf(&b, "\n%d. %s: %s\n", i+1, q.Header, q.Question)
		if q.MultiSelect {
			b.WriteString("(any number of options)\n")
		}
		for _, o := range q.Options {
			b.WriteString("- " + o.Label)
			if o.Recommended {
				b.WriteString(" (recommended)")
			}
			if o.Description != "" {
				b.WriteString(": " + o.Description)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}
