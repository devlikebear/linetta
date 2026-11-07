package publish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The gallery is what readers see on Linetta Works, read through the site's own
// search (GET /api/artworks/search, which needs no account) so the app can show
// it beside the writer's library. It sends nothing of the writer's: only the
// search words, a genre and a page.

// Limits the site's search enforces; asking past them is refused there.
const (
	GalleryMaxQuery = 50
	GalleryMaxPage  = 100
	// galleryMaxCoverBytes caps a cover read back. Covers are a few hundred KiB.
	galleryMaxCoverBytes = 8 << 20
)

// GalleryGenres are the site's genre keys (linetta-artworks
// backend/pb_hooks/lib/genres.js). The keys are in its addresses, so they stay.
var GalleryGenres = []string{"fantasy", "modern-fantasy", "romance", "romance-fantasy", "martial", "sf", "mystery", "horror", "literary", "etc"}

// GalleryQuery is one page of the gallery. Empty Q and Genre list every work,
// newest episode first.
type GalleryQuery struct {
	Q     string `json:"q"`
	Genre string `json:"genre"`
	Page  int    `json:"page"`
}

// GalleryAuthor is the writer of a work, as the site shows them.
type GalleryAuthor struct {
	Handle  string `json:"handle"`
	PenName string `json:"pen_name"`
}

// GalleryWork is one card.
type GalleryWork struct {
	ID           string        `json:"id"`
	Slug         string        `json:"slug"`
	Title        string        `json:"title"`
	Blurb        string        `json:"blurb"`
	Genres       []string      `json:"genres"`
	Category     string        `json:"category"`
	SerialStatus string        `json:"serial_status"`
	Author       GalleryAuthor `json:"author"`
	Episodes     int           `json:"episode_count"`
	LatestNumber int           `json:"latest_number"`
	LatestAt     string        `json:"latest_at"`
	// Cover is the cover's hash, empty when the work shows none.
	Cover string `json:"cover"`
	// URL is the work's page on the site.
	URL string `json:"url"`
}

// GalleryPage is what one query found.
type GalleryPage struct {
	Q          string        `json:"q"`
	Genre      string        `json:"genre"`
	Page       int           `json:"page"`
	TotalPages int           `json:"total_pages"`
	Total      int           `json:"total"`
	Items      []GalleryWork `json:"items"`
	// Site is the site's address, for a link to the whole of it.
	Site string `json:"site"`
}

// ErrGalleryQuery is a query the site would refuse.
var ErrGalleryQuery = errors.New("publish: gallery query out of range")

// Normalize trims the query the way the site does and checks it is one the
// site takes.
func (q GalleryQuery) Normalize() (GalleryQuery, error) {
	q.Q = strings.Join(strings.Fields(q.Q), " ")
	if utf8.RuneCountInString(q.Q) > GalleryMaxQuery {
		return q, fmt.Errorf("%w: the search is over %d characters", ErrGalleryQuery, GalleryMaxQuery)
	}
	if q.Genre != "" && !validGenre(q.Genre) {
		return q, fmt.Errorf("%w: no genre %q", ErrGalleryQuery, q.Genre)
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.Page < 1 || q.Page > GalleryMaxPage {
		return q, fmt.Errorf("%w: page %d", ErrGalleryQuery, q.Page)
	}
	return q, nil
}

func validGenre(key string) bool {
	for _, genre := range GalleryGenres {
		if genre == key {
			return true
		}
	}
	return false
}

// searchItem is one item of the site's answer.
type searchItem struct {
	Work struct {
		ID           string   `json:"id"`
		Slug         string   `json:"slug"`
		Title        string   `json:"title"`
		Blurb        string   `json:"blurb"`
		Genres       []string `json:"genres"`
		Category     string   `json:"category"`
		SerialStatus string   `json:"serial_status"`
	} `json:"work"`
	Author struct {
		Handle  string `json:"handle"`
		PenName string `json:"pen_name"`
	} `json:"author"`
	Latest struct {
		Number      int    `json:"number"`
		PublishedAt string `json:"published_at"`
	} `json:"latest"`
	EpisodeCount int     `json:"episode_count"`
	Cover        *string `json:"cover"`
}

// Gallery reads one page of the works readers can see.
func (c *Client) Gallery(ctx context.Context, in GalleryQuery) (GalleryPage, error) {
	q, err := in.Normalize()
	if err != nil {
		return GalleryPage{}, err
	}
	values := url.Values{}
	if q.Q != "" {
		values.Set("q", q.Q)
	}
	if q.Genre != "" {
		values.Set("genre", q.Genre)
	}
	values.Set("page", strconv.Itoa(q.Page))
	var res struct {
		Q          string       `json:"q"`
		Genre      string       `json:"genre"`
		Page       int          `json:"page"`
		TotalPages int          `json:"total_pages"`
		Total      int          `json:"total"`
		Items      []searchItem `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/artworks/search?"+values.Encode(), "", nil, &res); err != nil {
		return GalleryPage{}, err
	}
	out := GalleryPage{Q: res.Q, Genre: res.Genre, Page: res.Page, TotalPages: res.TotalPages, Total: res.Total, Items: []GalleryWork{}, Site: c.base}
	for _, item := range res.Items {
		work := GalleryWork{
			ID: item.Work.ID, Slug: item.Work.Slug, Title: item.Work.Title, Blurb: item.Work.Blurb,
			Genres: item.Work.Genres, Category: item.Work.Category, SerialStatus: item.Work.SerialStatus,
			Author:   GalleryAuthor{Handle: item.Author.Handle, PenName: item.Author.PenName},
			Episodes: item.EpisodeCount, LatestNumber: item.Latest.Number, LatestAt: item.Latest.PublishedAt,
			URL: c.workURL(item.Author.Handle, item.Work.Slug),
		}
		if work.Genres == nil {
			work.Genres = []string{}
		}
		if item.Cover != nil {
			work.Cover = *item.Cover
		}
		out.Items = append(out.Items, work)
	}
	return out, nil
}

// workURL is a work's page on the site: /{handle}/{slug}.
func (c *Client) workURL(handle, slug string) string {
	if handle == "" || slug == "" {
		return c.base
	}
	return c.base + "/" + url.PathEscape(handle) + "/" + url.PathEscape(slug)
}

// galleryIDPattern is a PocketBase record id. Anything else never reaches a path.
var galleryIDPattern = regexp.MustCompile(`^[a-z0-9]{15}$`)

// ValidGalleryID says whether id can name a work on the site.
func ValidGalleryID(id string) bool { return galleryIDPattern.MatchString(id) }

// GalleryCover reads a work's cover as readers see it, with its media type.
func (c *Client) GalleryCover(ctx context.Context, workID string) ([]byte, string, error) {
	if !ValidGalleryID(workID) {
		return nil, "", fmt.Errorf("%w: bad work id", ErrGalleryQuery)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/artworks/images/cover/"+workID, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return nil, "", &APIError{Status: res.StatusCode}
	}
	mime := strings.TrimSpace(strings.Split(res.Header.Get("Content-Type"), ";")[0])
	switch mime {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
	default:
		return nil, "", fmt.Errorf("linetta works: cover is %q, not a picture", mime)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, galleryMaxCoverBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("linetta works: read cover: %w", err)
	}
	if len(data) > galleryMaxCoverBytes {
		return nil, "", errors.New("linetta works: cover is too large")
	}
	return data, mime, nil
}
