package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// requestTimeout bounds one request. A blob is 4MiB, so this is what a connection
// of about 100KB/s needs; slower than that is not going to finish a work anyway.
const requestTimeout = 60 * time.Second

// maxResponseBytes caps what is read back: the largest answer is a sealed blob.
const maxResponseBytes = MaxBlobBytes + 1<<16

// A blob request is safe to repeat, and the site answers vault_busy under load
// by design. maxAttempts is what keeps a failure that will not clear, such as a
// record whose object is gone, from retrying without end.
const (
	maxAttempts   = 5
	maxRetryDelay = 30 * time.Second
)

// The site's codes the app acts on (docs/api.md in linetta-artworks, "비공개
// 보관함").
const (
	CodeUnauthorized       = "unauthorized"
	CodeNotEnabled         = "vault_not_enabled"
	CodeTermsOutdated      = "vault_terms_outdated"
	CodeQuotaExceeded      = "vault_quota_exceeded"
	CodeConflict           = "vault_conflict"
	CodeBlobsMissing       = "blobs_missing"
	CodeBusy               = "vault_busy"
	CodeStorageUnavailable = "vault_storage_unavailable"
	CodeUnavailable        = "vault_unavailable"
	CodeTooLarge           = "too_large"
	CodeNotFound           = "not_found"
	CodeRateLimited        = "rate_limited"
)

// Modes of an account (GET status).
const (
	ModeFull          = "full"
	ModeTermsOutdated = "terms_outdated"
	ModeReadOnly      = "read_only"
	ModeNone          = "none"
)

// APIError is a non-2xx answer from the site.
type APIError struct {
	Status  int
	Code    string
	Message string
	// Missing and HeadRevision come with blobs_missing and vault_conflict.
	Missing      []string
	HeadRevision int
	retryAfter   time.Duration
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("linetta works vault: HTTP %d", e.Status)
	}
	return fmt.Sprintf("linetta works vault: HTTP %d %s", e.Status, e.Code)
}

// Usage is what the account has stored against its quota.
type Usage struct {
	Bytes      int64 `json:"bytes"`
	QuotaBytes int64 `json:"quota_bytes"`
}

// Limits are the site's limits.
type Limits struct {
	BlobBytes        int `json:"blob_bytes"`
	RestorePoints    int `json:"restore_points"`
	Items            int `json:"items"`
	BlobsPerRevision int `json:"blobs_per_revision"`
}

// Account is GET status.
type Account struct {
	Mode          string `json:"mode"`
	Status        string `json:"status"`
	RetrieveUntil string `json:"retrieve_until"`
	TermsAccepted bool   `json:"terms_accepted"`
	Usage         Usage  `json:"usage"`
	Limits        Limits `json:"limits"`
}

// RemoteItem is one entry of GET items.
type RemoteItem struct {
	ItemID       string `json:"item_id"`
	HeadRevision int    `json:"head_revision"`
	Updated      string `json:"updated"`
	Bytes        int64  `json:"bytes"`
}

// Revision is one restore point.
type Revision struct {
	Revision     int      `json:"revision"`
	ManifestBlob string   `json:"manifest_blob"`
	BlobIDs      []string `json:"blob_ids"`
	Bytes        int64    `json:"bytes"`
	Created      string   `json:"created"`
}

// Client speaks the vault half of the site's API. Like the publish client it
// holds no credential; every call is handed the token.
type Client struct {
	base      string
	http      *http.Client
	userAgent string
	// wait sleeps between attempts (tests replace it).
	wait func(ctx context.Context, d time.Duration) error
}

// NewClient returns a client for the site at base.
func NewClient(base, version string) *Client {
	return &Client{
		base:      strings.TrimRight(base, "/"),
		http:      &http.Client{Timeout: requestTimeout},
		userAgent: "Linetta/" + version,
		wait: func(ctx context.Context, d time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				return nil
			}
		},
	}
}

// WithHTTPClient replaces the transport (tests).
func (c *Client) WithHTTPClient(h *http.Client) *Client { c.http = h; return c }

// Account returns the state of the token's account.
func (c *Client) Account(ctx context.Context, token string) (Account, error) {
	var out Account
	err := c.json(ctx, http.MethodGet, "/status", token, nil, &out)
	return out, err
}

// PutBlob stores one sealed blob. Sending one the site already has is a no-op.
func (c *Client) PutBlob(ctx context.Context, token, id string, sealed []byte) error {
	_, err := c.retry(ctx, func() ([]byte, error) {
		return c.once(ctx, http.MethodPut, "/blobs/"+url.PathEscape(id), token, sealed, "application/octet-stream")
	})
	return err
}

// GetBlob returns one sealed blob.
func (c *Client) GetBlob(ctx context.Context, token, id string) ([]byte, error) {
	return c.retry(ctx, func() ([]byte, error) {
		return c.once(ctx, http.MethodGet, "/blobs/"+url.PathEscape(id), token, nil, "")
	})
}

// Items lists the account's items.
func (c *Client) Items(ctx context.Context, token string) ([]RemoteItem, error) {
	var out struct {
		Items []RemoteItem `json:"items"`
	}
	err := c.json(ctx, http.MethodGet, "/items", token, nil, &out)
	return out.Items, err
}

// Revisions lists an item's restore points, newest first.
func (c *Client) Revisions(ctx context.Context, token, itemID string) ([]Revision, error) {
	var out struct {
		Revisions []Revision `json:"revisions"`
	}
	err := c.json(ctx, http.MethodGet, "/items/"+url.PathEscape(itemID)+"/revisions", token, nil, &out)
	return out.Revisions, err
}

// Commit makes a new revision of an item and returns its number.
func (c *Client) Commit(ctx context.Context, token, itemID string, base int, manifestBlob string, blobIDs []string) (int, error) {
	body := map[string]any{"base_revision": base, "manifest_blob": manifestBlob, "blob_ids": blobIDs}
	var out struct {
		HeadRevision int `json:"head_revision"`
	}
	err := c.json(ctx, http.MethodPost, "/items/"+url.PathEscape(itemID)+"/revisions", token, body, &out)
	return out.HeadRevision, err
}

// DeleteItem removes an item and every restore point it has.
func (c *Client) DeleteItem(ctx context.Context, token, itemID string) error {
	var out struct {
		Deleted bool `json:"deleted"`
	}
	return c.json(ctx, http.MethodDelete, "/items/"+url.PathEscape(itemID), token, nil, &out)
}

// json is for the small calls, which are not retried: an offline status check
// should say so at once, and a commit whose answer was lost looks like a
// conflict the second time. The next push recognises its own contents at the
// head instead (Service.push).
func (c *Client) json(ctx context.Context, method, path, token string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	raw, err := c.once(ctx, method, path, token, payload, "application/json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("linetta works vault: parse response: %w", err)
	}
	return nil
}

func (c *Client) retry(ctx context.Context, call func() ([]byte, error)) ([]byte, error) {
	delay := time.Second
	for attempt := 1; ; attempt++ {
		raw, err := call()
		if err == nil || attempt == maxAttempts || !retryable(err) {
			return raw, err
		}
		pause := delay
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.retryAfter > 0 {
			pause = min(apiErr.retryAfter, maxRetryDelay)
		}
		if waitErr := c.wait(ctx, pause); waitErr != nil {
			return nil, waitErr
		}
		delay *= 2
	}
}

// retryable is an answer that may be different next time: the site out of slots
// or rate-limiting, or no answer at all. A cancelled request is the writer
// leaving, not a failure.
func retryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status >= 500 || apiErr.Status == http.StatusTooManyRequests
	}
	return true
}

func (c *Client) once(ctx context.Context, method, path, token string, payload []byte, contentType string) ([]byte, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/api/artworks/vault"+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("linetta works vault: read response: %w", err)
	}
	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		return raw, nil
	}
	apiErr := &APIError{Status: res.StatusCode}
	var e struct {
		Code         string   `json:"code"`
		Message      string   `json:"message"`
		Missing      []string `json:"missing"`
		HeadRevision int      `json:"head_revision"`
	}
	if json.Unmarshal(raw, &e) == nil {
		apiErr.Code, apiErr.Message, apiErr.Missing, apiErr.HeadRevision = e.Code, e.Message, e.Missing, e.HeadRevision
	}
	if apiErr.Code == "" && res.StatusCode == http.StatusTooManyRequests {
		apiErr.Code = CodeRateLimited
	}
	if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds > 0 {
		apiErr.retryAfter = time.Duration(seconds) * time.Second
	}
	return nil, apiErr
}
