//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/export"
)

// A scene the writer formatted in the editor: a heading, emphasis, a link, a
// list and a quote — what a plain read flattens (#161).
const formattedSceneDoc = `{"type":"doc","content":[
  {"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"도착"}]},
  {"type":"paragraph","content":[
    {"type":"text","text":"그는 "},
    {"type":"text","marks":[{"type":"bold"}],"text":"굵게"},
    {"type":"text","text":" 말했고 "},
    {"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com/map"}}],"text":"지도"},
    {"type":"text","text":"를 펼쳤다."}
  ]},
  {"type":"bulletList","content":[
    {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"물"}]}]},
    {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"빵"}]}]}
  ]},
  {"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"돌아오지 마라."}]}]}
]}`

func seedFormattedScene(t *testing.T, app *App, nodeID string) {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"id": nodeID, "doc": formattedSceneDoc})
	if _, rpcErr := call(t, app, "nodes.update_content", string(params)); rpcErr != nil {
		t.Fatalf("nodes.update_content: %+v", rpcErr)
	}
}

type readSceneResult struct {
	Text           string `json:"text"`
	Format         string `json:"format"`
	ContentVersion int    `json:"content_version"`
}

func readSceneAs(t *testing.T, c *mcpClient, args map[string]any) (readSceneResult, string) {
	t.Helper()
	result := c.callTool("linetta_read_scene", args)
	if isToolError(result) {
		t.Fatalf("read_scene %v: %v", args, result)
	}
	raw := structuredJSON(t, result)
	var out readSceneResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode read_scene: %v", err)
	}
	return out, raw
}

// format: "markdown" returns exactly what the Markdown export writes for the
// scene — one converter, so the two cannot drift.
func TestMCPReadSceneMarkdownMatchesExport(t *testing.T) {
	app, c, _, nodeID := startWritableMCP(t)
	seedFormattedScene(t, app, nodeID)

	got, _ := readSceneAs(t, c, map[string]any{"node_id": nodeID, "format": "markdown"})
	want, err := export.DocToMarkdown([]byte(formattedSceneDoc))
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != strings.TrimSpace(want) {
		t.Errorf("markdown read:\n got %q\nwant %q", got.Text, strings.TrimSpace(want))
	}
	if got.Format != "markdown" {
		t.Errorf("format = %q, want markdown", got.Format)
	}
	for _, piece := range []string{"## 도착", "**굵게**", "[지도](https://example.com/map)", "- 물\n- 빵", "> 돌아오지 마라."} {
		if !strings.Contains(got.Text, piece) {
			t.Errorf("markdown read is missing %q:\n%s", piece, got.Text)
		}
	}
}

// Leaving format out — or asking for "plain" — is the read every existing
// client makes. Its bytes do not change: no markdown syntax, no new field.
func TestMCPReadSceneDefaultStaysPlain(t *testing.T) {
	app, c, _, nodeID := startWritableMCP(t)
	seedFormattedScene(t, app, nodeID)

	def, defRaw := readSceneAs(t, c, map[string]any{"node_id": nodeID})
	_, plainRaw := readSceneAs(t, c, map[string]any{"node_id": nodeID, "format": "plain"})
	if defRaw != plainRaw {
		t.Errorf("format omitted and format=plain differ:\n%s\n%s", defRaw, plainRaw)
	}
	if want := "도착\n그는 굵게 말했고 지도를 펼쳤다.\n물\n빵\n돌아오지 마라."; def.Text != want {
		t.Errorf("plain read = %q, want %q", def.Text, want)
	}
	if strings.Contains(defRaw, `"format"`) {
		t.Errorf("a default read grew a format field: %s", defRaw)
	}
}

// Both formats describe the same stored scene, so the version an agent must
// hand back to a write tool is the same whichever it read.
func TestMCPReadSceneFormatsShareContentVersion(t *testing.T) {
	app, c, _, nodeID := startWritableMCP(t)
	seedFormattedScene(t, app, nodeID)

	plain, _ := readSceneAs(t, c, map[string]any{"node_id": nodeID})
	md, _ := readSceneAs(t, c, map[string]any{"node_id": nodeID, "format": "markdown"})
	if plain.ContentVersion != md.ContentVersion {
		t.Errorf("content_version: plain %d, markdown %d", plain.ContentVersion, md.ContentVersion)
	}
	if plain.ContentVersion == 0 {
		t.Error("content_version is 0 after a save; the seed did not land")
	}
}

func TestMCPReadSceneRejectsUnknownFormat(t *testing.T) {
	_, c, _, nodeID := startWritableMCP(t)
	result := c.callTool("linetta_read_scene", map[string]any{"node_id": nodeID, "format": "html"})
	if !isToolError(result) {
		t.Fatalf("format=html was accepted: %v", result)
	}
	if msg := errorText(result); !strings.Contains(msg, "plain") || !strings.Contains(msg, "markdown") {
		t.Errorf("error should name the formats that work: %s", msg)
	}
}
