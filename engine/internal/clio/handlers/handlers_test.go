package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func TestImportPreviewInvalidParams(t *testing.T) {
	h := ImportPreview()
	for _, p := range []string{`"x"`, `{"content":"  "}`, `{"content":"충분히 긴 내용이 여기에 들어 있다","default_kind":"dm"}`, `{"file_name":"a.json","content":"{nope"}`} {
		_, err := h(context.Background(), json.RawMessage(p))
		var me *rpc.MethodError
		if !errors.As(err, &me) || me.Code != rpc.CodeInvalidParams {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestImportPreviewCounts(t *testing.T) {
	out, err := ImportPreview()(context.Background(), json.RawMessage(`{"file_name":"a.txt","content":"충분히 긴 첫 번째 글을 천천히 적어 둔다\n---\n충분히 긴 두 번째 글을 천천히 적어 둔다\n---\n짧음","default_kind":"reply"}`))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Total   int            `json:"total"`
		Skipped map[string]int `json:"skipped"`
	}
	_ = json.Unmarshal(out, &got)
	if got.Total != 2 || got.Skipped["short"] != 1 {
		t.Fatalf("%s", out)
	}
}
