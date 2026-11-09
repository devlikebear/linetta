//go:build !mobile

package engineapp

import (
	"encoding/json"
	"testing"
)

// ledger.knows_at answers from the real ledger and the real appearance index:
// the characters are the ones the scene mentions, the columns are the secrets
// still open there, and a cell is the character's last knows fact by then.
func TestKnowsAtRPC_tableForTheSceneCast(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)
	mustCall := func(method string, params any) json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(params)
		out, rpcErr := call(t, app, method, string(raw))
		if rpcErr != nil {
			t.Fatalf("%s: %+v", method, rpcErr)
		}
		return out
	}
	for _, name := range []string{"하람", "서린"} {
		mustCall("entities.create", map[string]string{"project_id": projectID, "kind": "character", "name": name})
	}
	mustCall("entities.create", map[string]string{"project_id": projectID, "kind": "place", "name": "북문"})
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"하람은 북문 앞에서 서린을 기다렸다."}]}]}`
	mustCall("nodes.update_content", map[string]string{"id": nodeID, "doc": doc})
	mustCall("mentions.relink_work", map[string]string{"project_id": projectID})

	var q struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(mustCall("ledger.save", map[string]any{
		"project_id": projectID, "kind": "question", "body": "하람은 누구인가",
		"answer": "왕자", "planted_node_id": nodeID,
	}), &q); err != nil {
		t.Fatal(err)
	}
	mustCall("ledger.save", map[string]any{"project_id": projectID, "kind": "question", "body": "답 없는 질문", "planted_node_id": nodeID})
	mustCall("ledger.save", map[string]any{
		"project_id": projectID, "kind": "knowledge", "subject": "하람", "attribute": "앎",
		"body": "제 정체를 앎", "about": q.ID, "value_num": 1, "planted_node_id": nodeID,
	})

	var got struct {
		Scenes []struct {
			NodeID string `json:"node_id"`
		} `json:"scenes"`
		NodeID  string `json:"node_id"`
		Secrets []struct {
			ID     string `json:"id"`
			Body   string `json:"body"`
			Answer string `json:"answer"`
		} `json:"secrets"`
		Rows []struct {
			Subject string `json:"subject"`
			Cells   map[string]struct {
				Knows bool `json:"knows"`
			} `json:"cells"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(mustCall("ledger.knows_at", map[string]string{"project_id": projectID, "node_id": nodeID}), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Scenes) != 1 || got.Scenes[0].NodeID != nodeID || got.NodeID != nodeID {
		t.Fatalf("scenes = %+v, node = %q", got.Scenes, got.NodeID)
	}
	if len(got.Secrets) != 1 || got.Secrets[0].ID != q.ID || got.Secrets[0].Answer != "" {
		t.Fatalf("secrets = %+v, want the answered question without its answer", got.Secrets)
	}
	if len(got.Rows) != 2 || got.Rows[0].Subject != "하람" || got.Rows[1].Subject != "서린" {
		t.Fatalf("rows = %+v, want the two characters in order of appearance", got.Rows)
	}
	if c, ok := got.Rows[0].Cells[q.ID]; !ok || !c.Knows {
		t.Fatalf("하람 cells = %+v", got.Rows[0].Cells)
	}
	if len(got.Rows[1].Cells) != 0 {
		t.Fatalf("서린 has no record but got %+v", got.Rows[1].Cells)
	}

	// No scene picked: only the list to pick from.
	if err := json.Unmarshal(mustCall("ledger.knows_at", map[string]string{"project_id": projectID}), &got); err != nil {
		t.Fatal(err)
	}
	if got.NodeID != "" || len(got.Rows) != 0 || len(got.Secrets) != 0 || len(got.Scenes) != 1 {
		t.Fatalf("without a scene got %+v", got)
	}
}

// ledger.save keeps everything it is given when it creates an entry, not only
// when it updates one: a question written already paid, with its reach and the
// scene it is due by, stays that way. The 2026-11-09 synthetic run found the
// create path dropping all of these without a word.
func TestLedgerSaveCreateKeepsEveryField(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"하람은 북문 앞에서 서린을 기다렸다."}]}]}`
	if _, rpcErr := call(t, app, "nodes.update_content", `{"id":"`+nodeID+`","doc":`+mustJSON(t, doc)+`}`); rpcErr != nil {
		t.Fatalf("nodes.update_content: %+v", rpcErr)
	}
	raw, _ := json.Marshal(map[string]any{
		"project_id": projectID, "kind": "question", "body": "하람은 누구인가", "reach": "work",
		"status": "paid", "planted_node_id": nodeID, "planted_quote": "북문 앞에서",
		"due_node_id": nodeID, "paid_node_id": nodeID, "paid_quote": "서린을 기다렸다",
	})
	out, rpcErr := call(t, app, "ledger.save", string(raw))
	if rpcErr != nil {
		t.Fatalf("ledger.save: %+v", rpcErr)
	}
	var e struct {
		Status       string `json:"status"`
		Reach        string `json:"reach"`
		PlantedQuote string `json:"planted_quote"`
		DueNodeID    string `json:"due_node_id"`
		PaidNodeID   string `json:"paid_node_id"`
		PaidQuote    string `json:"paid_quote"`
	}
	if err := json.Unmarshal(out, &e); err != nil {
		t.Fatal(err)
	}
	if e.Status != "paid" || e.Reach != "work" || e.PlantedQuote != "북문 앞에서" || e.DueNodeID != nodeID ||
		e.PaidNodeID != nodeID || e.PaidQuote != "서린을 기다렸다" {
		t.Fatalf("created entry = %+v", e)
	}
}

func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
