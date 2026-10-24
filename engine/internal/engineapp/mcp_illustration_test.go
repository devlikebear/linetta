//go:build !mobile

package engineapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/illustration"
	"github.com/devlikebear/linetta/engine/internal/illustref"
	"github.com/devlikebear/linetta/engine/internal/store"
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
	home := t.TempDir()
	app, c, projectID, nodeID := startWritableMCPAt(t, home)

	st, err := store.Open(context.Background(), filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	pic, err := illustration.NewRepo(st).Add(context.Background(), 1, illustration.NewInput{
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
	if _, err := illustration.NewRepo(st).Get(context.Background(), pic.ID); err != nil {
		t.Fatalf("the picture left the gallery: %v", err)
	}
}
