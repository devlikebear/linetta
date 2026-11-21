//go:build !mas && !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tarstools "github.com/devlikebear/tars/pkg/tools"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
)

// The built-in agent's web search (linetta_web_search) runs through the Aside
// browser's CLI: `aside repl` opens a search engine's results page in a
// background tab of the writer's own browser, reads the results off it and
// closes the tab. No model is involved and no search key is needed, so it is
// offered whenever Aside is installed — on a Mac, where Aside runs. tars'
// web_search tool does the reading; this file finds the binary and adapts the
// tool to mcphost.WebSearcher.

// asidePathEnv overrides where the aside binary is looked up (tars' name).
const asidePathEnv = "ASIDE_CLI_PATH"

// findAside returns the aside binary, or "". A desktop app opened from the
// Finder has launchd's PATH, which has none of the install locations, so they
// are checked by name. A stat or two per turn: cheap enough to ask every time,
// which is what lets a writer install Aside without restarting Linetta.
func findAside() string {
	var candidates []string
	if p := strings.TrimSpace(os.Getenv(asidePathEnv)); p != "" {
		candidates = append(candidates, p)
	}
	if p, err := exec.LookPath("aside"); err == nil {
		candidates = append(candidates, p)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", "aside"))
	}
	candidates = append(candidates, "/opt/homebrew/bin/aside", "/usr/local/bin/aside")
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// webSearchAvailable is agent.Deps.WebSearchAvailable.
func webSearchAvailable() bool { return findAside() != "" }

// asideWebSearch is the mcphost.WebSearcher the agent's tools use.
type asideWebSearch struct {
	find func() string
	// tool builds tars' web_search for the binary found. A seam for tests.
	tool func(path string) tarstools.Tool
}

func newAsideWebSearch() mcphost.WebSearcher {
	return asideWebSearch{find: findAside, tool: func(path string) tarstools.Tool {
		return tarstools.NewWebSearchToolWithOptions(tarstools.WebSearchOptions{
			Enabled:   true,
			Provider:  "aside",
			AsidePath: path,
			// The same question asked twice in a turn reads the page once.
			CacheTTL: time.Minute,
		})
	}}
}

// webSearchTimeout bounds one search: a page load in a background tab.
const webSearchTimeout = 45 * time.Second

func (a asideWebSearch) Search(ctx context.Context, query, engine string, count int) (string, error) {
	path := a.find()
	if path == "" {
		return "", errors.New("the Aside browser's CLI is not installed")
	}
	params, err := json.Marshal(struct {
		Query  string `json:"query"`
		Count  int    `json:"count"`
		Engine string `json:"engine,omitempty"`
	}{query, count, engine})
	if err != nil {
		return "", err
	}
	sctx, cancel := context.WithTimeout(ctx, webSearchTimeout)
	defer cancel()
	res, err := a.tool(path).Execute(sctx, params)
	if err != nil {
		return "", err
	}
	if res.IsError {
		var body struct {
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(res.Text()), &body) == nil && body.Message != "" {
			return "", errors.New(body.Message)
		}
		return "", errors.New(res.Text())
	}
	return res.Text(), nil
}
