//go:build !mobile

package mcphost

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

func newHost(t *testing.T, mode string, consent bool) (*Host, *settings.Store, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	st, err := settings.NewWithSecretStore(settings.NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	patch := settings.Patch{MCPMode: &mode, MCPPort: freePort(t)}
	if consent {
		version := settings.MCPConsentVersion
		at := int64(1)
		patch.MCPConsentVersion = &version
		patch.MCPConsentedAt = &at
	}
	if _, err := st.Set(context.Background(), patch); err != nil {
		t.Fatalf("settings.Set: %v", err)
	}
	h := New(Deps{Settings: st, Home: home})
	t.Cleanup(func() { _ = h.Stop() })
	return h, st, home
}

// freePort grabs a port the OS just handed out, then releases it, so parallel
// test runs do not collide on the fixed default.
func freePort(t *testing.T) *int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return &port
}

func endpoint(h *Host) string {
	return fmt.Sprintf("http://127.0.0.1:%d/mcp", h.Status().Port)
}

// A POST that should reach the handler; the body is a valid initialize call so
// only auth decides the outcome.
func post(t *testing.T, url string, headers map[string]string) *http.Response {
	t.Helper()
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize",` +
		`"params":{"protocolVersion":"2025-06-18","capabilities":{},` +
		`"clientInfo":{"name":"test","version":"1"}}}`)
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// Mode off must leave the machine untouched: nothing binds, nothing is written.
func TestStartIsNoopWhenModeOff(t *testing.T) {
	h, _, home := newHost(t, settings.MCPModeOff, true)
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.Status().Running {
		t.Fatal("mode off must not bind a listener")
	}
	if _, err := os.Stat(filepath.Join(home, DiscoveryFileName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("mode off must not write a discovery file")
	}
}

// Enabling without consent must fail loudly rather than quietly serving.
func TestStartRequiresConsent(t *testing.T) {
	h, _, _ := newHost(t, settings.MCPModeReadOnly, false)
	err := h.Start(context.Background())
	if !errors.Is(err, ErrConsentRequired) {
		t.Fatalf("Start without consent = %v, want ErrConsentRequired", err)
	}
	if h.Status().Running {
		t.Fatal("server must not run without consent")
	}
}

func TestStartWritesDiscoveryFileAndStopRemovesIt(t *testing.T) {
	h, st, home := newHost(t, settings.MCPModeReadOnly, true)
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status := h.Status()
	if !status.Running || status.Port == 0 {
		t.Fatalf("status = %+v, want a running server with a port", status)
	}

	path := filepath.Join(home, DiscoveryFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat discovery file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("discovery file mode = %o, want 600", perm)
		}
	}
	d, err := ReadDiscoveryFile(home)
	if err != nil {
		t.Fatalf("ReadDiscoveryFile: %v", err)
	}
	if d.Port != status.Port || d.Token != st.MCPToken() || d.PID != os.Getpid() {
		t.Fatalf("discovery = %+v, want port %d and the live token/pid", d, status.Port)
	}

	if err := h.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Error("Stop must remove the discovery file so a stale endpoint is never advertised")
	}
	if h.Status().Running {
		t.Error("status must report stopped after Stop")
	}
}

// The port is the writer's setting: a busy one is an error they can see and
// act on, never a silent bind somewhere else that breaks saved configs.
func TestStartReportsPortInUse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	st, err := settings.NewWithSecretStore(settings.NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	mode := settings.MCPModeReadOnly
	version := settings.MCPConsentVersion
	at := int64(1)
	if _, err := st.Set(context.Background(), settings.Patch{
		MCPMode: &mode, MCPPort: &busy, MCPConsentVersion: &version, MCPConsentedAt: &at,
	}); err != nil {
		t.Fatalf("settings.Set: %v", err)
	}

	h := New(Deps{Settings: st, Home: home})
	t.Cleanup(func() { _ = h.Stop() })
	err = h.Start(context.Background())
	if !errors.Is(err, ErrPortInUse) {
		t.Fatalf("Start on a busy port = %v, want ErrPortInUse", err)
	}
	if h.Status().Running {
		t.Fatal("a failed bind must not leave the host marked running")
	}
}

func TestAuthRejectsMissingAndWrongToken(t *testing.T) {
	h, _, _ := newHost(t, settings.MCPModeReadOnly, true)
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	url := endpoint(h)

	if resp := post(t, url, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", resp.StatusCode)
	}
	if resp := post(t, url, map[string]string{"Authorization": "Bearer nope"}); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", resp.StatusCode)
	}
	if resp := post(t, url, map[string]string{"Authorization": "nope"}); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("malformed scheme: status = %d, want 401", resp.StatusCode)
	}
}

// A web page on any site can POST to 127.0.0.1; a non-loopback Origin is the
// DNS-rebinding signature the MCP spec tells servers to reject.
func TestAuthRejectsForeignOrigin(t *testing.T) {
	h, st, _ := newHost(t, settings.MCPModeReadOnly, true)
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	auth := "Bearer " + st.MCPToken()
	resp := post(t, endpoint(h), map[string]string{
		"Authorization": auth,
		"Origin":        "https://evil.test",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin: status = %d, want 403 even with a valid token", resp.StatusCode)
	}
}

func TestAuthAllowsLoopbackOriginWithToken(t *testing.T) {
	h, st, _ := newHost(t, settings.MCPModeReadOnly, true)
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	auth := "Bearer " + st.MCPToken()
	for _, origin := range []string{"", "http://localhost:3000", "http://127.0.0.1:5173"} {
		headers := map[string]string{"Authorization": auth}
		if origin != "" {
			headers["Origin"] = origin
		}
		resp := post(t, endpoint(h), headers)
		// 200 proves the request reached the MCP handler and initialize
		// succeeded, not merely that auth declined to reject it.
		if resp.StatusCode != http.StatusOK {
			t.Errorf("origin %q: status = %d, want 200 from the MCP handler", origin, resp.StatusCode)
		}
	}
}

func TestUnitOriginAndHostChecks(t *testing.T) {
	for _, tc := range []struct {
		origin string
		want   bool
	}{
		{"", true},
		{"http://localhost", true},
		{"http://127.0.0.1:7391", true},
		{"http://[::1]:7391", true},
		{"https://evil.test", false},
		{"http://127.0.0.1.evil.test", false},
		{"not a url at all ::::", false},
	} {
		if got := originAllowed(tc.origin); got != tc.want {
			t.Errorf("originAllowed(%q) = %v, want %v", tc.origin, got, tc.want)
		}
	}
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"127.0.0.1:7391", true},
		{"localhost:7391", true},
		{"[::1]:7391", true},
		{"linetta.evil.test", false},
		{"", false},
	} {
		if got := hostAllowed(tc.host); got != tc.want {
			t.Errorf("hostAllowed(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

// Regression: Stop removed the discovery file unconditionally, so an engine
// that never served MCP — mode off, or a second instance sharing the home —
// erased a live server's endpoint on its way out. The server kept serving
// while the bridge had nothing left to read.
func TestStopKeepsAnotherHostsDiscoveryFile(t *testing.T) {
	live, _, home := newHost(t, settings.MCPModeReadOnly, true)
	if err := live.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	path := filepath.Join(home, DiscoveryFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("live server should have written a discovery file: %v", err)
	}

	// A second host over the same home that never starts (mode off).
	idle := New(Deps{Settings: idleSettings(t, home), Home: home})
	if err := idle.Start(context.Background()); err != nil {
		t.Fatalf("idle Start: %v", err)
	}
	if err := idle.Stop(); err != nil {
		t.Fatalf("idle Stop: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatal("an idle host's shutdown must not remove the live server's discovery file")
	}
	if !live.Status().Running {
		t.Fatal("the live server should still be serving")
	}

	// The owner still retracts its own file.
	if err := live.Stop(); err != nil {
		t.Fatalf("live Stop: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("the owning host must remove its discovery file on shutdown")
	}
}

// idleSettings returns a store over the same home with MCP off, standing in
// for an engine instance that shares the data directory but never serves.
func idleSettings(t *testing.T, home string) *settings.Store {
	t.Helper()
	t.Setenv("LINETTA_HOME", home)
	s, err := settings.NewWithSecretStore(settings.NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	off := settings.MCPModeOff
	if _, err := s.Set(context.Background(), settings.Patch{MCPMode: &off}); err != nil {
		t.Fatalf("settings.Set(off): %v", err)
	}
	return s
}
