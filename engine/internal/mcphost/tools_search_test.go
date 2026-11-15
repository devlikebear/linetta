package mcphost

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

// A search hit's paragraph range must use the numbers linetta_read_scene
// numbered=true prints, since the agent goes from one to the other and cites
// what it read.
func TestSearchManuscriptNamesTheParagraphReadSceneNumbers(t *testing.T) {
	ctx := context.Background()
	st, err := storetest.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	projects := project.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{
		Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	nodes := node.NewRepo(st)
	indexer := manuscript.NewIndexer(st.DB())
	nodes.SetManuscriptIndexer(indexer)
	d := ToolDeps{Projects: projects, Nodes: nodes, Manuscript: manuscript.NewSearcher(st.DB(), nodes, indexer)}

	filler := strings.Repeat("바람이 불었다. ", 40)
	var blocks []string
	for _, text := range []string{filler, "", filler, "수아는 은빛 열쇠를 서랍에 넣었다.", filler} {
		if text == "" {
			blocks = append(blocks, `{"type":"paragraph"}`)
			continue
		}
		blocks = append(blocks, fmt.Sprintf(`{"type":"paragraph","content":[{"type":"text","text":%q}]}`, text))
	}
	doc := `{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`
	sceneID := *p.LastOpenedNodeID
	if err := nodes.UpdateContent(ctx, sceneID, doc, 10); err != nil {
		t.Fatal(err)
	}

	res, out, err := d.searchManuscript(ctx, nil, searchManuscriptInput{ProjectID: p.ID, Query: "은빛 열쇠"})
	if err != nil || res != nil {
		t.Fatalf("search: res=%v err=%v", res, err)
	}
	if len(out.Hits) != 1 {
		t.Fatalf("hits = %+v", out.Hits)
	}
	hit := out.Hits[0]
	if hit.ParagraphFrom == 0 || hit.ParagraphFrom != hit.ParagraphTo {
		t.Fatalf("hit range = %d-%d, want one paragraph", hit.ParagraphFrom, hit.ParagraphTo)
	}
	numbered := numberedParagraphs(&doc)
	want := fmt.Sprintf("[%d] 수아는 은빛 열쇠", hit.ParagraphFrom)
	if !strings.Contains(numbered, want) {
		t.Fatalf("hit names paragraph %d, but read_scene numbers it differently:\n%s", hit.ParagraphFrom, numbered)
	}
}
