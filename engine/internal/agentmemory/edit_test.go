package agentmemory

import (
	"errors"
	"strings"
	"testing"
)

func TestAddAppendsALine(t *testing.T) {
	got, err := Apply(ScopeWorkNotes, "첫 줄", ActionAdd, "", "둘째 줄")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != "첫 줄\n둘째 줄" {
		t.Errorf("got %q", got)
	}
}

func TestAddToAnEmptyMemoryDoesNotLeaveALeadingNewline(t *testing.T) {
	got, err := Apply(ScopeWriterProfile, "", ActionAdd, "", "줄표 쓰지 않기")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != "줄표 쓰지 않기" {
		t.Errorf("got %q, want no leading newline", got)
	}
}

func TestReplaceSwapsTheWholeMatchingLine(t *testing.T) {
	body := "민준은 반말\n복선 X는 12화\n배경은 부산"
	got, err := Apply(ScopeWorkNotes, body, ActionReplace, "민준은", "민준은 3화부터 존댓말")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "민준은 3화부터 존댓말\n복선 X는 12화\n배경은 부산"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRemoveDeletesTheWholeMatchingLine(t *testing.T) {
	body := "민준은 반말\n복선 X는 12화\n배경은 부산"
	got, err := Apply(ScopeWorkNotes, body, ActionRemove, "복선 X", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != "민준은 반말\n배경은 부산" {
		t.Errorf("got %q", got)
	}
}

func TestRemovingTheOnlyLineLeavesAnEmptyBody(t *testing.T) {
	got, err := Apply(ScopeWorkNotes, "외줄", ActionRemove, "외줄", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want an empty body with no stray newline", got)
	}
}

// The issue specifies short-unique-substring matching. Ambiguity has to be an
// error: silently taking the first match would edit a line the agent did not
// mean, and it would never find out.
func TestAmbiguousFindIsRefused(t *testing.T) {
	body := "민준은 반말\n민준은 부산 출신"
	_, err := Apply(ScopeWorkNotes, body, ActionRemove, "민준은", "")
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("got %v, want ErrAmbiguous", err)
	}
	if !strings.Contains(err.Error(), "2") {
		t.Errorf("the error should say how many lines matched so the agent can lengthen its find; got %v", err)
	}
}

func TestNoMatchIsRefused(t *testing.T) {
	_, err := Apply(ScopeWorkNotes, "민준은 반말", ActionReplace, "지훈", "지훈은 존댓말")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("got %v, want ErrNoMatch", err)
	}
}

func TestApplyScreensTheNewText(t *testing.T) {
	if _, err := Apply(ScopeWorkNotes, "", ActionAdd, "", "안녕​하세요"); !errors.Is(err, ErrInvisible) {
		t.Fatalf("got %v, want ErrInvisible", err)
	}
}

func TestApplyRefusesOverBudgetWithoutTruncating(t *testing.T) {
	body := strings.Repeat("가", 2190)
	_, err := Apply(ScopeWorkNotes, body, ActionAdd, "", strings.Repeat("나", 20))
	if !errors.Is(err, ErrOverBudget) {
		t.Fatalf("got %v, want ErrOverBudget", err)
	}
	// The whole point of a budget the agent manages: it has to be told to make
	// room, not handed a quietly clipped memory.
	if !strings.Contains(err.Error(), "remove") && !strings.Contains(err.Error(), "replace") {
		t.Errorf("the error must tell the agent what to do next; got %v", err)
	}
}

// Removing must always be possible, even when the body is already over budget
// (a shrunk budget, or a hand-edited row) — otherwise the agent is stuck.
func TestRemoveWorksWhenTheBodyIsAlreadyOverBudget(t *testing.T) {
	body := strings.Repeat("가", 2300) + "\n지울 줄"
	got, err := Apply(ScopeWorkNotes, body, ActionRemove, "지울 줄", "")
	if err != nil {
		t.Fatalf("Apply: %v — a remove that shrinks the body must be allowed", err)
	}
	if strings.Contains(got, "지울 줄") {
		t.Error("the line was not removed")
	}
}

func TestRequiredArguments(t *testing.T) {
	if _, err := Apply(ScopeWorkNotes, "", ActionAdd, "", "  "); !errors.Is(err, ErrEmptyText) {
		t.Errorf("add with blank text: got %v", err)
	}
	if _, err := Apply(ScopeWorkNotes, "a", ActionRemove, " ", ""); !errors.Is(err, ErrEmptyFind) {
		t.Errorf("remove with blank find: got %v", err)
	}
	if _, err := Apply(ScopeWorkNotes, "", "rewrite", "", "x"); !errors.Is(err, ErrBadAction) {
		t.Errorf("unknown action: got %v", err)
	}
}

func TestAddedTextIsNormalisedToOneLine(t *testing.T) {
	// One memory is one line, so find stays unambiguous. A multi-line text
	// collapses rather than being refused: the agent's intent is clear.
	got, err := Apply(ScopeWorkNotes, "", ActionAdd, "", "첫째\n둘째")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != "첫째 둘째" {
		t.Errorf("got %q, want the newline collapsed to a space", got)
	}
}
