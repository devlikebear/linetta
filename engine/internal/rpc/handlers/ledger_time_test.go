package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/node"
)

// The panel's table: an event's counts in story order, the jump returned when
// no story date is known, and a conflict once one is.
func TestStoryTimeHandlerLaysOutAnEvent(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	repo := ledger.NewRepo(f.store, f.nodes)
	scenes := []string{f.nID}
	for i := range 7 {
		n, err := f.nodes.CreateSibling(ctx, scenes[len(scenes)-1], node.KindLeaf, fmt.Sprintf("%d화", i+2), "", int64(2000+i))
		if err != nil {
			t.Fatal(err)
		}
		scenes = append(scenes, n.ID)
	}
	if err := f.nodes.SetFlashback(ctx, scenes[4], true); err != nil {
		t.Fatal(err)
	}
	ev, err := repo.Add(ctx, 1, ledger.Entry{ProjectID: f.pID, Kind: ledger.KindTimeline, Body: "등대지기 실종", PlantedNodeID: scenes[0]})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := func(now int64, scene int, days float64, body string) {
		if _, err := repo.Add(ctx, now, ledger.Entry{ProjectID: f.pID, Kind: ledger.KindTimeline, Subject: ev.Body, Attribute: ledger.AttributeElapsed,
			Body: body, About: ev.ID, ValueNum: &days, PlantedNodeID: scenes[scene]}); err != nil {
			t.Fatal(err)
		}
	}
	elapsed(2, 2, 90, "석 달")
	elapsed(3, 6, 1095, "삼 년")

	type out struct {
		Scenes []KnowsScene       `json:"scenes"`
		Clocks map[string]float64 `json:"clocks"`
		Events []ledger.TimeRow   `json:"events"`
		Ages   []ledger.TimeRow   `json:"ages"`
	}
	read := func() out {
		t.Helper()
		raw, err := StoryTime(repo, f.nodes)(ctx, json.RawMessage(`{"project_id":"`+f.pID+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		var o out
		if err := json.Unmarshal(raw, &o); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"jump_days"`) && !strings.Contains(string(raw), `"conflicts":[{`) {
			t.Fatalf("neither a jump nor a conflict on the wire: %s", raw)
		}
		return o
	}
	o := read()
	if len(o.Scenes) != 8 || !o.Scenes[4].Flashback || o.Scenes[3].Flashback {
		t.Fatalf("scenes = %+v", o.Scenes)
	}
	if len(o.Events) != 1 || o.Events[0].Event == nil || o.Events[0].Event.ID != ev.ID || len(o.Events[0].Facts) != 2 {
		t.Fatalf("events = %+v", o.Events)
	}
	last := o.Events[0].Facts[1]
	if last.Entry.Body != "삼 년" || len(last.Conflicts) != 0 || last.Previous == nil || last.Previous.JumpDays != 1005 || last.Previous.ScenesBetween != 4 {
		t.Fatalf("삼 년 with no story date = %+v", last)
	}

	for scene, day := range map[int]float64{2: 100, 6: 112} {
		if _, err := repo.Add(ctx, 4, ledger.Entry{ProjectID: f.pID, Kind: ledger.KindTimeline, Subject: "작품", Attribute: ledger.AttributeDate,
			Body: fmt.Sprintf("%g일", day), ValueNum: &day, PlantedNodeID: scenes[scene]}); err != nil {
			t.Fatal(err)
		}
	}
	o = read()
	if o.Clocks[scenes[2]] != 100 || o.Clocks[scenes[6]] != 112 {
		t.Fatalf("clocks = %+v", o.Clocks)
	}
	last = o.Events[0].Facts[1]
	if len(last.Conflicts) != 1 || !strings.Contains(last.Conflicts[0].Reason, "12일이 흘렀는데") {
		t.Fatalf("삼 년 twelve days on = %+v", last)
	}
	if first := o.Events[0].Facts[0]; first.Conflicts == nil || len(first.Conflicts) != 0 || first.Previous != nil {
		t.Fatalf("the first count = %+v", first)
	}
}
