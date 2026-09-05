//go:build !mobile

package mcphost

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

type probeIn struct {
	ProjectID string `json:"project_id"`
}

func (p probeIn) scope() (string, string) { return p.ProjectID, "" }

// A Go context value does not survive the MCP transport — verified by spike —
// so the run id rides in _meta. This is the test that pins that decision.
func TestRecord_readsTheRunIDFromMeta(t *testing.T) {
	repo := newActivityRepo(t)
	d := ToolDeps{Activity: repo, Source: SourceAgent}
	h := record(d, "probe", func(context.Context, *mcp.CallToolRequest, probeIn) (*mcp.CallToolResult, any, error) {
		return nil, nil, nil
	})

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "probe"}}
	req.Params.SetMeta(map[string]any{MetaRunID: "run-42"})
	if _, _, err := h(context.Background(), req, probeIn{ProjectID: "p1"}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	got, err := repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].Source != SourceAgent || got[0].RunID != "run-42" {
		t.Errorf("entry = %+v, want source=agent run_id=run-42", got[0])
	}
}

// The ordinary external call: no _meta of its own, and the log says so.
func TestRecord_externalCallWithNoMetaIsLoggedAsExternal(t *testing.T) {
	repo := newActivityRepo(t)
	d := ToolDeps{Activity: repo}
	h := record(d, "probe", func(context.Context, *mcp.CallToolRequest, probeIn) (*mcp.CallToolResult, any, error) {
		return nil, nil, nil
	})

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "probe"}}
	if _, _, err := h(context.Background(), req, probeIn{ProjectID: "p1"}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	got, err := repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].Source != SourceExternal || got[0].RunID != "" {
		t.Errorf("entry = %+v, want an external row with no run id", got[0])
	}
}

// An external client can put anything in _meta. It must not be able to claim
// it is the built-in agent, or the activity log stops being evidence.
func TestRecord_externalCallerCannotClaimARunID(t *testing.T) {
	repo := newActivityRepo(t)
	d := ToolDeps{Activity: repo} // Source unset: external
	h := record(d, "probe", func(context.Context, *mcp.CallToolRequest, probeIn) (*mcp.CallToolResult, any, error) {
		return nil, nil, nil
	})

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "probe"}}
	req.Params.SetMeta(map[string]any{MetaRunID: "forged"})
	if _, _, err := h(context.Background(), req, probeIn{ProjectID: "p1"}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	got, err := repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].Source != SourceExternal || got[0].RunID != "" {
		t.Errorf("entry = %+v, want an unforgeable external row", got[0])
	}
}

// mcp_project_id restricts external clients to one work. The built-in agent's
// scope is the work whose panel is open, so the restriction must not apply —
// otherwise a writer who locked MCP to one work loses the panel everywhere else.
func TestAllowedProjectID_doesNotRestrictTheAgent(t *testing.T) {
	st := newRestrictedSettings(t, "p-locked")
	if got := (ToolDeps{Settings: st}).allowedProjectID(); got != "p-locked" {
		t.Errorf("external allowedProjectID = %q, want p-locked", got)
	}
	if got := (ToolDeps{Settings: st, Source: SourceAgent}).allowedProjectID(); got != "" {
		t.Errorf("agent allowedProjectID = %q, want empty", got)
	}
}

func newRestrictedSettings(t *testing.T, projectID string) *settings.Store {
	t.Helper()
	t.Setenv("LINETTA_HOME", t.TempDir())
	st, err := settings.NewWithSecretStore(settings.NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if _, err := st.Set(context.Background(), settings.Patch{MCPProjectID: &projectID}); err != nil {
		t.Fatalf("restrict: %v", err)
	}
	return st
}
