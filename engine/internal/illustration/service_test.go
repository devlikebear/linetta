package illustration

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/visual"
)

type fakePainter struct {
	got    PaintRequest
	calls  int
	result PaintResult
	err    error
	// deadline records whether Paint was given one.
	deadline bool
}

func (p *fakePainter) Paint(ctx context.Context, req PaintRequest) (PaintResult, error) {
	p.calls++
	p.got = req
	_, p.deadline = ctx.Deadline()
	return p.result, p.err
}

var refPNG = []byte("\x89PNG\r\n\x1a\nreference")

func serviceFixture(t *testing.T) (fixtureT, *Service, *fakePainter, *visual.Repo, string, string) {
	t.Helper()
	f := fixture(t)
	visuals := visual.NewRepo(f.s)
	entities := entity.NewRepo(f.s)
	haejin, err := entities.Create(f.ctx, 1, entity.NewInput{ProjectID: f.project, Name: "해진"})
	if err != nil {
		t.Fatal(err)
	}
	minho, err := entities.Create(f.ctx, 1, entity.NewInput{ProjectID: f.project, Name: "민호"})
	if err != nil {
		t.Fatal(err)
	}
	painter := &fakePainter{result: PaintResult{Data: encodeJPEG(t, 20, 30), MIME: "image/jpeg", Provider: "gemini-native", Model: "gemini-3.1-flash-image", RevisedPrompt: "rewritten"}}
	svc := NewService(f.repo, visuals, painter, func() int64 { return 42 })
	return f, svc, painter, visuals, haejin.ID, minho.ID
}

func TestGenerateDrawsAndStores(t *testing.T) {
	f, svc, painter, visuals, haejin, _ := serviceFixture(t)
	if _, err := visuals.AddImage(f.ctx, 1, haejin, "image/png", "정면", refPNG); err != nil {
		t.Fatal(err)
	}

	m, err := svc.Generate(f.ctx, GenerateInput{
		ProjectID: f.project, SourceNodeID: f.scene, Prompt: "Art style:\nwebtoon",
		CharacterIDs: []string{haejin}, IncludeReferences: true, Aspect: AspectPortrait, Caption: "우산",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if painter.got.Prompt != "Art style:\nwebtoon" || painter.got.Aspect != AspectPortrait || !painter.deadline {
		t.Fatalf("painter got %+v (deadline %v)", painter.got, painter.deadline)
	}
	if len(painter.got.References) != 1 || !bytes.Equal(painter.got.References[0].Data, refPNG) || painter.got.References[0].MIME != "image/png" {
		t.Fatalf("references = %+v", painter.got.References)
	}
	if m.Prompt != "Art style:\nwebtoon" || m.Provider != "gemini-native" || m.Model != "gemini-3.1-flash-image" || m.CreatedAt != 42 || m.Caption != "우산" {
		t.Fatalf("stored %+v (want the prompt that was sent, not the rewrite)", m)
	}
	if m.SourceNodeID == nil || *m.SourceNodeID != f.scene || m.Width != 20 || m.Height != 30 {
		t.Fatalf("stored %+v", m)
	}
}

func TestGenerateSendsAtMostFourReferencesInCharacterOrder(t *testing.T) {
	f, svc, painter, visuals, haejin, minho := serviceFixture(t)
	for i := 0; i < 3; i++ {
		if _, err := visuals.AddImage(f.ctx, 1, minho, "image/png", "m", append(refPNG, byte('m'), byte(i))); err != nil {
			t.Fatal(err)
		}
		if _, err := visuals.AddImage(f.ctx, 1, haejin, "image/png", "h", append(refPNG, byte('h'), byte(i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Generate(f.ctx, GenerateInput{ProjectID: f.project, Prompt: "p", CharacterIDs: []string{minho, haejin}, IncludeReferences: true}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	refs := painter.got.References
	if len(refs) != MaxReferences {
		t.Fatalf("%d references, want %d", len(refs), MaxReferences)
	}
	if refs[0].Data[len(refs[0].Data)-2] != 'm' || refs[3].Data[len(refs[3].Data)-2] != 'h' {
		t.Fatal("references are not in character order")
	}
	if painter.got.Aspect != AspectSquare {
		t.Fatalf("default aspect = %q", painter.got.Aspect)
	}

	painter.got = PaintRequest{}
	if _, err := svc.Generate(f.ctx, GenerateInput{ProjectID: f.project, Prompt: "p", CharacterIDs: []string{minho}}); err != nil {
		t.Fatal(err)
	}
	if len(painter.got.References) != 0 {
		t.Fatal("references were sent without IncludeReferences")
	}
}

func TestGenerateRefusesBeforeDrawing(t *testing.T) {
	f, svc, painter, _, _, _ := serviceFixture(t)
	cases := map[string]struct {
		in   GenerateInput
		want error
	}{
		"empty prompt":   {GenerateInput{ProjectID: f.project, Prompt: " "}, ErrInvalidInput},
		"bad aspect":     {GenerateInput{ProjectID: f.project, Prompt: "p", Aspect: "panorama"}, ErrInvalidInput},
		"unknown work":   {GenerateInput{ProjectID: "nope", Prompt: "p"}, ErrProjectNotFound},
		"foreign scene":  {GenerateInput{ProjectID: f.project, SourceNodeID: "nope", Prompt: "p"}, ErrNodeNotFound},
		"unknown person": {GenerateInput{ProjectID: f.project, Prompt: "p", CharacterIDs: []string{"nope"}}, ErrInvalidInput},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Generate(f.ctx, tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	if painter.calls != 0 {
		t.Fatalf("the provider was called %d times for requests refused up front", painter.calls)
	}
}

func TestGenerateStoresNothingWhenTheDrawingFails(t *testing.T) {
	f, svc, painter, _, _, _ := serviceFixture(t)
	painter.err = errors.New("refused by provider")
	if _, err := svc.Generate(f.ctx, GenerateInput{ProjectID: f.project, Prompt: "p"}); err == nil {
		t.Fatal("want the provider's error")
	}
	painter.err, painter.result.Data = nil, []byte("not an image")
	if _, err := svc.Generate(f.ctx, GenerateInput{ProjectID: f.project, Prompt: "p"}); !errors.Is(err, ErrBadContent) {
		t.Fatalf("garbage from the provider: %v", err)
	}
	if count, _, _ := f.repo.Usage(f.ctx, f.project); count != 0 {
		t.Fatalf("%d illustrations stored after failed drawings", count)
	}
}
