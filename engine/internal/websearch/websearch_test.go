package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSearchAsksForSnippetsAndTrimsThem(t *testing.T) {
	var path string
	var query url.Values
	long := strings.Repeat("가", maxSnippetRunes+1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query = r.URL.Path, r.URL.Query()
		_, _ = w.Write([]byte(`{"results":[
			{"title":" 경복궁 ","url":"https://example.com/a","content":"` + long + `","engine":"wikipedia"},
			{"title":"extra","url":"https://example.com/b","content":"past the limit"}
		]}`))
	}))
	defer ts.Close()

	for _, base := range []string{ts.URL, ts.URL + "/", ts.URL + "/search"} {
		got, err := Client{HTTP: ts.Client()}.Search(context.Background(), base, "경복궁 건축 연도", 1)
		if err != nil {
			t.Fatalf("Search(%q): %v", base, err)
		}
		if path != "/search" || query.Get("q") != "경복궁 건축 연도" || query.Get("format") != "json" || query.Get("max_results") != "1" {
			t.Fatalf("base %q: path %q query %v", base, path, query)
		}
		if query.Has("include_raw_content") {
			t.Fatalf("page text must stay off: %v", query)
		}
		if len(got) != 1 || got[0].Title != "경복궁" || len([]rune(got[0].Content)) != maxSnippetRunes {
			t.Fatalf("unexpected results: %+v", got)
		}
	}
}

func TestSearchBoundsTheLimit(t *testing.T) {
	var asked []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Query().Get("max_results"))
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer ts.Close()

	for _, limit := range []int{0, 500} {
		if _, err := (Client{HTTP: ts.Client()}).Search(context.Background(), ts.URL, "q", limit); err != nil {
			t.Fatalf("Search: %v", err)
		}
	}
	if len(asked) != 2 || asked[0] != "5" || asked[1] != "10" {
		t.Fatalf("max_results = %v", asked)
	}
}

func TestSearchFailures(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "denied" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer ts.Close()

	cases := []struct{ base, query, want string }{
		{ts.URL, "denied", "server answered 403"},
		{ts.URL, "html", "decode response"},
		{"ftp://example.com", "q", "http or https"},
		{"http://bad host", "q", "invalid address"},
	}
	for _, tc := range cases {
		_, err := Client{HTTP: ts.Client()}.Search(context.Background(), tc.base, tc.query, 1)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Search(%q, %q) error = %v, want %q", tc.base, tc.query, err, tc.want)
		}
	}
}

func TestSearchUnreachableServer(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	base := ts.URL
	ts.Close()
	if _, err := (Client{}).Search(context.Background(), base, "q", 1); err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("error = %v", err)
	}
}
