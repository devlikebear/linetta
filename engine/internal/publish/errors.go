package publish

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func reason(code string, err error) error {
	return &rpc.ReasonError{Reason: code, Err: err}
}

// classify turns a failed call to the site into a reason code the pane can
// explain. A ReasonError passes through. A 404 is classified by the caller,
// who knows whether the work or the episode was missing.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var re *rpc.ReasonError
	if errors.As(err, &re) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return reason(reasonForAPIError(apiErr), err)
	}
	if isTimeout(err) {
		return reason(rpc.ReasonPublishTimeout, err)
	}
	return reason(rpc.ReasonPublishUnreachable, err)
}

func reasonForAPIError(e *APIError) string {
	switch e.Code {
	case CodeUnauthorized:
		return rpc.ReasonPublishUnauthorized
	case CodePublishNotEnabled:
		return rpc.ReasonPublishNotEnabled
	case CodeQuotaExceeded:
		return rpc.ReasonPublishQuotaExceeded
	case CodeTooLarge:
		return rpc.ReasonPublishTooLarge
	case CodeInvalidSlug:
		return rpc.ReasonPublishInvalidSlug
	case CodeSlugTaken:
		return rpc.ReasonPublishSlugTaken
	case CodeRateLimited:
		return rpc.ReasonPublishRateLimited
	}
	switch {
	case e.Status == 401:
		return rpc.ReasonPublishUnauthorized
	case e.Status == 408 || e.Status == 504:
		return rpc.ReasonPublishTimeout
	case e.Status >= 500:
		return rpc.ReasonPublishUnavailable
	}
	// invalid_request, hash_mismatch, a number_taken that survived the
	// retry: the app sent something the site will not take. That is a bug
	// to report, not something the writer can fix.
	return rpc.ReasonPublishRejected
}

// isUnauthorized reports the site refusing the token.
func isUnauthorized(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.Status == 401 || apiErr.Code == CodeUnauthorized)
}

// isNotFound reports the site's 404.
func isNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.Status == 404 || apiErr.Code == CodeNotFound)
}

// isOffline reports a failure that says nothing about the site's data — the
// request never got an answer — as opposed to the site answering no.
func isOffline(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status >= 500 || apiErr.Status == 429
	}
	return err != nil && !errors.Is(err, context.Canceled)
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "client.timeout exceeded")
}
