package engineapp

import (
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/codexauth"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// codexReason is the only thing standing between a raw internal error string
// — e.g. "codexauth: both callback ports are in use (1455, 1457): listen tcp
// ..." — and UI text. The handler test constructs a *rpc.ReasonError by hand,
// so it never exercises this mapping; this table test does (#92 review,
// finding 3).
func TestCodexReason(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReason string
	}{
		{
			name:       "port in use maps to the reason code",
			err:        codexauth.ErrPortInUse,
			wantReason: rpc.ReasonCodexPortInUse,
		},
		{
			// A plain string that merely reads like ErrPortInUse's message
			// must not map — only errors.Is(err, ErrPortInUse) may, or a
			// coincidental log message would start rendering as UI text.
			name:       "an unwrapped lookalike message does not map",
			err:        errors.New("codexauth: both callback ports are in use (1455, 1457): listen tcp 127.0.0.1:1455: bind: address already in use"),
			wantReason: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := codexReason(tc.err)
			var re *rpc.ReasonError
			isReasonError := errors.As(got, &re)
			if tc.wantReason == "" {
				if isReasonError {
					t.Fatalf("codexReason(%v) = %v, want the error to pass through unwrapped", tc.err, got)
				}
				if got != tc.err {
					t.Fatalf("codexReason(%v) = %v, want the same error back", tc.err, got)
				}
				return
			}
			if !isReasonError {
				t.Fatalf("codexReason(%v) = %v, want a *rpc.ReasonError", tc.err, got)
			}
			if re.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", re.Reason, tc.wantReason)
			}
		})
	}
}

// An error unrelated to the login package entirely — not merely a plain
// string that happens to mention the same words — must also stay unwrapped.
func TestCodexReason_anUnrelatedErrorStaysUnwrapped(t *testing.T) {
	sentinel := errors.New("some other package's sentinel")
	got := codexReason(sentinel)
	if got != sentinel {
		t.Fatalf("codexReason(%v) = %v, want the same error back unchanged", sentinel, got)
	}
	var re *rpc.ReasonError
	if errors.As(got, &re) {
		t.Fatalf("codexReason wrapped an unrelated error as %v", re)
	}
}
