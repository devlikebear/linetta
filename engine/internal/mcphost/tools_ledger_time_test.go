package mcphost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// #346's case through the tool: an agent names the event, records "석 달" in
// one scene and "삼 년" four scenes on. With no story date the edit is saved
// with no conflict, and the jump comes back for the settlement pass to judge.
func TestAgentRecordsElapsedTimeAndSeesTheJump(t *testing.T) {
	ctx, d, pid := ledgerDeps(t)
	scenes, err := d.Nodes.ListByProject(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	first := scenes[0]
	var later node.Node
	for i := range 4 {
		if later, err = d.Nodes.CreateSibling(ctx, first.ID, node.KindLeaf, "후속", "", int64(10+i)); err != nil {
			t.Fatal(err)
		}
	}
	kind, body := ledger.KindTimeline, "등대지기 실종"
	_, out, err := d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, Kind: kind, Body: &body, PlantedNodeID: &first.ID})
	if err != nil || out.Entry == nil {
		t.Fatalf("event = %+v, %v", out.Entry, err)
	}
	event := out.Entry.ID
	attr, subject := ledger.AttributeElapsed, "등대지기 실종"
	record := func(at, said string, days float64) editLedgerOutput {
		t.Helper()
		res, out, err := d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, Kind: kind, Subject: &subject, Attribute: &attr,
			Body: &said, About: &event, ValueNum: &days, PlantedNodeID: &at})
		if err != nil || (res != nil && res.IsError) || out.Entry == nil {
			t.Fatalf("elapsed %s = %+v, %+v, %v", said, res, out, err)
		}
		return out
	}
	record(first.ID, "석 달", 90)
	out = record(later.ID, "삼 년", 1095)
	if len(out.Conflicts) != 0 || out.Previous == nil || out.Previous.JumpDays != 1005 || out.Previous.ScenesBetween < 1 {
		t.Fatalf("jump = %+v, conflicts %+v", out.Previous, out.Conflicts)
	}
	raw, _ := json.Marshal(out.Previous)
	if !strings.Contains(string(raw), `"jump_days":1005`) {
		t.Fatalf("previous on the wire = %s", raw)
	}
}

// What the agent is told about an elapsed time is in the tool's own schema.
func TestEditLedgerSchemaTellsElapsedTime(t *testing.T) {
	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: ServerVersion}, nil)
	ToolDeps{}.Register(srv, settings.MCPModeFull, AllToolGroups())
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ss.Close() }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "linetta-schema-test", Version: ServerVersion}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.Close() }()
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		if tool.Name != "linetta_edit_ledger" {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		schema := string(raw)
		for _, want := range []string{"경과", "나이", "경과 and 작중 날짜 in days, 나이 in years", "경과: id of the timeline event counted from"} {
			if !strings.Contains(schema, want) {
				t.Errorf("input schema lacks %q", want)
			}
		}
		out, _ := json.Marshal(tool.OutputSchema)
		if !strings.Contains(string(out), "jump_days") || !strings.Contains(string(out), "scenes_between") {
			t.Errorf("output schema lacks the jump: %s", out)
		}
		return
	}
	t.Fatal("linetta_edit_ledger was not served")
}
