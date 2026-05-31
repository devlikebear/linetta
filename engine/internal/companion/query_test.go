package companion

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
)

func TestParseQuery(t *testing.T) {
	full := "잠깐 찾아볼게요.\n```linetta-query\n{\"queries\":[{\"tool\":\"search_entities\",\"args\":{\"query\":\"하나\"}}]}\n```"
	qr, present, err := ParseQuery(full)
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(qr.Queries) != 1 || qr.Queries[0].Tool != "search_entities" || qr.Queries[0].Args["query"] != "하나" {
		t.Fatalf("qr=%+v", qr)
	}
}

func TestParseQuery_None(t *testing.T) {
	if _, present, err := ParseQuery("그냥 대화"); present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
}

func TestParseQuery_Malformed(t *testing.T) {
	if _, present, err := ParseQuery("```linetta-query\n{bad}\n```"); !present || err == nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
}

func TestRunQueries(t *testing.T) {
	svc, _, projectID := newSvc(t, "안녕")
	ctx := context.Background()

	if _, err := svc.entities.Create(ctx, 1000, entity.NewInput{
		ProjectID: projectID, Kind: entity.KindCharacter, Name: "하나", Role: "POV",
	}); err != nil {
		t.Fatalf("seed entity: %v", err)
	}

	out := svc.runQueries(ctx, projectID, []Query{
		{Tool: "search_entities", Args: map[string]string{"query": "하나"}},
		{Tool: "bogus"},
	})
	if !strings.Contains(out, "하나") {
		t.Fatalf("expected entity name in output: %s", out)
	}
	if !strings.Contains(out, "알 수 없는 도구") {
		t.Fatalf("expected unknown-tool message in output: %s", out)
	}
}

func TestRunQueries_ListScenes(t *testing.T) {
	svc, _, projectID := newSvc(t, "안녕")
	ctx := context.Background()

	p, err := svc.projects.Get(ctx, projectID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if p.LastOpenedNodeID == nil {
		t.Fatal("project has no LastOpenedNodeID; expected auto-created leaf")
	}
	firstSceneID := *p.LastOpenedNodeID

	out := svc.runOneQuery(ctx, projectID, Query{Tool: "list_scenes"})
	if !strings.Contains(out, firstSceneID) {
		t.Fatalf("list_scenes should list the first scene id %q:\n%s", firstSceneID, out)
	}
}
