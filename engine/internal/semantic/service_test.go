package semantic

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

// fakeEmbedder maps a text to which of a few topic words it holds: enough
// for "meaning" that the tests can predict.
type fakeEmbedder struct{ calls, texts atomic.Int32 }

var topics = []string{"피아노", "검", "바다", "편지"}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.calls.Add(1)
	f.texts.Add(int32(len(texts)))
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, len(topics)+1)
		v[len(topics)] = 0.1
		for j, w := range topics {
			if strings.Contains(t, w) {
				v[j] = 1
			}
		}
		var n float64
		for _, x := range v {
			n += float64(x * x)
		}
		for j := range v {
			v[j] /= float32(math.Sqrt(n))
		}
		out[i] = v
	}
	return out, nil
}

func (f *fakeEmbedder) Close() error { return nil }

type fixture struct {
	st      *store.Store
	svc     *Service
	emb     *fakeEmbedder
	nodes   *node.Repo
	idx     *manuscript.Indexer
	project string
	on      atomic.Bool
}

func doc(paras ...string) string {
	var content []map[string]any
	for _, p := range paras {
		content = append(content, map[string]any{"type": "paragraph", "content": []map[string]any{{"type": "text", "text": p}}})
	}
	b, _ := json.Marshal(map[string]any{"type": "doc", "content": content})
	return string(b)
}

// installed lays the model's files where Models looks, sized as the spec says.
func installed(t *testing.T) *Models {
	t.Helper()
	root := t.TempDir()
	spec := ModelSpec{Name: "fake", Dir: "fake", ONNXFile: "m.onnx", Files: []ModelFile{{Path: "m.onnx", Size: 3}}}
	if err := os.MkdirAll(filepath.Join(root, "fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fake", "m.onnx"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewModels(spec, root, nil)
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := storetest.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &fixture{st: st, emb: &fakeEmbedder{}, nodes: node.NewRepo(st), idx: manuscript.NewIndexer(st.DB())}
	p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	f.project = p.ID
	f.on.Store(true)
	f.svc = NewService(st.DB(), installed(t), f.on.Load, nil, func(context.Context, string, string) (Embedder, error) { return f.emb, nil })
	return f
}

func (f *fixture) scene(t *testing.T, paras ...string) string {
	t.Helper()
	ctx := context.Background()
	n, err := f.nodes.CreateRoot(ctx, f.project, node.KindLeaf, "씬", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	f.write(t, n.ID, paras...)
	return n.ID
}

func (f *fixture) write(t *testing.T, id string, paras ...string) {
	t.Helper()
	d := doc(paras...)
	if err := f.nodes.UpdateContent(context.Background(), id, d, 3); err != nil {
		t.Fatal(err)
	}
	if err := f.idx.Upsert(context.Background(), f.project, id, d); err != nil {
		t.Fatal(err)
	}
}

func TestSyncEmbedsOnlyWhatChanged(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a := f.scene(t, "누나가 피아노를 쳤다.", "비가 왔다.", "")
	f.scene(t, "검을 뽑았다.", "비가 왔다.")
	if err := f.svc.Sync(ctx, f.project); err != nil {
		t.Fatal(err)
	}
	// Three distinct paragraphs; the repeated one and the blank one once / not at all.
	if got := f.emb.texts.Load(); got != 3 {
		t.Fatalf("embedded %d paragraphs, want 3", got)
	}
	p, _ := f.svc.Progress(ctx, f.project)
	if p.Done != 3 || p.Total != 3 {
		t.Fatalf("progress = %+v", p)
	}
	if err := f.svc.Sync(ctx, f.project); err != nil {
		t.Fatal(err)
	}
	if got := f.emb.texts.Load(); got != 3 {
		t.Fatalf("an unchanged work was embedded again: %d", got)
	}
	f.write(t, a, "누나가 피아노를 쳤다.", "바다가 보였다.")
	if err := f.svc.Sync(ctx, f.project); err != nil {
		t.Fatal(err)
	}
	if got := f.emb.texts.Load(); got != 4 {
		t.Fatalf("after one new paragraph: %d embedded, want 4", got)
	}
	have, _ := vectors(ctx, f.st.DB(), f.project, "fake")
	if len(have) != 4 {
		t.Fatalf("vectors = %d, want 4", len(have))
	}
}

func TestSyncDropsVectorsOfRemovedParagraphs(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a := f.scene(t, "편지를 썼다.", "검을 닦았다.")
	_ = f.svc.Sync(ctx, f.project)
	f.write(t, a, "편지를 썼다.")
	_ = f.svc.Sync(ctx, f.project)
	have, _ := vectors(ctx, f.st.DB(), f.project, "fake")
	if len(have) != 1 {
		t.Fatalf("vectors = %d, want 1", len(have))
	}
}

func TestSearchFindsTheParagraphByMeaning(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.scene(t, "검을 뽑았다.", "비가 왔다.")
	b := f.scene(t, "비가 왔다.", "누나가 피아노 앞에 앉았다.")
	if _, ok, _ := f.svc.Search(ctx, f.project, "피아노 치던 기억", 5); ok {
		t.Fatal("search answered before anything was embedded")
	}
	_ = f.svc.Sync(ctx, f.project)
	got, ok, err := f.svc.Search(ctx, f.project, "피아노 치던 기억", 5)
	if err != nil || !ok || len(got) == 0 {
		t.Fatalf("search = %v %v %v", got, ok, err)
	}
	if got[0].NodeID != b || got[0].Para != 2 {
		t.Fatalf("best = %+v, want scene b paragraph 2", got[0])
	}
}

func TestNothingRunsWhileOff(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.scene(t, "검을 뽑았다.")
	f.on.Store(false)
	if err := f.svc.Sync(ctx, f.project); err != nil {
		t.Fatal(err)
	}
	if f.emb.calls.Load() != 0 {
		t.Fatal("embedded while off")
	}
	if _, ok, _ := f.svc.Search(ctx, f.project, "검", 3); ok {
		t.Fatal("searched while off")
	}
}

func TestTheWorkerEmbedsAfterAKick(t *testing.T) {
	f := setup(t)
	f.svc.delay = 10 * time.Millisecond
	f.svc.Start()
	defer f.svc.Close()
	f.scene(t, "바다가 보였다.")
	f.svc.Kick(f.project)
	deadline := time.Now().Add(5 * time.Second)
	for f.emb.texts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if f.emb.texts.Load() != 1 {
		t.Fatalf("worker embedded %d", f.emb.texts.Load())
	}
}

func TestForgetDropsEveryVector(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.scene(t, "바다가 보였다.")
	_ = f.svc.Sync(ctx, f.project)
	if err := f.svc.Forget(ctx); err != nil {
		t.Fatal(err)
	}
	if have, _ := vectors(ctx, f.st.DB(), f.project, "fake"); len(have) != 0 {
		t.Fatalf("vectors left: %d", len(have))
	}
}

func TestQuantizeKeepsTheOrder(t *testing.T) {
	q := []float32{0.6, 0.8, 0}
	near := quantize([]float32{0.6, 0.8, 0})
	far := quantize([]float32{0, 0.8, 0.6})
	if dot(q, near) <= dot(q, far) || math.Abs(float64(dot(q, near))-1) > 0.02 {
		t.Fatalf("dot near %.3f far %.3f", dot(q, near), dot(q, far))
	}
}
