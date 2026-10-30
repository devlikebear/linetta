//go:build !mobile

package engineapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/illustration"
	"github.com/devlikebear/linetta/engine/internal/illustref"
)

func sceneDoc(t *testing.T, app *App, nodeID string) string {
	t.Helper()
	raw, rpcErr := call(t, app, "nodes.get", fmt.Sprintf(`{"id":%q}`, nodeID))
	if rpcErr != nil {
		t.Fatalf("nodes.get: %+v", rpcErr)
	}
	var n struct {
		ContentDoc string `json:"content_doc"`
	}
	if err := json.Unmarshal(raw, &n); err != nil {
		t.Fatal(err)
	}
	return n.ContentDoc
}

func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// An agent that reads a scene with a picture in it and writes the text back
// keeps the picture where it was; one that leaves the line out takes it out;
// a placeholder naming a picture the work does not have stays text.
func TestMCPSceneRoundTripKeepsIllustrations(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCPAt(t, t.TempDir())

	// The app's own gallery, not a second connection to library.db: a write
	// from another connection while the engine holds the file can fail with
	// SQLITE_BUSY, which busy_timeout does not retry (seen on Windows CI).
	gallery, ok := app.mcpTools.Illustrations.(*illustration.Repo)
	if !ok {
		t.Fatalf("mcp tools gallery is %T", app.mcpTools.Illustrations)
	}
	pic, err := gallery.Add(context.Background(), 1, illustration.NewInput{
		ProjectID: projectID, Prompt: "p", Provider: "openai-codex", Model: "m", Data: tinyJPEG(t),
	})
	if err != nil {
		t.Fatal(err)
	}

	doc := fmt.Sprintf(`{"type":"doc","content":[`+
		`{"type":"paragraph","content":[{"type":"text","text":"비가 내렸다."}]},`+
		`{"type":"illustration","attrs":{"id":%q,"caption":"우산"}},`+
		`{"type":"paragraph","content":[{"type":"text","text":"그는 걸었다."}]}]}`, pic.ID)
	if _, rpcErr := call(t, app, "nodes.update_content", fmt.Sprintf(`{"id":%q,"doc":%q}`, nodeID, doc)); rpcErr != nil {
		t.Fatalf("nodes.update_content: %+v", rpcErr)
	}

	body, version := readScene(t, c, nodeID)
	line := illustref.Placeholder(pic.ID, "우산")
	if !strings.Contains(body, "비가 내렸다.\n"+line+"\n그는 걸었다.") {
		t.Fatalf("read_scene did not carry the picture in place:\n%s", body)
	}
	md := c.callTool("linetta_read_scene", map[string]any{"node_id": nodeID, "format": "markdown"})
	if !strings.Contains(structuredJSON(t, md), strings.ReplaceAll(line, `"`, `\"`)) {
		t.Fatalf("markdown read lost the picture: %s", structuredJSON(t, md))
	}

	// The same text back: the picture stays, between the same paragraphs.
	writeSceneText(t, c, nodeID, body, version)
	ids, err := illustref.IDs(sceneDoc(t, app, nodeID))
	if err != nil || len(ids) != 1 || ids[0] != pic.ID {
		t.Fatalf("after a faithful rewrite the scene holds %v (%v)", ids, err)
	}
	after := sceneDoc(t, app, nodeID)
	if strings.Index(after, "비가 내렸다") > strings.Index(after, pic.ID) || strings.Index(after, pic.ID) > strings.Index(after, "그는 걸었다") {
		t.Fatalf("the picture moved: %s", after)
	}

	// A made-up id stays text, so nothing else gets drawn there.
	_, version = readScene(t, c, nodeID)
	writeSceneText(t, c, nodeID, "비가 내렸다.\n\n[[illustration:0000-made-up]]", version)
	if ids, _ := illustref.IDs(sceneDoc(t, app, nodeID)); len(ids) != 0 {
		t.Fatalf("a made-up id became a picture: %v", ids)
	}
	if body, _ := readScene(t, c, nodeID); !strings.Contains(body, "[[illustration:0000-made-up]]") {
		t.Fatalf("the made-up placeholder was not kept as text: %q", body)
	}

	// Leaving the line out takes the picture out of the scene, not the gallery.
	_, version = readScene(t, c, nodeID)
	writeSceneText(t, c, nodeID, "비가 내렸다.\n\n그는 걸었다.", version)
	if ids, _ := illustref.IDs(sceneDoc(t, app, nodeID)); len(ids) != 0 {
		t.Fatalf("the picture is still in the scene: %v", ids)
	}
	if _, err := gallery.Get(context.Background(), pic.ID); err != nil {
		t.Fatalf("the picture left the gallery: %v", err)
	}
}

// An external MCP client can only leave a request: it shows up for the
// writer through illustration_requests.list, marked external, with nothing
// drawn; and a gallery picture can be made a scene's representative.
func TestMCPIllustrationRequestReachesTheWriter(t *testing.T) {
	home := t.TempDir()
	app, c, projectID, nodeID := startWritableMCPAt(t, home)

	res := c.callTool("linetta_request_illustration", map[string]any{
		"project_id": projectID, "node_id": nodeID, "purpose": "insert", "prompt": "비 오는 골목", "note": "씬 1",
	})
	var out struct {
		RequestID string `json:"request_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, res)), &out); err != nil || out.Status != "waiting_for_writer" || out.RequestID == "" {
		t.Fatalf("request = %s, %v", structuredJSON(t, res), err)
	}
	raw, rpcErr := call(t, app, "illustration_requests.list", fmt.Sprintf(`{"project_id":%q}`, projectID))
	if rpcErr != nil {
		t.Fatalf("list: %+v", rpcErr)
	}
	var list []illustration.Request
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != out.RequestID || list[0].Source != illustration.SourceExternal ||
		list[0].Status != illustration.RequestPending || list[0].TargetNodeID != nodeID {
		t.Fatalf("list = %+v", list)
	}

	gallery := app.mcpTools.Gallery
	if gallery == nil || app.mcpTools.IllustrationRequests == nil {
		t.Fatal("the MCP tools have no gallery or requests")
	}
	pic, err := gallery.Add(context.Background(), 1, illustration.NewInput{ProjectID: projectID, Prompt: "p", Provider: "x", Model: "m", Data: tinyJPEG(t)})
	if err != nil {
		t.Fatal(err)
	}
	c.callTool("linetta_set_representative_illustration", map[string]any{"node_id": nodeID, "illustration_id": pic.ID})
	reps, err := gallery.Representatives(context.Background(), projectID)
	if err != nil || reps[nodeID] != pic.ID {
		t.Fatalf("representatives = %v, %v", reps, err)
	}
	if _, rpcErr := call(t, app, "illustrations.delete", fmt.Sprintf(`{"id":%q}`, pic.ID)); rpcErr == nil || !strings.Contains(string(rpcErr.Data), "illustration_is_representative") {
		t.Fatalf("deleting a representative picture: %+v", rpcErr)
	}
}
