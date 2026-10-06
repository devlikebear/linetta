//go:build !mobile

package engineapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type styleStats struct {
	Scenes         int     `json:"scenes"`
	Sentences      int     `json:"sentences"`
	SentenceMean   float64 `json:"sentence_mean"`
	SentenceMedian int     `json:"sentence_median"`
	DialogueShare  float64 `json:"dialogue_share"`
	Endings        []struct {
		Ending string  `json:"ending"`
		Count  int     `json:"count"`
		Share  float64 `json:"share"`
	} `json:"endings"`
}

func measureStyle(t *testing.T, c *mcpClient, args map[string]any) styleStats {
	t.Helper()
	result := c.callTool("linetta_analyze_style", args)
	if isToolError(result) {
		t.Fatalf("linetta_analyze_style %v: %v", args, result)
	}
	var out struct {
		Stats styleStats `json:"stats"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &out); err != nil {
		t.Fatalf("decode analyze_style: %v", err)
	}
	return out.Stats
}

func storyBrief(t *testing.T, c *mcpClient, nodeID string) string {
	t.Helper()
	result := c.callTool("linetta_get_story_context", map[string]any{"node_id": nodeID})
	if isToolError(result) {
		t.Fatalf("get_story_context: %v", result)
	}
	return structuredJSON(t, result)
}

type draftState struct {
	Pending bool `json:"pending"`
	Draft   struct {
		Body   string `json:"body"`
		Author string `json:"author"`
	} `json:"draft"`
	Limit int `json:"limit"`
}

func getStyleDraft(t *testing.T, app *App, projectID string) draftState {
	t.Helper()
	raw, rpcErr := call(t, app, "style.get_draft", fmt.Sprintf(`{"project_id":%q}`, projectID))
	if rpcErr != nil {
		t.Fatalf("style.get_draft: %+v", rpcErr)
	}
	var out draftState
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// The #163 contract, first clause: two manuscripts that plainly differ in
// style come back from the measuring tool with numbers that tell them apart.
func TestMCPAnalyzeStyleMeasuresTwoStylesDifferently(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)
	writeSceneText(t, c, nodeID, "비가 왔다. 그는 걸었다. 문이 닫혔다.\n\n아무도 없었다. 그는 웃었다.", 0)
	terse := measureStyle(t, c, map[string]any{"node_id": nodeID})

	writeSceneText(t, c, nodeID,
		"“오늘은 정말이지 하루 종일 비가 올 것 같은데, 우산은 챙기셨어요?” 그녀가 창밖을 내다보며 물었어요.\n\n"+
			"“아니요, 깜빡하고 그냥 나왔는데 어쩌면 좋을지 모르겠네요.” 그가 머리를 긁적이며 대답했어요.", 1)
	talky := measureStyle(t, c, map[string]any{"project_id": projectID})

	if terse.Sentences != 5 || terse.Scenes != 1 {
		t.Errorf("terse = %+v", terse)
	}
	if !(terse.SentenceMean < 10 && talky.SentenceMean > 20) {
		t.Errorf("sentence length: terse %.2f, talky %.2f", terse.SentenceMean, talky.SentenceMean)
	}
	if !(terse.DialogueShare == 0 && talky.DialogueShare > 0.5) {
		t.Errorf("dialogue share: terse %.2f, talky %.2f", terse.DialogueShare, talky.DialogueShare)
	}
	if len(talky.Endings) == 0 || talky.Endings[0].Ending != "어요" {
		t.Errorf("talky endings = %+v", talky.Endings)
	}
}

// The second clause, and the point of the whole design: what an agent
// proposes is not in the story brief until the writer approves it.
func TestMCPStyleProfileReachesTheBriefOnlyAfterApproval(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)
	writeSceneText(t, c, nodeID, "비가 왔다. 그는 걸었다.", 0)
	const profile = "문장이 짧다. 과거형으로 끝난다. 대화보다 서술이 많다."

	result := c.callTool("linetta_propose_style_profile", map[string]any{"project_id": projectID, "profile": profile})
	if isToolError(result) {
		t.Fatalf("propose: %v", result)
	}
	var proposed struct {
		Status     string `json:"status"`
		Characters int    `json:"characters"`
		Limit      int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &proposed); err != nil {
		t.Fatal(err)
	}
	if proposed.Status != "pending_writer_approval" || proposed.Limit != 2200 || proposed.Characters != len([]rune(profile)) {
		t.Errorf("propose result = %+v", proposed)
	}

	// Proposed, not approved: nothing of it is in the brief.
	if brief := storyBrief(t, c, nodeID); strings.Contains(brief, "문장이 짧다") {
		t.Fatalf("the draft is already in the story brief:\n%s", brief)
	}
	draft := getStyleDraft(t, app, projectID)
	if !draft.Pending || draft.Draft.Body != profile || draft.Draft.Author != "external" || draft.Limit != 2200 {
		t.Fatalf("draft = %+v", draft)
	}

	// The writer edits it and approves.
	const approved = "문장이 짧다. 과거형으로 끝난다."
	params, _ := json.Marshal(map[string]any{"project_id": projectID, "body": approved, "mode": "replace"})
	raw, rpcErr := call(t, app, "style.approve_draft", string(params))
	if rpcErr != nil {
		t.Fatalf("style.approve_draft: %+v", rpcErr)
	}
	if !strings.Contains(string(raw), approved) {
		t.Errorf("approve reply = %s", raw)
	}
	brief := storyBrief(t, c, nodeID)
	if !strings.Contains(brief, approved) {
		t.Errorf("the approved profile is not in the story brief:\n%s", brief)
	}
	if strings.Contains(brief, "대화보다 서술이 많다") {
		t.Error("the brief carries the sentence the writer cut before approving")
	}
	if getStyleDraft(t, app, projectID).Pending {
		t.Error("the draft is still pending after approval")
	}
}

// The third clause: a draft never exceeds the limit. It is refused, not cut.
func TestMCPStyleProfileOverTheLimitIsRefused(t *testing.T) {
	app, c, projectID, _ := startWritableMCP(t)
	result := c.callTool("linetta_propose_style_profile", map[string]any{
		"project_id": projectID, "profile": strings.Repeat("가", 2201),
	})
	if !isToolError(result) {
		t.Fatalf("a 2201-character profile was accepted: %v", result)
	}
	if msg := errorText(result); !strings.Contains(msg, "2201") || !strings.Contains(msg, "2200") {
		t.Errorf("the refusal should give both numbers: %s", msg)
	}
	if getStyleDraft(t, app, projectID).Pending {
		t.Error("a refused proposal left a draft behind")
	}
	if r := c.callTool("linetta_propose_style_profile", map[string]any{"project_id": projectID, "profile": "  "}); !isToolError(r) {
		t.Errorf("an empty profile was accepted: %v", r)
	}
}

// Discarding is the writer saying no: the draft goes, the notes stay as they were.
func TestStyleDraftDiscardLeavesNotesAlone(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)
	if r := c.callTool("linetta_propose_style_profile", map[string]any{"project_id": projectID, "profile": "버릴 초안."}); isToolError(r) {
		t.Fatalf("propose: %v", r)
	}
	if _, rpcErr := call(t, app, "style.discard_draft", fmt.Sprintf(`{"project_id":%q}`, projectID)); rpcErr != nil {
		t.Fatalf("style.discard_draft: %+v", rpcErr)
	}
	if getStyleDraft(t, app, projectID).Pending {
		t.Error("the draft survived a discard")
	}
	if strings.Contains(storyBrief(t, c, nodeID), "버릴 초안") {
		t.Error("a discarded draft reached the story brief")
	}
	// With nothing pending there is nothing to approve.
	params, _ := json.Marshal(map[string]any{"project_id": projectID, "body": "버릴 초안.", "mode": "replace"})
	_, rpcErr := call(t, app, "style.approve_draft", string(params))
	if rpcErr == nil || !strings.Contains(string(rpcErr.Data), "style_no_draft") {
		t.Errorf("approving with no draft should be refused with style_no_draft, got %+v", rpcErr)
	}
}

// An agent on a read-only server can measure but cannot propose, and no
// server, in any mode, offers a tool that approves.
func TestMCPStyleProfileToolsByMode(t *testing.T) {
	_, readOnly := startMCPServer(t)
	names := strings.Join(readOnly.toolNames(), ",")
	if !strings.Contains(names, "linetta_analyze_style") {
		t.Error("read_only is missing linetta_analyze_style")
	}
	if strings.Contains(names, "linetta_propose_style_profile") {
		t.Error("read_only exposed linetta_propose_style_profile")
	}

	_, full, _, _ := startWritableMCP(t)
	for _, name := range full.toolNames() {
		if strings.Contains(name, "approve") {
			t.Errorf("tool %q lets an agent approve; approval is the writer's alone", name)
		}
	}
}
