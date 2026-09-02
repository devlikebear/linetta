package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

// Classify reduces a provider failure to a reason code the UI can translate.
// A ReasonError passes through untouched. Everything else becomes one of
// auth_failed / rate_limited / unreachable: HTTP status first, then the few
// phrases tars uses for local credential problems, then the default. The
// English message keeps only the first line, capped, so a provider's JSON
// body does not ride along into logs verbatim (the v0.8.5 lesson).
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
	if errors.As(err, &pe) {
		switch pe.StatusCode {
		case 401, 403:
			reason = rpc.ReasonProviderAuthFailed
		case 402, 429:
			reason = rpc.ReasonProviderRateLimited
		}
	}
	if reason == rpc.ReasonProviderUnreachable {
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "api key is required"),
			strings.Contains(msg, "invalid api key"),
			strings.Contains(msg, "refresh token"),
			strings.Contains(msg, "unauthorized"),
			strings.Contains(msg, "auth.json"):
			reason = rpc.ReasonProviderAuthFailed
		case strings.Contains(msg, "rate limit"),
			strings.Contains(msg, "quota"),
			strings.Contains(msg, "insufficient credit"):
			reason = rpc.ReasonProviderRateLimited
		}
	}
	return &rpc.ReasonError{Reason: reason, Err: fmt.Errorf("%s: %s", id, firstLine(err))}
}

func firstLine(err error) string {
	line := strings.TrimSpace(strings.SplitN(err.Error(), "\n", 2)[0])
	const max = 200
	if len(line) > max {
		return line[:max] + "…"
	}
	return line
}
