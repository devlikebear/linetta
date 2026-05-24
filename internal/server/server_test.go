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

	"github.com/devlikebear/linetta/internal/store"
	"github.com/devlikebear/linetta/internal/work"
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

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "linetta.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return New(work.NewRepository(db), Options{})
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
