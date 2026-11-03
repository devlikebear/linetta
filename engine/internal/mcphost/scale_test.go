//go:build !mobile

package mcphost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// A work at the size the plan is asking about, grown at the rates a real one
// was measured to grow: 3.5 elements and 3.6 ledger entries an episode. Most of
// the ledger is what closed arcs left behind. It takes a minute, so it runs only
// when asked:
//
//	LINETTA_SCALE_TEST=1 go test ./internal/mcphost -run TestThousandEpisodeWork -v
func TestThousandEpisodeWork(t *testing.T) {
	if os.Getenv("LINETTA_SCALE_TEST") == "" {
		t.Skip("set LINETTA_SCALE_TEST=1 to run")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	mentions := mention.NewRepo(st)
	nodes := node.NewRepo(st)
	nodes.SetMentionResyncer(func(ctx context.Context, id, doc string) error {
		return mentions.ResyncForNode(ctx, id, mention.Collect([]byte(doc)))
	})
	projects := project.NewRepo(st)
	entities := entity.NewRepo(st)
	repo := ledger.NewRepo(st, nodes).WithSubjects(entities)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "천 화", LengthTarget: "series", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if err := nodes.Delete(ctx, *p.LastOpenedNodeID, 2); err != nil {
		t.Fatal(err)
	}
	style := strings.Repeat("문장 호흡과 대사 규칙을 적은 작가 메모. ", 60)
	if _, err := projects.Update(ctx, 3, project.UpdateInput{ID: p.ID, StyleNotes: &style}); err != nil {
		t.Fatal(err)
	}

	var ids []string
	for i := range 3500 {
		e, err := entities.Create(ctx, 4, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: fmt.Sprintf("인물%d", i), Role: "조연"})
		if err != nil {
			t.Fatal(err)
		}
		summary := strings.Repeat("이 요소의 배경과 사정. ", 18)
		if err := entities.Update(ctx, 5, entity.UpdateInput{ID: e.ID, Summary: &summary}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	body := strings.Repeat("주인공은 패를 품에 넣고 객잔 문을 나섰다. ", 170)
	var scenes []node.Node
	for v := range 10 {
		volume, err := nodes.CreateRoot(ctx, p.ID, node.KindContainer, fmt.Sprintf("%d권", v+1), "", int64(10+v))
		if err != nil {
			t.Fatal(err)
		}
		if err := nodes.SetWrittenSummary(ctx, volume.ID, strings.Repeat("권의 흐름. ", 50), volume.ContentVersion); err != nil {
			t.Fatal(err)
		}
		for a := range 5 {
			arc, err := nodes.CreateChild(ctx, volume.ID, node.KindContainer, fmt.Sprintf("%d-%d아크", v+1, a+1), "", int64(100+a))
			if err != nil {
				t.Fatal(err)
			}
			for e := range 20 {
				n, err := nodes.CreateChild(ctx, arc.ID, node.KindLeaf, fmt.Sprintf("%d화", len(scenes)+1), "", int64(200+e))
				if err != nil {
					t.Fatal(err)
				}
				mentionsDoc := ""
				for k := range 8 {
					id := ids[(len(scenes)*3+k)%len(ids)]
					mentionsDoc += fmt.Sprintf(`,{"type":"mention","attrs":{"id":%q,"label":"인물"}}`, id)
				}
				doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + body + `"}` + mentionsDoc + `]}]}`
				if err := nodes.UpdateContent(ctx, n.ID, doc, int64(300+e)); err != nil {
					t.Fatal(err)
				}
				got, err := nodes.Get(ctx, n.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := nodes.SetWrittenSummary(ctx, n.ID, "요약 "+strings.Repeat("이 화에서 벌어진 일. ", 40), got.ContentVersion); err != nil {
					t.Fatal(err)
				}
				scenes = append(scenes, got)
			}
		}
	}
	for i := range 3600 {
		scene := scenes[i%len(scenes)]
		e := ledger.Entry{ProjectID: p.ID, Kind: ledger.KindPromise, Subject: fmt.Sprintf("약속%d", i), Body: strings.Repeat("회수해야 할 내용 ", 8), PlantedNodeID: scene.ID, Reach: ledger.ReachArc}
		if i%3 == 0 {
			e.Kind, e.Attribute, e.Subject = ledger.KindNumber, "위치", fmt.Sprintf("인물%d", i%3500)
			e.Reach = ""
		}
		if _, err := repo.Add(ctx, 6, e); err != nil {
			t.Fatal(err)
		}
	}

	builder := storycontext.NewContextBuilder(projects, nodes, mentions, thread.NewRepo(st), beat.NewRepo(st),
		note.NewRepo(st), relationship.NewRepo(st)).WithLedgerSource(repo)
	d := ToolDeps{Projects: projects, Nodes: nodes, Entities: entities, Context: builder, Ledger: repo, Source: SourceAgent}

	var times []time.Duration
	worst := 0
	inBrief := map[int]int{}
	for _, at := range []int{19, 99, 299, 499, 699, 899, 999} {
		for range 4 {
			began := time.Now()
			_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: scenes[at].ID}, AllToolGroups())
			if err != nil {
				t.Fatal(err)
			}
			times = append(times, time.Since(began))
			raw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			worst = max(worst, utf8.RuneCount(raw))
		}
		brief, err := repo.ContextLedger(ctx, p.ID, scenes[at].ID)
		if err != nil {
			t.Fatal(err)
		}
		inBrief[at] = len(brief.Due) + len(brief.Open)
	}
	slices.Sort(times)
	p95 := times[len(times)*95/100]
	fmt.Printf("SCALE episodes=%d worst_result=%d p95=%s\n", len(scenes), worst, p95.Round(time.Millisecond))
	if worst > toolResultCap-1000 {
		t.Errorf("a tool result ran to %d runes", worst)
	}
	if p95 > time.Second {
		t.Errorf("p95 = %s", p95)
	}

	for name, read := range map[string]func(string) (int, string, error){
		"ledger": func(c string) (int, string, error) {
			_, out, err := d.listLedger(ctx, nil, listLedgerInput{ProjectID: p.ID, Cursor: c})
			return len(out.Entries), out.NextCursor, err
		},
		"elements": func(c string) (int, string, error) {
			_, out, err := d.listCharacters(ctx, nil, listCharactersInput{ProjectID: p.ID, Cursor: c})
			return len(out.Entities), out.NextCursor, err
		},
		"outline": func(c string) (int, string, error) {
			_, out, err := d.getOutline(ctx, nil, getOutlineInput{ProjectID: p.ID, Cursor: c})
			return len(out.Outline), out.NextCursor, err
		},
	} {
		rows, pages, cursor := 0, 0, ""
		for {
			n, next, err := read(cursor)
			if err != nil {
				t.Fatal(err)
			}
			rows, pages = rows+n, pages+1
			if next == "" {
				break
			}
			cursor = next
		}
		fmt.Printf("SCALE %s rows=%d pages=%d\n", name, rows, pages)
	}
	fmt.Printf("SCALE promises_in_brief=%v\n", inBrief)

	last := scenes[len(scenes)-1]
	got, err := repo.ContextLedger(ctx, p.ID, last.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.Due) + len(got.Open); n > 40 {
		t.Errorf("%d promises in the brief at the end of a thousand episodes", n)
	}
}
