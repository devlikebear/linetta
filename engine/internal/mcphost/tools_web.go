package mcphost

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WebToolNames lists the tools ToolGroups.Web registers. Like
// HistoryToolNames they are in neither ReadToolNames nor WriteToolNames: an
// external client has a web of its own, and these are the built-in agent's.
var WebToolNames = []string{
	"linetta_web_search",
}

// WebSearcher looks a query up on the web and returns the results as text
// the model reads. engineapp backs it with the Aside browser's CLI, which
// reads a search engine's results page in the writer's own browser.
type WebSearcher interface {
	Search(ctx context.Context, query, engine string, count int) (string, error)
}

const (
	// maxWebQueryRunes bounds a query. A search is a question, not a passage:
	// a query long enough to carry a paragraph of the manuscript is refused.
	maxWebQueryRunes = 200
	maxWebResults    = 10
)

// ---------- linetta_web_search ----------

type webSearchInput struct {
	Query  string `json:"query" jsonschema:"what to look up, in your own words; never the manuscript's text"`
	Engine string `json:"engine,omitempty" jsonschema:"google (default), naver, or duckduckgo"`
	Count  int    `json:"count,omitempty" jsonschema:"results to return, 1-10, default 5"`
}

type webSearchOutput struct {
	Results string `json:"results"`
}

func (d ToolDeps) registerWebTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_web_search",
		Description: "Search the web for real-world facts — places, history, jobs, how things work — and get " +
			"titles, links and snippets. Runs in the writer's own browser. The query leaves this computer: " +
			"ask in general terms and never paste the manuscript or the work's invented names into it.",
	}, record(d, "linetta_web_search", d.webSearch))
}

func (d ToolDeps) webSearch(ctx context.Context, _ *mcp.CallToolRequest, in webSearchInput) (*mcp.CallToolResult, webSearchOutput, error) {
	if d.WebSearch == nil {
		return toolErr("web search is not available on this computer"), webSearchOutput{}, nil
	}
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return toolErr("query is required"), webSearchOutput{}, nil
	}
	if n := utf8.RuneCountInString(query); n > maxWebQueryRunes {
		return toolErr("query is %d characters; the limit is %d — ask the question, do not paste text", n, maxWebQueryRunes), webSearchOutput{}, nil
	}
	engine := strings.ToLower(strings.TrimSpace(in.Engine))
	switch engine {
	case "", "google", "naver", "duckduckgo":
	default:
		return toolErr("engine must be google, naver or duckduckgo"), webSearchOutput{}, nil
	}
	count := in.Count
	if count <= 0 {
		count = 5
	}
	if count > maxWebResults {
		count = maxWebResults
	}
	text, err := d.WebSearch.Search(ctx, query, engine, count)
	if err != nil {
		return toolErr("web search failed: %v", err), webSearchOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, webSearchOutput{Results: text}, nil
}
