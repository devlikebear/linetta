package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/organize"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

func TestAgentOrganizeStartsOnChangedScenesAndRecordsTheOutcome(t *testing.T) {
	ctx := context.Background()
	s, err := storetest.Open(ctx, filepath.Join(t.TempDir(), "o.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr, nr := project.NewRepo(s), node.NewRepo(s)
	p, _ := pr.Create(ctx, 1, project.NewInput{Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first", OutlinePreset: project.OutlinePresetWebNovel})
	ep, _ := nr.CreateSibling(ctx, *p.LastOpenedNodeID, node.KindContainer, "12화", "", 2)
	sc, _ := nr.CreateChild(ctx, ep.ID, node.KindLeaf, "씬", "", 3)
	text := strings.Repeat("하린이 창문을 두드렸다. ", 8)
	_ = nr.UpdateContent(ctx, sc.ID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"`+text+`"}]}]}`, 4)
	_ = nr.Delete(ctx, *p.LastOpenedNodeID, 5)

	ag := &fakeAgent{}
	h := AgentOrganize(ag, organize.NewRepo(s, nr, pr), func(context.Context) string { return "ko" }, func() int64 { return 9 })
	params := json.RawMessage(`{"project_id":"` + p.ID + `","node_id":"` + sc.ID + `"}`)
	res, err := h(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	var out organizeResult
	_ = json.Unmarshal(res, &out)
	if !out.Started || out.Scenes != 1 || out.EpisodeLabel != "12화" || ag.organized == nil || ag.organized.Language != "ko" {
		t.Fatalf("result = %+v, start = %+v", out, ag.organized)
	}
	ag.organizeEnd(ctx, organize.OutcomeDone)

	// Read as it is: asking again starts nothing.
	ag.organized = nil
	res, _ = h(ctx, params)
	_ = json.Unmarshal(res, &out)
	if out.Started || out.Unchanged != 1 || ag.organized != nil {
		t.Fatalf("second ask = %+v", out)
	}

	ag.organizeErr = &rpc.ReasonError{Reason: rpc.ReasonOrganizeTrial, Err: errors.New("trial")}
	_, err = h(ctx, json.RawMessage(`{"project_id":"`+p.ID+`","node_id":"`+sc.ID+`","force":true}`))
	var me *rpc.MethodError
	if !errors.As(err, &me) || !strings.Contains(string(me.Data), "organize_trial") {
		t.Fatalf("trial err = %v", err)
	}
	_, err = h(ctx, json.RawMessage(`{"project_id":"`+p.ID+`","node_id":"nowhere"}`))
	if !errors.As(err, &me) || !strings.Contains(string(me.Data), "organize_not_in_episode") {
		t.Fatalf("outside err = %v", err)
	}
}
