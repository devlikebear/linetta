package importmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseDocument_readsEmbeddedPictures(t *testing.T) {
	md := "# 비 오는 밤\n\n## 씬 1\n\n비가 내렸다.\n\n![우산 \\[노란\\] 하나][illustration-a]\n\n그는 걸었다.\n\n" +
		"[illustration-a]: data:image/jpeg;base64,SlBFRw==\n" +
		"[illustration-b]: data:image/png;base64,not*base64\n"
	doc := ParseDocument(md)
	if len(doc.Images) != 1 || doc.Images["a"].MIME != "image/jpeg" || string(doc.Images["a"].Data) != "JPEG" {
		t.Fatalf("images = %+v", doc.Images)
	}
	if len(doc.Outline.Roots) != 1 {
		t.Fatalf("roots = %+v", doc.Outline.Roots)
	}
	raw, _ := json.Marshal(doc.Outline.Roots[0].Body)
	body := string(raw)
	if !strings.Contains(body, `{"type":"illustration","attrs":{"caption":"우산 [노란] 하나","id":"a"}}`) {
		t.Fatalf("no illustration node: %s", body)
	}
	if strings.Contains(body, "SlBFRw") {
		t.Fatalf("a decoded definition stayed in the prose: %s", body)
	}
	// A definition that does not decode is left as the writer's text.
	if !strings.Contains(body, "not*base64") {
		t.Fatalf("a broken definition disappeared: %s", body)
	}
}

func TestParseBlocks_inlineImageStaysText(t *testing.T) {
	raw, _ := json.Marshal(ParseBlocks("앞 ![x][illustration-a] 뒤"))
	if strings.Contains(string(raw), `"illustration"`) {
		t.Fatalf("a mid-sentence reference became a picture: %s", raw)
	}
}
