package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/internal/agent"
	"github.com/devlikebear/linetta/internal/memory"
	"github.com/devlikebear/linetta/internal/store"
	"github.com/devlikebear/linetta/internal/work"
	tesserarun "github.com/devlikebear/tessera/pkg/run"
)

func TestHealth(t *testing.T) {
	handler := newTestHandler(t)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if !strings.Contains(res.Body.String(), `"ok":true`) {
		t.Fatalf("body = %s, want ok true", res.Body.String())
	}
}

func TestWorkRoutesCreateListAndGetWork(t *testing.T) {
	handler := newTestHandler(t)

	created := postJSON[work.Work](t, handler, "/api/works", map[string]string{
		"title":   "Green Harbor",
		"genre":   "climate fiction",
		"premise": "A caretaker hears a forgotten machine singing.",
	}, http.StatusCreated)
	if created.ID == "" || created.Title != "Green Harbor" {
		t.Fatalf("created work = %+v, want id and title", created)
	}

	works := getJSON[[]work.Work](t, handler, "/api/works", http.StatusOK)
	if len(works) != 1 || works[0].ID != created.ID {
		t.Fatalf("works = %+v, want created work", works)
	}

	got := getJSON[work.Work](t, handler, "/api/works/"+created.ID, http.StatusOK)
	if got.ID != created.ID || got.Premise != created.Premise {
		t.Fatalf("got = %+v, want created %+v", got, created)
	}
}

func TestWorkRoutesReportMissingWork(t *testing.T) {
	handler := newTestHandler(t)

	var payload map[string]string
	res := requestJSON(t, handler, http.MethodGet, "/api/works/missing", nil)
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["error"] == "" {
		t.Fatalf("payload = %+v, want error", payload)
	}
}

func TestEpisodeRoutesAreScopedToWork(t *testing.T) {
	handler := newTestHandler(t)

	first := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "First"}, http.StatusCreated)
	second := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "Second"}, http.StatusCreated)

	episode := postJSON[work.Episode](t, handler, "/api/works/"+first.ID+"/episodes", map[string]string{
		"title": "Opening Bell",
	}, http.StatusCreated)
	if episode.WorkID != first.ID {
		t.Fatalf("episode.WorkID = %q, want %q", episode.WorkID, first.ID)
	}
	_ = postJSON[work.Episode](t, handler, "/api/works/"+second.ID+"/episodes", map[string]string{
		"title": "Other Opening",
	}, http.StatusCreated)

	episodes := getJSON[[]work.Episode](t, handler, "/api/works/"+first.ID+"/episodes", http.StatusOK)
	if len(episodes) != 1 || episodes[0].ID != episode.ID {
		t.Fatalf("episodes = %+v, want only first work episode %+v", episodes, episode)
	}
}

func TestEpisodeStatusRouteUpdatesStatus(t *testing.T) {
	handler := newTestHandler(t)
	createdWork := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "Status API Work"}, http.StatusCreated)
	episode := postJSON[work.Episode](t, handler, "/api/works/"+createdWork.ID+"/episodes", map[string]string{
		"title": "Episode 1",
	}, http.StatusCreated)

	updated := patchJSON[work.Episode](t, handler, "/api/works/"+createdWork.ID+"/episodes/"+episode.ID+"/status", map[string]string{
		"status": string(work.EpisodeStatusReady),
	}, http.StatusOK)
	if updated.Status != work.EpisodeStatusReady {
		t.Fatalf("status = %q, want %q", updated.Status, work.EpisodeStatusReady)
	}

	res := requestJSON(t, handler, http.MethodPatch, "/api/works/"+createdWork.ID+"/episodes/"+episode.ID+"/status", map[string]string{
		"status": "unknown",
	})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", res.Code, res.Body.String())
	}
}

func TestBlueprintRoutesSaveAndGetEpisodeBlueprint(t *testing.T) {
	handler := newTestHandler(t)
	createdWork := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "Blueprint Work"}, http.StatusCreated)
	episode := postJSON[work.Episode](t, handler, "/api/works/"+createdWork.ID+"/episodes", map[string]string{
		"title": "Episode 1",
	}, http.StatusCreated)

	saved := putJSON[work.EpisodeBlueprint](t, handler, "/api/works/"+createdWork.ID+"/episodes/"+episode.ID+"/blueprint", map[string]string{
		"premise":         "Mira hears the harbor singing.",
		"theme":           "Memory as civic infrastructure",
		"situation":       "A pump changes rhythm before sunset.",
		"must_include":    "The lullaby clue",
		"must_avoid":      "No exposition dump",
		"structure_notes": "Open with ritual, end with a message.",
	}, http.StatusOK)
	if saved.WorkID != createdWork.ID || saved.EpisodeID != episode.ID {
		t.Fatalf("saved blueprint = %+v", saved)
	}

	got := getJSON[work.EpisodeBlueprint](t, handler, "/api/works/"+createdWork.ID+"/episodes/"+episode.ID+"/blueprint", http.StatusOK)
	if got.ID != saved.ID || got.Premise != saved.Premise {
		t.Fatalf("got = %+v, want saved %+v", got, saved)
	}
}

func TestEpisodeVersionRoutesCreateAndList(t *testing.T) {
	handler := newTestHandler(t)
	createdWork := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "Version API Work"}, http.StatusCreated)
	episode := postJSON[work.Episode](t, handler, "/api/works/"+createdWork.ID+"/episodes", map[string]string{
		"title": "Episode 1",
	}, http.StatusCreated)

	created := postJSON[work.EpisodeVersion](t, handler, "/api/works/"+createdWork.ID+"/episodes/"+episode.ID+"/versions", map[string]string{
		"source_artifact_id": "artifact_1",
		"body":               "Adopted manuscript body.",
		"note":               "adopt edited draft",
	}, http.StatusCreated)
	if created.ID == "" || created.SourceArtifactID != "artifact_1" {
		t.Fatalf("created version = %+v, want id and source artifact", created)
	}

	versions := getJSON[[]work.EpisodeVersion](t, handler, "/api/works/"+createdWork.ID+"/episodes/"+episode.ID+"/versions", http.StatusOK)
	if len(versions) != 1 || versions[0].ID != created.ID {
		t.Fatalf("versions = %+v, want created version %+v", versions, created)
	}
}

func TestBlueprintRoutesRejectWrongWork(t *testing.T) {
	handler := newTestHandler(t)
	first := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "First"}, http.StatusCreated)
	second := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "Second"}, http.StatusCreated)
	episode := postJSON[work.Episode](t, handler, "/api/works/"+first.ID+"/episodes", map[string]string{
		"title": "Episode 1",
	}, http.StatusCreated)

	res := requestJSON(t, handler, http.MethodPut, "/api/works/"+second.ID+"/episodes/"+episode.ID+"/blueprint", map[string]string{
		"premise": "Wrong owner",
	})
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", res.Code, res.Body.String())
	}
}

func TestRunRoutesCreateRunAndReturnArtifactsEventsAndStream(t *testing.T) {
	handler := newTestHandler(t)
	createdWork := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "Run Work"}, http.StatusCreated)
	episode := postJSON[work.Episode](t, handler, "/api/works/"+createdWork.ID+"/episodes", map[string]string{
		"title": "Episode 1",
	}, http.StatusCreated)
	_ = putJSON[work.EpisodeBlueprint](t, handler, "/api/works/"+createdWork.ID+"/episodes/"+episode.ID+"/blueprint", map[string]string{
		"premise":         "Mira hears the harbor singing.",
		"theme":           "Memory as infrastructure",
		"situation":       "A pump changes rhythm before sunset.",
		"must_include":    "lullaby clue",
		"must_avoid":      "exposition dump",
		"structure_notes": "Open with ritual, end with a message.",
	}, http.StatusOK)

	result := postJSON[agent.EpisodeRunResult](t, handler, "/api/works/"+createdWork.ID+"/episodes/"+episode.ID+"/runs", map[string]string{
		"approved_by": "tester",
	}, http.StatusCreated)
	if result.RunID == "" || result.Closure != string(tesserarun.ClosureNormal) {
		t.Fatalf("run result = %+v, want run id and normal closure", result)
	}

	artifacts := getJSON[[]agent.Artifact](t, handler, "/api/runs/"+result.RunID+"/artifacts", http.StatusOK)
	if len(artifacts) != 7 {
		t.Fatalf("artifacts len = %d, want 7", len(artifacts))
	}

	events := getJSON[[]tesserarun.Event](t, handler, "/api/runs/"+result.RunID+"/events", http.StatusOK)
	if len(events) == 0 {
		t.Fatal("expected stored events")
	}

	stream := requestJSON(t, handler, http.MethodGet, "/api/runs/"+result.RunID+"/events/stream", nil)
	if stream.Code != http.StatusOK {
		t.Fatalf("stream status = %d, want 200; body=%s", stream.Code, stream.Body.String())
	}
	if got := stream.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("stream content-type = %q, want text/event-stream", got)
	}
	if !strings.Contains(stream.Body.String(), "task.succeeded") {
		t.Fatalf("stream body missing task.succeeded:\n%s", stream.Body.String())
	}
}

func TestProposalAndContinuityRoutes(t *testing.T) {
	env := newTestEnvironment(t)
	ctx := context.Background()
	workItem, err := env.Work.CreateWork(ctx, work.CreateWorkInput{Title: "Proposal API Work"})
	if err != nil {
		t.Fatalf("CreateWork() error = %v", err)
	}
	episode, err := env.Work.CreateEpisode(ctx, workItem.ID, "Episode 1")
	if err != nil {
		t.Fatalf("CreateEpisode() error = %v", err)
	}
	runID := insertServerTestRun(t, env.DB, workItem.ID, episode.ID)
	proposal, err := env.Memory.CreateProposal(ctx, memory.CreateProposalInput{
		WorkID:     workItem.ID,
		EpisodeID:  episode.ID,
		RunID:      runID,
		ChangeType: memory.ProposalChangeCreate,
		Kind:       memory.KindCharacter,
		Title:      "Proposed Character",
		AfterBody:  "A person extracted from the draft.",
		Reason:     "Detected recurring figure.",
	})
	if err != nil {
		t.Fatalf("CreateProposal() error = %v", err)
	}
	issue, err := env.Memory.CreateIssue(ctx, memory.CreateIssueInput{
		WorkID:    workItem.ID,
		EpisodeID: episode.ID,
		RunID:     runID,
		Severity:  memory.IssueWarning,
		Title:     "Ambiguous timeline",
		Body:      "Needs an anchored date.",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	proposals := getJSON[[]memory.Proposal](t, env.Handler, "/api/works/"+workItem.ID+"/proposals?status=pending", http.StatusOK)
	if len(proposals) != 1 || proposals[0].ID != proposal.ID {
		t.Fatalf("proposals = %+v, want pending proposal %+v", proposals, proposal)
	}

	approved := postJSON[memory.Proposal](t, env.Handler, "/api/proposals/"+proposal.ID+"/approve", map[string]string{
		"actor": "human",
	}, http.StatusOK)
	if approved.Status != memory.ProposalApproved || approved.TargetItemID == "" {
		t.Fatalf("approved = %+v, want approved proposal", approved)
	}

	issues := getJSON[[]memory.ContinuityIssue](t, env.Handler, "/api/works/"+workItem.ID+"/episodes/"+episode.ID+"/continuity", http.StatusOK)
	if len(issues) != 1 || issues[0].ID != issue.ID {
		t.Fatalf("issues = %+v, want issue %+v", issues, issue)
	}
	resolved := patchJSON[memory.ContinuityIssue](t, env.Handler, "/api/continuity/"+issue.ID, map[string]string{
		"status": string(memory.IssueResolved),
	}, http.StatusOK)
	if resolved.Status != memory.IssueResolved {
		t.Fatalf("resolved status = %q, want resolved", resolved.Status)
	}
}

func TestMemoryRoutesCreateUpdateArchiveAndListDecisions(t *testing.T) {
	handler := newTestHandler(t)
	createdWork := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "Canon Work"}, http.StatusCreated)

	created := postJSON[memory.Item](t, handler, "/api/works/"+createdWork.ID+"/memory", map[string]string{
		"kind":       string(memory.KindCharacter),
		"title":      "Mira",
		"body":       "A tide-garden caretaker.",
		"status":     string(memory.StatusDraft),
		"importance": string(memory.ImportanceHigh),
		"reason":     "Initial seed",
		"actor":      "human",
	}, http.StatusCreated)
	if created.WorkID != createdWork.ID || created.Status != memory.StatusDraft {
		t.Fatalf("created memory item = %+v", created)
	}

	updated := patchJSON[memory.Item](t, handler, "/api/works/"+createdWork.ID+"/memory/"+created.ID, map[string]string{
		"title":      "Mira",
		"body":       "A tide-garden caretaker who hears old infrastructure singing.",
		"status":     string(memory.StatusCanon),
		"importance": string(memory.ImportanceHigh),
		"reason":     "Promote to canon",
		"actor":      "human",
	}, http.StatusOK)
	if updated.Status != memory.StatusCanon {
		t.Fatalf("updated status = %q, want canon", updated.Status)
	}

	items := getJSON[[]memory.Item](t, handler, "/api/works/"+createdWork.ID+"/memory?kind=character&status=canon", http.StatusOK)
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("items = %+v, want updated item", items)
	}

	archived := postJSON[memory.Item](t, handler, "/api/works/"+createdWork.ID+"/memory/"+created.ID+"/archive", map[string]string{
		"reason": "Superseded",
		"actor":  "human",
	}, http.StatusOK)
	if archived.Status != memory.StatusArchived {
		t.Fatalf("archived status = %q, want archived", archived.Status)
	}

	decisions := getJSON[[]memory.Decision](t, handler, "/api/works/"+createdWork.ID+"/memory/decisions", http.StatusOK)
	if len(decisions) != 3 {
		t.Fatalf("decisions len = %d, want 3", len(decisions))
	}
}

func TestMemoryRoutesDoNotExposeItemsAcrossWorks(t *testing.T) {
	handler := newTestHandler(t)
	first := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "First"}, http.StatusCreated)
	second := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "Second"}, http.StatusCreated)
	item := postJSON[memory.Item](t, handler, "/api/works/"+first.ID+"/memory", map[string]string{
		"kind":  string(memory.KindWorldFact),
		"title": "Harbor",
	}, http.StatusCreated)

	res := requestJSON(t, handler, http.MethodGet, "/api/works/"+second.ID+"/memory/"+item.ID, nil)
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", res.Code, res.Body.String())
	}
}

func TestMemorySearchRoute(t *testing.T) {
	handler := newTestHandler(t)
	createdWork := postJSON[work.Work](t, handler, "/api/works", map[string]string{"title": "Search Work"}, http.StatusCreated)
	needle := postJSON[memory.Item](t, handler, "/api/works/"+createdWork.ID+"/memory", map[string]string{
		"kind":  string(memory.KindSource),
		"title": "Tide Gardens",
		"body":  "Research notes about coastal infrastructure.",
	}, http.StatusCreated)
	_ = postJSON[memory.Item](t, handler, "/api/works/"+createdWork.ID+"/memory", map[string]string{
		"kind":  string(memory.KindSource),
		"title": "Unrelated",
		"body":  "No matching term.",
	}, http.StatusCreated)

	items := getJSON[[]memory.Item](t, handler, "/api/works/"+createdWork.ID+"/memory/search?q=coastal", http.StatusOK)
	if len(items) != 1 || items[0].ID != needle.ID {
		t.Fatalf("items = %+v, want search result %+v", items, needle)
	}
}

type testEnvironment struct {
	DB      *store.DB
	Work    *work.Repository
	Memory  *memory.Repository
	Handler http.Handler
}

func newTestHandler(t *testing.T) http.Handler {
	return newTestEnvironment(t).Handler
}

func newTestEnvironment(t *testing.T) testEnvironment {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "linetta.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	workRepo := work.NewRepository(db)
	memoryRepo := memory.NewRepository(db, workRepo)
	handler := New(workRepo, Options{
		Memory: memoryRepo,
		Agent:  agent.NewRunner(db, workRepo, memoryRepo),
	})
	return testEnvironment{DB: db, Work: workRepo, Memory: memoryRepo, Handler: handler}
}

func insertServerTestRun(t *testing.T, db *store.DB, workID, episodeID string) string {
	t.Helper()
	runID := "run_server_test"
	if _, err := db.Conn().ExecContext(context.Background(), `
		INSERT INTO agent_runs (id, work_id, episode_id, status, tessera_run_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, runID, workID, episodeID, "closed", runID, "2026-05-24T09:00:00Z"); err != nil {
		t.Fatalf("insert run error = %v", err)
	}
	return runID
}

func postJSON[T any](t *testing.T, handler http.Handler, path string, payload any, wantStatus int) T {
	t.Helper()
	res := requestJSON(t, handler, http.MethodPost, path, payload)
	if res.Code != wantStatus {
		t.Fatalf("POST %s status = %d, want %d; body=%s", path, res.Code, wantStatus, res.Body.String())
	}
	var decoded T
	if err := json.Unmarshal(res.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, res.Body.String())
	}
	return decoded
}

func patchJSON[T any](t *testing.T, handler http.Handler, path string, payload any, wantStatus int) T {
	t.Helper()
	res := requestJSON(t, handler, http.MethodPatch, path, payload)
	if res.Code != wantStatus {
		t.Fatalf("PATCH %s status = %d, want %d; body=%s", path, res.Code, wantStatus, res.Body.String())
	}
	var decoded T
	if err := json.Unmarshal(res.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, res.Body.String())
	}
	return decoded
}

func putJSON[T any](t *testing.T, handler http.Handler, path string, payload any, wantStatus int) T {
	t.Helper()
	res := requestJSON(t, handler, http.MethodPut, path, payload)
	if res.Code != wantStatus {
		t.Fatalf("PUT %s status = %d, want %d; body=%s", path, res.Code, wantStatus, res.Body.String())
	}
	var decoded T
	if err := json.Unmarshal(res.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, res.Body.String())
	}
	return decoded
}

func getJSON[T any](t *testing.T, handler http.Handler, path string, wantStatus int) T {
	t.Helper()
	res := requestJSON(t, handler, http.MethodGet, path, nil)
	if res.Code != wantStatus {
		t.Fatalf("GET %s status = %d, want %d; body=%s", path, res.Code, wantStatus, res.Body.String())
	}
	var decoded T
	if err := json.Unmarshal(res.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, res.Body.String())
	}
	return decoded
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatalf("json.Encode() error = %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
