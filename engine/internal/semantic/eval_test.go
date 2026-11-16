//go:build !mobile

package semantic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

// TestEvalMeaningBeatsWords is the #384 measurement as a regression test:
// the 40-scene eval corpus (testdata/eval, made by scripts/gen-semantic-eval.py)
// searched by words and by auto through the real model, indexer and ranking.
// The spike measured 2.5% and 70.0% of questions with the right scene in the
// top 10. It runs only when LINETTA_SEMANTIC_MODEL names the model's folder;
// CI has none and skips it.
func TestEvalMeaningBeatsWords(t *testing.T) {
	dir := os.Getenv("LINETTA_SEMANTIC_MODEL")
	if dir == "" {
		t.Skip("LINETTA_SEMANTIC_MODEL not set")
	}
	var corpus struct {
		Scenes []struct {
			Title      string   `json:"title"`
			Paragraphs []string `json:"paragraphs"`
		} `json:"scenes"`
		Gold []struct {
			Scene int    `json:"scene"`
			Query string `json:"query"`
		} `json:"gold"`
	}
	b, err := os.ReadFile(filepath.Join("testdata", "eval", "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &corpus); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, err := storetest.Open(ctx, filepath.Join(t.TempDir(), "eval.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	nodes := node.NewRepo(st)
	indexer := manuscript.NewIndexer(st.DB())
	nodes.SetManuscriptIndexer(indexer)
	p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{Title: "eval", Genres: []string{"fantasy"}, LengthTarget: "short", DefaultPOV: "third_limited"})
	if err != nil {
		t.Fatal(err)
	}
	sceneOf := map[string]int{}
	prev := *p.LastOpenedNodeID
	for i, sc := range corpus.Scenes {
		n, err := nodes.CreateSibling(ctx, prev, node.KindLeaf, sc.Title, "", 2)
		if err != nil {
			t.Fatal(err)
		}
		prev = n.ID
		if err := nodes.UpdateContent(ctx, n.ID, doc(sc.Paragraphs...), 3); err != nil {
			t.Fatal(err)
		}
		sceneOf[n.ID] = i
	}

	spec := Model
	spec.Dir = filepath.Base(dir)
	models := NewModels(spec, filepath.Dir(dir), nil)
	if !models.Status().Installed {
		t.Fatalf("%s does not hold the pinned model's files", dir)
	}
	svc := NewService(st.DB(), models, func() bool { return true }, nil, OpenEmbedder)
	t.Cleanup(func() { _ = svc.Close() })
	if err := svc.Sync(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	searcher := manuscript.NewSearcher(st.DB(), nodes, indexer)
	searcher.SetSemantic(svc)

	recall := func(mode string) float64 {
		hit := 0
		for _, g := range corpus.Gold {
			hits, _, err := searcher.QueryMode(ctx, p.ID, g.Query, 10, mode)
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range hits {
				if sceneOf[h.NodeID] == g.Scene {
					hit++
					break
				}
			}
		}
		return float64(hit) / float64(len(corpus.Gold))
	}
	words, auto := recall(manuscript.ModeWords), recall(manuscript.ModeAuto)
	t.Logf("right scene in the top 10: words %.1f%%, auto %.1f%%", 100*words, 100*auto)
	// The issue's bar: meaning lifts it by at least 15 points. The floor
	// below the spike's 70% catches a regression without failing on noise.
	if auto-words < 0.15 || auto < 0.60 {
		t.Fatalf("words %.1f%%, auto %.1f%%: want auto at least 60%% and 15 points above words", 100*words, 100*auto)
	}
}
