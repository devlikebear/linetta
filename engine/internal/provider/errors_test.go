package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

type netTimeout struct{}

func (netTimeout) Error() string   { return "i/o timeout" }
func (netTimeout) Timeout() bool   { return true }
func (netTimeout) Temporary() bool { return true }

func TestClassify_tellsTheNetworkApartFromTheProvider(t *testing.T) {
	status := func(code int, body string) error {
		return &llm.ProviderError{Provider: "gemini-native", Operation: "stream", StatusCode: code, Message: body}
	}
	transport := func(cause error) error {
		return &llm.ProviderError{Provider: "gemini-native", Operation: "stream", Cause: &url.Error{Op: "Post", URL: "https://x.test", Err: cause}}
	}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"401", status(401, "nope"), rpc.ReasonProviderAuthFailed},
		{"403", status(403, "nope"), rpc.ReasonProviderAuthFailed},
		{"402", status(402, "pay"), rpc.ReasonProviderRateLimited},
		{"429", status(429, "slow"), rpc.ReasonProviderRateLimited},
		{"400 bad request", status(400, "Function call is missing a thought_signature"), rpc.ReasonProviderRejected},
		{"404 unknown model", status(404, "models/x is not found"), rpc.ReasonProviderRejected},
		{"413 too large", status(413, "too large"), rpc.ReasonProviderRejected},
		// Gemini answers a bad key with 400, not 401.
		{"400 bad key", status(400, "API key not valid. Please pass a valid API key."), rpc.ReasonProviderAuthFailed},
		{"400 quota", status(400, "Quota exceeded for metric"), rpc.ReasonProviderRateLimited},
		{"500", status(500, "internal"), rpc.ReasonProviderUnavailable},
		{"503 overloaded", status(503, "The model is overloaded."), rpc.ReasonProviderUnavailable},
		{"504", status(504, "gateway timeout"), rpc.ReasonProviderTimeout},
		{"408", status(408, "request timeout"), rpc.ReasonProviderTimeout},
		{"client timeout", transport(netTimeout{}), rpc.ReasonProviderTimeout},
		{"context deadline", fmt.Errorf("stream: %w", context.DeadlineExceeded), rpc.ReasonProviderTimeout},
		{"timeout in text only", errors.New("Post x: net/http: request canceled (Client.Timeout exceeded while awaiting headers)"), rpc.ReasonProviderTimeout},
		{"connection refused", transport(&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}), rpc.ReasonProviderUnreachable},
		{"dns", transport(&net.DNSError{Err: "no such host", Name: "x.test"}), rpc.ReasonProviderUnreachable},
		{"plain network text", errors.New("dial tcp: connection refused"), rpc.ReasonProviderUnreachable},
		{"local credential phrase", errors.New("auth.json missing"), rpc.ReasonProviderAuthFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var re *rpc.ReasonError
			if !errors.As(Classify("gemini-native", tc.err), &re) {
				t.Fatalf("not a ReasonError")
			}
			if re.Reason != tc.want {
				t.Errorf("reason = %q, want %q", re.Reason, tc.want)
			}
		})
	}
}

func TestClassify_passesAReasonErrorThrough(t *testing.T) {
	in := &rpc.ReasonError{Reason: rpc.ReasonProviderConsentRequired, Err: errors.New("x")}
	if got := Classify("anthropic", in); got != in {
		t.Errorf("got %v, want the same error back", got)
	}
}
