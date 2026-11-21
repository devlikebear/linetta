//go:build !mas && !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tarstools "github.com/devlikebear/tars/pkg/tools"
)

func fakeSearchTool(seen *string, res tarstools.Result) func(string) tarstools.Tool {
	return func(string) tarstools.Tool {
		return tarstools.Tool{Execute: func(_ context.Context, params json.RawMessage) (tarstools.Result, error) {
			*seen = string(params)
			return res, nil
		}}
	}
}

func TestAsideWebSearchHandsTheQueryToTarsAndReturnsItsText(t *testing.T) {
	var seen string
	ok := tarstools.JSONTextResult(map[string]any{"provider": "aside:naver", "count": 1}, false)
	s := asideWebSearch{find: func() string { return "/bin/aside" }, tool: fakeSearchTool(&seen, ok)}
	got, err := s.Search(context.Background(), "포도청", "naver", 3)
	if err != nil || !strings.Contains(got, "aside:naver") {
		t.Fatalf("Search = %q, %v", got, err)
	}
	if seen != `{"query":"포도청","count":3,"engine":"naver"}` {
		t.Errorf("params = %s", seen)
	}
}

func TestAsideWebSearchReportsTarsMessageAndAMissingCLI(t *testing.T) {
	var seen string
	blocked := tarstools.JSONTextResult(map[string]any{"message": "the search engine asked for a human check"}, true)
	s := asideWebSearch{find: func() string { return "/bin/aside" }, tool: fakeSearchTool(&seen, blocked)}
	if _, err := s.Search(context.Background(), "x", "", 5); err == nil || err.Error() != "the search engine asked for a human check" {
		t.Errorf("err = %v", err)
	}
	missing := asideWebSearch{find: func() string { return "" }, tool: fakeSearchTool(&seen, blocked)}
	if _, err := missing.Search(context.Background(), "x", "", 5); err == nil {
		t.Error("no CLI was not an error")
	}
}
