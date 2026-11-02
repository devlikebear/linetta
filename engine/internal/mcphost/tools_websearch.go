//go:build !mobile

package mcphost

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/websearch"
)

// WebSearchToolNames is registered only for ToolGroups.WebSearch. The desktop
// panel's parity test reads this literal.
var WebSearchToolNames = []string{"linetta_web_search"}

type webSearchInput struct {
	Query      string `json:"query" jsonschema:"keywords for the fact to check, never manuscript text"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"1 to 10, default 5"`
}

type webSearchOutput struct {
	Results []websearch.Result `json:"results"`
}

func (d ToolDeps) registerWebSearchTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: WebSearchToolNames[0],
		Description: "Search the web through the writer's RustXNG server and return titles, URLs and short " +
			"snippets. Use it to check real-world facts such as history, places and terms. The query leaves " +
			"this computer for outside search engines, so write only the keywords you need: never paste " +
			"manuscript text, character names or plot details into it.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, record(d, WebSearchToolNames[0], d.webSearch))
}

func (d ToolDeps) webSearch(ctx context.Context, _ *mcp.CallToolRequest, in webSearchInput) (*mcp.CallToolResult, webSearchOutput, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return nil, webSearchOutput{}, errors.New("query is required")
	}
	if d.Settings == nil {
		return nil, webSearchOutput{}, errors.New("web search is not available")
	}
	base := d.Settings.WebSearchBaseURL()
	if base == "" {
		return nil, webSearchOutput{}, errors.New("web search is off: set the RustXNG address in Settings")
	}
	results, err := websearch.Client{}.Search(ctx, base, query, in.MaxResults)
	if err != nil {
		return nil, webSearchOutput{}, err
	}
	return nil, webSearchOutput{Results: results}, nil
}
