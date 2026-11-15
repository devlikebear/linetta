package proposals

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/storyops"
)

func entityItem(name string) ItemInput {
	return ItemInput{Op: storyops.Op{Type: "create_entity", Kind: "character", Name: name}}
}

// A name the writer turns down stays down for a tidy-up, under any spacing,
// until they let it back; a batch they asked for may still bring it.
func TestDismissedNames_keepATurnedDownNameOutOfATidyUp(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	res, problems, err := e.svc.Submit(ctx, now(), SubmitInput{
		ProjectID: e.project.ID, Purpose: PurposeOrganize,
		Items: []ItemInput{entityItem("김민호"), {Op: storyops.Op{Type: "create_thread", Name: "복수"}}},
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("submit: %v %s", err, problemText(problems))
	}
	if n, err := e.svc.Reject(ctx, now()+1, e.project.ID, []string{res.Items[0].ID, res.Items[1].ID}); err != nil || n != 2 {
		t.Fatalf("reject: n=%d err=%v", n, err)
	}
	names, err := e.svc.Dismissed(ctx, e.project.ID)
	if err != nil || len(names) != 1 || names[0].Name != "김민호" || names[0].Kind != "character" {
		t.Fatalf("dismissed = %+v err=%v; only a new name is remembered", names, err)
	}

	_, problems, err = e.svc.Submit(ctx, now()+2, SubmitInput{
		ProjectID: e.project.ID, Purpose: PurposeOrganize, Items: []ItemInput{entityItem("김 민호")},
	}, SourceAgent)
	if err != nil || !strings.Contains(problemText(problems), `the writer turned "김민호" down before`) {
		t.Fatalf("tidy-up with a turned-down name: err=%v %s", err, problemText(problems))
	}
	_, problems, err = e.svc.Submit(ctx, now()+2, SubmitInput{
		ProjectID: e.project.ID, Purpose: PurposeGeneral, Items: []ItemInput{entityItem("김민호")},
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("a batch the writer asked for was refused: %v %s", err, problemText(problems))
	}

	if err := e.svc.ForgetDismissed(ctx, e.project.ID, "character", "김 민호"); err != nil {
		t.Fatal(err)
	}
	_, problems, err = e.svc.Submit(ctx, now()+3, SubmitInput{
		ProjectID: e.project.ID, Purpose: PurposeOrganize, Items: []ItemInput{entityItem("김민호")},
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("after forgetting, the name is still refused: %v %s", err, problemText(problems))
	}
}

func TestPartialName(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"민준", "강민준", true},
		{"민준", "남궁민준", true},
		{"민준", "어머니민준", false},
		{"Harin", "Harin Kang", true},
		{"kang", "Harin Kang", true},
		{"준", "민준", false},
		{"민준", "민준", false},
		{"서울", "서울역", false},
		{"열쇠", "은빛 열쇠", true},
	}
	for _, c := range cases {
		if got := partialName(c.a, c.b); got != c.want {
			t.Errorf("partialName(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
		if got := partialName(c.b, c.a); got != c.want {
			t.Errorf("partialName(%q, %q) = %v, want %v (either order)", c.b, c.a, got, c.want)
		}
	}
}

// The writer sees "민준" might be "강민준" written short, and nothing for a
// place that merely shares a word with a character.
func TestFindDuplicatesMarksAShortNameAsPartial(t *testing.T) {
	candidates := []dedupeCandidate{
		{EntityID: "e1", Name: "강민준", Kind: "character"},
		{EntityID: "e2", Name: "민준 거리", Kind: "place"},
		{EntityID: "e3", Name: "하린", Kind: "character", Aliases: []string{"민준"}},
	}
	dups := findDuplicates("민준", "character", candidates)
	if len(dups) != 2 || dups[0].EntityID != "e1" || dups[0].Matched != "partial" || dups[1].EntityID != "e3" || dups[1].Matched != "exact" {
		t.Fatalf("dups = %+v", dups)
	}
}
