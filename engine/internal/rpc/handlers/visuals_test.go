package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/visual"
	"path/filepath"
	"testing"
)

func TestVisualHandlers(t *testing.T) {
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
	e, err := entity.NewRepo(s).Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "해진"})
	if err != nil {
		t.Fatal(err)
	}
	r := visual.NewRepo(s)
	now := func() int64 { return 42 }
	call := func(h rpc.Handler, in any) json.RawMessage {
		t.Helper()
		data, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		out, err := h(ctx, data)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := call(ListVisualImages(r), map[string]string{"entity_id": e.ID})
	if string(out) != "[]" {
		t.Fatal(string(out))
	}
	call(SetVisualSheet(r, now), visual.Sheet{EntityID: e.ID, Hair: " 검정 "})
	out = call(GetVisualSheet(r), map[string]string{"entity_id": e.ID})
	var sheet visual.Sheet
	if err := json.Unmarshal(out, &sheet); err != nil || sheet.Hair != "검정" || sheet.UpdatedAt != 42 {
		t.Fatalf("sheet: %s %v", out, err)
	}
	call(SetArtStyle(r, now), visual.ArtStyle{ProjectID: p.ID, Style: " 수채화 "})
	out = call(GetArtStyle(r), map[string]string{"project_id": p.ID})
	var style visual.ArtStyle
	if err := json.Unmarshal(out, &style); err != nil || style.Style != "수채화" {
		t.Fatalf("style: %s %v", out, err)
	}
	out = call(AddVisualImage(r, now), map[string]string{"entity_id": e.ID, "mime": "image/png", "data_base64": "iVBORw0KGgo="})
	var meta visual.ImageMeta
	if err := json.Unmarshal(out, &meta); err != nil {
		t.Fatal(err)
	}
	out = call(GetVisualImage(r), map[string]string{"id": meta.ID})
	var image visualImageResult
	if err := json.Unmarshal(out, &image); err != nil || image.DataBase64 != "iVBORw0KGgo=" {
		t.Fatalf("image: %s %v", out, err)
	}
	out = call(UpdateVisualImage(r), map[string]string{"id": meta.ID, "caption": " ref "})
	if err := json.Unmarshal(out, &meta); err != nil || meta.Caption != "ref" {
		t.Fatalf("caption: %s %v", out, err)
	}
	call(DeleteVisualImage(r), map[string]string{"id": meta.ID})
	for _, tc := range []struct {
		h      rpc.Handler
		in     string
		reason string
	}{
		{GetVisualImage(r), `{"id":"missing"}`, rpc.ReasonImageNotFound},
		{SetVisualSheet(r, now), `{"entity_id":"missing"}`, rpc.ReasonEntityNotFound},
		{AddVisualImage(r, now), `{"entity_id":"` + e.ID + `","mime":"image/jpeg","data_base64":"iVBORw0KGgo="}`, rpc.ReasonUnsupportedImageType},
		{AddVisualImage(r, now), `{"entity_id":"` + e.ID + `","data_base64":"!"}`, ""},
		{GetVisualSheet(r), `{}`, ""},
	} {
		_, err := tc.h(ctx, json.RawMessage(tc.in))
		var method *rpc.MethodError
		if !errors.As(err, &method) || method.Code != rpc.CodeInvalidParams {
			t.Fatalf("error: %v", err)
		}
		if tc.reason != "" && string(method.Data) != string(rpc.ReasonData(tc.reason)) {
			t.Fatalf("reason: %s", method.Data)
		}
	}
}
