package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/direction"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type directionFix struct {
	store *store.Store
	repo  *direction.Repo
	nr    *node.Repo
	pID   string
	nID   string
	now   Clock
}

func newDirectionFixture(t *testing.T) directionFix {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	nr := node.NewRepo(s)
	return directionFix{
		store: s, repo: direction.NewRepo(s, nr, pr), nr: nr,
		pID: p.ID, nID: *p.LastOpenedNodeID,
		now: func() int64 { return 1000 },
	}
}

func TestDirectionHandlers_getFocus_notSet(t *testing.T) {
	f := newDirectionFixture(t)
	res, err := GetFocus(f.repo)(context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out direction.FocusState
	_ = json.Unmarshal(res, &out)
	if out.Present {
		t.Errorf("got %+v, want Present=false", out)
	}
}

func TestDirectionHandlers_setFocus_andGetFocus(t *testing.T) {
	f := newDirectionFixture(t)
	if err := f.nr.UpdateContent(context.Background(), f.nID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"글"}]}]}`, 1000); err != nil {
		t.Fatalf("write scene: %v", err)
	}
	res, err := SetFocus(f.repo, f.now)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`","body":"둘의 오해를 아직 풀지 않는다"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out direction.FocusState
	_ = json.Unmarshal(res, &out)
	if !out.Present || out.Body != "둘의 오해를 아직 풀지 않는다" {
		t.Fatalf("got %+v", out)
	}
	if out.AnchorNodeID != f.nID {
		t.Fatalf("anchor = %q, want %q (engine's choice)", out.AnchorNodeID, f.nID)
	}

	res, err = GetFocus(f.repo)(context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var again direction.FocusState
	_ = json.Unmarshal(res, &again)
	if !again.Present || again.Body != out.Body {
		t.Fatalf("GetFocus after SetFocus = %+v", again)
	}
}

func TestDirectionHandlers_setFocus_overLimitRejected(t *testing.T) {
	f := newDirectionFixture(t)
	long := strings.Repeat("가", direction.MaxFocusRunes+1)
	body, _ := json.Marshal(long)
	params := json.RawMessage(`{"project_id":"` + f.pID + `","body":` + string(body) + `}`)
	_, err := SetFocus(f.repo, f.now)(context.Background(), params)
	if err == nil {
		t.Fatal("expected an error for an over-limit focus")
	}
}

func TestDirectionHandlers_setFocus_missingProject(t *testing.T) {
	f := newDirectionFixture(t)
	_, err := SetFocus(f.repo, f.now)(context.Background(), json.RawMessage(`{"project_id":"missing","body":"초점"}`))
	if err == nil {
		t.Fatal("expected an error for a missing project")
	}
}

func TestDirectionHandlers_sceneIntent_getAbsent(t *testing.T) {
	f := newDirectionFixture(t)
	res, err := GetSceneIntent(f.repo)(context.Background(), json.RawMessage(`{"node_id":"`+f.nID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out getSceneIntentResult
	_ = json.Unmarshal(res, &out)
	if out.Present {
		t.Fatalf("got %+v, want Present=false", out)
	}
}

func TestDirectionHandlers_sceneIntent_setAndGet(t *testing.T) {
	f := newDirectionFixture(t)
	params := json.RawMessage(`{"node_id":"` + f.nID + `","goal":"둘의 거리를 좁힌다","keep":["침묵"],"avoid":["고백"]}`)
	res, err := SetSceneIntent(f.repo, f.now)(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var si direction.SceneIntent
	_ = json.Unmarshal(res, &si)
	if si.Goal != "둘의 거리를 좁힌다" || len(si.Keep) != 1 || si.Keep[0] != "침묵" || len(si.Avoid) != 1 || si.Avoid[0] != "고백" {
		t.Fatalf("got %+v", si)
	}

	res, err = GetSceneIntent(f.repo)(context.Background(), json.RawMessage(`{"node_id":"`+f.nID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out getSceneIntentResult
	_ = json.Unmarshal(res, &out)
	if !out.Present || out.Intent.Goal != "둘의 거리를 좁힌다" {
		t.Fatalf("got %+v", out)
	}
}

func TestDirectionHandlers_sceneIntent_notScene(t *testing.T) {
	f := newDirectionFixture(t)
	container, err := f.nr.CreateSibling(context.Background(), f.nID, node.KindLeaf, "2화", "", 1000)
	if err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	if err := f.nr.ConvertLeafToContainer(context.Background(), container.ID, 1000); err != nil {
		t.Fatalf("convert to container: %v", err)
	}
	_, err = SetSceneIntent(f.repo, f.now)(context.Background(),
		json.RawMessage(`{"node_id":"`+container.ID+`","goal":"목표"}`))
	if err == nil {
		t.Fatal("expected an error for a non-scene node")
	}
}
