package vault

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const testToken = "lwk_test"

// fakeSite follows the vault half of docs/api.md in linetta-artworks, for one
// account, including what a client can only learn the hard way: commit reports
// missing blobs before a conflict, and names at most a limited number of them.
type fakeSite struct {
	mu        sync.Mutex
	srv       *httptest.Server
	mode      string
	blobs     map[string][]byte
	items     map[string]*fakeItem
	missedCap int

	puts, gets, commits, requests int
	putsByID                      map[string]int

	// failPut and failGet answer with a status instead of serving, when set.
	// failCommitAfterSaving commits and then loses the answer.
	failPut, failGet      func(id string) int
	failCommitAfterSaving bool
	retryAfter            string
}

type fakeItem struct {
	head      int
	updated   string
	revisions []Revision
}

func newFakeSite(t *testing.T) *fakeSite {
	t.Helper()
	f := &fakeSite{mode: ModeFull, blobs: map[string][]byte{}, items: map[string]*fakeItem{}, missedCap: 1000, putsByID: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"code": code, "message": code})
}

func (f *fakeSite) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if r.Header.Get("Authorization") != "Bearer "+testToken {
		fail(w, 401, CodeUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/artworks/vault")
	switch {
	case path == "/status" && r.Method == http.MethodGet:
		writeJSON(w, 200, Account{Mode: f.mode, Status: "approved", TermsAccepted: true, Usage: Usage{QuotaBytes: 200 << 20}, Limits: Limits{BlobBytes: MaxBlobBytes}})
	case f.mode == ModeNone:
		fail(w, 403, CodeNotEnabled)
	case path == "/items" && r.Method == http.MethodGet:
		f.listItems(w)
	case strings.HasPrefix(path, "/blobs/"):
		f.blob(w, r, strings.TrimPrefix(path, "/blobs/"))
	case strings.HasPrefix(path, "/items/") && strings.HasSuffix(path, "/revisions"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/items/"), "/revisions")
		if r.Method == http.MethodPost {
			f.commit(w, r, id)
			return
		}
		item, ok := f.items[id]
		if !ok {
			fail(w, 404, CodeNotFound)
			return
		}
		list := append([]Revision(nil), item.revisions...)
		sort.Slice(list, func(i, j int) bool { return list[i].Revision > list[j].Revision })
		writeJSON(w, 200, map[string]any{"revisions": list, "head_revision": item.head})
	default:
		fail(w, 404, CodeNotFound)
	}
}

func (f *fakeSite) listItems(w http.ResponseWriter) {
	out := []RemoteItem{}
	for id, item := range f.items {
		out = append(out, RemoteItem{ItemID: id, HeadRevision: item.head, Updated: item.updated})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ItemID < out[j].ItemID })
	writeJSON(w, 200, map[string]any{"items": out})
}

func (f *fakeSite) blob(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method == http.MethodPut {
		f.puts++
		f.putsByID[id]++
		if f.failPut != nil {
			if status := f.failPut(id); status != 0 {
				if f.retryAfter != "" {
					w.Header().Set("Retry-After", f.retryAfter)
				}
				fail(w, status, CodeBusy)
				return
			}
		}
		body, _ := io.ReadAll(r.Body)
		if _, had := f.blobs[id]; !had {
			f.blobs[id] = body
		}
		writeJSON(w, 200, map[string]any{"blob_id": id, "bytes": len(body)})
		return
	}
	f.gets++
	if f.failGet != nil {
		if status := f.failGet(id); status != 0 {
			fail(w, status, CodeStorageUnavailable)
			return
		}
	}
	data, ok := f.blobs[id]
	if !ok {
		fail(w, 404, CodeNotFound)
		return
	}
	_, _ = w.Write(data)
}

func (f *fakeSite) commit(w http.ResponseWriter, r *http.Request, itemID string) {
	f.commits++
	var in struct {
		BaseRevision int      `json:"base_revision"`
		ManifestBlob string   `json:"manifest_blob"`
		BlobIDs      []string `json:"blob_ids"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	ids := append(append([]string{}, in.BlobIDs...), in.ManifestBlob)
	missing := []string{}
	for _, id := range ids {
		if _, ok := f.blobs[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		writeJSON(w, 409, map[string]any{"code": CodeBlobsMissing, "missing": missing[:min(len(missing), f.missedCap)]})
		return
	}
	item, ok := f.items[itemID]
	if !ok {
		item = &fakeItem{}
		f.items[itemID] = item
	}
	if item.head != in.BaseRevision {
		writeJSON(w, 409, map[string]any{"code": CodeConflict, "head_revision": item.head})
		return
	}
	item.head++
	item.updated = strconv.Itoa(item.head)
	item.revisions = append(item.revisions, Revision{Revision: item.head, ManifestBlob: in.ManifestBlob, BlobIDs: ids, Bytes: 1})
	if len(item.revisions) > 5 {
		item.revisions = item.revisions[1:]
	}
	if f.failCommitAfterSaving {
		f.failCommitAfterSaving = false
		writeJSON(w, 502, map[string]any{})
		return
	}
	writeJSON(w, 200, map[string]any{"head_revision": item.head})
}

func (f *fakeSite) counts() (puts, gets, commits int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.puts, f.gets, f.commits
}

func (f *fakeSite) onlyItem(t *testing.T) (string, *fakeItem) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.items) != 1 {
		t.Fatalf("site holds %d items, want 1", len(f.items))
	}
	for id, item := range f.items {
		return id, item
	}
	return "", nil
}
