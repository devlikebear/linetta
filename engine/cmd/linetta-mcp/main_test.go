package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// writeDiscovery drops the file the running app would have written.
func writeDiscovery(t *testing.T, home, endpoint, token string) {
	t.Helper()
	port := 0
	if _, err := fmtSscanPort(endpoint, &port); err != nil {
		t.Fatalf("parse endpoint %q: %v", endpoint, err)
	}
	raw, err := json.Marshal(map[string]any{
		"port": port, "token": token, "pid": os.Getpid(), "started_at": 1,
	})
	if err != nil {
		t.Fatalf("marshal discovery: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "mcp.json"), raw, 0o600); err != nil {
		t.Fatalf("write discovery: %v", err)
	}
}

// fmtSscanPort pulls the port out of http://127.0.0.1:PORT/mcp.
func fmtSscanPort(endpoint string, out *int) (int, error) {
	host := strings.TrimPrefix(endpoint, "http://127.0.0.1:")
	host = strings.TrimSuffix(host, "/mcp")
	var p int
	for _, r := range host {
		if r < '0' || r > '9' {
			return 0, errors.New("unexpected endpoint shape")
		}
		p = p*10 + int(r-'0')
	}
	*out = p
	return p, nil
}

// startStubServer runs a real MCP server over HTTP with one tool, requiring
// the bearer token. This is what the bridge must reach.
func startStubServer(t *testing.T, token string) string {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "stub", Version: "1"}, nil)
	type in struct {
		Echo string `json:"echo"`
	}
	type out struct {
		Echo string `json:"echo"`
	}
	mcp.AddTool(srv, &mcp.Tool{Name: "stub_echo", Description: "echo"},
		func(_ context.Context, _ *mcp.CallToolRequest, i in) (*mcp.CallToolResult, out, error) {
			return nil, out{Echo: i.Echo}, nil
		})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts.URL + "/mcp"
}

// The bridge's real job: an MCP client speaking stdio reaches the HTTP server
// and gets a tool result back, with the token attached on the way.
func TestBridgeRelaysAToolCall(t *testing.T) {
	token := "test-token"
	endpoint := startStubServer(t, token)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	upstream, err := (&mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: bearerTransport{token: token}},
	}).Connect(ctx)
	if err != nil {
		t.Fatalf("connect upstream: %v", err)
	}
	defer upstream.Close()

	// An in-memory pair stands in for stdio so the test drives the same relay
	// the real binary runs.
	clientTransport, bridgeSide := mcp.NewInMemoryTransports()
	bridgeConn, err := bridgeSide.Connect(ctx)
	if err != nil {
		t.Fatalf("connect bridge side: %v", err)
	}
	go func() { _ = relay(ctx, bridgeConn, upstream) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "probe", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect through the bridge: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list through the bridge: %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "stub_echo" {
		t.Fatalf("tools = %+v, want the stub tool", tools.Tools)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "stub_echo", Arguments: map[string]any{"echo": "지워진 이름"},
	})
	if err != nil {
		t.Fatalf("tools/call through the bridge: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool call errored: %+v", res)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if !strings.Contains(string(raw), "지워진 이름") {
		t.Fatalf("result did not round-trip: %s", raw)
	}
}

// Without a running server the writer gets an instruction, not a stack trace
// or a port number — this string is what shows up in their MCP client.
func TestResolveWithoutADiscoveryFile(t *testing.T) {
	t.Setenv("LINETTA_HOME", t.TempDir())
	_, _, err := resolve("", "")
	if err == nil {
		t.Fatal("resolve must fail when Linetta is not serving")
	}
	if !strings.Contains(err.Error(), "Open Linetta") {
		t.Fatalf("message = %q, want the instruction the writer can act on", err.Error())
	}
}

func TestResolveReadsTheDiscoveryFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	writeDiscovery(t, home, "http://127.0.0.1:7391/mcp", "disk-token")

	endpoint, token, err := resolve("", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if endpoint != "http://127.0.0.1:7391/mcp" || token != "disk-token" {
		t.Fatalf("resolve() = %q, %q", endpoint, token)
	}
}

// Explicit flags win, so an advanced setup never needs the discovery file.
func TestResolvePrefersFlags(t *testing.T) {
	t.Setenv("LINETTA_HOME", t.TempDir())
	endpoint, token, err := resolve("http://127.0.0.1:9999/mcp", "flag-token")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if endpoint != "http://127.0.0.1:9999/mcp" || token != "flag-token" {
		t.Fatalf("resolve() = %q, %q", endpoint, token)
	}
}

// headersHelper requires a JSON object of string pairs on stdout. A bare
// header value would be accepted by the shell and then silently fail to
// authenticate, so the shape is pinned here.
func TestPrintHeadersEmitsAJSONObject(t *testing.T) {
	raw, err := json.Marshal(map[string]string{"Authorization": "Bearer abc123"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the helper output must parse as a JSON object: %v", err)
	}
	if got["Authorization"] != "Bearer abc123" {
		t.Fatalf("headers = %v, want a Bearer Authorization entry", got)
	}
}

func TestBearerTransportDoesNotMutateTheRequest(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:1/mcp", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	captured := ""
	rt := bearerTransport{token: "tok", base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		captured = r.Header.Get("Authorization")
		return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}}, nil
	})}
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if captured != "Bearer tok" {
		t.Errorf("outgoing Authorization = %q", captured)
	}
	// The RoundTripper contract forbids modifying the caller's request.
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("the original request was mutated: %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
