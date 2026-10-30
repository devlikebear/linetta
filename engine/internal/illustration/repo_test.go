package illustration

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type fixtureT struct {
	ctx     context.Context
	s       *store.Store
	repo    *Repo
	project string
	scene   string
}

func fixture(t *testing.T) fixtureT {
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
	scene, err := node.NewRepo(s).CreateRoot(ctx, p.ID, node.KindLeaf, "씬 1", "비 오는 밤", 1)
	if err != nil {
		t.Fatal(err)
	}
	return fixtureT{ctx: ctx, s: s, repo: NewRepo(s), project: p.ID, scene: scene.ID}
}

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// noisyPNG is a PNG that compresses badly, so a modest size runs past MaxBytes.
func noisyPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestAddListGetCaptionDelete(t *testing.T) {
	f := fixture(t)
	data := encodeJPEG(t, 32, 48)
	m, err := f.repo.Add(f.ctx, 10, NewInput{
		ProjectID: f.project, SourceNodeID: f.scene, Caption: "  우산 아래  ",
		Prompt: "Art style:\nwebtoon", Provider: "gemini-native", Model: "gemini-3.1-flash-image", Data: data,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if m.MIME != "image/jpeg" || m.Width != 32 || m.Height != 48 || m.ByteSize != len(data) || m.Caption != "우산 아래" {
		t.Fatalf("meta = %+v", m)
	}
	if m.SourceNodeID == nil || *m.SourceNodeID != f.scene {
		t.Fatalf("source = %v", m.SourceNodeID)
	}

	if _, err := f.repo.Add(f.ctx, 20, NewInput{ProjectID: f.project, Prompt: "p", Provider: "openai-codex", Model: "gpt-5.6-luna", Data: data}); err != nil {
		t.Fatalf("Add without a scene: %v", err)
	}
	list, err := f.repo.ListByProject(f.ctx, f.project)
	if err != nil || len(list) != 2 || list[0].CreatedAt != 20 || list[1].ID != m.ID {
		t.Fatalf("list = %+v, %v (want newest first)", list, err)
	}
	count, size, err := f.repo.Usage(f.ctx, f.project)
	if err != nil || count != 2 || size != int64(2*len(data)) {
		t.Fatalf("usage = %d %d %v", count, size, err)
	}

	got, err := f.repo.Get(f.ctx, m.ID)
	if err != nil || !bytes.Equal(got.Data, data) || got.Prompt != "Art style:\nwebtoon" {
		t.Fatalf("Get = %+v, %v", got.Meta, err)
	}
	updated, err := f.repo.UpdateCaption(f.ctx, m.ID, "표지")
	if err != nil || updated.Caption != "표지" {
		t.Fatalf("UpdateCaption = %+v, %v", updated, err)
	}
	if err := f.repo.Delete(f.ctx, m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	for name, err := range map[string]error{
		"get":     func() error { _, err := f.repo.Get(f.ctx, m.ID); return err }(),
		"delete":  f.repo.Delete(f.ctx, m.ID),
		"caption": func() error { _, err := f.repo.UpdateCaption(f.ctx, m.ID, "x"); return err }(),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s after delete: %v", name, err)
		}
	}
}

func TestAddValidation(t *testing.T) {
	f := fixture(t)
	other, err := project.NewRepo(f.s).Create(f.ctx, 1, project.NewInput{Title: "Other", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	jpg := encodeJPEG(t, 8, 8)
	cases := map[string]struct {
		in   NewInput
		want error
	}{
		"empty":               {NewInput{ProjectID: f.project, Data: nil}, ErrBadContent},
		"not an image":        {NewInput{ProjectID: f.project, Data: []byte("plain text")}, ErrBadContent},
		"gif":                 {NewInput{ProjectID: f.project, Data: []byte("GIF89a....")}, ErrBadContent},
		"unknown work":        {NewInput{ProjectID: "nope", Data: jpg}, ErrProjectNotFound},
		"scene of other work": {NewInput{ProjectID: other.ID, SourceNodeID: f.scene, Data: jpg}, ErrNodeNotFound},
		"too large jpeg":      {NewInput{ProjectID: f.project, Data: append(jpg, make([]byte, MaxBytes)...)}, ErrTooLarge},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.repo.Add(f.ctx, 1, tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAddReencodesALargePNGAsJPEG(t *testing.T) {
	f := fixture(t)
	data := noisyPNG(t, 1400, 1400)
	if len(data) <= MaxBytes {
		t.Fatalf("fixture PNG is only %d bytes; it must exceed MaxBytes", len(data))
	}
	m, err := f.repo.Add(f.ctx, 1, NewInput{ProjectID: f.project, Prompt: "p", Provider: "gemini-native", Model: "gemini-2.5-flash-image", Data: data})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if m.MIME != "image/jpeg" || m.ByteSize > MaxBytes || m.Width != 1400 || m.Height != 1400 {
		t.Fatalf("meta = %+v", m)
	}
}

func TestDeletingTheSceneKeepsThePictureAndTheWorkTakesIt(t *testing.T) {
	f := fixture(t)
	m, err := f.repo.Add(f.ctx, 1, NewInput{ProjectID: f.project, SourceNodeID: f.scene, Prompt: "p", Provider: "x", Model: "y", Data: encodeJPEG(t, 4, 4)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB().ExecContext(f.ctx, "DELETE FROM nodes WHERE id = ?", f.scene); err != nil {
		t.Fatal(err)
	}
	got, err := f.repo.Get(f.ctx, m.ID)
	if err != nil || got.SourceNodeID != nil {
		t.Fatalf("after the scene is deleted: %+v, %v", got.Meta, err)
	}
	if _, err := f.s.DB().ExecContext(f.ctx, "DELETE FROM projects WHERE id = ?", f.project); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.Get(f.ctx, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after the work is deleted: %v", err)
	}
}

func TestConcurrentAddsRespectTheLimit(t *testing.T) {
	f := fixture(t)
	data := encodeJPEG(t, 2, 2)
	for i := 0; i < MaxPerProject-2; i++ {
		if _, err := f.repo.Add(f.ctx, int64(i), NewInput{ProjectID: f.project, Prompt: "p", Provider: "x", Model: "y", Data: data}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.repo.Add(f.ctx, 999, NewInput{ProjectID: f.project, Prompt: "p", Provider: "x", Model: "y", Data: data})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	if count, _, _ := f.repo.Usage(f.ctx, f.project); count != MaxPerProject {
		t.Fatalf("count = %d, want exactly %d", count, MaxPerProject)
	}
	tooMany := 0
	for err := range errs {
		if errors.Is(err, ErrTooMany) {
			tooMany++
		}
	}
	if tooMany == 0 {
		t.Fatal("no add was refused at the limit")
	}
}

func TestDeleteRefusesTheWorksCover(t *testing.T) {
	f := fixture(t)
	m, err := f.repo.Add(f.ctx, 10, NewInput{ProjectID: f.project, Prompt: "p", Provider: "openai-codex", Model: "m", Data: encodeJPEG(t, 8, 8)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB().ExecContext(f.ctx,
		`INSERT INTO publish_covers (project_id, illustration_id, updated_at) VALUES (?, ?, 1)`, f.project, m.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.Delete(f.ctx, m.ID); !errors.Is(err, ErrIsCover) {
		t.Fatalf("Delete cover = %v, want ErrIsCover", err)
	}
	if _, err := f.s.DB().ExecContext(f.ctx, `DELETE FROM publish_covers`); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.Delete(f.ctx, m.ID); err != nil {
		t.Fatalf("Delete after the cover is unset: %v", err)
	}
}
