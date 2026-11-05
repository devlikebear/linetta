package storycontext

import (
	"context"
	"fmt"
	"slices"
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
	if out := build(); !strings.Contains(out, "회차 목표 분량: 3000자(공백 포함)") {
		t.Fatalf("target missing:\n%s", out)
	}
}

func TestEpisodeTargetNamesTheWorksCharCountBasis(t *testing.T) {
	f := newCtxFixture(t)
	ctx := context.Background()
	p := f.project(t, project.NewInput{Title: "t", LengthTarget: "series", DefaultPOV: "first"})
	without := "without_spaces"
	if _, err := f.projects.Update(ctx, 1500, project.UpdateInput{ID: p.ID, CharCountBasis: &without}); err != nil {
		t.Fatal(err)
	}
	c, err := f.builder().Build(ctx, *p.LastOpenedNodeID, "", "", Options{Language: "ko"})
	if err != nil {
		t.Fatal(err)
	}
	// The default length means a different length without spaces, so it is
	// said once the writer chose that basis.
	if _, user := Render(c); !strings.Contains(user, "회차 목표 분량: 5000자(공백 제외)") {
		t.Fatalf("basis missing:\n%s", user)
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

// realisticWork seeds a work at the size the 2026-11 measurements found: the
// fixture above kept its cards short, which is how a brief that overran the
// tool cap in real use still passed. Every string here is made up.
type fakeLedger struct{ brief LedgerBrief }

func (f fakeLedger) ContextLedger(context.Context, string, string) (LedgerBrief, error) {
	return f.brief, nil
}

type fakeFacts struct{ cards []FactBrief }

func (f fakeFacts) ContextFacts(context.Context, string, string) ([]FactBrief, error) {
	return f.cards, nil
}

func seedRealisticWork(t *testing.T) (longWork, *ContextBuilder) {
	t.Helper()
	ctx := context.Background()
	f := newCtxFixture(t)
	p := f.project(t, project.NewInput{Title: "장편", LengthTarget: "series", DefaultPOV: "first"})
	style := "문체 기준: 입말 호흡. " + strings.Repeat("문장 호흡과 대사 규칙을 적은 작가 메모. ", 60)
	p, err := f.projects.Update(ctx, 1000, project.UpdateInput{ID: p.ID, StyleNotes: &style})
	if err != nil {
		t.Fatal(err)
	}
	er := entity.NewRepo(f.store)
	mentions := ""
	for i := range 56 {
		e, err := er.Create(ctx, 1000, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: fmt.Sprintf("인물%d", i), Role: "조연"})
		if err != nil {
			t.Fatal(err)
		}
		summary := strings.Repeat("이 요소의 배경과 사정을 적은 설명. ", 12)
		attrs := map[string]string{"외모": strings.Repeat("긴 묘사 ", 15), "말투": strings.Repeat("긴 묘사 ", 15)}
		if err := er.Update(ctx, 1000, entity.UpdateInput{ID: e.ID, Summary: &summary, Attributes: &attrs}); err != nil {
			t.Fatal(err)
		}
		if i < 16 {
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
	part, err := f.nodes.CreateRoot(ctx, p.ID, node.KindContainer, "1부", "", 1010)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.nodes.SetWrittenSummary(ctx, part.ID, strings.Repeat("부의 흐름을 적은 문장. ", 70), part.ContentVersion); err != nil {
		t.Fatal(err)
	}
	w.parts = append(w.parts, part)
	body := strings.Repeat("주인공은 패를 품에 넣고 객잔 문을 나섰다. ", 170)
	for i := range 25 {
		label := fmt.Sprintf("%d화", i+1)
		n, err := f.nodes.CreateChild(ctx, part.ID, node.KindLeaf, label, "", int64(2000+i))
		if err != nil {
			t.Fatal(err)
		}
		if i < 16 {
			doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + body + `"}` + mentions + `]}]}`
			if err := f.nodes.UpdateContent(ctx, n.ID, doc, int64(3000+i)); err != nil {
				t.Fatal(err)
			}
			got, err := f.nodes.Get(ctx, n.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.nodes.SetWrittenSummary(ctx, n.ID, label+" 요약. "+strings.Repeat("이 화에서 벌어진 일. ", 70), got.ContentVersion); err != nil {
				t.Fatal(err)
			}
		}
		got, err := f.nodes.Get(ctx, n.ID)
		if err != nil {
			t.Fatal(err)
		}
		w.scenes = append(w.scenes, got)
	}
	owed := make([]string, 0, 9)
	for i := range 9 {
		owed = append(owed, fmt.Sprintf("[복선] 약속 %d: %s", i, strings.Repeat("회수해야 할 내용 ", 22)))
	}
	cards := make([]FactBrief, 0, 12)
	for i := range 12 {
		cards = append(cards, FactBrief{ID: fmt.Sprintf("fact-%d", i), Status: "uncertain", Claim: strings.Repeat("일반 지식을 적은 카드 ", 30)})
	}
	b := f.builder().
		WithLedgerSource(fakeLedger{brief: LedgerBrief{Due: owed[:5], Open: owed[5:], Omitted: 36}}).
		WithFactSource(fakeFacts{cards: cards})
	return w, b
}

func briefOf(t *testing.T, b *ContextBuilder, nodeID string) Context {
	t.Helper()
	c, err := b.BuildFull(context.Background(), nodeID, "", "", Options{Language: "ko"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The world section and the fact dossier were what ran off the end of a real
// brief. Both are bounded now, whichever scene the brief is for.
func TestRealisticWorkKeepsWorldAndFactsInBudget(t *testing.T) {
	w, b := seedRealisticWork(t)
	for _, i := range []int{9, 15, 16, 24} {
		c := briefOf(t, b, w.scenes[i].ID)
		world := 0
		for _, e := range c.Entities {
			world += entityCardRunes(e)
			for _, line := range e.Recent {
				world += utf8.RuneCountInString(line) + 5
			}
		}
		if world > entityBriefMaxRunes {
			t.Errorf("scene %d: world section is %d runes", i, world)
		}
		facts := 0
		for _, f := range c.Facts {
			facts += factCardRunes(f)
		}
		if facts > factBriefMaxRunes {
			t.Errorf("scene %d: fact dossier is %d runes", i, facts)
		}
		if c.FactsOmitted == 0 {
			t.Errorf("scene %d: a shortened dossier must say so", i)
		}
	}
}

func TestSceneMentionsKeepTheirCardsAheadOfTheCore(t *testing.T) {
	w, b := seedRealisticWork(t)
	c := briefOf(t, b, w.scenes[15].ID)
	if len(c.Entities) == 0 || c.Entities[0].Name != "인물0" {
		t.Fatalf("the scene's own cast must come first: %+v", c.Entities)
	}
	for _, e := range c.Entities {
		if e.Summary == "" {
			t.Fatalf("a card lost its summary: %+v", e)
		}
	}
}

func TestSceneFactsComeBeforeTheWorksGeneralCards(t *testing.T) {
	cards := []FactBrief{
		{ID: "general", Status: "uncertain", Claim: strings.Repeat("일반 ", 400)},
		{ID: "work", Status: "intentional_fiction", Claim: strings.Repeat("설정 ", 200)},
		{ID: "scene", Status: "uncertain", Claim: strings.Repeat("회차 ", 200), Scene: true},
	}
	got, omitted := budgetFacts(cards)
	if len(got) != 2 || got[0].ID != "scene" || got[1].ID != "work" || omitted != 1 {
		t.Fatalf("got %+v, omitted %d", got, omitted)
	}
}

func TestFitBriefDropsTheBackOfTheBriefAndSaysWhat(t *testing.T) {
	w, b := seedRealisticWork(t)
	c := briefOf(t, b, w.scenes[15].ID)
	brief, omitted := FitBrief(c, 12000)
	if n := EscapedRunes(brief); n > 12000 {
		t.Fatalf("brief is %d runes", n)
	}
	if !slices.Contains(omitted, "facts") || !slices.Contains(omitted, "entities") {
		t.Fatalf("omitted = %v", omitted)
	}
	for _, keep := range []string{"문체 기준", "## 지금 회수할 것"} {
		if !strings.Contains(brief, keep) {
			t.Errorf("fitting dropped %q", keep)
		}
	}
}

func TestFitBriefLeavesABriefThatFitsAlone(t *testing.T) {
	w, b := seedRealisticWork(t)
	c := briefOf(t, b, w.scenes[15].ID)
	_, want := Render(c)
	got, omitted := FitBrief(c, 1_000_000)
	if got != want || len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
}
