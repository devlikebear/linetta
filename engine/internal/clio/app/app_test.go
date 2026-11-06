package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func call(t *testing.T, a *App, method string, params any) (json.RawMessage, *rpc.Error) {
	t.Helper()
	p, _ := json.Marshal(params)
	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, method, p)
	out, err := a.Handle(context.Background(), []byte(req))
	if err != nil {
		t.Fatal(err)
	}
	var msg rpc.Message
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatal(err)
	}
	return msg.Result, msg.Error
}

func open(t *testing.T) *App {
	t.Helper()
	a, err := Open(context.Background(), Options{Home: filepath.Join(t.TempDir(), "home")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func sample(n int) string {
	var parts []string
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf("오늘은 %d번째로 글을 쓰는 날이라 조금 길게 적어 본다", i))
	}
	return strings.Join(parts, "\n---\n")
}

func TestInvalidParams(t *testing.T) {
	a := open(t)
	cases := map[string]any{
		"posts.import_preview": map[string]any{"content": ""},
		"posts.import":         map[string]any{"content": "충분히 긴 내용이 여기에 들어 있다", "platform": "mastodon"},
		"posts.list":           map[string]any{"limit": 100000},
		"voice.profile":        map[string]any{"platform": "nope"},
	}
	for m, p := range cases {
		if _, e := call(t, a, m, p); e == nil || e.Code != rpc.CodeInvalidParams {
			t.Errorf("%s: %+v", m, e)
		}
	}
	if _, e := call(t, a, "posts.import", "not an object"); e == nil || e.Code != rpc.CodeInvalidParams {
		t.Errorf("bad shape: %+v", e)
	}
	if _, e := call(t, a, "posts.list", map[string]any{"kind": "dm"}); e == nil || e.Code != rpc.CodeInvalidParams {
		t.Errorf("list kind: %+v", e)
	}
}

func TestRoundTrip(t *testing.T) {
	a := open(t)
	in := map[string]any{"file_name": "p.txt", "content": sample(25), "default_kind": "post"}

	raw, e := call(t, a, "posts.import_preview", in)
	if e != nil {
		t.Fatal(e)
	}
	var prev struct {
		Total    int            `json:"total"`
		Preview  []any          `json:"preview"`
		Skipped  map[string]int `json:"skipped"`
		Contexts []struct {
			Count int `json:"count"`
		} `json:"contexts"`
	}
	_ = json.Unmarshal(raw, &prev)
	if prev.Total != 25 || len(prev.Preview) != 5 || len(prev.Contexts) != 1 || prev.Contexts[0].Count != 25 {
		t.Fatalf("preview %s", raw)
	}

	raw, e = call(t, a, "posts.import", in)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(raw), `"inserted":25`) {
		t.Fatalf("import %s", raw)
	}
	raw, _ = call(t, a, "posts.import", in)
	if !strings.Contains(string(raw), `"inserted":0`) || !strings.Contains(string(raw), `"skipped":25`) {
		t.Fatalf("reimport %s", raw)
	}

	raw, _ = call(t, a, "posts.list", map[string]any{"limit": 10})
	var list []map[string]any
	_ = json.Unmarshal(raw, &list)
	if len(list) != 10 {
		t.Fatalf("list %d", len(list))
	}
	raw, _ = call(t, a, "posts.counts", nil)
	if !strings.Contains(string(raw), `"count":25`) {
		t.Fatalf("counts %s", raw)
	}

	raw, e = call(t, a, "voice.profile", map[string]any{"platform": "x", "kind": "reply"})
	if e != nil {
		t.Fatal(e)
	}
	var prof struct {
		N        int    `json:"n"`
		Fallback string `json:"fallback"`
		Examples []string
	}
	_ = json.Unmarshal(raw, &prof)
	if prof.N != 25 || prof.Fallback != "x/*" {
		t.Fatalf("profile %s", raw)
	}
}
