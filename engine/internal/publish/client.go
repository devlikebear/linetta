// Package publish puts a writer's episodes on Linetta Works
// (works.marvin-42.com), the reading site for novels written in Linetta.
//
// It is optional and off until the writer connects an account in Settings:
// nothing here dials the site before then. The wire contract is the site's,
// written down in docs/api.md of the linetta-artworks repository (api_version
// 1); this package follows it rather than defining its own. The site's own
// command-line client, scripts/publish-cli.mjs there, is the reference for
// what a correct client does.
package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultBaseURL is the production site.
const DefaultBaseURL = "https://works.marvin-42.com"

// BaseURLEnv points the app at another site, for development against a local
// server. It is read once, when the engine starts.
const BaseURLEnv = "LINETTA_PUBLISH_BASE_URL"

// requestTimeout bounds one request. An episode body is at most a few hundred
// kilobytes, so anything slower than this is a connection that is not going
// to finish.
const requestTimeout = 30 * time.Second

// maxResponseBytes caps what is read back. The largest answer the site gives
// is a work's episode list, which carries no bodies.
const maxResponseBytes = 4 << 20

// BaseURLFromEnv returns the site to talk to: BaseURLEnv when set, otherwise
// the production site.
func BaseURLFromEnv() string {
	if v := strings.TrimSpace(os.Getenv(BaseURLEnv)); v != "" {
		return strings.TrimRight(v, "/")
	}
	return DefaultBaseURL
}

// Serial statuses and visibilities the site accepts.
const (
	SerialOngoing   = "ongoing"
	SerialCompleted = "completed"

	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

// Episode statuses the site reports (docs/api.md, "회차 상태"). Only public
// and private occur until the site's moderation ships, but every one of them
// has to be shown correctly from the first release.
const (
	RemotePublic      = "public"
	RemotePrivate     = "private"
	RemotePending     = "pending"
	RemoteRejected    = "rejected"
	RemoteNeedsReview = "needs_review"
)

// APIError is a non-2xx answer from the site. Code is the site's error code
// (docs/api.md, "오류"); Message is its Korean sentence, kept for logs only.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("linetta works: HTTP %d", e.Status)
	}
	return fmt.Sprintf("linetta works: HTTP %d %s", e.Status, e.Code)
}

// Pairing is pair/start's answer. DeviceSecret is held in memory only.
type Pairing struct {
	PairingID    string `json:"pairing_id"`
	DeviceSecret string `json:"device_secret"`
	ConfirmCode  string `json:"confirm_code"`
	VerifyURL    string `json:"verify_url"`
	ExpiresIn    int    `json:"expires_in"`
	Interval     int    `json:"interval"`
}

// Poll statuses.
const (
	PollPending  = "pending"
	PollApproved = "approved"
	PollDenied   = "denied"
	PollExpired  = "expired"
)

// PollResult is pair/poll's answer. Token is set only when approved, once.
type PollResult struct {
	Status  string `json:"status"`
	Token   string `json:"token"`
	Handle  string `json:"handle"`
	PenName string `json:"pen_name"`
}

// Me is the connected account.
type Me struct {
	Handle     string `json:"handle"`
	PenName    string `json:"pen_name"`
	CanPublish bool   `json:"can_publish"`
}

// Work is the site's work object.
type Work struct {
	ProjectID       string   `json:"project_id"`
	Slug            string   `json:"slug"`
	Title           string   `json:"title"`
	Blurb           string   `json:"blurb"`
	Genres          []string `json:"genres"`
	SerialStatus    string   `json:"serial_status"`
	Language        string   `json:"language"`
	Visibility      string   `json:"visibility"`
	URL             string   `json:"url"`
	LastPublishedAt string   `json:"last_published_at"`
}

// WorkInput is the body of PUT works/{project_id}. Optional fields left
// empty are not sent, so an update leaves them as they are on the site.
type WorkInput struct {
	Title        string   `json:"title"`
	Slug         string   `json:"slug,omitempty"`
	Blurb        *string  `json:"blurb,omitempty"`
	Genres       []string `json:"genres,omitempty"`
	SerialStatus string   `json:"serial_status,omitempty"`
	Language     string   `json:"language,omitempty"`
}

// RemoteEpisode is the site's episode object.
type RemoteEpisode struct {
	NodeID           string `json:"node_id"`
	Number           int    `json:"number"`
	Title            string `json:"title"`
	ContentHash      string `json:"content_hash"`
	CharCount        int    `json:"char_count"`
	Status           string `json:"status"`
	Visibility       string `json:"visibility"`
	Moderation       string `json:"moderation"`
	ModerationReason string `json:"moderation_reason"`
	URL              string `json:"url"`
	PublishedAt      string `json:"published_at"`
	Updated          string `json:"updated"`
	// Images are the pictures the site holds for the episode.
	Images []RemoteImage `json:"images,omitempty"`
}

// RemoteImage is a picture the site holds for an episode. The bytes under an
// illustration id never change, so a matching hash means nothing to upload.
type RemoteImage struct {
	IllustrationID string `json:"illustration_id"`
	SHA256         string `json:"sha256"`
}

// EpisodeInput is the body of PUT works/{project_id}/episodes/{node_id}.
type EpisodeInput struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	BodyMD      string `json:"body_md"`
	ContentHash string `json:"content_hash"`
}

// Client speaks the site's API. It holds no credential: every call that
// needs one is handed the token, so the token lives in exactly one place
// (the secret store) and nothing here can log it by accident.
type Client struct {
	base      string
	http      *http.Client
	userAgent string
	// retryDelay is how long an idempotent PUT waits before its one retry.
	retryDelay time.Duration
}

// NewClient returns a client for the site at base. version goes into the
// User-Agent, so the site can tell which app release is talking to it.
func NewClient(base, version string) *Client {
	return &Client{
		base:       strings.TrimRight(base, "/"),
		http:       &http.Client{Timeout: requestTimeout},
		userAgent:  "Linetta/" + version,
		retryDelay: time.Second,
	}
}

// WithHTTPClient replaces the transport (tests).
func (c *Client) WithHTTPClient(h *http.Client) *Client { c.http = h; return c }

// BaseURL is the site this client talks to.
func (c *Client) BaseURL() string { return c.base }

// PairStart begins connecting this device. No credential.
func (c *Client) PairStart(ctx context.Context, deviceName string) (Pairing, error) {
	var out Pairing
	err := c.do(ctx, http.MethodPost, "/api/artworks/pair/start", "", map[string]string{"device_name": deviceName}, &out)
	return out, err
}

// PairPoll asks whether the writer has answered in the browser yet.
func (c *Client) PairPoll(ctx context.Context, pairingID, deviceSecret string) (PollResult, error) {
	var out PollResult
	err := c.do(ctx, http.MethodPost, "/api/artworks/pair/poll", "",
		map[string]string{"pairing_id": pairingID, "device_secret": deviceSecret}, &out)
	return out, err
}

// Me returns the account the token belongs to.
func (c *Client) Me(ctx context.Context, token string) (Me, error) {
	var out Me
	err := c.do(ctx, http.MethodGet, "/api/artworks/me", token, nil, &out)
	return out, err
}

// PutWork creates or updates a work.
func (c *Client) PutWork(ctx context.Context, token, projectID string, in WorkInput) (created bool, w Work, err error) {
	var out struct {
		Created bool `json:"created"`
		Work    Work `json:"work"`
	}
	err = c.do(ctx, http.MethodPut, workPath(projectID), token, in, &out)
	return out.Created, out.Work, err
}

// GetWork returns a work and its episodes in number order.
func (c *Client) GetWork(ctx context.Context, token, projectID string) (Work, []RemoteEpisode, error) {
	var out struct {
		Work     Work            `json:"work"`
		Episodes []RemoteEpisode `json:"episodes"`
	}
	err := c.do(ctx, http.MethodGet, workPath(projectID), token, nil, &out)
	return out.Work, out.Episodes, err
}

// PutEpisode publishes an episode or replaces its text.
func (c *Client) PutEpisode(ctx context.Context, token, projectID, nodeID string, in EpisodeInput) (created bool, ep RemoteEpisode, err error) {
	var out struct {
		Created bool          `json:"created"`
		Episode RemoteEpisode `json:"episode"`
	}
	err = c.do(ctx, http.MethodPut, episodePath(projectID, nodeID), token, in, &out)
	return out.Created, out.Episode, err
}

// PutEpisodeImage uploads one picture an episode's text shows. The body is
// the image bytes; the site answers the same thing again without a change.
func (c *Client) PutEpisodeImage(ctx context.Context, token, projectID, nodeID, illustrationID, mime string, data []byte) (created bool, img RemoteImage, err error) {
	var out struct {
		Created bool        `json:"created"`
		Image   RemoteImage `json:"image"`
	}
	path := episodePath(projectID, nodeID) + "/images/" + url.PathEscape(illustrationID)
	err = c.send(ctx, http.MethodPut, path, token, data, mime, &out)
	return out.Created, out.Image, err
}

// SetEpisodeVisibility makes an episode public or private.
func (c *Client) SetEpisodeVisibility(ctx context.Context, token, projectID, nodeID, visibility string) (RemoteEpisode, error) {
	var out struct {
		Episode RemoteEpisode `json:"episode"`
	}
	err := c.do(ctx, http.MethodPatch, episodePath(projectID, nodeID), token, map[string]string{"visibility": visibility}, &out)
	return out.Episode, err
}

// ReorderEpisodes renumbers a work's episodes 1, 2, 3… in order. order must
// name every episode the site has for the work exactly once.
func (c *Client) ReorderEpisodes(ctx context.Context, token, projectID string, order []string) ([]RemoteEpisode, error) {
	var out struct {
		Episodes []RemoteEpisode `json:"episodes"`
	}
	err := c.do(ctx, http.MethodPut, workPath(projectID)+"/episodes:reorder", token, map[string][]string{"order": order}, &out)
	return out.Episodes, err
}

func workPath(projectID string) string {
	return "/api/artworks/works/" + url.PathEscape(projectID)
}

func episodePath(projectID, nodeID string) string {
	return workPath(projectID) + "/episodes/" + url.PathEscape(nodeID)
}

// do sends one request and decodes a 2xx answer into out. A PUT is retried
// once when the connection failed or the site's gateway said it was briefly
// unavailable: every PUT in the API is idempotent, so sending it twice is
// harmless, while a POST (pairing) or PATCH is never repeated.
func (c *Client) do(ctx context.Context, method, path, token string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	return c.send(ctx, method, path, token, payload, "application/json", out)
}

// send is do with the body already encoded as contentType.
func (c *Client) send(ctx context.Context, method, path, token string, payload []byte, contentType string, out any) error {
	err := c.once(ctx, method, path, token, payload, contentType, out)
	if method == http.MethodPut && retryable(ctx, err) {
		select {
		case <-ctx.Done():
			return err
		case <-time.After(c.retryDelay):
		}
		err = c.once(ctx, method, path, token, payload, contentType, out)
	}
	return err
}

func retryable(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	return true
}

func (c *Client) once(ctx context.Context, method, path, token string, payload []byte, contentType string, out any) error {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		// net/http's error names the URL, never the headers, so the token
		// cannot ride along; the body is not in it either.
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("linetta works: read response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		apiErr := &APIError{Status: res.StatusCode}
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &e) == nil {
			apiErr.Code, apiErr.Message = e.Code, e.Message
		}
		// PocketBase's own rate limiter answers 429 without a code.
		if apiErr.Code == "" && res.StatusCode == http.StatusTooManyRequests {
			apiErr.Code = CodeRateLimited
		}
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("linetta works: parse response: %w", err)
	}
	return nil
}

// The site's error codes the app acts on (docs/api.md, "오류").
const (
	CodeUnauthorized      = "unauthorized"
	CodePublishNotEnabled = "publish_not_enabled"
	CodeQuotaExceeded     = "quota_exceeded"
	CodeTooLarge          = "too_large"
	CodeInvalidRequest    = "invalid_request"
	CodeInvalidSlug       = "invalid_slug"
	CodeSlugTaken         = "slug_taken"
	CodeHashMismatch      = "hash_mismatch"
	CodeNumberTaken       = "number_taken"
	CodeNotFound          = "not_found"
	CodeRateLimited       = "rate_limited"
	CodeAlreadyClaimed    = "already_claimed"
	CodeImagesMissing     = "images_missing"
	CodeImageChanged      = "image_changed"
)

// MaxImagesPerEpisode is the site's limit on pictures in one episode
// (docs/api.md, "한도"). The app stops before uploading any when an
// episode shows more.
const MaxImagesPerEpisode = 30

// IsCode reports whether err is the site answering with code.
func IsCode(err error, code string) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}
