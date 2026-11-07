//go:build !mobile

package engineapp

import (
	"encoding/json"
	"testing"
	"time"
)

// ledger.pending_settlement answers from the real record and the real switch:
// a scene written before settlement was switched on is not waiting; the same
// scene edited afterwards is.
func TestPendingSettlementRPC_countsFromTheMomentItWasSwitchedOn(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)
	write := func(text string) {
		t.Helper()
		doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
		params, _ := json.Marshal(map[string]string{"id": nodeID, "doc": doc})
		if _, rpcErr := call(t, app, "nodes.update_content", string(params)); rpcErr != nil {
			t.Fatalf("nodes.update_content: %+v", rpcErr)
		}
	}
	pending := func() (bool, []string) {
		t.Helper()
		out, rpcErr := call(t, app, "ledger.pending_settlement", `{"project_id":"`+projectID+`"}`)
		if rpcErr != nil {
			t.Fatalf("ledger.pending_settlement: %+v", rpcErr)
		}
		var got struct {
			Enabled bool `json:"enabled"`
			Items   []struct {
				NodeID string `json:"node_id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, it := range got.Items {
			ids = append(ids, it.NodeID)
		}
		return got.Enabled, ids
	}

	write("먼저 쓴 문장.")
	if on, ids := pending(); on || len(ids) != 0 {
		t.Fatalf("settlement off = enabled %v items %v", on, ids)
	}
	time.Sleep(5 * time.Millisecond)
	if _, rpcErr := call(t, app, "settings.set", `{"agent_settlement_enabled":true}`); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	if on, ids := pending(); !on || len(ids) != 0 {
		t.Fatalf("just switched on = enabled %v items %v, want nothing waiting", on, ids)
	}
	time.Sleep(5 * time.Millisecond)
	write("고쳐 쓴 문장.")
	if _, ids := pending(); len(ids) != 1 || ids[0] != nodeID {
		t.Fatalf("after an edit = %v, want the scene", ids)
	}
}
