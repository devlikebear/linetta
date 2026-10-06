//go:build !mobile

package engineapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type styleViolation struct {
	NodeID    string `json:"node_id"`
	Label     string `json:"label"`
	Rule      string `json:"rule"`
	Paragraph int    `json:"paragraph"`
	Phrase    string `json:"phrase"`
	Excerpt   string `json:"excerpt"`
	Length    int    `json:"length"`
	Limit     int    `json:"limit"`
}

type styleReport struct {
	Rules struct {
		AvoidPhrases     []string `json:"avoid_phrases"`
		MaxSentenceChars int      `json:"max_sentence_chars"`
	} `json:"rules"`
	ScenesChecked int              `json:"scenes_checked"`
	Total         int              `json:"total"`
	Violations    []styleViolation `json:"violations"`
	Truncated     bool             `json:"truncated"`
}

func setStyleRules(t *testing.T, app *App, projectID string, phrases []string, maxSentence int) {
	t.Helper()
	params, _ := json.Marshal(map[string]any{
		"project_id": projectID, "avoid_phrases": phrases, "max_sentence_chars": maxSentence,
	})
	if _, rpcErr := call(t, app, "style.set_rules", string(params)); rpcErr != nil {
		t.Fatalf("style.set_rules: %+v", rpcErr)
	}
}

func analyzeStyle(t *testing.T, c *mcpClient, args map[string]any) styleReport {
	t.Helper()
	result := c.callTool("linetta_analyze_style", args)
	if isToolError(result) {
		t.Fatalf("linetta_analyze_style %v: %v", args, result)
	}
	var out styleReport
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &out); err != nil {
		t.Fatalf("decode analyze_style: %v", err)
	}
	return out
}

func writeSceneText(t *testing.T, c *mcpClient, nodeID, text string, version int) {
	t.Helper()
	if r := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": text, "expected_content_version": version,
	}); isToolError(r) {
		t.Fatalf("write_scene: %v", r)
	}
}

// The #162 contract, end to end over the real MCP endpoint: an agent writes a
// scene that uses a phrase the writer listed, and the check names the scene,
// the paragraph and the phrase.
func TestMCPAnalyzeStylePinpointsAnAvoidedPhrase(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)
	setStyleRules(t, app, projectID, []string{"그것은 마치", "수 있었다"}, 0)
	writeSceneText(t, c, nodeID, "비가 그쳤다.\n\n그는 문을 열 수 있었다. 그것은 마치 꿈같았다.", 0)

	rep := analyzeStyle(t, c, map[string]any{"node_id": nodeID})
	if rep.ScenesChecked != 1 || rep.Total != 2 || len(rep.Violations) != 2 {
		t.Fatalf("report = %+v", rep)
	}
	for i, wantPhrase := range []string{"수 있었다", "그것은 마치"} {
		v := rep.Violations[i]
		if v.Rule != "avoid_phrase" || v.Phrase != wantPhrase || v.NodeID != nodeID || v.Paragraph != 2 {
			t.Errorf("violation %d = %+v, want %q in paragraph 2 of %s", i, v, wantPhrase, nodeID)
		}
		if !strings.Contains(v.Excerpt, wantPhrase) {
			t.Errorf("excerpt %q does not show %q", v.Excerpt, wantPhrase)
		}
	}
	if len(rep.Rules.AvoidPhrases) != 2 {
		t.Errorf("the report should carry the rules it ran against, got %+v", rep.Rules)
	}
}

// A scene that keeps the rules reports nothing.
func TestMCPAnalyzeStyleCleanSceneHasNoViolations(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)
	setStyleRules(t, app, projectID, []string{"그것은 마치", "수 있었다"}, 40)
	writeSceneText(t, c, nodeID, "비가 그쳤다.\n\n그는 문을 열었다. 밖은 조용했다.", 0)

	rep := analyzeStyle(t, c, map[string]any{"project_id": projectID})
	if rep.Total != 0 || len(rep.Violations) != 0 || rep.Truncated {
		t.Fatalf("clean scene reported %+v", rep)
	}
}

func TestMCPAnalyzeStyleFlagsALongSentence(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)
	setStyleRules(t, app, projectID, nil, 20)
	long := "그는 아주 오랫동안 아무 말도 하지 않고 창밖만 바라보다가 문득 일어났다."
	writeSceneText(t, c, nodeID, "짧다. "+long, 0)

	rep := analyzeStyle(t, c, map[string]any{"node_id": nodeID})
	if rep.Total != 1 || rep.Violations[0].Rule != "sentence_too_long" ||
		rep.Violations[0].Limit != 20 || rep.Violations[0].Length != len([]rune(long)) {
		t.Fatalf("report = %+v", rep)
	}
}

// The check reads. The scene's content_version — what a later write has to
// present — is the same before and after, and so is its text.
func TestMCPAnalyzeStyleLeavesTheSceneAlone(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)
	setStyleRules(t, app, projectID, []string{"정말"}, 0)
	writeSceneText(t, c, nodeID, "정말 좋았다.", 0)

	// Text and version, not the raw payload: the background summarizer may
	// write the scene's summary while the test runs, and that is not the check.
	before, _ := readSceneAs(t, c, map[string]any{"node_id": nodeID})
	if rep := analyzeStyle(t, c, map[string]any{"node_id": nodeID}); rep.Total != 1 {
		t.Fatalf("report = %+v", rep)
	}
	after, _ := readSceneAs(t, c, map[string]any{"node_id": nodeID})
	if before.ContentVersion != after.ContentVersion || before.Text != after.Text {
		t.Errorf("the check changed the scene:\nbefore %+v\nafter  %+v", before, after)
	}
}

// With no rules saved the tool says so through an empty rule set instead of
// reporting a clean bill of health nobody checked for.
func TestMCPAnalyzeStyleWithoutRulesChecksNothing(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)
	writeSceneText(t, c, nodeID, "그것은 마치 꿈같았다.", 0)

	rep := analyzeStyle(t, c, map[string]any{"project_id": projectID})
	if rep.Total != 0 || len(rep.Rules.AvoidPhrases) != 0 || rep.Rules.MaxSentenceChars != 0 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestMCPAnalyzeStyleLimitCapsTheListNotTheCount(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)
	setStyleRules(t, app, projectID, []string{"정말"}, 0)
	writeSceneText(t, c, nodeID, strings.Repeat("정말 ", 60), 0)

	rep := analyzeStyle(t, c, map[string]any{"node_id": nodeID})
	if rep.Total != 60 || len(rep.Violations) != 50 || !rep.Truncated {
		t.Errorf("default: total %d, listed %d, truncated %v", rep.Total, len(rep.Violations), rep.Truncated)
	}
	rep = analyzeStyle(t, c, map[string]any{"node_id": nodeID, "limit": 5})
	if rep.Total != 60 || len(rep.Violations) != 5 || !rep.Truncated {
		t.Errorf("limit 5: total %d, listed %d, truncated %v", rep.Total, len(rep.Violations), rep.Truncated)
	}
}

func TestMCPAnalyzeStyleRefusesAWrongWork(t *testing.T) {
	_, c, _, nodeID := startWritableMCP(t)
	if r := c.callTool("linetta_analyze_style", map[string]any{"node_id": nodeID, "project_id": "some-other-work"}); !isToolError(r) {
		t.Errorf("a mismatched project_id was accepted: %v", r)
	}
	if r := c.callTool("linetta_analyze_style", map[string]any{}); !isToolError(r) {
		t.Errorf("a call naming neither a work nor a scene was accepted: %v", r)
	}
}

// The panel's own path: style.check returns what the MCP tool returns, and
// rules the engine will not store come back with a reason the UI translates.
func TestStyleRPCChecksAndRefuses(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)
	setStyleRules(t, app, projectID, []string{"정말"}, 0)
	writeSceneText(t, c, nodeID, "정말 좋았다. 정말이다.", 0)

	raw, rpcErr := call(t, app, "style.check", fmt.Sprintf(`{"project_id":%q}`, projectID))
	if rpcErr != nil {
		t.Fatalf("style.check: %+v", rpcErr)
	}
	var rep styleReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Total != 2 || rep.ScenesChecked != 1 || len(rep.Rules.AvoidPhrases) != 1 {
		t.Fatalf("style.check = %s", raw)
	}

	got, rpcErr := call(t, app, "style.get_rules", fmt.Sprintf(`{"project_id":%q}`, projectID))
	if rpcErr != nil || !strings.Contains(string(got), `"정말"`) {
		t.Fatalf("style.get_rules = %s, %+v", got, rpcErr)
	}

	refusals := map[string]string{
		"style_phrase_too_long":  fmt.Sprintf(`{"project_id":%q,"avoid_phrases":[%q]}`, projectID, strings.Repeat("가", 81)),
		"style_sentence_limit":   fmt.Sprintf(`{"project_id":%q,"max_sentence_chars":-1}`, projectID),
		"project_not_found":      `{"project_id":"no-such-work","avoid_phrases":["정말"]}`,
		"style_too_many_phrases": fmt.Sprintf(`{"project_id":%q,"avoid_phrases":%s}`, projectID, manyPhrasesJSON(101)),
	}
	for reason, params := range refusals {
		_, rpcErr := call(t, app, "style.set_rules", params)
		if rpcErr == nil || !strings.Contains(string(rpcErr.Data), reason) {
			t.Errorf("style.set_rules should refuse with %s, got %+v", reason, rpcErr)
		}
	}
	if _, rpcErr := call(t, app, "style.check", fmt.Sprintf(`{"project_id":%q,"node_id":"no-such-scene"}`, projectID)); rpcErr == nil {
		t.Error("style.check accepted a scene that is not in the work")
	}
}

func manyPhrasesJSON(n int) string {
	phrases := make([]string, n)
	for i := range phrases {
		phrases[i] = fmt.Sprintf("표현%d", i)
	}
	raw, _ := json.Marshal(phrases)
	return string(raw)
}
