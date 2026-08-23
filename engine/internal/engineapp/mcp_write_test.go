//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
)

// startWritableMCP brings up a server in full mode with one seeded scene.
func startWritableMCP(t *testing.T) (*App, *mcpClient, string, string) {
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
		`{"mcp_mode":"full","mcp_port":%d,"mcp_consent_version":1,"mcp_consented_at":1}`, port)
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

	created, rpcErr := call(t, app, "projects.create",
		`{"title":"쓰기 테스트","genres":["fantasy"],"length_target":"short","default_pov":"first"}`)
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
	return app, c, proj.ID, *proj.LastOpenedNodeID
}

func readScene(t *testing.T, c *mcpClient, nodeID string) (body string, version int) {
	t.Helper()
	result := c.callTool("linetta_read_scene", map[string]any{"node_id": nodeID})
	if isToolError(result) {
		t.Fatalf("read_scene: %v", result)
	}
	var out struct {
		Body           string `json:"body"`
		ContentVersion int    `json:"content_version"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &out); err != nil {
		t.Fatalf("decode read_scene: %v", err)
	}
	return out.Body, out.ContentVersion
}

// read_only must not merely refuse writes — the tools must be absent, so a
// misbehaving agent cannot call one at all.
func TestMCPReadOnlyHidesWriteTools(t *testing.T) {
	_, c := startMCPServer(t) // read_only
	names := strings.Join(c.toolNames(), ",")
	for _, w := range mcphost.WriteToolNames {
		if strings.Contains(names, w) {
			t.Errorf("read_only exposed the write tool %q", w)
		}
	}
	if len(c.toolNames()) != len(mcphost.ReadToolNames) {
		t.Errorf("read_only tool count = %d, want %d", len(c.toolNames()), len(mcphost.ReadToolNames))
	}
}

func TestMCPFullModeExposesWriteTools(t *testing.T) {
	_, c, _, _ := startWritableMCP(t)
	names := strings.Join(c.toolNames(), ",")
	for _, w := range mcphost.WriteToolNames {
		if !strings.Contains(names, w) {
			t.Errorf("full mode is missing the write tool %q", w)
		}
	}
	want := len(mcphost.ReadToolNames) + len(mcphost.WriteToolNames)
	if got := len(c.toolNames()); got != want {
		t.Errorf("full mode tool count = %d, want %d", got, want)
	}
}

// The happy path: read, write, and the prose is really there — plus a
// snapshot id the agent can revert with.
func TestMCPWriteSceneWritesAndSnapshots(t *testing.T) {
	app, c, _, nodeID := startWritableMCP(t)
	_, version := readScene(t, c, nodeID)

	const prose = "비가 그친 골목에서 그는 그림자를 잃었다.\n\n가로등은 켜져 있었다."
	result := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": prose, "expected_content_version": version,
	})
	if isToolError(result) {
		t.Fatalf("write_scene: %v", result)
	}
	var out struct {
		ContentVersion int    `json:"content_version"`
		WordCount      int    `json:"word_count"`
		SnapshotID     string `json:"snapshot_id"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &out); err != nil {
		t.Fatalf("decode write_scene: %v", err)
	}
	if out.ContentVersion <= version {
		t.Errorf("content_version did not advance: %d -> %d", version, out.ContentVersion)
	}
	if out.WordCount == 0 {
		t.Error("word count was not recomputed")
	}

	body, _ := readScene(t, c, nodeID)
	if !strings.Contains(body, "그림자를 잃었다") {
		t.Fatalf("prose not stored: %q", body)
	}

	// The snapshot is the revert path for prose; undo_last_change's batch id
	// restores the outline and leaves bodies alone.
	raw, rpcErr := call(t, app, "snapshots.list_for_node", fmt.Sprintf(`{"node_id":%q}`, nodeID))
	if rpcErr != nil {
		t.Fatalf("snapshots.list_for_node: %+v", rpcErr)
	}
	if !strings.Contains(string(raw), "companion-before") {
		t.Errorf("no pre-write snapshot recorded: %s", raw)
	}
}

// The whole point of expected_content_version: a write against stale state is
// refused instead of silently overwriting the writer's own edits.
func TestMCPWriteSceneRefusesStaleVersion(t *testing.T) {
	app, c, _, nodeID := startWritableMCP(t)
	_, stale := readScene(t, c, nodeID)

	// The writer edits in the app, so the agent's version is now behind.
	if _, rpcErr := call(t, app, "nodes.update_content",
		fmt.Sprintf(`{"id":%q,"doc":"{\"type\":\"doc\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"작가가 직접 쓴 문장\"}]}]}"}`, nodeID)); rpcErr != nil {
		t.Fatalf("nodes.update_content: %+v", rpcErr)
	}

	result := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": "에이전트가 덮어쓰려는 문장", "expected_content_version": stale,
	})
	if !isToolError(result) {
		t.Fatal("a stale write must be refused")
	}
	if msg := toolErrorText(result); !strings.Contains(msg, "read") {
		t.Errorf("the conflict message should tell the agent to re-read: %s", msg)
	}
	body, _ := readScene(t, c, nodeID)
	if !strings.Contains(body, "작가가 직접 쓴 문장") {
		t.Fatalf("the writer's text was lost: %q", body)
	}
}

func TestMCPWriteSceneRejectsMissingVersionAndContainers(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)

	if result := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": "버전 없이",
	}); !isToolError(result) {
		t.Error("a write without expected_content_version must be refused")
	}

	// A container has no body; writing to one is a mistake worth naming.
	raw, rpcErr := call(t, app, "nodes.create_child",
		fmt.Sprintf(`{"parent_id":%q,"kind":"container","label":"1부"}`, nodeID))
	if rpcErr != nil {
		// The seeded node is a leaf, so create a container at the root instead.
		raw, rpcErr = call(t, app, "nodes.create_sibling",
			fmt.Sprintf(`{"sibling_id":%q,"kind":"container","label":"1부"}`, nodeID))
		if rpcErr != nil {
			t.Skipf("could not create a container to test with: %+v", rpcErr)
		}
	}
	var container struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &container); err != nil || container.ID == "" {
		t.Skip("could not decode the container node")
	}
	_ = projectID
	if result := c.callTool("linetta_write_scene", map[string]any{
		"node_id": container.ID, "text": "본문", "expected_content_version": 1,
	}); !isToolError(result) {
		t.Error("writing a body to a container must be refused")
	}
}

// write_summary is what fills the empty summary sections the brief reports.
func TestMCPWriteSummaryForScene(t *testing.T) {
	_, c, _, nodeID := startWritableMCP(t)
	_, version := readScene(t, c, nodeID)
	writeResult := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": "그는 골목에서 그림자를 잃었다.", "expected_content_version": version,
	})
	if isToolError(writeResult) {
		t.Fatalf("write_scene: %v", writeResult)
	}
	_, fresh := readScene(t, c, nodeID)

	const summary = "서린이 골목에서 자신의 그림자가 사라진 것을 처음 자각한다."
	result := c.callTool("linetta_write_summary", map[string]any{
		"node_id": nodeID, "summary": summary, "expected_content_version": fresh,
	})
	if isToolError(result) {
		t.Fatalf("write_summary: %v", result)
	}

	// A fresh summary must read back as fresh, not stale.
	scene := c.callTool("linetta_read_scene", map[string]any{"node_id": nodeID})
	var out struct {
		Summary        string `json:"summary"`
		SummaryIsStale bool   `json:"summary_is_stale"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, scene)), &out); err != nil {
		t.Fatalf("decode read_scene: %v", err)
	}
	if out.Summary != summary {
		t.Errorf("summary = %q, want %q", out.Summary, summary)
	}
	if out.SummaryIsStale {
		t.Error("a summary written against the current version must not read as stale")
	}
}

// A summary of text that has since changed would make the brief lie, so a
// stale version is refused.
func TestMCPWriteSummaryRefusesStaleVersion(t *testing.T) {
	_, c, _, nodeID := startWritableMCP(t)
	_, stale := readScene(t, c, nodeID)
	if result := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": "새 본문", "expected_content_version": stale,
	}); isToolError(result) {
		t.Fatalf("write_scene: %v", result)
	}

	result := c.callTool("linetta_write_summary", map[string]any{
		"node_id": nodeID, "summary": "낡은 본문에 대한 요약", "expected_content_version": stale,
	})
	if !isToolError(result) {
		t.Fatal("a summary written against stale text must be refused")
	}
}

func TestMCPWriteSummarySynopsisAndArgumentChecks(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)

	const synopsis = "존재가 지워진 남자가 자신을 지운 조직을 추적한다."
	if result := c.callTool("linetta_write_summary", map[string]any{
		"project_id": projectID, "summary": synopsis,
	}); isToolError(result) {
		t.Fatalf("synopsis write: %v", result)
	}
	works := c.callTool("linetta_list_works", map[string]any{})
	if !strings.Contains(structuredJSON(t, works), "자신을 지운 조직") {
		t.Errorf("synopsis not stored: %s", structuredJSON(t, works))
	}

	if result := c.callTool("linetta_write_summary", map[string]any{"summary": "대상 없음"}); !isToolError(result) {
		t.Error("a summary with no target must be refused")
	}
	if result := c.callTool("linetta_write_summary", map[string]any{
		"node_id": nodeID, "project_id": projectID, "summary": "둘 다",
	}); !isToolError(result) {
		t.Error("passing both node_id and project_id must be refused")
	}
}

// toolErrorText returns the human-readable text of a tool error, which is what
// the agent actually reads (structuredContent holds the zero value on errors).
func toolErrorText(result map[string]any) string {
	content, _ := result["content"].([]any)
	parts := []string{}
	for _, raw := range content {
		block, _ := raw.(map[string]any)
		if text, ok := block["text"].(string); ok {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}
