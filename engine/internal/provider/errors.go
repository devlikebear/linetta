package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

// Classify reduces a provider failure to a reason code the UI can translate.
// A ReasonError passes through untouched. Everything else is read in three
// passes: the HTTP status, then whether the request ran out of time, then the
// few phrases tars and the providers use for credential and quota problems.
// What none of them recognises stays unreachable. The English message keeps
// only the first line, capped, so a provider's JSON body does not ride along
// into logs verbatim (the v0.8.5 lesson).
func Classify(id string, err error) error {
	if err == nil {
		return nil
	}
	var re *rpc.ReasonError
	if errors.As(err, &re) {
		return err
	}
	reason := rpc.ReasonProviderUnreachable
	var pe *llm.ProviderError
	if errors.As(err, &pe) && pe.StatusCode > 0 {
		reason = reasonForStatus(pe.StatusCode)
	} else if isTimeout(err) {
		reason = rpc.ReasonProviderTimeout
	}
	// A 4xx is not always the request's fault: Gemini answers a bad API key
	// with 400 INVALID_ARGUMENT and some gateways report an exhausted quota
	// the same way, so the phrases still get a say over "rejected".
	if reason == rpc.ReasonProviderUnreachable || reason == rpc.ReasonProviderRejected {
		msg := strings.ToLower(err.Error())
		switch {
		// The Claude Code CLI reports through its exit and its own words, not
		// an HTTP status, so its failures are only ever recognisable here.
		case strings.Contains(msg, "executable not found"):
			reason = rpc.ReasonProviderNotConfigured
		case strings.Contains(msg, "/login"),
			strings.Contains(msg, "not logged in"),
			strings.Contains(msg, "oauth token"),
			strings.Contains(msg, "api key is required"),
			strings.Contains(msg, "invalid api key"),
			strings.Contains(msg, "api key not valid"),
			strings.Contains(msg, "refresh token"),
			strings.Contains(msg, "unauthorized"),
			strings.Contains(msg, "auth.json"):
			reason = rpc.ReasonProviderAuthFailed
		case strings.Contains(msg, "rate limit"),
			strings.Contains(msg, "usage limit"),
			strings.Contains(msg, "limit reached"),
			strings.Contains(msg, "credit balance"),
			strings.Contains(msg, "quota"),
			strings.Contains(msg, "insufficient credit"):
			reason = rpc.ReasonProviderRateLimited
		}
	}
	if reason == rpc.ReasonProviderRejected && rejectsModel(err.Error()) {
		reason = rpc.ReasonProviderModelUnsupported
	}
	return &rpc.ReasonError{Reason: reason, Err: fmt.Errorf("%s: %s", id, firstLine(err))}
}

// reasonForStatus maps an HTTP status the provider actually sent.
func reasonForStatus(code int) string {
	switch {
	case code == 401 || code == 403:
		return rpc.ReasonProviderAuthFailed
	case code == 402 || code == 429:
		return rpc.ReasonProviderRateLimited
	case code == 408 || code == 504:
		return rpc.ReasonProviderTimeout
	case code >= 500:
		return rpc.ReasonProviderUnavailable
	case code >= 400:
		return rpc.ReasonProviderRejected
	}
	return rpc.ReasonProviderUnreachable
}

// rejectsModel reports a 400/404 whose body says the model is the problem —
// Codex's "The 'x' model is not supported when using Codex with a ChatGPT
// account", Gemini's "models/x is not found", Anthropic's not_found_error on
// "model: x". Both halves are required, so a refusal that merely mentions a
// model next to some other complaint stays a plain rejection.
func rejectsModel(msg string) bool {
	msg = strings.ToLower(msg)
	if !strings.Contains(msg, "model") {
		return false
	}
	for _, phrase := range []string{"not supported", "unsupported", "not found", "not_found", "does not exist"} {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}

// isTimeout reports a request that was sent but not answered in time — an
// http.Client timeout, a context deadline, a net.Error saying so. A dial that
// was refused or a name that did not resolve is not one of these: that is the
// network, and stays unreachable.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "client.timeout exceeded") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "timed out") // the CLI: "cli timed out: no output for 15m0s"
}

func firstLine(err error) string {
	line := strings.TrimSpace(strings.SplitN(err.Error(), "\n", 2)[0])
	const max = 200
	if len(line) > max {
		return line[:max] + "…"
	}
	return line
}
