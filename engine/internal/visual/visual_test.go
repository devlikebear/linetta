package visual

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func fixture(t *testing.T) (context.Context, *store.Store, *Repo, string, string) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	p, err := project.NewRepo(s).Create(ctx, 1, project.NewInput{Title: "T", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := entity.NewRepo(s).Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "해진"})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, s, NewRepo(s), p.ID, e.ID
}

var pngData = []byte("\x89PNG\r\n\x1a\nimage")

func TestSheetAndStyleUpsert(t *testing.T) {
	ctx, _, r, p, e := fixture(t)
	sheet, err := r.GetSheet(ctx, e)
	if err != nil || !sheet.Empty() {
		t.Fatalf("empty: %+v %v", sheet, err)
	}
	style, err := r.GetArtStyle(ctx, p)
	if err != nil || style.Style != "" {
		t.Fatalf("empty style: %+v %v", style, err)
	}
	for _, now := range []int64{2, 3} {
		got, err := r.SetSheet(ctx, now, Sheet{EntityID: e, AgeRange: " 20대 ", Hair: " 검은 머리 ", Notes: " 고정 "})
		if err != nil || got.AgeRange != "20대" || got.UpdatedAt != now {
			t.Fatalf("set: %+v %v", got, err)
		}
		style, err = r.SetArtStyle(ctx, now, ArtStyle{ProjectID: p, Style: " 수채화 ", NegativePrompt: " 글자 "})
		if err != nil || style.Style != "수채화" || style.UpdatedAt != now {
			t.Fatalf("style: %+v %v", style, err)
		}
	}
	if _, err = r.SetSheet(ctx, 4, Sheet{EntityID: "missing"}); !errors.Is(err, ErrEntityNotFound) {
		t.Fatal(err)
	}
	if _, err = r.SetArtStyle(ctx, 4, ArtStyle{ProjectID: "missing"}); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	sheets, err := r.ListSheetsByProject(ctx, p)
	if err != nil || len(sheets) != 1 || sheets[e].Hair != "검은 머리" {
		t.Fatalf("list: %+v %v", sheets, err)
	}
	if _, err = r.SetSheet(ctx, 5, Sheet{EntityID: e}); err != nil {
		t.Fatal(err)
	}
	sheet, err = r.GetSheet(ctx, e)
	if err != nil || !sheet.Empty() || sheet.UpdatedAt != 5 {
		t.Fatalf("clear: %+v %v", sheet, err)
	}
}
func TestImagesValidationAndCascade(t *testing.T) {
	ctx, s, r, p, e := fixture(t)
	for _, tc := range []struct {
		mime string
		data []byte
		want error
	}{
		{"image/png", nil, ErrUnsupportedImageType}, {"text/plain", pngData, ErrUnsupportedImageType},
		{"image/jpeg", pngData, ErrUnsupportedImageType}, {"image/png", make([]byte, MaxImageBytes+1), ErrImageTooLarge},
	} {
		if _, err := r.AddImage(ctx, 1, e, tc.mime, "", tc.data); !errors.Is(err, tc.want) {
			t.Fatalf("%s: %v", tc.mime, err)
		}
	}
	if _, err := r.AddImage(ctx, 1, "missing", "image/png", "", pngData); !errors.Is(err, ErrEntityNotFound) {
		t.Fatal(err)
	}
	images, err := r.ListImages(ctx, e)
	if err != nil || images == nil || len(images) != 0 {
		t.Fatalf("empty list: %v %v", images, err)
	}
	var first ImageMeta
	for i := 0; i < MaxImagesPerEntity; i++ {
		m, err := r.AddImage(ctx, 2, e, "image/png", " caption ", pngData)
		if err != nil {
			t.Fatal(err)
		}
		if m.Ordinal != i || m.Caption != "caption" {
			t.Fatal(m)
		}
		if i == 0 {
			first = m
		}
	}
	if _, err := r.AddImage(ctx, 2, e, "image/png", "", pngData); !errors.Is(err, ErrTooManyImages) {
		t.Fatal(err)
	}
	got, err := r.GetImage(ctx, first.ID)
	if err != nil || !bytes.Equal(got.Data, pngData) {
		t.Fatalf("image: %+v %v", got, err)
	}
	updated, err := r.UpdateImageCaption(ctx, first.ID, " changed ")
	if err != nil || updated.Caption != "changed" {
		t.Fatalf("caption: %+v %v", updated, err)
	}
	if err := r.DeleteImage(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteImage(ctx, first.ID); !errors.Is(err, ErrImageNotFound) {
		t.Fatal(err)
	}
	if _, err := r.UpdateImageCaption(ctx, first.ID, ""); !errors.Is(err, ErrImageNotFound) {
		t.Fatal(err)
	}
	if _, err := r.GetImage(ctx, first.ID); !errors.Is(err, ErrImageNotFound) {
		t.Fatal(err)
	}
	if _, err := r.SetSheet(ctx, 2, Sheet{EntityID: e, Hair: "red"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetArtStyle(ctx, 2, ArtStyle{ProjectID: p, Style: "ink"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx, "DELETE FROM entities WHERE id = ?", e); err != nil {
		t.Fatal(err)
	}
	images, err = r.ListImages(ctx, e)
	if err != nil || len(images) != 0 {
		t.Fatalf("cascade: %v %v", images, err)
	}
	sheets, err := r.ListSheetsByProject(ctx, p)
	if err != nil || len(sheets) != 0 {
		t.Fatalf("sheet cascade: %v %v", sheets, err)
	}
	if _, err := s.DB().ExecContext(ctx, "DELETE FROM projects WHERE id = ?", p); err != nil {
		t.Fatal(err)
	}
	style, err := r.GetArtStyle(ctx, p)
	if err != nil || style.Style != "" {
		t.Fatalf("style cascade: %+v %v", style, err)
	}
}
func TestAllAllowedImageTypes(t *testing.T) {
	ctx, _, r, _, e := fixture(t)
	for mime, data := range map[string][]byte{"image/png": pngData, "image/jpeg": {0xff, 0xd8, 0xff, 0xe0}, "image/gif": []byte("GIF89aimage"), "image/webp": []byte("RIFF\x00\x00\x00\x00WEBPVP8 image")} {
		if _, err := r.AddImage(ctx, 1, e, mime, "", data); err != nil {
			t.Errorf("%s: %v", mime, err)
		}
	}
}
func TestBuildPrompt(t *testing.T) {
	chars := []CharacterVisual{
		{Name: "해진", Role: "주인공", Sheet: Sheet{AgeRange: "20대", Build: "마른", Hair: "검은 머리", Outfit: "흰 옷", Signature: "안경", Palette: "파랑", Notes: " 항상 고정 "}, Images: []ImageMeta{{ID: "a"}, {ID: "b"}}},
		{Name: "빈 인물"},
	}
	style := ArtStyle{Style: " 수채화 ", NegativePrompt: "글자 금지"}
	scene := "1컷: 문을 연다\n2컷: 웃는다"

	single := BuildPrompt(style, chars, scene, "")
	want := "Art style:\n수채화\n\n" +
		"Characters:\n" +
		"- 해진 (주인공)\n" +
		"  - age: 20대\n  - build: 마른\n  - hair: 검은 머리\n  - outfit: 흰 옷\n" +
		"  - signature expression/props: 안경\n  - color palette: 파랑\n  - always: 항상 고정\n" +
		"  - reference images: 2 attached; match them\n" +
		"- 빈 인물\n\n" +
		"Scene:\n" + scene + "\n\n" +
		"Format:\nSingle illustration.\n\n" +
		"Keep consistent:\nDraw each character exactly as described above and in their reference images; do not redesign them.\n\n" +
		"Negative prompt:\n글자 금지"
	if single != want {
		t.Fatalf("single prompt:\n%s\n--- want ---\n%s", single, want)
	}
	if BuildPrompt(style, chars, scene, FormatSingle) != single {
		t.Fatal("empty format must mean single")
	}
	if single != BuildPrompt(style, chars, scene, "") {
		t.Fatal("nondeterministic")
	}

	four := BuildPrompt(style, chars, scene, FormatFourPanel)
	if !strings.Contains(four, "Four-panel comic (4컷만화)") || !strings.Contains(four, "all four panels") || !strings.Contains(four, scene) {
		t.Fatalf("four-panel prompt:\n%s", four)
	}

	// Nothing the writer left blank is printed, and nothing is invented.
	empty := BuildPrompt(ArtStyle{}, nil, "고양이", "")
	if empty != "Scene:\n고양이\n\nFormat:\nSingle illustration." {
		t.Fatalf("empty prompt:\n%s", empty)
	}
}

func TestConcurrentUploadsRespectLimit(t *testing.T) {
	ctx, _, repo, _, entityID := fixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.AddImage(ctx, 1, entityID, "image/png", "", pngData)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	accepted, refused := 0, 0
	for err := range errs {
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrTooManyImages) {
			refused++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 8 || refused != 8 {
		t.Fatalf("accepted %d, refused %d", accepted, refused)
	}
}
