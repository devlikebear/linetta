//go:build !mobile

package engineapp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
)

// mcpClient drives the live HTTP endpoint the way an external agent does:
// initialize, then real MCP calls. Responses come back as SSE, so each call
// reads the first data: line.
type mcpClient struct {
	t         *testing.T
	url       string
	token     string
	sessionID string
	nextID    int
}

func startMCPServer(t *testing.T) (*App, *mcpClient) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	app, err := Open(context.Background(), Options{Home: home})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	port := freeTestPort(t)
	patch := fmt.Sprintf(
		`{"mcp_mode":"read_only","mcp_port":%d,"mcp_consent_version":1,"mcp_consented_at":1}`, port)
	if _, rpcErr := call(t, app, "settings.set", patch); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	if _, rpcErr := call(t, app, "mcp.enable", ""); rpcErr != nil {
		t.Fatalf("mcp.enable: %+v", rpcErr)
	}
	d, err := mcphost.ReadDiscoveryFile(home)
	if err != nil {
		t.Fatalf("ReadDiscoveryFile: %v", err)
	}
	c := &mcpClient{t: t, url: fmt.Sprintf("http://127.0.0.1:%d/mcp", d.Port), token: d.Token}
	c.initialize()
	return app, c
}

func (c *mcpClient) rpc(method string, params any) map[string]any {
	c.t.Helper()
	c.nextID++
	payload := map[string]any{"jsonrpc": "2.0", "id": c.nextID, "method": method}
	if params != nil {
		payload["params"] = params
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		c.t.Fatalf("marshal %s: %v", method, err)
	}
	req, err := http.NewRequest(http.MethodPost, c.url, strings.NewReader(string(raw)))
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s: %v", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.t.Fatalf("%s: status %d", method, resp.StatusCode)
	}
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
		c.sessionID = id
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(line[len("data:"):])), &envelope); err != nil {
			c.t.Fatalf("decode %s event: %v", method, err)
		}
		return envelope
	}
	c.t.Fatalf("%s: no data event in response", method)
	return nil
}

func (c *mcpClient) initialize() {
	c.t.Helper()
	c.rpc("initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "engineapp-test", "version": "1"},
	})
}

func (c *mcpClient) toolNames() []string {
	c.t.Helper()
	envelope := c.rpc("tools/list", map[string]any{})
	result, _ := envelope["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if name, ok := tool["name"].(string); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (c *mcpClient) callTool(name string, args map[string]any) map[string]any {
	c.t.Helper()
	envelope := c.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	if errObj, ok := envelope["error"]; ok {
		c.t.Fatalf("tools/call %s transport error: %v", name, errObj)
	}
	result, _ := envelope["result"].(map[string]any)
	return result
}

// The read surface is a contract external agents build against: it must be
// exactly the nine documented tools, and read_only must expose no others.
func TestMCPReadOnlyExposesExactlyTheReadTools(t *testing.T) {
	_, c := startMCPServer(t)

	got := c.toolNames()
	want := append([]string{}, mcphost.ReadToolNames...)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("tools/list returned %d tools (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tools/list = %v, want %v", got, want)
		}
	}
}

// An end-to-end read: list works, walk the outline, read a scene, and confirm
// the content_version a later write would have to present.
func TestMCPReadToolsRoundTrip(t *testing.T) {
	app, c := startMCPServer(t)

	created, rpcErr := call(t, app, "projects.create", `{"title":"MCP 왕복","genres":["fantasy"],"length_target":"short","default_pov":"first"}`)
	if rpcErr != nil {
		t.Fatalf("projects.create: %+v", rpcErr)
	}
	var proj struct {
		ID               string  `json:"id"`
		LastOpenedNodeID *string `json:"last_opened_node_id"`
	}
	if err := json.Unmarshal(created, &proj); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	works := c.callTool("linetta_list_works", map[string]any{})
	if isToolError(works) {
		t.Fatalf("linetta_list_works errored: %v", works)
	}
	if !strings.Contains(structuredJSON(t, works), proj.ID) {
		t.Fatalf("new work missing from linetta_list_works: %s", structuredJSON(t, works))
	}

	outline := c.callTool("linetta_get_outline", map[string]any{"project_id": proj.ID})
	if isToolError(outline) {
		t.Fatalf("linetta_get_outline errored: %v", outline)
	}

	scene := c.callTool("linetta_read_scene", map[string]any{"node_id": *proj.LastOpenedNodeID})
	if isToolError(scene) {
		t.Fatalf("linetta_read_scene errored: %v", scene)
	}
	var readOut struct {
		NodeID         string `json:"node_id"`
		ContentVersion int    `json:"content_version"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, scene)), &readOut); err != nil {
		t.Fatalf("decode read_scene: %v", err)
	}
	if readOut.NodeID != *proj.LastOpenedNodeID {
		t.Fatalf("read_scene returned node %q, want %q", readOut.NodeID, *proj.LastOpenedNodeID)
	}

	// The brief must come back complete and error-free with no LLM provider
	// reachable — the whole bring-your-own-agent premise rests on this.
	brief := c.callTool("linetta_get_story_context", map[string]any{"node_id": *proj.LastOpenedNodeID})
	if isToolError(brief) {
		t.Fatalf("linetta_get_story_context errored: %v", brief)
	}
	var briefOut struct {
		Brief            string   `json:"brief"`
		IncludedSections []string `json:"included_sections"`
		EmptySections    []string `json:"empty_sections"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, brief)), &briefOut); err != nil {
		t.Fatalf("decode story context: %v", err)
	}
	if len(briefOut.IncludedSections)+len(briefOut.EmptySections) == 0 {
		t.Fatal("story context reported no sections at all")
	}
}

// A bad id is a tool error the agent can act on, never a transport failure.
func TestMCPUnknownIDsReturnToolErrors(t *testing.T) {
	_, c := startMCPServer(t)
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"linetta_get_outline", map[string]any{"project_id": "no-such-work"}},
		{"linetta_read_scene", map[string]any{"node_id": "no-such-scene"}},
		{"linetta_get_story_context", map[string]any{"node_id": "no-such-scene"}},
		{"linetta_where_does_appear", map[string]any{"entity_id": "no-such-entity"}},
	} {
		result := c.callTool(tc.tool, tc.args)
		if !isToolError(result) {
			t.Errorf("%s with a bad id should return a tool error, got %v", tc.tool, result)
		}
	}
}

// Every call lands in the audit trail, successes and failures alike.
func TestMCPCallsAreRecordedInActivity(t *testing.T) {
	app, c := startMCPServer(t)
	c.callTool("linetta_list_works", map[string]any{})
	c.callTool("linetta_read_scene", map[string]any{"node_id": "no-such-scene"})

	raw, rpcErr := call(t, app, "mcp.activity", `{"limit":10}`)
	if rpcErr != nil {
		t.Fatalf("mcp.activity: %+v", rpcErr)
	}
	var entries []struct {
		Tool string `json:"tool"`
		OK   bool   `json:"ok"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("activity has %d entries, want the two calls just made", len(entries))
	}
	sawOK, sawFail := false, false
	for _, e := range entries {
		if e.Tool == "linetta_list_works" && e.OK {
			sawOK = true
		}
		if e.Tool == "linetta_read_scene" && !e.OK {
			sawFail = true
		}
	}
	if !sawOK || !sawFail {
		t.Fatalf("activity must record both outcomes; entries = %+v", entries)
	}
}

func isToolError(result map[string]any) bool {
	v, _ := result["isError"].(bool)
	return v
}

// structuredJSON returns the tool's structured output as JSON text.
func structuredJSON(t *testing.T, result map[string]any) string {
	t.Helper()
	if sc, ok := result["structuredContent"]; ok {
		raw, err := json.Marshal(sc)
		if err != nil {
			t.Fatalf("marshal structured content: %v", err)
		}
		return string(raw)
	}
	content, _ := result["content"].([]any)
	for _, raw := range content {
		block, _ := raw.(map[string]any)
		if text, ok := block["text"].(string); ok {
			return text
		}
	}
	return ""
}

// A server restricted to one work must not read another work's data, even
// when the agent supplies a valid id for it.
func TestMCPProjectRestrictionBlocksOtherWorks(t *testing.T) {
	app, c := startMCPServer(t)

	mk := func(title string) (string, string) {
		t.Helper()
		raw, rpcErr := call(t, app, "projects.create",
			fmt.Sprintf(`{"title":%q,"genres":["fantasy"],"length_target":"short","default_pov":"first"}`, title))
		if rpcErr != nil {
			t.Fatalf("projects.create: %+v", rpcErr)
		}
		var p struct {
			ID               string  `json:"id"`
			LastOpenedNodeID *string `json:"last_opened_node_id"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatalf("decode project: %v", err)
		}
		return p.ID, *p.LastOpenedNodeID
	}
	allowed, _ := mk("허용된 작품")
	blocked, blockedNode := mk("차단된 작품")

	if _, rpcErr := call(t, app, "settings.set",
		fmt.Sprintf(`{"mcp_project_id":%q}`, allowed)); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}

	if result := c.callTool("linetta_get_outline", map[string]any{"project_id": blocked}); !isToolError(result) {
		t.Error("a restricted server must refuse another work's outline")
	}
	// Node ids must be checked too: reaching a scene by id would bypass the
	// project-level check entirely.
	if result := c.callTool("linetta_read_scene", map[string]any{"node_id": blockedNode}); !isToolError(result) {
		t.Error("a restricted server must refuse a scene from another work")
	}
	if result := c.callTool("linetta_get_outline", map[string]any{"project_id": allowed}); isToolError(result) {
		t.Errorf("the allowed work must still be readable: %v", result)
	}

	works := c.callTool("linetta_list_works", map[string]any{})
	body := structuredJSON(t, works)
	if strings.Contains(body, blocked) {
		t.Errorf("linetta_list_works leaked a restricted work: %s", body)
	}
	if !strings.Contains(body, allowed) {
		t.Errorf("linetta_list_works dropped the allowed work: %s", body)
	}
}
