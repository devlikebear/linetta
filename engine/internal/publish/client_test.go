package publish

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func reasonOf(err error) string {
	var re *rpc.ReasonError
	if errors.As(err, &re) {
		return re.Reason
	}
	return ""
}

func TestClient_meSendsTheTokenAndVersion(t *testing.T) {
	var gotAuth, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotUA = r.Header.Get("Authorization"), r.Header.Get("User-Agent")
		writeJSON(w, 200, map[string]any{"handle": "devlikebear", "pen_name": "마곰", "can_publish": true, "extra": 1})
	}))
	defer srv.Close()
	me, err := NewClient(srv.URL+"/", "1.9.0").Me(context.Background(), "lwk_x")
	if err != nil {
		t.Fatal(err)
	}
	if me.Handle != "devlikebear" || !me.CanPublish {
		t.Fatalf("me = %+v", me)
	}
	if gotAuth != "Bearer lwk_x" || gotUA != "Linetta/1.9.0" {
		t.Fatalf("auth %q, user agent %q", gotAuth, gotUA)
	}
}

func TestClient_errorsCarryTheSiteCodeAndClassify(t *testing.T) {
	cases := []struct {
		status int
		code   string
		want   string
	}{
		{401, CodeUnauthorized, rpc.ReasonPublishUnauthorized},
		{403, CodePublishNotEnabled, rpc.ReasonPublishNotEnabled},
		{403, CodeQuotaExceeded, rpc.ReasonPublishQuotaExceeded},
		{413, CodeTooLarge, rpc.ReasonPublishTooLarge},
		{429, CodeRateLimited, rpc.ReasonPublishRateLimited},
		// PocketBase's own limiter answers without a body.
		{429, "", rpc.ReasonPublishRateLimited},
		{409, CodeSlugTaken, rpc.ReasonPublishSlugTaken},
		{400, CodeHashMismatch, rpc.ReasonPublishRejected},
		{500, "", rpc.ReasonPublishUnavailable},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeErr(w, c.status, c.code)
		}))
		_, err := NewClient(srv.URL, "t").Me(context.Background(), "lwk_x")
		srv.Close()
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != c.status {
			t.Errorf("%d %s: err = %v", c.status, c.code, err)
			continue
		}
		if got := reasonOf(classify(err)); got != c.want {
			t.Errorf("%d %s: reason %q, want %q", c.status, c.code, got, c.want)
		}
		if strings.Contains(err.Error(), "lwk_x") {
			t.Errorf("the token leaked into the error: %v", err)
		}
	}
}

func TestClient_timeoutAndUnreachableAreTheirOwnReasons(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	c := NewClient(srv.URL, "t").WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond})
	_, err := c.Me(context.Background(), "lwk_x")
	if got := reasonOf(classify(err)); got != rpc.ReasonPublishTimeout {
		t.Fatalf("slow site: reason %q (%v)", got, err)
	}

	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL
	dead.Close()
	_, err = NewClient(url, "t").Me(context.Background(), "lwk_x")
	if got := reasonOf(classify(err)); got != rpc.ReasonPublishUnreachable {
		t.Fatalf("closed site: reason %q (%v)", got, err)
	}
}

func TestClient_putIsRetriedOnceButPatchAndPostAreNot(t *testing.T) {
	var puts, patches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if puts.Add(1) == 1 {
				writeErr(w, 503, "")
				return
			}
			writeJSON(w, 200, map[string]any{"created": true, "work": map[string]any{"project_id": "p"}})
		case http.MethodPatch:
			patches.Add(1)
			writeErr(w, 503, "")
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "t")
	c.retryDelay = 0
	created, _, err := c.PutWork(context.Background(), "lwk_x", "p", WorkInput{Title: "T"})
	if err != nil || !created || puts.Load() != 2 {
		t.Fatalf("PUT: created %v, err %v, attempts %d", created, err, puts.Load())
	}
	if _, err := c.SetEpisodeVisibility(context.Background(), "lwk_x", "p", "n", "private"); err == nil || patches.Load() != 1 {
		t.Fatalf("PATCH: err %v, attempts %d", err, patches.Load())
	}
}

func TestClient_putIsNotRetriedOnAnAnswer(t *testing.T) {
	var puts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		puts.Add(1)
		writeErr(w, 413, CodeTooLarge)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "t")
	c.retryDelay = 0
	_, _, err := c.PutEpisode(context.Background(), "lwk_x", "p", "n", EpisodeInput{Number: 1})
	if !IsCode(err, CodeTooLarge) || puts.Load() != 1 {
		t.Fatalf("err %v, attempts %d", err, puts.Load())
	}
}

func TestClient_escapesIDsInPaths(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		writeJSON(w, 200, map[string]any{"episode": map[string]any{}})
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL, "t").SetEpisodeVisibility(context.Background(), "tok", "a/b", "c d", "public"); err != nil {
		t.Fatal(err)
	}
	if path != "/api/artworks/works/a%2Fb/episodes/c%20d" {
		t.Fatalf("path = %q", path)
	}
}

func TestBaseURLFromEnv(t *testing.T) {
	t.Setenv(BaseURLEnv, "")
	if got := BaseURLFromEnv(); got != DefaultBaseURL {
		t.Fatalf("default = %q", got)
	}
	t.Setenv(BaseURLEnv, "http://127.0.0.1:8090/")
	if got := BaseURLFromEnv(); got != "http://127.0.0.1:8090" {
		t.Fatalf("override = %q", got)
	}
}
