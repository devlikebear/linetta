package visual

import (
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
)

func TestCompose(t *testing.T) {
	ctx, s, repo, projectID, haejinID := fixture(t)
	entities := entity.NewRepo(s)
	minho, err := entities.Create(ctx, 1, entity.NewInput{ProjectID: projectID, Name: "민호"})
	if err != nil {
		t.Fatal(err)
	}
	aliases := []string{"민"}
	if err := entities.Update(ctx, 1, entity.UpdateInput{ID: minho.ID, Aliases: &aliases}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetSheet(ctx, 1, Sheet{EntityID: haejinID, Hair: "짧은 은발"}); err != nil {
		t.Fatal(err)
	}
	ref, err := repo.AddImage(ctx, 1, haejinID, "image/png", "정면", pngData)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetArtStyle(ctx, 1, ArtStyle{ProjectID: projectID, Style: "webtoon", NegativePrompt: "text"}); err != nil {
		t.Fatal(err)
	}
	all, err := entity.NewRepo(s).ListByProject(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}

	c, err := repo.Compose(ctx, projectID, all, ComposeInput{Names: []string{"민", "해진"}, Scene: "둘이 우산을 쓴다"})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if c.Format != FormatSingle || c.NegativePrompt != "text" {
		t.Fatalf("format/negative = %q/%q", c.Format, c.NegativePrompt)
	}
	for _, want := range []string{"webtoon", "짧은 은발", "둘이 우산을 쓴다"} {
		if !strings.Contains(c.Prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, c.Prompt)
		}
	}
	// Sorted by name: 민호 before 해진.
	if len(c.Characters) != 2 || c.Characters[0].EntityID != minho.ID || c.Characters[1].EntityID != haejinID {
		t.Fatalf("characters = %+v", c.Characters)
	}
	if len(c.ReferenceImageIDs) != 1 || c.ReferenceImageIDs[0] != ref.ID {
		t.Fatalf("references = %v", c.ReferenceImageIDs)
	}
	if len(c.MissingSheets) != 1 || c.MissingSheets[0] != "민호" {
		t.Fatalf("missing sheets = %v", c.MissingSheets)
	}
}

func TestComposeRefusesWhatTheCallerCanFix(t *testing.T) {
	ctx, s, repo, projectID, _ := fixture(t)
	all, err := entity.NewRepo(s).ListByProject(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		in   ComposeInput
		want string
	}{
		"no scene":       {ComposeInput{Names: []string{"해진"}}, "scene is required"},
		"no characters":  {ComposeInput{Scene: "s"}, "name the characters"},
		"bad format":     {ComposeInput{Names: []string{"해진"}, Scene: "s", Format: "manga"}, "format must be"},
		"unknown name":   {ComposeInput{Names: []string{"도윤"}, Scene: "s"}, `unknown character "도윤"; available characters: 해진`},
		"id not in work": {ComposeInput{EntityIDs: []string{"nope"}, Scene: "s"}, `character "nope" is not in this work`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := repo.Compose(ctx, projectID, all, tc.in)
			var ie *InputError
			if !errors.As(err, &ie) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want an InputError containing %q", err, tc.want)
			}
		})
	}
}

func TestResolveCharactersWithoutNamesReturnsTheCast(t *testing.T) {
	all := []entity.Entity{
		{ID: "b", Name: "해진", Kind: entity.KindCharacter},
		{ID: "p", Name: "서울", Kind: entity.KindPlace},
		{ID: "a", Name: "민호", Kind: entity.KindCharacter},
	}
	got, err := ResolveCharacters(all, nil, nil)
	if err != nil || len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if all[0].ID != "b" {
		t.Fatal("ResolveCharacters reordered the caller's slice")
	}
	if _, err := ResolveCharacters([]entity.Entity{
		{ID: "1", Name: "민", Kind: entity.KindCharacter},
		{ID: "2", Name: "민수", Aliases: []string{"민"}, Kind: entity.KindCharacter},
	}, nil, []string{"민"}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous name: %v", err)
	}
}
