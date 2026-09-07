package agentskills

import (
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
)

// Sentinel causes, alongside the ones in skill.go. The caller turns each
// into a tool error the agent reads, so each one has to say what to change.
var (
	ErrTooLong            = errors.New("agentskills: body too long")
	ErrDescriptionTooLong = errors.New("agentskills: description too long")
)

// Guard screens a Skill before it can reach a language model's prompt: it is
// the last check between a document Parse accepted (frontmatter is
// well-formed, name and description are present) and a body that a model
// will actually read.
//
// A skill body is prompt-bound the same way an agentmemory document is, so
// it needs the same invisible-character screening — agentmemory.ScreenInvisible
// covers zero-width characters, bidi controls, the Unicode tag block, the
// Variation Selector Supplement, Hangul filler jamo, and Khmer inherent
// vowels, with a deliberate exception for a zero-width joiner inside an
// emoji sequence. But a skill body IS markdown: headings are its normal
// structure, not an escape attempt the way they are in a one-line memory.
// So Guard calls agentmemory.ScreenInvisible, not agentmemory.Screen — using
// the latter here would refuse every skill that has a section heading,
// which is nearly all of them.
//
// Guard also enforces the two size budgets declared in skill.go
// (MaxBodyRunes, MaxDescriptionRunes), which Parse deliberately does not —
// see MaxBodyRunes' doc comment. Sizes are checked in runes, not bytes: a
// byte budget would let a script with multi-byte runes (this app's
// languages very much included) in at a shorter apparent length than an
// ASCII body of the same limit, which is not what "8000 runes" is supposed
// to mean.
func Guard(s Skill) error {
	if n := len([]rune(s.Body)); n > MaxBodyRunes {
		return fmt.Errorf("%w: body is %d runes, over the %d-rune limit", ErrTooLong, n, MaxBodyRunes)
	}
	if n := len([]rune(s.Description)); n > MaxDescriptionRunes {
		return fmt.Errorf("%w: description is %d runes, over the %d-rune limit", ErrDescriptionTooLong, n, MaxDescriptionRunes)
	}
	if err := agentmemory.ScreenInvisible(s.Body); err != nil {
		return fmt.Errorf("skill body: %w", err)
	}
	if err := agentmemory.ScreenInvisible(s.Description); err != nil {
		return fmt.Errorf("skill description: %w", err)
	}
	return nil
}
