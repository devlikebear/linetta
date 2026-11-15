package agent

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
)

func organizeService(t *testing.T, c *passClaudeCode, withPropose bool) *Service {
	t.Helper()
	rec := &recorder{}
	var reads, writes atomic.Int32
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers: cliProviders{client: c},
		History:   companion.NewHistoryRepo(st.DB()),
		Scope:     fakeScope{titles: map[string]string{"p1": "제목"}},
		Register: func(s *mcp.Server, g mcphost.ToolGroups) {
			passTools(&reads, &writes)(s, g)
			if withPropose {
				mcp.AddTool(s, &mcp.Tool{Name: "linetta_propose_story_ops", Description: "propose"},
					func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
						return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "{}"}}}, struct{}{}, nil
					})
			}
		},
		Notify:       rec.notify,
		Language:     func() string { return "ko" },
		Clock:        func() int64 { return 1700000000000 },
		MaxToolCalls: func() int { return 10 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func TestOrganizeReadsAndProposesOnly(t *testing.T) {
	c := &passClaudeCode{systems: []string{"이미 턴을 쓴 것으로 친다"}, offered: [][]string{nil}}
	svc := organizeService(t, c, true)
	outcome := make(chan string, 1)
	err := svc.StartOrganize(context.Background(), OrganizeRequest{
		ProjectID: "p1", EpisodeLabel: "12화", Language: "ko",
		Scenes: []OrganizeScene{{NodeID: "s1"}, {NodeID: "s2", LedgerSettled: true}},
	}, func(_ context.Context, o string) { outcome <- o })
	if err != nil {
		t.Fatal(err)
	}
	if got := <-outcome; got != "done" {
		t.Fatalf("outcome = %q", got)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.systems[1], "tidying up") {
		t.Fatalf("system = %.60q", c.systems[1])
	}
	user := c.lastMessages[1].Content
	for _, want := range []string{"자동 정리 · 12화", "- s1\n", "- s2 (ledger already settled)"} {
		if !strings.Contains(user, want) {
			t.Fatalf("request lacks %q:\n%s", want, user)
		}
	}
	offered := c.offered[1]
	if !slices.Contains(offered, "linetta_propose_story_ops") || !slices.Contains(offered, "linetta_read_scene") ||
		slices.Contains(offered, "linetta_report_continuity") || slices.Contains(offered, "linetta_write_scene") {
		t.Fatalf("offered %v", offered)
	}
}

func TestOrganizeNeedsTheInbox(t *testing.T) {
	svc := organizeService(t, &passClaudeCode{}, false)
	err := svc.StartOrganize(context.Background(), OrganizeRequest{ProjectID: "p1", Scenes: []OrganizeScene{{NodeID: "s1"}}}, nil)
	if !errors.Is(err, ErrOrganizeUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestOrganizeWithNothingToReadStartsNothing(t *testing.T) {
	c := &passClaudeCode{}
	svc := organizeService(t, c, true)
	if err := svc.StartOrganize(context.Background(), OrganizeRequest{ProjectID: "p1"}, nil); err != nil {
		t.Fatal(err)
	}
	if c.passes() != 0 {
		t.Fatalf("passes = %d", c.passes())
	}
}
