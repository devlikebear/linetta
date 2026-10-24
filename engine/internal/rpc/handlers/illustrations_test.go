package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/illustration"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/visual"
)

type fakeIllustrationGenerator struct {
	got illustration.GenerateInput
	out illustration.Meta
	err error
}

func (f *fakeIllustrationGenerator) Generate(_ context.Context, in illustration.GenerateInput) (illustration.Meta, error) {
	f.got = in
	return f.out, f.err
}

type fakeImageProviders struct{ models []string }

func (f fakeImageProviders) List(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`[{"id":"gemini-native","active":true}]`), nil
}
func (f fakeImageProviders) ListModels(context.Context, string) ([]string, error) {
	return f.models, nil
}

func reasonOfMethodError(t *testing.T, err error) (int, string) {
	t.Helper()
	var me *rpc.MethodError
	if !errors.As(err, &me) {
		t.Fatalf("want a MethodError, got %v", err)
	}
	var data struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(me.Data, &data)
	return me.Code, data.Reason
}

func TestIllustrationHandlers(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	p, err := project.NewRepo(s).Create(ctx, 1, project.NewInput{Title: "T", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entity.NewRepo(s).Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "해진"}); err != nil {
		t.Fatal(err)
	}
	repo := illustration.NewRepo(s)
	call := func(h rpc.Handler, in any) (json.RawMessage, error) {
		t.Helper()
		data, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		return h(ctx, data)
	}

	// Compose: the prompt the dialog shows, and a fixable mistake as invalid params.
	out, err := call(IllustrationsCompose(visual.NewRepo(s), entity.NewRepo(s)), map[string]any{"project_id": p.ID, "names": []string{"해진"}, "scene": "비"})
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	var comp visual.Composition
	if err := json.Unmarshal(out, &comp); err != nil || comp.Format != "single" || len(comp.Characters) != 1 {
		t.Fatalf("compose = %s, %v", out, err)
	}
	_, err = call(IllustrationsCompose(visual.NewRepo(s), entity.NewRepo(s)), map[string]any{"project_id": p.ID, "names": []string{"도윤"}, "scene": "비"})
	if code, _ := reasonOfMethodError(t, err); code != rpc.CodeInvalidParams {
		t.Fatalf("unknown name: code %d", code)
	}

	// Generate: input reaches the service; its errors keep their reasons.
	gen := &fakeIllustrationGenerator{out: illustration.Meta{ID: "i1"}}
	out, err = call(IllustrationsGenerate(gen), map[string]any{"project_id": p.ID, "prompt": "p", "aspect": "portrait", "include_references": true, "character_ids": []string{"e1"}})
	if err != nil || string(out) == "" || gen.got.Aspect != "portrait" || !gen.got.IncludeReferences || gen.got.CharacterIDs[0] != "e1" {
		t.Fatalf("generate: %s %v, got %+v", out, err, gen.got)
	}
	for name, tc := range map[string]struct {
		err    error
		code   int
		reason string
	}{
		"provider reason": {&rpc.ReasonError{Reason: rpc.ReasonImageRefused, Err: errors.New("blocked")}, rpc.CodeInvalidParams, rpc.ReasonImageRefused},
		"invalid input":   {fmt.Errorf("%w: prompt is empty", illustration.ErrInvalidInput), rpc.CodeInvalidParams, ""},
		"gallery full":    {illustration.ErrTooMany, 0, rpc.ReasonTooManyIllustrations},
	} {
		t.Run(name, func(t *testing.T) {
			gen.err = tc.err
			_, err := call(IllustrationsGenerate(gen), map[string]any{"project_id": p.ID, "prompt": "p"})
			code, reason := reasonOfMethodError(t, err)
			if reason != tc.reason || (tc.code != 0 && code != tc.code) {
				t.Fatalf("code=%d reason=%q", code, reason)
			}
		})
	}
	if _, err := call(IllustrationsGenerate(gen), map[string]any{"prompt": "p"}); err == nil {
		t.Fatal("generate without project_id must be refused")
	}

	// Gallery: list with totals, get with bytes, caption, delete.
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 3, 3)), nil); err != nil {
		t.Fatal(err)
	}
	m, err := repo.Add(ctx, 1, illustration.NewInput{ProjectID: p.ID, Prompt: "p", Provider: "x", Model: "y", Data: jpg.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	out, err = call(IllustrationsList(repo), map[string]string{"project_id": p.ID})
	var list struct {
		Items []illustration.Meta `json:"items"`
		Count int                 `json:"count"`
		Bytes int64               `json:"bytes"`
	}
	if err != nil || json.Unmarshal(out, &list) != nil || list.Count != 1 || list.Bytes != int64(jpg.Len()) || list.Items[0].ID != m.ID {
		t.Fatalf("list = %s, %v", out, err)
	}
	out, err = call(IllustrationsGet(repo), map[string]string{"id": m.ID})
	var got struct {
		ID         string `json:"id"`
		DataBase64 string `json:"data_base64"`
	}
	if err != nil || json.Unmarshal(out, &got) != nil || got.ID != m.ID || got.DataBase64 != base64.StdEncoding.EncodeToString(jpg.Bytes()) {
		t.Fatalf("get = %.80s, %v", out, err)
	}
	if out, err := call(IllustrationsUpdate(repo), map[string]string{"id": m.ID, "caption": "표지"}); err != nil || !bytes.Contains(out, []byte("표지")) {
		t.Fatalf("update = %s, %v", out, err)
	}
	if _, err := call(IllustrationsDelete(repo), map[string]string{"id": m.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = call(IllustrationsGet(repo), map[string]string{"id": m.ID})
	if _, reason := reasonOfMethodError(t, err); reason != rpc.ReasonIllustrationNotFound {
		t.Fatalf("get after delete: %q", reason)
	}
}

func TestImageProviderHandlers(t *testing.T) {
	out, err := ImageProvidersList(fakeImageProviders{})(context.Background(), nil)
	if err != nil || string(out) != `[{"id":"gemini-native","active":true}]` {
		t.Fatalf("list = %s, %v", out, err)
	}
	out, err = ImageProvidersListModels(fakeImageProviders{})(context.Background(), json.RawMessage(`{"provider":"gemini-native"}`))
	if err != nil || string(out) != `{"models":[]}` {
		t.Fatalf("models = %s, %v", out, err)
	}
}
