package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRun(t *testing.T) {
	f := filepath.Join(t.TempDir(), "p.txt")
	_ = os.WriteFile(f, []byte("오늘은 날씨가 좋아서 산책을 했다 정말 ㅋㅋ\n---\n내일도 비슷하면 좋겠다 정말로 그렇다\n---\n짧음"), 0o600)
	for _, args := range [][]string{{f, "--kind", "reply"}, {"--kind", "reply", f}} {
		var out bytes.Buffer
		if err := run(args, &out); err != nil {
			t.Fatal(err)
		}
		var got struct {
			Posts    int `json:"posts"`
			Contexts map[string]struct {
				N        int                               `json:"n"`
				Features map[string]struct{ Mean float64 } `json:"features"`
			} `json:"contexts"`
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Posts != 2 || got.Contexts["x/reply"].N != 2 || got.Contexts["x/*"].Features["laugh"].Mean != 0.5 {
			t.Fatalf("%s", out.String())
		}
	}
	if err := run(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("want usage error")
	}
}
