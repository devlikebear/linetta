package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/continuity"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type continuityFix struct {
	store    *store.Store
	repo     *continuity.Repo
	nr       *node.Repo
	entities *entity.Repo
	facts    *fact.Repo
	beats    *beat.Repo
	pID      string
	nID      string
	now      Clock
}

func newContinuityHandlerFixture(t *testing.T) continuityFix {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	nr := node.NewRepo(s)
	return continuityFix{
		store: s, repo: continuity.NewRepo(s), nr: nr,
		entities: entity.NewRepo(s), facts: fact.NewRepo(s), beats: beat.NewRepo(s),
		pID: p.ID, nID: *p.LastOpenedNodeID,
		now: func() int64 { return 2000 },
	}
}

func (f continuityFix) record(t *testing.T, obs ...continuity.ObservationInput) continuity.Check {
	t.Helper()
	n, err := f.nr.Get(context.Background(), f.nID)
	if err != nil {
		t.Fatalf("Get node: %v", err)
	}
	check, err := f.repo.Record(context.Background(), f.now(), continuity.ReportInput{
		ProjectID: f.pID, NodeID: f.nID, SceneVersion: n.ContentVersion, Observations: obs,
	}, continuity.SourceAgent)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	return check
}

func cleanObs() continuity.ObservationInput {
	return continuity.ObservationInput{
		Category: continuity.CategoryOther, Kind: continuity.KindQuestion, Severity: continuity.SeverityLow,
		Summary: "확인 필요", ParagraphFrom: 1, ParagraphTo: 1, Quote: "본문",
	}
}

func TestContinuityHandlers_list_breadcrumbAndRefLabels(t *testing.T) {
	f := newContinuityHandlerFixture(t)
	e, err := f.entities.Create(context.Background(), 1, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "하린"})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	obs := cleanObs()
	obs.Kind = continuity.KindContradiction
	obs.Refs = []continuity.Ref{{Type: continuity.RefTypeEntity, ID: e.ID}}
	f.record(t, obs)

	res, err := ListContinuity(f.repo, f.nr, f.entities, f.facts, f.beats)(
		context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("ListContinuity: %v", err)
	}
	var out struct {
		Checks []continuityCheckOut `json:"checks"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(out.Checks))
	}
	check := out.Checks[0]
	if check.Breadcrumb == "" {
		t.Error("breadcrumb is empty")
	}
	if len(check.Observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(check.Observations))
	}
	if got := check.Observations[0].Refs[0].Label; got != "하린" {
		t.Errorf("ref label = %q, want %q", got, "하린")
	}
	if check.Stale {
		t.Error("a check right after recording must not be stale")
	}
}

func TestContinuityHandlers_list_staleAfterSceneEdit(t *testing.T) {
	f := newContinuityHandlerFixture(t)
	f.record(t, cleanObs())

	if err := f.nr.UpdateContent(context.Background(), f.nID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"바뀐 본문"}]}]}`, 2500); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	res, err := ListContinuity(f.repo, f.nr, f.entities, f.facts, f.beats)(
		context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("ListContinuity: %v", err)
	}
	var out struct {
		Checks []continuityCheckOut `json:"checks"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Checks[0].Stale {
		t.Error("check should be stale after the scene's content_version moved")
	}
}

func TestContinuityHandlers_setStatus(t *testing.T) {
	f := newContinuityHandlerFixture(t)
	f.record(t, cleanObs())
	listRes, err := ListContinuity(f.repo, f.nr, f.entities, f.facts, f.beats)(
		context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("ListContinuity: %v", err)
	}
	var out struct {
		Checks []continuityCheckOut `json:"checks"`
	}
	if err := json.Unmarshal(listRes, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	obsID := out.Checks[0].Observations[0].ID

	var method string
	var payload any
	notify := func(m string, p any) { method, payload = m, p }
	res, err := SetContinuityStatus(f.repo, f.now, notify)(context.Background(),
		json.RawMessage(`{"observation_id":"`+obsID+`","status":"dismissed"}`))
	if err != nil {
		t.Fatalf("SetContinuityStatus: %v", err)
	}
	var obs continuity.Observation
	if err := json.Unmarshal(res, &obs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// A ContextPanel in another window — or the one the writer came back to
	// after closing Story World — hears about this change rather than
	// showing a stale open-observation count until something else happens
	// to remount it.
	if method != "continuity.changed" {
		t.Errorf("method = %q, want continuity.changed", method)
	}
	if p, ok := payload.(continuityChangedPayload); !ok || p.ProjectID != f.pID || p.NodeID != f.nID {
		t.Errorf("payload = %+v, want project_id %q and node_id %q", payload, f.pID, f.nID)
	}
	if obs.Status != continuity.StatusDismissed {
		t.Errorf("status = %q, want %q", obs.Status, continuity.StatusDismissed)
	}
}

func TestContinuityHandlers_setStatus_cannotTouchReplaced(t *testing.T) {
	f := newContinuityHandlerFixture(t)
	f.record(t, cleanObs())
	listRes, _ := ListContinuity(f.repo, f.nr, f.entities, f.facts, f.beats)(
		context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	var out struct {
		Checks []continuityCheckOut `json:"checks"`
	}
	_ = json.Unmarshal(listRes, &out)
	obsID := out.Checks[0].Observations[0].ID

	f.record(t) // a second, empty check replaces the first's open observation

	notified := ""
	notify := func(m string, _ any) { notified = m }
	_, err := SetContinuityStatus(f.repo, f.now, notify)(context.Background(),
		json.RawMessage(`{"observation_id":"`+obsID+`","status":"resolved"}`))
	if err == nil {
		t.Fatal("SetStatus on a replaced observation should fail")
	}
	if notified != "" {
		t.Errorf("a refused status change emitted %q; nothing changed", notified)
	}
}

func TestContinuityHandlers_openCounts(t *testing.T) {
	f := newContinuityHandlerFixture(t)
	f.record(t, cleanObs(), cleanObs())

	res, err := ContinuityOpenCounts(f.repo)(context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("ContinuityOpenCounts: %v", err)
	}
	var out struct {
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Counts[f.nID] != 2 {
		t.Errorf("counts[node] = %d, want 2", out.Counts[f.nID])
	}
}
