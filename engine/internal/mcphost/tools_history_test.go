//go:build !mobile

package mcphost

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newHistoryDeps(t *testing.T) (context.Context, ToolDeps, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	projects := project.NewRepo(st)
	p, err := projects.Create(ctx, 42, project.NewInput{
		Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	d := ToolDeps{
		History:  companion.NewHistoryRepo(st.DB()),
		Memory:   agentmemory.NewRepo(st.DB()),
		Projects: projects,
		Activity: NewActivityRepo(st.DB()),
		Source:   SourceAgent,
	}
	return ctx, d, p.ID
}

func appendHistory(t *testing.T, d ToolDeps, projectID, id, runID, role, content string, at int64) {
	t.Helper()
	if err := d.History.Append(t.Context(), companion.HistoryMessage{
		ID: id, ProjectID: projectID, RunID: runID, Role: role, Intent: companion.HistoryAgentIntent,
		Status: companion.HistoryStatusDone, Content: content, CreatedAt: at,
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
}

// The conversation holds what the writer pasted and said in passing. An
// external client must not be able to reach it through any tool set a switch
// can produce.
func TestHistoryToolsNeverReachAnExternalServer(t *testing.T) {
	for _, groups := range []ToolGroups{AllToolGroups(), ToolGroupsFrom(nil)} {
		for _, mode := range []string{settings.MCPModeFull, settings.MCPModeReadOnly} {
			got := registeredToolNamesWithGroups(t, mode, groups)
			for _, name := range HistoryToolNames {
				if slices.Contains(got, name) {
					t.Errorf("%s reached an external %s server", name, mode)
				}
			}
		}
	}
	for _, name := range HistoryToolNames {
		if slices.Contains(ReadToolNames, name) || slices.Contains(WriteToolNames, name) {
			t.Errorf("%s is on a list external clients are offered", name)
		}
	}
}

func TestTheHistoryGroupRegistersBothTools(t *testing.T) {
	got := registeredToolNamesOf(t, ToolDeps{}, settings.MCPModeFull, ToolGroups{History: true})
	for _, name := range HistoryToolNames {
		if !slices.Contains(got, name) {
			t.Errorf("%s missing from the agent's server", name)
		}
	}
}

func TestSearchHistoryLeavesOutTheRunningTurn(t *testing.T) {
	ctx, d, projectID := newHistoryDeps(t)
	appendHistory(t, d, projectID, "old", "r1", "user", "무형검은 12화에 나온다", 1)
	appendHistory(t, d, projectID, "now", "r2", "user", "무형검 언제 나오지?", 2)

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{MetaRunID: "r2"}}}
	res, out, err := d.searchHistory(ctx, req, searchHistoryInput{ProjectID: projectID, Query: "무형검"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("searchHistory: err=%v res=%+v", err, res)
	}
	if len(out.Hits) != 1 || out.Hits[0].MessageID != "old" {
		t.Fatalf("hits = %+v", out.Hits)
	}
	if out.Hits[0].At == "" || out.Hits[0].Role != "user" || !strings.Contains(out.Hits[0].Snippet, "무형검") {
		t.Errorf("hit = %+v", out.Hits[0])
	}
}

func TestSearchHistoryNamesABoundaryItCannotFind(t *testing.T) {
	ctx, d, projectID := newHistoryDeps(t)
	res, _, err := d.searchHistory(ctx, nil, searchHistoryInput{ProjectID: projectID, Query: "무형검", BeforeMessageID: "gone"})
	if err != nil || res == nil || !res.IsError {
		t.Fatalf("searchHistory: err=%v res=%+v", err, res)
	}
}

func TestReadHistoryCutsAPastedChapterAndCapsItsReach(t *testing.T) {
	ctx, d, projectID := newHistoryDeps(t)
	for i, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		appendHistory(t, d, projectID, id, "r", "user", id, int64(i+1))
	}
	long := strings.Repeat("가", maxHistoryMessageChars+10)
	appendHistory(t, d, projectID, "long", "r", "user", long, 100)

	huge := 99
	res, out, err := d.readHistory(ctx, nil, readHistoryInput{ProjectID: projectID, MessageID: "long", Before: &huge})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("readHistory: err=%v res=%+v", err, res)
	}
	if len(out.Messages) != maxHistoryNeighbours+1 {
		t.Fatalf("got %d messages", len(out.Messages))
	}
	last := out.Messages[len(out.Messages)-1]
	if !last.Truncated || len([]rune(last.Content)) != maxHistoryMessageChars+1 {
		t.Errorf("last = %d runes, truncated %v", len([]rune(last.Content)), last.Truncated)
	}

	none := 0
	res, out, err = d.readHistory(ctx, nil, readHistoryInput{ProjectID: projectID, MessageID: "c", Before: &none, After: &none})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("readHistory: err=%v res=%+v", err, res)
	}
	if len(out.Messages) != 1 || out.Messages[0].MessageID != "c" {
		t.Errorf("zero neighbours = %+v", out.Messages)
	}
}

func TestHistoryToolsRefuseInABuildWithNoDatabase(t *testing.T) {
	ctx, d, projectID := newHistoryDeps(t)
	d.History = nil
	if res, _, _ := d.searchHistory(ctx, nil, searchHistoryInput{ProjectID: projectID, Query: "무형검"}); res == nil || !res.IsError {
		t.Errorf("searchHistory res = %+v", res)
	}
	if res, _, _ := d.readHistory(ctx, nil, readHistoryInput{ProjectID: projectID, MessageID: "x"}); res == nil || !res.IsError {
		t.Errorf("readHistory res = %+v", res)
	}
}

func TestSearchHistoryPutsArchivedNotesFirstAndNamesTheirSource(t *testing.T) {
	ctx, d, projectID := newHistoryDeps(t)
	appendHistory(t, d, projectID, "m1", "r1", "user", "곰배는 조연으로 하자", 1)
	if _, err := d.Memory.Save(ctx, agentmemory.ScopeWorkNotes, projectID, "곰배는 흑풍채 두목", 2); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := d.Memory.Archive(ctx, projectID, "곰배", "m1", 3); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	res, out, err := d.searchHistory(ctx, nil, searchHistoryInput{ProjectID: projectID, Query: "곰배"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("searchHistory: err=%v res=%+v", err, res)
	}
	if len(out.Hits) != 2 {
		t.Fatalf("hits = %+v", out.Hits)
	}
	first, second := out.Hits[0], out.Hits[1]
	if first.Source != historySourceNoteArchive || first.MessageID != "m1" || first.Snippet != "곰배는 흑풍채 두목" {
		t.Errorf("first = %+v", first)
	}
	if second.Source != historySourceConversation || second.Role != "user" {
		t.Errorf("second = %+v", second)
	}
}
