package mcphost

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
)

const toolResultCap = 24000

type capacityLedger struct{}

func (capacityLedger) ContextLedger(context.Context, string, string) (storycontext.LedgerBrief, error) {
	lines := make([]string, 0, 12)
	for i := range 12 {
		lines = append(lines, fmt.Sprintf("[결정] 사실 %d: %s", i, strings.Repeat("정해진 내용 ", 20)))
	}
	// The secrets section at its budget: 1,500 runes of answers and who knows them.
	secrets := make([]string, 0, 10)
	for i := range 10 {
		secrets = append(secrets, fmt.Sprintf("질문 %d — %s (작가만 앎)", i, strings.Repeat("작가만 아는 답 ", 16)))
	}
	// The story time section at its budget: 600 runes of elapsed times.
	times := make([]string, 0, 6)
	for i := range 6 {
		times = append(times, fmt.Sprintf("사건 %d(1화): 3화에서 %s(약 90일)", i, strings.Repeat("석 달 ", 22)))
	}
	return storycontext.LedgerBrief{Due: lines[:6], Settled: lines[6:], Secrets: secrets[:6], Knows: secrets[6:], Time: times}, nil
}

type capacityFacts struct{}

func (capacityFacts) ContextFacts(context.Context, string, string) ([]storycontext.FactBrief, error) {
	cards := make([]storycontext.FactBrief, 0, 12)
	for i := range 12 {
		cards = append(cards, storycontext.FactBrief{ID: fmt.Sprintf("f%d", i), Status: "uncertain", Claim: strings.Repeat("긴 자료 ", 90)})
	}
	return cards, nil
}

// The cap the agent's tool results live under is what the brief must fit, and it
// is measured on the JSON text: a real written scene's brief ran 6,000 to 8,000
// runes past it while every test on the rendered text passed.
func TestAgentBriefFitsTheToolResultCap(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, nil)
	d.Context = d.Context.WithLedgerSource(capacityLedger{}).WithFactSource(capacityFacts{})
	body := strings.Repeat("주인공은 패를 품에 넣고 객잔 문을 나섰다. ", 2000)
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + body + `"}]}]}`
	if err := d.Nodes.UpdateContent(ctx, nodeID, doc, 10); err != nil {
		t.Fatal(err)
	}

	d.Source = SourceAgent
	_, agent, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID}, AllToolGroups())
	if err != nil {
		t.Fatal(err)
	}
	if n := storycontext.EscapedRunes(agent.Brief); n > agentBriefMaxRunes {
		t.Fatalf("the agent's brief is %d runes as JSON", n)
	}
	// The whole result, at its largest: elements_not_in_brief full.
	agent.ElementsNotInBrief = make([]unlistedElement, maxUnlistedElements)
	for i := range agent.ElementsNotInBrief {
		agent.ElementsNotInBrief[i] = unlistedElement{EntityID: strings.Repeat("0", 36), Kind: "character", Name: "이름이 조금 긴 인물"}
	}
	raw, err := json.Marshal(agent)
	if err != nil {
		t.Fatal(err)
	}
	if n := utf8.RuneCount(raw); n > toolResultCap-1000 {
		t.Fatalf("the tool result is %d runes against a cap of %d", n, toolResultCap)
	}
	if strings.Contains(agent.Brief, "객잔 문을 나섰다") {
		t.Error("the agent reads the scene with linetta_read_scene; its brief must not carry it too")
	}

	d.Source = SourceExternal
	_, external, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID}, AllToolGroups())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(external.Brief, "객잔 문을 나섰다") {
		t.Error("an external client's brief must keep the scene")
	}
	if len(external.OmittedSections) != 0 {
		t.Errorf("an external client's brief is not fitted: %v", external.OmittedSections)
	}
}

func TestAgentBriefSaysWhatItLeftOut(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, nil)
	d.Context = d.Context.WithLedgerSource(capacityLedger{}).WithFactSource(capacityFacts{})
	n, err := d.Nodes.Get(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("작가 메모 ", 4500)
	p, err := d.Projects.Update(ctx, 2, project.UpdateInput{ID: n.ProjectID, StyleNotes: &huge})
	if err != nil {
		t.Fatal(err)
	}
	if utf8.RuneCountInString(p.StyleNotes) < 20000 {
		t.Fatal("style notes did not stick")
	}
	d.Source = SourceAgent
	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID}, AllToolGroups())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(out.OmittedSections, "facts") {
		t.Fatalf("omitted = %v", out.OmittedSections)
	}
	if !strings.Contains(out.Brief, "작가 메모") {
		t.Error("style notes are never left out")
	}
}
