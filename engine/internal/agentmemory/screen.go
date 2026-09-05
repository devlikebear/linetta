// Package agentmemory owns the two curated documents every agent working on a
// Linetta manuscript reads: a global writer profile and per-work notes.
//
// It must never import tars/pkg/llm. mcphost and storycontext import this
// package, and scripts/validate-story-core-deps.sh forbids a model client in
// their dependency graph.
package agentmemory

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Sentinel causes. The caller turns each into a tool error the agent reads, so
// each one has to say what to change.
var (
	ErrInvisible = errors.New("agentmemory: invisible character")
	ErrControl   = errors.New("agentmemory: control character")
	ErrDelimiter = errors.New("agentmemory: markdown heading")
)

// blockDelimiter is how the injected block separates its sections. Memory
// content containing it could claim the block ended and the rest is something
// else, so it is refused at the door.
const blockDelimiter = "\n## "

// Screen refuses text that must not reach a prompt.
//
// It screens for characters a writer cannot see while reviewing their own
// memory but a model still reads: zero-width characters, bidi controls,
// Unicode tag characters, and the format category generally.
//
// It deliberately does NOT match phrases like "ignore previous instructions".
// Linetta's users write novels: a thriller contains that sentence honestly,
// and a note like "민준은 이전 지시를 무시하는 인물" is ordinary character
// description. A phrase filter would fire on exactly the material this app
// exists to hold, and a rephrasing walks past it anyway. What stands in its
// place is containment — see the frame in agent/prompt.go and
// storycontext/render.go, which says the block is the writer's notes about the
// writing and does not change what the tools do.
func Screen(text string) error {
	if strings.Contains("\n"+text, blockDelimiter) {
		return fmt.Errorf("%w: a memory line may not start a markdown heading (\"## \")", ErrDelimiter)
	}
	for _, r := range text {
		switch {
		case r == '\n' || r == '\t':
			// The only two control characters memory is written with.
		case unicode.IsControl(r):
			return fmt.Errorf("%w: U+%04X is not allowed in a memory", ErrControl, r)
		case unicode.Is(unicode.Cf, r) || isTagChar(r) || r == '­':
			return fmt.Errorf("%w: U+%04X is invisible and is not allowed in a memory", ErrInvisible, r)
		}
	}
	return nil
}

// isTagChar reports the Unicode tag block (U+E0000..U+E007F), which encodes
// ASCII invisibly.
func isTagChar(r rune) bool { return r >= 0xE0000 && r <= 0xE007F }
