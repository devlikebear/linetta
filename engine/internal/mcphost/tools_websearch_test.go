//go:build !mobile

package mcphost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

func TestWebSearchToolIsRegisteredOnlyForItsGroup(t *testing.T) {
	if slices.Contains(registeredToolNamesOf(t, ToolDeps{}, settings.MCPModeFull, AllToolGroups()), WebSearchToolNames[0]) {
		t.Fatalf("%s must stay off unless asked for", WebSearchToolNames[0])
	}
	groups := AllToolGroups()
	groups.WebSearch = true
	if !slices.Contains(registeredToolNamesOf(t, ToolDeps{}, settings.MCPModeReadOnly, groups), WebSearchToolNames[0]) {
		t.Fatalf("%s missing with the group on", WebSearchToolNames[0])
	}
}

func TestToolGroupsFromLeavesWebSearchToTheBuiltInAgent(t *testing.T) {
	store := storeWithGroups(t, AllToolGroups())
	url := "http://127.0.0.1:29483"
	if _, err := store.Set(context.Background(), settings.Patch{WebSearchBaseURL: &url}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if ToolGroupsFrom(store).WebSearch {
		t.Fatal("an external MCP client must not get the writer's search server")
	}
}

func TestWebSearchHandler(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"t","url":"https://example.com","content":"c"}]}`))
	}))
	defer ts.Close()
	store := storeWithGroups(t, AllToolGroups())
	d := ToolDeps{Settings: store}

	if _, _, err := d.webSearch(context.Background(), nil, webSearchInput{Query: "  "}); err == nil {
		t.Fatal("blank query accepted")
	}
	if _, _, err := (ToolDeps{}).webSearch(context.Background(), nil, webSearchInput{Query: "q"}); err == nil {
		t.Fatal("no settings accepted")
	}
	_, _, err := d.webSearch(context.Background(), nil, webSearchInput{Query: "q"})
	if err == nil || !strings.Contains(err.Error(), "Settings") {
		t.Fatalf("unset address error = %v", err)
	}

	url := ts.URL
	if _, err := store.Set(context.Background(), settings.Patch{WebSearchBaseURL: &url}); err != nil {
		t.Fatalf("set: %v", err)
	}
	_, out, err := d.webSearch(context.Background(), nil, webSearchInput{Query: "q"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].URL != "https://example.com" {
		t.Fatalf("results = %+v", out.Results)
	}
}
