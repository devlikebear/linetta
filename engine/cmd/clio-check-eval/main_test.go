package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name string, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	var posts []map[string]any
	for i := 0; i < 40; i++ {
		posts = append(posts, map[string]any{"id": i, "platform": "x", "kind": "post", "text": fmt.Sprintf("오늘 %d번째 메모를 적었다", i)})
	}
	const original = "오늘 시험삼아 쓴 글이다"
	posts = append(posts, map[string]any{"id": 99, "platform": "x", "kind": "post", "text": original})
	rows := []map[string]any{{
		"id": 1, "note": "비밀 메모", "original": original,
		"A": "결론적으로 핵심은 꾸준함입니다 🚀",
		"B": "오늘도 한 줄 적었다",
		"C": "써 보니 괜찮았다. 다음엔 해 볼게요.",
	}}
	pp, rp := write(t, dir, "posts.json", posts), write(t, dir, "rows.json", rows)

	var out bytes.Buffer
	if err := run([]string{pp, rp}, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"1 excluded", "40 used", "ai_phrase", "register_mixed", "row 0 id=1 ctx=x/post"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, leak := range []string{"결론적으로", "시험삼아", "꾸준함", "비밀 메모", "적었다"} {
		if strings.Contains(got, leak) {
			t.Errorf("output leaks text %q:\n%s", leak, got)
		}
	}
	// Arm A is flagged, the original is not.
	if !strings.Contains(got, "A         1") || !strings.Contains(got, "original  0") {
		t.Errorf("arm counts wrong:\n%s", got)
	}
}

func TestRunUsage(t *testing.T) {
	if err := run(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("want usage error")
	}
}
