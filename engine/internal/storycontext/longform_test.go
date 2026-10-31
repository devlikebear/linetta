package storycontext

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

// longWork seeds a work the size that broke the brief in real use: two written
// parts, each scene mentioning the same cast several times, with summaries of
// the length an agent writes.
type longWork struct {
	f      *ctxFixture
	p      project.Project
	scenes []node.Node
	parts  []node.Node
}

func seedLongWork(t *testing.T, episodes int) longWork {
	t.Helper()
	ctx := context.Background()
	f := newCtxFixture(t)
	p := f.project(t, project.NewInput{Title: "장편", LengthTarget: "series", DefaultPOV: "first"})
	style := "문체 기준: 입말 호흡."
	p, err := f.projects.Update(ctx, 1000, project.UpdateInput{ID: p.ID, StyleNotes: &style})
	if err != nil {
		t.Fatal(err)
	}
	er := entity.NewRepo(f.store)

	mentions := ""
	for i := range 12 {
		e, err := er.Create(ctx, 1000, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: fmt.Sprintf("인물%d", i), Role: "주연"})
		if err != nil {
			t.Fatal(err)
		}
		for range 3 {
			mentions += `,{"type":"mention","attrs":{"id":"` + e.ID + `","label":"` + e.Name + `"}}`
		}
	}

	first, err := f.nodes.Get(ctx, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.nodes.Delete(ctx, first.ID, 1001); err != nil {
		t.Fatal(err)
	}
	w := longWork{f: f, p: p}
	for i, label := range []string{"1부", "2부"} {
		part, err := f.nodes.CreateRoot(ctx, p.ID, node.KindContainer, label, "", int64(1010+i))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.nodes.SetWrittenSummary(ctx, part.ID, label+" 줄거리. "+strings.Repeat("부의 흐름을 적은 문장. ", 60), part.ContentVersion); err != nil {
			t.Fatal(err)
		}
		w.parts = append(w.parts, part)
	}
	for i := range episodes {
		label := fmt.Sprintf("%d화", i+1)
		n, err := f.nodes.CreateChild(ctx, w.parts[min(i*2/episodes, 1)].ID, node.KindLeaf, label, "", int64(2000+i))
		if err != nil {
			t.Fatal(err)
		}
		doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + label + ` 본문 "}` + mentions + `]}]}`
		if err := f.nodes.UpdateContent(ctx, n.ID, doc, int64(3000+i)); err != nil {
			t.Fatal(err)
		}
		got, err := f.nodes.Get(ctx, n.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.nodes.SetWrittenSummary(ctx, n.ID, label+" 요약. "+strings.Repeat("이 화에서 벌어진 일을 적은 문장. ", 35), got.ContentVersion); err != nil {
			t.Fatal(err)
		}
		w.scenes = append(w.scenes, got)
	}
	return w
}

func (w longWork) brief(t *testing.T, i int) (Context, string) {
	t.Helper()
	c, err := w.f.builder().Build(context.Background(), w.scenes[i].ID, "", "", Options{Language: "ko"})
	if err != nil {
		t.Fatal(err)
	}
	_, user := Render(c)
	return c, user
}

func TestLongWorkKeepsTheAdjacentScenes(t *testing.T) {
	w := seedLongWork(t, 30)
	c, user := w.brief(t, 15)
	if len(c.Hierarchical.NearbyLeafSummaries) < 2 {
		t.Fatalf("nearby = %d, want the scene before and after", len(c.Hierarchical.NearbyLeafSummaries))
	}
	if !strings.Contains(user, "## 직전·직후 씬 발췌") {
		t.Fatal("brief has no adjacent scenes")
	}
}

func TestLongWorkBriefCarriesEveryWrittenPart(t *testing.T) {
	w := seedLongWork(t, 30)
	_, user := w.brief(t, 3)
	for _, want := range []string{"## 부별 흐름", "1부 줄거리", "2부 줄거리"} {
		if !strings.Contains(user, want) {
			t.Errorf("brief is missing %q", want)
		}
	}
}

func TestPartSummariesFavourTheCurrentPartAndStayInBudget(t *testing.T) {
	w := seedLongWork(t, 30)
	c, _ := w.brief(t, 3)
	parts := c.Hierarchical.PartSummaries
	if len(parts) != 2 {
		t.Fatalf("parts = %d", len(parts))
	}
	total := 0
	for _, s := range parts {
		total += utf8.RuneCountInString(s.Body)
	}
	if total > partSummariesMaxRunes+2 {
		t.Fatalf("parts total %d runes, budget %d", total, partSummariesMaxRunes)
	}
}

func TestDerivedPartRollupsStayOutOfTheBrief(t *testing.T) {
	f := newCtxFixture(t)
	ctx := context.Background()
	p := f.project(t, project.NewInput{Title: "t", LengthTarget: "series", DefaultPOV: "first"})
	part, err := f.nodes.CreateRoot(ctx, p.ID, node.KindContainer, "1권", "", 1500)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.nodes.SetSummary(ctx, part.ID, "자동으로 묶은 요약", part.ContentVersion); err != nil {
		t.Fatal(err)
	}
	c, err := f.builder().Build(ctx, *p.LastOpenedNodeID, "", "", Options{Language: "ko"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Hierarchical.PartSummaries) != 0 {
		t.Fatalf("derived rollup reached the brief: %+v", c.Hierarchical.PartSummaries)
	}
}

func TestEntityDossierNeverReadsAheadOrRepeatsAScene(t *testing.T) {
	w := seedLongWork(t, 12)
	c, _ := w.brief(t, 5)
	for _, e := range c.Entities {
		seen := map[string]bool{}
		for _, line := range e.Recent {
			if seen[line] {
				t.Errorf("%s repeats %q", e.Name, line)
			}
			seen[line] = true
			for _, later := range []string{"[7화]", "[8화]", "[9화]", "[10화]", "[11화]", "[12화]"} {
				if strings.HasPrefix(line, later) {
					t.Errorf("%s reads ahead: %q", e.Name, line)
				}
			}
		}
	}
	shown := append(append([]SceneSummary{}, c.Hierarchical.NearbyLeafSummaries...), c.RelatedScenes...)
	for _, s := range shown {
		label := s.Label[strings.LastIndex(s.Label, " / ")+1:]
		label = strings.TrimSpace(strings.TrimPrefix(label, "/"))
		for _, line := range c.Entities[0].Recent {
			if strings.HasPrefix(line, "["+label+"]") {
				t.Errorf("dossier repeats a scene the brief already shows: %q", line)
			}
		}
	}
}

// The built-in agent caps a tool result at 24,000 runes; everything past that
// never reached the model.
func TestLongWorkBriefFitsAToolResultAndLeadsWithTheStyle(t *testing.T) {
	const toolResultCap = 24000
	w := seedLongWork(t, 100)
	_, user := w.brief(t, 90)
	if n := utf8.RuneCountInString(user); n > toolResultCap {
		t.Fatalf("brief is %d runes", n)
	}
	style, world := strings.Index(user, "문체 기준"), strings.Index(user, "## 세계관 요소")
	if style < 0 || world < 0 || style > world {
		t.Fatalf("style notes at %d, world elements at %d", style, world)
	}
}

func TestUnwrittenSceneBriefCarriesItsPlanAndWhatFollows(t *testing.T) {
	f := newCtxFixture(t)
	ctx := context.Background()
	p := f.project(t, project.NewInput{Title: "t", LengthTarget: "series", DefaultPOV: "first"})
	cur := *p.LastOpenedNodeID
	ids := []string{cur}
	for i := range 4 {
		n, err := f.nodes.CreateSibling(ctx, ids[len(ids)-1], node.KindLeaf, fmt.Sprintf("%d화", i+2), "", int64(1500+i))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}
	for i, id := range ids {
		if err := f.nodes.SetWrittenSummary(ctx, id, fmt.Sprintf("[계획] %d번째 계획", i+1), 0); err != nil {
			t.Fatal(err)
		}
	}
	c, err := f.builder().Build(ctx, cur, "", "", Options{Language: "ko"})
	if err != nil {
		t.Fatal(err)
	}
	_, user := Render(c)
	for _, want := range []string{"## 현재 씬 계획", "1번째 계획", "2번째 계획", "3번째 계획", "4번째 계획"} {
		if !strings.Contains(user, want) {
			t.Errorf("brief is missing %q:\n%s", want, user)
		}
	}
	if strings.Contains(user, "5번째 계획") {
		t.Error("brief reaches past the planned horizon")
	}
}

func TestWrittenSceneHasNoPlanSection(t *testing.T) {
	w := seedLongWork(t, 3)
	_, user := w.brief(t, 1)
	if strings.Contains(user, "## 현재 씬 계획") {
		t.Fatal("a written scene's summary is not a plan")
	}
}

func TestEpisodeTargetRendersOnlyWhenTheWriterSetOne(t *testing.T) {
	f := newCtxFixture(t)
	ctx := context.Background()
	p := f.project(t, project.NewInput{Title: "t", LengthTarget: "series", DefaultPOV: "first"})
	build := func() string {
		c, err := f.builder().Build(ctx, *p.LastOpenedNodeID, "", "", Options{Language: "ko"})
		if err != nil {
			t.Fatal(err)
		}
		_, user := Render(c)
		return user
	}
	if out := build(); strings.Contains(out, "회차 목표 분량") {
		t.Fatalf("default target rendered:\n%s", out)
	}
	target := 3000
	if _, err := f.projects.Update(ctx, 1500, project.UpdateInput{ID: p.ID, EpisodeCharTarget: &target}); err != nil {
		t.Fatal(err)
	}
	if out := build(); !strings.Contains(out, "회차 목표 분량: 3000자") {
		t.Fatalf("target missing:\n%s", out)
	}
}

func TestEntityAttributesRenderInAStableOrder(t *testing.T) {
	c := Context{Entities: []EntityBrief{{Name: "해진", Kind: "character", Attributes: map[string]string{"c": "3", "a": "1", "b": "2", "d": "4"}}}}
	for range 20 {
		if out := buildUser(c); !strings.Contains(out, "(a:1, b:2, c:3, d:4)") {
			t.Fatalf("attributes out of order:\n%s", out)
		}
	}
}

func TestDossierTellsEachSceneOnceAcrossTheCast(t *testing.T) {
	w := seedLongWork(t, 12)
	c, _ := w.brief(t, 10)
	told := map[string]int{}
	for _, e := range c.Entities {
		for _, line := range e.Recent {
			if strings.Contains(line, "요약.") {
				told[line]++
			}
		}
	}
	if len(told) == 0 {
		t.Fatal("no scene was told at all")
	}
	for line, n := range told {
		if n != 1 {
			t.Errorf("told %d times: %q", n, line)
		}
	}
	if got := c.Entities[1].Recent; len(got) == 0 || strings.Contains(got[0], "요약.") {
		t.Errorf("a later entity should get the label alone: %q", got)
	}
}
