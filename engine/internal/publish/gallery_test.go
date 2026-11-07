package publish

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gallerySite(t *testing.T) (*Client, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.String()+" auth="+r.Header.Get("Authorization"))
		switch {
		case r.URL.Path == "/api/artworks/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"q":"검","genre":"martial","page":2,"total_pages":3,"total":41,"items":[
				{"work":{"id":"8t7ns29bgyp4pig","slug":"work-mzg8dx","title":"검이 없는 검객","blurb":"소개","genres":["무협"],"category":"martial","serial_status":"ongoing"},
				 "author":{"id":"a","handle":"devlikebear","pen_name":"마곰"},"latest":{"number":9,"published_at":"2026-11-05 11:27:07.882Z"},"episode_count":9,"cover":"abc"},
				{"work":{"id":"zzzzzzzzzzzzzzz","slug":"s","title":"표지 없음","blurb":"","genres":null,"category":"etc","serial_status":"completed"},
				 "author":{"id":"b","handle":"h","pen_name":""},"latest":{"number":1,"published_at":""},"episode_count":1,"cover":null}]}`))
		case r.URL.Path == "/api/artworks/images/cover/8t7ns29bgyp4pig":
			w.Header().Set("Content-Type", "image/webp")
			_, _ = w.Write([]byte("WEBP"))
		case strings.HasPrefix(r.URL.Path, "/api/artworks/images/cover/"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "test"), &seen
}

func TestGalleryReadsThePublicSearchWithoutTheAccount(t *testing.T) {
	c, seen := gallerySite(t)
	page, err := c.Gallery(context.Background(), GalleryQuery{Q: "  검   객 ", Genre: "martial", Page: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 1 || !strings.Contains((*seen)[0], "q=%EA%B2%80+%EA%B0%9D") || !strings.Contains((*seen)[0], "genre=martial") ||
		!strings.Contains((*seen)[0], "page=2") || !strings.HasSuffix((*seen)[0], "auth=") {
		t.Fatalf("request = %v", *seen)
	}
	if page.Total != 41 || page.TotalPages != 3 || len(page.Items) != 2 || page.Site == "" {
		t.Fatalf("page = %+v", page)
	}
	first := page.Items[0]
	if first.Title != "검이 없는 검객" || first.Author.PenName != "마곰" || first.Episodes != 9 || first.LatestNumber != 9 ||
		first.Cover != "abc" || first.URL != page.Site+"/devlikebear/work-mzg8dx" {
		t.Fatalf("first = %+v", first)
	}
	if second := page.Items[1]; second.Cover != "" || second.Genres == nil {
		t.Fatalf("second = %+v", second)
	}
}

func TestGalleryRefusesWhatTheSiteWould(t *testing.T) {
	c, seen := gallerySite(t)
	for _, q := range []GalleryQuery{
		{Q: strings.Repeat("가", GalleryMaxQuery+1)},
		{Genre: "cooking"},
		{Page: GalleryMaxPage + 1},
		{Page: -1},
	} {
		if _, err := c.Gallery(context.Background(), q); !errors.Is(err, ErrGalleryQuery) {
			t.Errorf("%+v: %v", q, err)
		}
	}
	if len(*seen) != 0 {
		t.Fatalf("a refused query reached the site: %v", *seen)
	}
}

func TestGalleryCover(t *testing.T) {
	c, seen := gallerySite(t)
	data, mime, err := c.GalleryCover(context.Background(), "8t7ns29bgyp4pig")
	if err != nil || string(data) != "WEBP" || mime != "image/webp" {
		t.Fatalf("cover = %q %q %v", data, mime, err)
	}
	if _, _, err := c.GalleryCover(context.Background(), "aaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("a missing cover read as one")
	}
	if _, _, err := c.GalleryCover(context.Background(), "../../me"); !errors.Is(err, ErrGalleryQuery) {
		t.Fatalf("a path for an id: %v", err)
	}
	if len(*seen) != 2 {
		t.Fatalf("requests = %v", *seen)
	}
}
