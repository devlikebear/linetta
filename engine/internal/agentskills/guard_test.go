package agentskills

import (
	"errors"
	"strings"
	"testing"
)

// hangulSyllable is a 1-rune, 3-byte-in-UTF-8 character. Fixtures built from
// it make a byte-length check and a rune-length check disagree, which is the
// point: a test built only from ASCII would pass under either limit and
// prove nothing about which one Guard actually enforces.
const hangulSyllable = "가"

func repeatRunes(r string, n int) string {
	return strings.Repeat(r, n)
}

func TestGuardAcceptsBodyAtMaxBodyRunes(t *testing.T) {
	s := Skill{
		Name:        "ok",
		Description: "an ordinary description",
		Body:        repeatRunes(hangulSyllable, MaxBodyRunes),
	}
	if got := len([]rune(s.Body)); got != MaxBodyRunes {
		t.Fatalf("fixture bug: body is %d runes, want %d", got, MaxBodyRunes)
	}
	if err := Guard(s); err != nil {
		t.Errorf("Guard(body at exactly MaxBodyRunes) = %v, want nil", err)
	}
}

func TestGuardRejectsBodyOverMaxBodyRunes(t *testing.T) {
	s := Skill{
		Name:        "ok",
		Description: "an ordinary description",
		Body:        repeatRunes(hangulSyllable, MaxBodyRunes+1),
	}
	if err := Guard(s); !errors.Is(err, ErrTooLong) {
		t.Errorf("Guard(body over MaxBodyRunes) = %v, want ErrTooLong", err)
	}
}

func TestGuardRejectsDescriptionOverMaxDescriptionRunes(t *testing.T) {
	s := Skill{
		Name:        "ok",
		Description: repeatRunes(hangulSyllable, MaxDescriptionRunes+1),
		Body:        "fine",
	}
	if err := Guard(s); !errors.Is(err, ErrDescriptionTooLong) {
		t.Errorf("Guard(description over MaxDescriptionRunes) = %v, want ErrDescriptionTooLong", err)
	}
}

func TestGuardAcceptsDescriptionAtMaxDescriptionRunes(t *testing.T) {
	s := Skill{
		Name:        "ok",
		Description: repeatRunes(hangulSyllable, MaxDescriptionRunes),
		Body:        "fine",
	}
	if err := Guard(s); err != nil {
		t.Errorf("Guard(description at exactly MaxDescriptionRunes) = %v, want nil", err)
	}
}

func TestGuardRejectsInvisibleCharacterAndNamesIt(t *testing.T) {
	s := Skill{
		Name:        "ok",
		Description: "an ordinary description",
		Body:        "안녕​하세요", // zero-width space
	}
	err := Guard(s)
	if err == nil {
		t.Fatal("Guard(body with zero-width space) = nil, want an error")
	}
	if !strings.Contains(err.Error(), "U+200B") {
		t.Errorf("Guard error must name the offending code point; got %v", err)
	}
}

// TestGuardAcceptsMarkdownHeadings is the test that pins the whole point of
// this task. A skill body is markdown: headings are its normal structure,
// not an escape attempt the way they are in agentmemory's single-line notes.
// If this test is ever the one skipped, Guard has quietly regressed to
// agentmemory.Screen's behavior and every real skill with a heading breaks.
func TestGuardAcceptsMarkdownHeadings(t *testing.T) {
	s := Skill{
		Name:        "ok",
		Description: "an ordinary description",
		Body: "# Title\n\n" +
			"## Section one\n\n" +
			"Some instructions.\n\n" +
			"### Subsection\n\n" +
			"###### Deep heading\n",
	}
	if err := Guard(s); err != nil {
		t.Errorf("Guard(body full of markdown headings) = %v, want nil", err)
	}
}

func TestGuardAcceptsEmojiZWJSequence(t *testing.T) {
	s := Skill{
		Name:        "ok",
		Description: "an ordinary description",
		Body:        "family emoji 👨‍👩‍👧‍👦 in a skill body",
	}
	if err := Guard(s); err != nil {
		t.Errorf("Guard(body with family ZWJ emoji) = %v, want nil", err)
	}
}

func TestGuardAcceptsSkinTonedEmojiZWJSequence(t *testing.T) {
	skinToneWomanHealthWorker := "\U0001F469\U0001F3FD‍⚕️"
	s := Skill{
		Name:        "ok",
		Description: "an ordinary description",
		Body:        "profile note: " + skinToneWomanHealthWorker,
	}
	if err := Guard(s); err != nil {
		t.Errorf("Guard(body with skin-toned ZWJ emoji) = %v, want nil", err)
	}
}
