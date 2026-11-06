package voice_test

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/clio/voice"
	"github.com/devlikebear/linetta/engine/internal/clio/voiceimport"
)

type golden struct {
	Posts    []string             `json:"posts"`
	Features []map[string]float64 `json:"features"`
	Endings  map[string]float64   `json:"endings"`
	Segments [][]string           `json:"segments"`
}

// TestPythonParity holds the Go port against scripts/eval-voice.py. Regenerate
// the golden with testdata/gen_golden.py when the Python definitions change.
func TestPythonParity(t *testing.T) {
	var g golden
	raw, err := os.ReadFile("testdata/parity_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("testdata/parity_posts.txt")
	if err != nil {
		t.Fatal(err)
	}
	res, err := voiceimport.Parse("parity_posts.txt", src, voiceimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Posts) != len(g.Posts) {
		t.Fatalf("loaded %d posts, python %d", len(res.Posts), len(g.Posts))
	}
	for i, p := range res.Posts {
		if p.Body != g.Posts[i] {
			t.Fatalf("post %d differs:\n go %q\n py %q", i, p.Body, g.Posts[i])
		}
	}

	for i, body := range g.Posts {
		got := voice.Features(body)
		for j, name := range voice.FeatureNames {
			want, ok := g.Features[i][name]
			if !ok {
				t.Fatalf("golden lacks %s", name)
			}
			if math.Abs(got.Values()[j]-want) > 1e-9 {
				t.Errorf("post %d %s: go %v py %v", i, name, got.Values()[j], want)
			}
		}
		segs := voice.Segments(body)
		if len(segs) != len(g.Segments[i]) {
			t.Errorf("post %d segments: go %q py %q", i, segs, g.Segments[i])
			continue
		}
		for k := range segs {
			if segs[k] != g.Segments[i][k] {
				t.Errorf("post %d segment %d: go %q py %q", i, k, segs[k], g.Segments[i][k])
			}
		}
	}

	ends := voice.EndingsOf(g.Posts)
	if len(ends) != len(g.Endings) {
		t.Errorf("endings: go %d keys, py %d", len(ends), len(g.Endings))
	}
	for e, want := range g.Endings {
		if math.Abs(ends[e]-want) > 1e-9 {
			t.Errorf("ending %q: go %v py %v", e, ends[e], want)
		}
	}
}
