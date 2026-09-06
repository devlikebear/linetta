//go:build !mobile

package mcphost

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

// registeredToolNames asks a server registered for one mode what tools it
// actually serves, over a real tools/list.
//
// mcp.NewInMemoryTransports is what makes this possible in this package: it
// is a genuine client/server pair with no listener, no port and no keychain,
// so the assertion below runs anywhere — including a machine whose locked
// keychain hangs internal/engineapp's own server fixtures. The mode's tool
// set is otherwise only observable from outside the process (mcp.Server's
// tool map is unexported), which is how a tool registered on the wrong side
// of the split would go unnoticed.
//
// ToolDeps is deliberately zero-valued: registration reads no collaborator,
// and no handler is called here.
func registeredToolNames(t *testing.T, mode string) []string {
	t.Helper()
	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{
		Name: ServerName, Version: ServerVersion,
	}, nil)
	ToolDeps{}.Register(srv, mode)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	var names []string
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		names = append(names, tool.Name)
	}
	return names
}

// A read_only server does not merely refuse writes — the write tools are
// absent from tools/list, so a connected client cannot call them at all.
// This is the assertion that catches a tool registered on the wrong side of
// the split, which is the difference between a read-only server and one that
// hands out writes.
func TestRegisterServesExactlyTheDocumentedToolsForEachMode(t *testing.T) {
	full := append(append([]string{}, ReadToolNames...), WriteToolNames...)
	for _, tc := range []struct {
		mode string
		want []string
	}{
		{settings.MCPModeReadOnly, ReadToolNames},
		{settings.MCPModeFull, full},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			got := registeredToolNames(t, tc.mode)
			served := map[string]bool{}
			for _, n := range got {
				if served[n] {
					t.Errorf("%s: tool %q registered twice", tc.mode, n)
				}
				served[n] = true
			}
			for _, n := range tc.want {
				if !served[n] {
					t.Errorf("%s: %q is documented but not served", tc.mode, n)
				}
				delete(served, n)
			}
			for n := range served {
				t.Errorf("%s: %q is served but not in the documented list for this mode", tc.mode, n)
			}
		})
	}
}

// The two names Task 5 added, on the sides the brief put them: the read tool
// is on a read_only server, the write tool is not.
func TestReadOnlyServesReadSkillAndNotEditSkill(t *testing.T) {
	served := map[string]bool{}
	for _, n := range registeredToolNames(t, settings.MCPModeReadOnly) {
		served[n] = true
	}
	if !served["linetta_read_skill"] {
		t.Error("a read_only server must serve linetta_read_skill")
	}
	if served["linetta_edit_skill"] {
		t.Error("a read_only server must not serve linetta_edit_skill — it would hand out a way to rewrite the writer's skills")
	}
}
