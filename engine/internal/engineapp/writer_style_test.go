//go:build !mobile

package engineapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type writerStyleViolation struct {
	Rule, Phrase, Source string
}

func styleCheck(t *testing.T, app *App, projectID string) []writerStyleViolation {
	t.Helper()
	out, rpcErr := call(t, app, "style.check", `{"project_id":"`+projectID+`"}`)
	if rpcErr != nil {
		t.Fatalf("style.check: %+v", rpcErr)
	}
	var rep struct{ Violations []writerStyleViolation }
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatal(err)
	}
	return rep.Violations
}

func phraseSource(vs []writerStyleViolation, phrase string) (string, bool) {
	for _, v := range vs {
		if v.Rule == "avoid_phrase" && v.Phrase == phrase {
			return v.Source, true
		}
	}
	return "", false
}

// #179, #180, #181: a phrase only the writer listed is caught in every work,
// marked as the writer's, through the app and an agent alike; a work that
// opts out does not see it, and the check writes nothing.
func TestWriterStyle_checkedThroughTheAppAndTheAgent(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"그는 문을 여는 것이었다."}]}]}`
	params, _ := json.Marshal(map[string]string{"id": nodeID, "doc": doc})
	if _, rpcErr := call(t, app, "nodes.update_content", string(params)); rpcErr != nil {
		t.Fatalf("update_content: %+v", rpcErr)
	}
	if _, rpcErr := call(t, app, "style.set_writer_rules", `{"avoid_phrases":["것이었다"],"max_sentence_chars":0}`); rpcErr != nil {
		t.Fatalf("set_writer_rules: %+v", rpcErr)
	}
	before := nodeVersion(t, app, nodeID)

	if src, ok := phraseSource(styleCheck(t, app, projectID), "것이었다"); !ok || src != "writer" {
		t.Fatalf("style.check source = %q, %v", src, ok)
	}

	res := c.callTool("linetta_analyze_style", map[string]any{"project_id": projectID})
	if isToolError(res) {
		t.Fatalf("analyze_style: %v", res)
	}
	var out struct {
		Violations []writerStyleViolation
		Rules      struct {
			IgnoresWriterStyle bool `json:"ignores_writer_style"`
		}
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, res)), &out); err != nil {
		t.Fatal(err)
	}
	if src, ok := phraseSource(out.Violations, "것이었다"); !ok || src != "writer" {
		t.Fatalf("analyze_style source = %q, %v", src, ok)
	}

	// The rules RPC shows the work's own, the writer's and what is checked.
	raw, rpcErr := call(t, app, "style.get_rules", `{"project_id":"`+projectID+`"}`)
	if rpcErr != nil {
		t.Fatalf("get_rules: %+v", rpcErr)
	}
	var rules struct {
		AvoidPhrases []string `json:"avoid_phrases"`
		Writer       struct {
			AvoidPhrases []string `json:"avoid_phrases"`
		}
		Effective struct {
			WriterPhrases []string `json:"writer_phrases"`
		}
	}
	_ = json.Unmarshal(raw, &rules)
	if len(rules.AvoidPhrases) != 0 || len(rules.Writer.AvoidPhrases) != 1 || len(rules.Effective.WriterPhrases) != 1 {
		t.Fatalf("get_rules = %s", raw)
	}

	// Opting out.
	if _, rpcErr := call(t, app, "style.set_rules", `{"project_id":"`+projectID+`","avoid_phrases":[],"ignore_writer_style":true}`); rpcErr != nil {
		t.Fatalf("set_rules: %+v", rpcErr)
	}
	if _, ok := phraseSource(styleCheck(t, app, projectID), "것이었다"); ok {
		t.Fatal("an opted-out work still broke the writer's phrase")
	}
	res = c.callTool("linetta_analyze_style", map[string]any{"project_id": projectID})
	_ = json.Unmarshal([]byte(structuredJSON(t, res)), &out)
	if _, ok := phraseSource(out.Violations, "것이었다"); ok || !out.Rules.IgnoresWriterStyle {
		t.Fatalf("analyze_style after opting out: %+v", out)
	}

	if after := nodeVersion(t, app, nodeID); after != before {
		t.Fatalf("checking changed the scene: version %d → %d", before, after)
	}
}

func nodeVersion(t *testing.T, app *App, nodeID string) int {
	t.Helper()
	out, rpcErr := call(t, app, "nodes.get", `{"id":"`+nodeID+`"}`)
	if rpcErr != nil {
		t.Fatalf("nodes.get: %+v", rpcErr)
	}
	var n struct {
		ContentVersion int `json:"content_version"`
	}
	_ = json.Unmarshal(out, &n)
	return n.ContentVersion
}

// The device-only "phrases to keep out of every work" setting from before is
// carried into the writer's rules once, and the setting is emptied.
func TestWriterStyle_adoptsTheOldDeviceSetting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	isolateHomeDir(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(home, "settings.json"), []byte(`{"style_avoid_phrases":["그것은 마치"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app := openTestApp(t, home)
	raw, rpcErr := call(t, app, "style.get_writer_rules", "")
	if rpcErr != nil {
		t.Fatalf("get_writer_rules: %+v", rpcErr)
	}
	var w struct {
		AvoidPhrases []string `json:"avoid_phrases"`
	}
	if err := json.Unmarshal(raw, &w); err != nil || len(w.AvoidPhrases) != 1 || w.AvoidPhrases[0] != "그것은 마치" {
		t.Fatalf("writer rules = %s", raw)
	}
	cfg, rpcErr := call(t, app, "settings.get", "")
	if rpcErr != nil {
		t.Fatalf("settings.get: %+v", rpcErr)
	}
	var s struct {
		StyleAvoidPhrases []string `json:"style_avoid_phrases"`
	}
	_ = json.Unmarshal(cfg, &s)
	if len(s.StyleAvoidPhrases) != 0 {
		t.Fatalf("old setting still holds %v", s.StyleAvoidPhrases)
	}
}
