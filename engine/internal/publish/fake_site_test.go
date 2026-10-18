package publish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeSite is an in-memory Linetta Works speaking docs/api.md, enough of it
// for the client, the connect flow and the service to be tested end to end
// without a network.
type fakeSite struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	token    string
	works    map[string]*fakeWork
	requests []string
	// poll answers pair/poll in order; the last one repeats.
	poll []string
	// fail makes the next request matching "METHOD /path-prefix" answer with
	// the given status and code, once.
	fail []fakeFailure
}

type fakeFailure struct {
	match  string
	status int
	code   string
}

type fakeWork struct {
	work     Work
	episodes []RemoteEpisode
}

const fakeToken = "lwk_test_token"

func newFakeSite(t *testing.T) *fakeSite {
	t.Helper()
	f := &fakeSite{t: t, token: fakeToken, works: map[string]*fakeWork{}, poll: []string{PollApproved}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/artworks/pair/start", f.pairStart)
	mux.HandleFunc("POST /api/artworks/pair/poll", f.pairPoll)
	mux.HandleFunc("GET /api/artworks/me", f.auth(f.me))
	mux.HandleFunc("PUT /api/artworks/works/{pid}", f.auth(f.putWork))
	mux.HandleFunc("GET /api/artworks/works/{pid}", f.auth(f.getWork))
	mux.HandleFunc("PUT /api/artworks/works/{pid}/episodes/{nid}", f.auth(f.putEpisode))
	mux.HandleFunc("PATCH /api/artworks/works/{pid}/episodes/{nid}", f.auth(f.patchEpisode))
	mux.HandleFunc("PUT /api/artworks/works/{pid}/episodes:reorder", f.auth(f.reorder))
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		for i, fl := range f.fail {
			if strings.HasPrefix(r.Method+" "+r.URL.Path, fl.match) {
				f.fail = append(f.fail[:i], f.fail[i+1:]...)
				f.mu.Unlock()
				writeErr(w, fl.status, fl.code)
				return
			}
		}
		f.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSite) client() *Client {
	c := NewClient(f.srv.URL, "test")
	c.retryDelay = 0
	return c
}

func (f *fakeSite) failNext(match string, status int, code string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = append(f.fail, fakeFailure{match: match, status: status, code: code})
}

func (f *fakeSite) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.requests...)
}

func (f *fakeSite) episodes(pid string) []RemoteEpisode {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := f.works[pid]
	if w == nil {
		return nil
	}
	return append([]RemoteEpisode{}, w.episodes...)
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	body["api_version"] = 1
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, code string) {
	if code == "" {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, map[string]any{"code": code, "message": "실패"})
}

func (f *fakeSite) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		ok := f.token != "" && r.Header.Get("Authorization") == "Bearer "+f.token
		f.mu.Unlock()
		if !ok {
			writeErr(w, 401, CodeUnauthorized)
			return
		}
		next(w, r)
	}
}

func (f *fakeSite) pairStart(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"pairing_id": "pair1", "device_secret": "secret", "confirm_code": "NWCQ-BKWX",
		"verify_url": f.srv.URL + "/connect?pairing=pair1", "expires_in": 600, "interval": 0,
	})
}

func (f *fakeSite) pairPoll(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PairingID    string `json:"pairing_id"`
		DeviceSecret string `json:"device_secret"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.DeviceSecret != "secret" {
		writeErr(w, 401, CodeUnauthorized)
		return
	}
	f.mu.Lock()
	status := f.poll[0]
	if len(f.poll) > 1 {
		f.poll = f.poll[1:]
	}
	f.mu.Unlock()
	body := map[string]any{"status": status}
	if status == PollApproved {
		body["token"], body["handle"], body["pen_name"] = f.token, "devlikebear", "마곰"
	}
	writeJSON(w, 200, body)
}

func (f *fakeSite) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"handle": "devlikebear", "pen_name": "마곰", "can_publish": true})
}

func (f *fakeSite) putWork(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("pid")
	var in WorkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Title == "" {
		writeErr(w, 400, CodeInvalidRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.works[pid]
	if !ok {
		slug := in.Slug
		if slug == "" {
			slug = "work-" + pid
		}
		existing = &fakeWork{work: Work{
			ProjectID: pid, Slug: slug, SerialStatus: SerialOngoing, Language: "ko",
			Visibility: VisibilityPublic, URL: f.srv.URL + "/@devlikebear/" + slug,
		}}
		f.works[pid] = existing
	}
	existing.work.Title = in.Title
	if in.Blurb != nil {
		existing.work.Blurb = *in.Blurb
	}
	if in.Genres != nil {
		existing.work.Genres = in.Genres
	}
	if in.SerialStatus != "" {
		existing.work.SerialStatus = in.SerialStatus
	}
	if in.Language != "" {
		existing.work.Language = in.Language
	}
	writeJSON(w, 200, map[string]any{"created": !ok, "work": existing.work})
}

func (f *fakeSite) getWork(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	work, ok := f.works[r.PathValue("pid")]
	if !ok {
		writeErr(w, 404, CodeNotFound)
		return
	}
	writeJSON(w, 200, map[string]any{"work": work.work, "episodes": work.episodes})
}

func (f *fakeSite) putEpisode(w http.ResponseWriter, r *http.Request) {
	var in EpisodeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Number < 1 || in.BodyMD == "" {
		writeErr(w, 400, CodeInvalidRequest)
		return
	}
	sum := sha256.Sum256([]byte(in.BodyMD))
	if hex.EncodeToString(sum[:]) != in.ContentHash {
		writeErr(w, 400, CodeHashMismatch)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	work, ok := f.works[r.PathValue("pid")]
	if !ok {
		writeErr(w, 404, CodeNotFound)
		return
	}
	nid := r.PathValue("nid")
	idx := -1
	for i, ep := range work.episodes {
		if ep.NodeID == nid {
			idx = i
			continue
		}
		if ep.Number == in.Number {
			writeErr(w, 409, CodeNumberTaken)
			return
		}
	}
	ep := RemoteEpisode{NodeID: nid, Status: RemotePublic, Visibility: VisibilityPublic, Moderation: "passed",
		PublishedAt: "2026-10-18 02:30:40.260Z"}
	if idx >= 0 {
		ep = work.episodes[idx]
	}
	ep.Number, ep.Title, ep.ContentHash = in.Number, in.Title, in.ContentHash
	ep.CharCount = len([]rune(strings.Join(strings.Fields(in.BodyMD), "")))
	ep.URL = fmt.Sprintf("%s/%d", work.work.URL, in.Number)
	if idx >= 0 {
		work.episodes[idx] = ep
	} else {
		work.episodes = append(work.episodes, ep)
	}
	sort.Slice(work.episodes, func(i, j int) bool { return work.episodes[i].Number < work.episodes[j].Number })
	writeJSON(w, 200, map[string]any{"created": idx < 0, "episode": ep})
}

func (f *fakeSite) patchEpisode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Visibility string `json:"visibility"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	work, ok := f.works[r.PathValue("pid")]
	if !ok {
		writeErr(w, 404, CodeNotFound)
		return
	}
	for i, ep := range work.episodes {
		if ep.NodeID == r.PathValue("nid") {
			ep.Visibility, ep.Status = in.Visibility, in.Visibility
			work.episodes[i] = ep
			writeJSON(w, 200, map[string]any{"episode": ep})
			return
		}
	}
	writeErr(w, 404, CodeNotFound)
}

func (f *fakeSite) reorder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Order []string `json:"order"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	work, ok := f.works[r.PathValue("pid")]
	if !ok {
		writeErr(w, 404, CodeNotFound)
		return
	}
	byID := map[string]RemoteEpisode{}
	for _, ep := range work.episodes {
		byID[ep.NodeID] = ep
	}
	if len(in.Order) != len(byID) {
		writeErr(w, 400, CodeInvalidRequest)
		return
	}
	next := make([]RemoteEpisode, 0, len(in.Order))
	for i, id := range in.Order {
		ep, ok := byID[id]
		if !ok {
			writeErr(w, 400, CodeInvalidRequest)
			return
		}
		delete(byID, id)
		ep.Number = i + 1
		ep.URL = fmt.Sprintf("%s/%d", work.work.URL, ep.Number)
		next = append(next, ep)
	}
	work.episodes = next
	writeJSON(w, 200, map[string]any{"episodes": next})
}
