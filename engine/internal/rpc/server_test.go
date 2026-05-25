package rpc

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestServer_ping_roundtrip(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	out := &lineCapture{}

	s := NewServer()
	s.Handle("ping", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"pong"`), nil
	})

	if err := s.Serve(context.Background(), in, out); err != nil && err != io.EOF {
		t.Fatalf("Serve: %v", err)
	}

	want := `{"jsonrpc":"2.0","id":1,"result":"pong"}` + "\n"
	if got := out.String(); got != want {
		t.Errorf("response = %q, want %q", got, want)
	}
}

func TestServer_method_not_found(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"nope"}` + "\n")
	out := &lineCapture{}

	s := NewServer()
	if err := s.Serve(context.Background(), in, out); err != nil && err != io.EOF {
		t.Fatalf("Serve: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `"code":-32601`) {
		t.Errorf("expected method-not-found code in %q", got)
	}
	if !strings.Contains(got, `"id":2`) {
		t.Errorf("expected id=2 in %q", got)
	}
}

func TestServer_notification_is_silent(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"ping"}` + "\n")
	out := &lineCapture{}

	s := NewServer()
	s.Handle("ping", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"pong"`), nil
	})

	if err := s.Serve(context.Background(), in, out); err != nil && err != io.EOF {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("notification produced output: %q", out.String())
	}
}

// lineCapture is a tiny io.Writer that also exposes the buffer string.
type lineCapture struct{ b strings.Builder }

func (l *lineCapture) Write(p []byte) (int, error) { return l.b.Write(p) }
func (l *lineCapture) String() string              { return l.b.String() }
func (l *lineCapture) Len() int                    { return l.b.Len() }

func TestServer_notifierEmitsDuringServe(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"slow"}` + "\n")
	out := &lineCapture{}

	s := NewServer()
	notifier := s.Notifier()
	s.Handle("slow", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		_ = notifier.Notify("progress", map[string]any{"step": 1})
		_ = notifier.Notify("progress", map[string]any{"step": 2})
		return json.RawMessage(`"ok"`), nil
	})
	if err := s.Serve(context.Background(), in, out); err != nil && err != io.EOF {
		t.Fatalf("Serve: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), out.String())
	}
	if !strings.Contains(lines[0], `"method":"progress"`) || !strings.Contains(lines[0], `"step":1`) {
		t.Errorf("line 0 = %q", lines[0])
	}
	if !strings.Contains(lines[1], `"method":"progress"`) || !strings.Contains(lines[1], `"step":2`) {
		t.Errorf("line 1 = %q", lines[1])
	}
	if !strings.Contains(lines[2], `"id":1`) || !strings.Contains(lines[2], `"result":"ok"`) {
		t.Errorf("line 2 = %q", lines[2])
	}
}

func TestServer_notifyBeforeServe_isError(t *testing.T) {
	s := NewServer()
	if err := s.Notifier().Notify("foo", nil); err == nil {
		t.Error("expected error when Notify is called before Serve")
	}
}
