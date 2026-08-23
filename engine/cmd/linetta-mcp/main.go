//go:build !mobile

// Command linetta-mcp bridges a stdio MCP client to Linetta's loopback HTTP
// server.
//
// Claude Desktop cannot reach a local HTTP MCP server: its config file only
// validates stdio entries, and its connector path resolves URLs from
// Anthropic's cloud, which needs a public CA-signed endpoint. This binary is
// the stdio front door — it holds no story logic, so tool changes never
// require shipping a new bridge.
//
// It is a message-level relay, not a byte pump: the SDK's transports own SSE
// response streams, the standalone GET stream for server-initiated messages,
// and Mcp-Session-Id state on their respective sides.
//
// The mobile tag excludes mcphost, and this binary links it for the discovery
// file contract. There is no mobile client to bridge to, so the whole command
// is tagged out rather than given a stub.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/paths"
)

// notRunningMessage is what the writer actually sees in their MCP client when
// Linetta is closed, so it says what to do rather than naming a port.
const notRunningMessage = "Linetta is not serving MCP. Open Linetta and turn on MCP in Settings, then reconnect."

func main() {
	var (
		urlFlag      = flag.String("url", "", "MCP endpoint override, e.g. http://127.0.0.1:7391/mcp")
		tokenFlag    = flag.String("token", "", "bearer token override")
		printHeaders = flag.Bool("print-headers", false,
			"print the auth headers as a JSON object and exit (for Claude Code's headersHelper)")
	)
	flag.Parse()

	endpoint, token, err := resolve(*urlFlag, *tokenFlag)
	if err != nil {
		fail(err)
	}

	if *printHeaders {
		// headersHelper contract: a JSON object of string key/value pairs on
		// stdout. Emitting a bare header value here would fail silently.
		out, err := json.Marshal(map[string]string{"Authorization": "Bearer " + token})
		if err != nil {
			fail(err)
		}
		fmt.Println(string(out))
		return
	}

	if err := run(context.Background(), endpoint, token); err != nil {
		fail(err)
	}
}

// resolve finds the endpoint and token, preferring explicit flags over the
// discovery file the running app writes.
func resolve(urlFlag, tokenFlag string) (endpoint, token string, err error) {
	endpoint, token = strings.TrimSpace(urlFlag), strings.TrimSpace(tokenFlag)
	if endpoint != "" && token != "" {
		return endpoint, token, nil
	}
	home, err := paths.Home()
	if err != nil {
		return "", "", fmt.Errorf("locate Linetta's data directory: %w", err)
	}
	d, err := mcphost.ReadDiscoveryFile(home)
	if err != nil {
		// A missing or unreadable discovery file means the server is not up.
		// The writer does not care which; they care what to do about it.
		return "", "", errors.New(notRunningMessage)
	}
	if endpoint == "" {
		endpoint = fmt.Sprintf("http://127.0.0.1:%d/mcp", d.Port)
	}
	if token == "" {
		token = d.Token
	}
	return endpoint, token, nil
}

// bearerTransport attaches the token to every request. It clones rather than
// mutating: RoundTrippers must not modify the request they are given.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func run(ctx context.Context, endpoint, token string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpTransport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: bearerTransport{token: token}},
	}
	upstream, err := httpTransport.Connect(ctx)
	if err != nil {
		return errors.New(notRunningMessage)
	}
	defer upstream.Close()

	client, err := (&mcp.StdioTransport{}).Connect(ctx)
	if err != nil {
		return fmt.Errorf("open stdio: %w", err)
	}
	defer client.Close()

	return relay(ctx, client, upstream)
}

// relay pumps messages both ways until either side closes. The client closing
// stdin is how Claude Desktop stops the bridge, so that path exits cleanly.
func relay(ctx context.Context, client, upstream mcp.Connection) error {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	record := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil && err != nil {
			firstErr = err
		}
	}

	pump := func(from, to mcp.Connection, clientQuitIsClean bool) {
		defer wg.Done()
		for {
			msg, err := from.Read(ctx)
			if err != nil {
				// EOF on stdin means the client quit; anything else is worth
				// reporting through the exit code.
				if !(clientQuitIsClean && isClosed(err)) {
					record(err)
				}
				_ = from.Close()
				_ = to.Close()
				return
			}
			if err := to.Write(ctx, msg); err != nil {
				record(err)
				_ = from.Close()
				_ = to.Close()
				return
			}
		}
	}

	wg.Add(2)
	go pump(client, upstream, true)
	go pump(upstream, client, false)
	wg.Wait()
	return firstErr
}

// isClosed reports whether err is the ordinary end of a connection rather than
// a failure worth surfacing.
func isClosed(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, os.ErrClosed) ||
		strings.Contains(err.Error(), "use of closed")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
