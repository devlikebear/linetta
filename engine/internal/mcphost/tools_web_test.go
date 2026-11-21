package mcphost

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

type fakeWebSearcher struct {
	query, engine string
	count         int
	err           error
}

func (f *fakeWebSearcher) Search(_ context.Context, query, engine string, count int) (string, error) {
	f.query, f.engine, f.count = query, engine, count
	if f.err != nil {
		return "", f.err
	}
	return "1. 조선시대 포도청 — https://example.test", nil
}

// An external client has a web of its own; the search tool is the built-in
// agent's, and no switch puts it on an external server.
func TestWebSearchNeverReachesAnExternalServer(t *testing.T) {
	for _, groups := range []ToolGroups{AllToolGroups(), ToolGroupsFrom(nil)} {
		for _, mode := range []string{settings.MCPModeFull, settings.MCPModeReadOnly} {
			if slices.Contains(registeredToolNamesWithGroups(t, mode, groups), "linetta_web_search") {
				t.Errorf("linetta_web_search reached an external %s server", mode)
			}
		}
	}
	for _, name := range WebToolNames {
		if slices.Contains(ReadToolNames, name) || slices.Contains(WriteToolNames, name) {
			t.Errorf("%s is on a list external clients are offered", name)
		}
	}
}

func TestTheWebGroupRegistersTheSearchTool(t *testing.T) {
	got := registeredToolNamesOf(t, ToolDeps{}, settings.MCPModeFull, ToolGroups{Web: true})
	if !slices.Contains(got, "linetta_web_search") {
		t.Fatalf("linetta_web_search missing: %v", got)
	}
	if trial := registeredToolNamesOf(t, ToolDeps{}, settings.MCPModeFull, ToolGroups{Web: true, Trial: true, Review: true}); slices.Contains(trial, "linetta_web_search") {
		t.Error("the trial serves linetta_web_search")
	}
}

func TestWebSearchPassesTheQueryOnAndBoundsIt(t *testing.T) {
	f := &fakeWebSearcher{}
	d := ToolDeps{WebSearch: f, Source: SourceAgent}
	res, out, err := d.webSearch(context.Background(), nil, webSearchInput{Query: " 조선 포도청 조직 ", Engine: "Naver", Count: 50})
	if err != nil || res == nil || res.IsError {
		t.Fatalf("webSearch: err=%v res=%+v", err, res)
	}
	if f.query != "조선 포도청 조직" || f.engine != "naver" || f.count != maxWebResults {
		t.Errorf("searcher got %q %q %d", f.query, f.engine, f.count)
	}
	if !strings.Contains(out.Results, "포도청") {
		t.Errorf("results = %q", out.Results)
	}

	for _, in := range []webSearchInput{
		{Query: ""},
		{Query: strings.Repeat("가", maxWebQueryRunes+1)},
		{Query: "x", Engine: "bing"},
	} {
		if res, _, _ := d.webSearch(context.Background(), nil, in); res == nil || !res.IsError {
			t.Errorf("%+v was not refused", in)
		}
	}
	f.err = errors.New("the search engine asked for a human check")
	if res, _, _ := d.webSearch(context.Background(), nil, webSearchInput{Query: "x"}); res == nil || !res.IsError {
		t.Error("a failed search is not an error result")
	}
	if res, _, _ := (ToolDeps{}).webSearch(context.Background(), nil, webSearchInput{Query: "x"}); res == nil || !res.IsError {
		t.Error("no searcher is not refused")
	}
}
