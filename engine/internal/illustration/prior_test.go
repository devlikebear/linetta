package illustration

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
)

func addPicture(t *testing.T, f fixtureT, at int64, prompt string, size int) Meta {
	t.Helper()
	m, err := f.repo.Add(f.ctx, at, NewInput{ProjectID: f.project, Prompt: prompt, Provider: "x", Model: "m", Data: encodeJPEG(t, size, size)})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func ids(metas []Meta) []string {
	out := []string{}
	for _, m := range metas {
		out = append(out, m.ID)
	}
	return out
}

func TestSuggestPrior_prefersPicturesOfTheCharactersThenKeptThenNewest(t *testing.T) {
	f, _, _, _, haejin, minho := serviceFixture(t)
	both := addPicture(t, f, 1, "Characters:\n- 해진\n- 민호", 4)
	onlyHaejin := addPicture(t, f, 2, "Characters:\n- 해진", 5)
	keptHaejin := addPicture(t, f, 3, "Characters:\n- 해진", 6)
	addPicture(t, f, 4, "Characters:\n- 서윤", 7)
	if _, err := f.repo.SetRepresentative(f.ctx, 5, f.scene, keptHaejin.ID); err != nil {
		t.Fatal(err)
	}

	got, err := f.repo.SuggestPrior(f.ctx, f.project, []string{haejin, minho}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{both.ID, keptHaejin.ID}; strings.Join(ids(got), ",") != strings.Join(want, ",") {
		t.Fatalf("suggested %v, want %v (both characters, then the kept one)", ids(got), want)
	}

	got, _ = f.repo.SuggestPrior(f.ctx, f.project, []string{haejin}, 3)
	if len(got) != MaxPriorReferences || got[0].ID != keptHaejin.ID || got[1].ID != onlyHaejin.ID {
		t.Fatalf("suggested %v, want kept then newest of 해진's, capped at %d", ids(got), MaxPriorReferences)
	}
}

func TestSuggestPrior_matchesAliasesAndRequestCharacters(t *testing.T) {
	f, _, _, _, haejin, minho := serviceFixture(t)
	if _, err := f.s.DB().ExecContext(f.ctx, `UPDATE entities SET aliases = '["Doyun"]' WHERE id = ?`, minho); err != nil {
		t.Fatal(err)
	}
	byAlias := addPicture(t, f, 1, "doyun at dawn", 4)
	byRequest := addPicture(t, f, 2, "a quiet street", 5)
	if _, err := f.s.DB().ExecContext(f.ctx, `INSERT INTO illustration_requests
		(id, project_id, purpose, prompt, character_ids_json, include_references, aspect, caption, note, source, status, illustration_id, created_at)
		VALUES ('r1', ?, 'gallery', 'p', ?, 0, 'square', '', '', 'agent', 'done', ?, 1)`, f.project, `["`+haejin+`"]`, byRequest.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.repo.SuggestPrior(f.ctx, f.project, []string{minho}, 0); len(got) != 1 || got[0].ID != byAlias.ID {
		t.Fatalf("by alias: %v", ids(got))
	}
	if got, _ := f.repo.SuggestPrior(f.ctx, f.project, []string{haejin}, 0); len(got) != 1 || got[0].ID != byRequest.ID {
		t.Fatalf("by request: %v", ids(got))
	}
}

func TestSuggestPrior_fallsBackToOnePictureForTheStyle(t *testing.T) {
	f, _, _, _, haejin, _ := serviceFixture(t)
	if got, err := f.repo.SuggestPrior(f.ctx, f.project, []string{haejin}, 0); err != nil || len(got) != 0 {
		t.Fatalf("empty gallery: %v %v", got, err)
	}
	kept := addPicture(t, f, 1, "a street", 4)
	addPicture(t, f, 2, "a park", 5)
	if _, err := f.repo.SetRepresentative(f.ctx, 3, f.scene, kept.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.repo.SuggestPrior(f.ctx, f.project, []string{haejin}, 0); len(got) != 1 || got[0].ID != kept.ID {
		t.Fatalf("named characters in no picture: %v, want only the kept one", ids(got))
	}
	if got, _ := f.repo.SuggestPrior(f.ctx, f.project, nil, 0); len(got) != 2 || got[0].ID != kept.ID {
		t.Fatalf("no characters: %v, want kept first", ids(got))
	}
	if _, err := f.repo.SuggestPrior(f.ctx, f.project, []string{"nobody"}, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown character = %v", err)
	}
}

func TestGenerate_sendsPriorIllustrationsAfterCharacterReferencesWithTheNote(t *testing.T) {
	f, svc, painter, visuals, haejin, _ := serviceFixture(t)
	if _, err := visuals.AddImage(f.ctx, 1, haejin, "image/png", "정면", refPNG); err != nil {
		t.Fatal(err)
	}
	prior := addPicture(t, f, 1, "Characters:\n- 해진", 9)
	stored, _ := f.repo.Get(f.ctx, prior.ID)

	m, err := svc.Generate(f.ctx, GenerateInput{
		ProjectID: f.project, Prompt: "Scene:\n아침", CharacterIDs: []string{haejin}, IncludeReferences: true,
		PriorIllustrationIDs: []string{prior.ID, prior.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	refs := painter.got.References
	if len(refs) != 2 || !bytes.Equal(refs[0].Data, refPNG) || !bytes.Equal(refs[1].Data, stored.Data) || refs[1].MIME != "image/jpeg" {
		t.Fatalf("references = %d, want the character's then the earlier picture once", len(refs))
	}
	if !strings.HasPrefix(painter.got.Prompt, "Scene:\n아침\n\nEarlier illustrations:\n") || m.Prompt != painter.got.Prompt {
		t.Fatalf("prompt sent %q, stored %q", painter.got.Prompt, m.Prompt)
	}
	// Sending the stored prompt again does not repeat the note.
	if again := WithPriorNote(m.Prompt); again != m.Prompt {
		t.Fatalf("note repeated: %q", again)
	}
}

func TestGenerate_refusesPriorIllustrationsFromElsewhere(t *testing.T) {
	f, svc, painter, _, _, _ := serviceFixture(t)
	other, err := project.NewRepo(f.s).Create(f.ctx, 1, project.NewInput{Title: "다른 작품", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.repo.Add(f.ctx, 1, NewInput{ProjectID: other.ID, Prompt: "p", Provider: "x", Model: "m", Data: encodeJPEG(t, 4, 4)})
	if err != nil {
		t.Fatal(err)
	}
	a, b, c := addPicture(t, f, 1, "a", 4), addPicture(t, f, 2, "b", 5), addPicture(t, f, 3, "c", 6)
	for name, in := range map[string][]string{
		"another work": {foreign.ID},
		"unknown":      {"missing"},
		"too many":     {a.ID, b.ID, c.ID},
	} {
		_, err := svc.Generate(f.ctx, GenerateInput{ProjectID: f.project, Prompt: "p", PriorIllustrationIDs: in})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	if painter.calls != 0 {
		t.Fatalf("drew %d times for refused requests", painter.calls)
	}
	// Without earlier pictures nothing is added to the prompt.
	if _, err := svc.Generate(f.ctx, GenerateInput{ProjectID: f.project, Prompt: "p"}); err != nil || painter.got.Prompt != "p" {
		t.Fatalf("plain prompt = %q, %v", painter.got.Prompt, err)
	}
}
