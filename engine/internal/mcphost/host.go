//go:build !mobile

// Package mcphost serves Linetta's story tools to external MCP clients
// (Claude Code, Claude Desktop) over a loopback-only Streamable HTTP endpoint.
//
// It is hosted inside the running app rather than in a separate process
// because engineapp.Open unconditionally starts background jobs (backup,
// snapshot thinning, summarizer, folder/git sync) and because the UI refresh
// path — Go notifier → C callback → Tauri emit → useEngineEvent — is
// in-process only. A second process would double the jobs and leave the writer
// staring at a stale scene.
//
// Part of the MCP-first pivot (#47).
package mcphost

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

// ServerName and ServerVersion identify this server to clients.
const (
	ServerName    = "linetta"
	ServerVersion = "0.1.0"
)

// shutdownGrace bounds how long Stop waits for in-flight tool calls.
const shutdownGrace = 3 * time.Second

// ErrPortInUse means the configured port is taken. The writer must pick
// another one — the host never silently falls back to a different port,
// because every saved client config points at the configured one.
var ErrPortInUse = errors.New("mcphost: port already in use")

// ErrConsentRequired means MCP access has not been accepted yet.
var ErrConsentRequired = errors.New("mcphost: MCP consent is required before starting the server")

// Deps are the collaborators the host needs. Tools are registered separately
// (see tools_read.go) so this file stays about lifecycle and auth.
type Deps struct {
	Settings *settings.Store
	// Tools registers the tool set for the current mode on a fresh server.
	Tools func(s *mcp.Server, mode string)
	// Home is $LINETTA_HOME, where the discovery file lives.
	Home string
}

// Host owns the listener, the MCP server, and the discovery file.
type Host struct {
	deps Deps

	mu      sync.Mutex
	httpSrv *http.Server
	port    int
	token   string
	running bool
}

// New returns a Host. Nothing binds until Start is called.
func New(deps Deps) *Host { return &Host{deps: deps} }

// Status reports whether the server is listening and on which port.
type Status struct {
	Running   bool   `json:"running"`
	Mode      string `json:"mode"`
	Port      int    `json:"port,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	TokenSet  bool   `json:"token_set"`
}

// Status returns the current listener state.
func (h *Host) Status() Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := Status{
		Running:   h.running,
		Mode:      h.deps.Settings.MCPMode(),
		ProjectID: h.deps.Settings.MCPProjectID(),
		TokenSet:  h.deps.Settings.MCPToken() != "",
	}
	if h.running {
		st.Port = h.port
	}
	return st
}

// Start binds the loopback listener and writes the discovery file. It is a
// no-op when the mode is off or the server is already running.
func (h *Host) Start(ctx context.Context) error {
	mode := h.deps.Settings.MCPMode()
	if mode == settings.MCPModeOff {
		return nil
	}
	if !h.deps.Settings.HasMCPConsent() {
		return ErrConsentRequired
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running {
		return nil
	}

	token, err := h.deps.Settings.EnsureMCPToken()
	if err != nil {
		return err
	}
	port := h.deps.Settings.MCPPort()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		if isAddrInUse(err) {
			return fmt.Errorf("%w: %d", ErrPortInUse, port)
		}
		return fmt.Errorf("mcphost: listen on 127.0.0.1:%d: %w", port, err)
	}

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		srv := mcp.NewServer(&mcp.Implementation{
			Name:    ServerName,
			Title:   "Linetta",
			Version: ServerVersion,
		}, nil)
		if h.deps.Tools != nil {
			h.deps.Tools(srv, mode)
		}
		return srv
	}, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", authMiddleware(token, handler))

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	h.httpSrv = srv
	h.port = port
	h.token = token
	h.running = true

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logf("serve: %v", err)
		}
	}()

	if err := writeDiscoveryFile(h.deps.Home, port, token); err != nil {
		logf("discovery file: %v", err)
	}
	return nil
}

// Stop shuts the listener down and removes the discovery file. Safe to call
// when not running.
func (h *Host) Stop() error {
	h.mu.Lock()
	srv := h.httpSrv
	wasRunning := h.running
	h.httpSrv = nil
	h.running = false
	h.port = 0
	h.token = ""
	h.mu.Unlock()

	// Only a host that actually served may retract the discovery file. An
	// engine that never started MCP — mode off, or a second instance — would
	// otherwise erase a live server's endpoint on its way out, leaving the
	// bridge with nothing to read while the server is still up.
	if wasRunning {
		removeDiscoveryFile(h.deps.Home)
	}
	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	return srv.Shutdown(ctx)
}

// Restart applies changed settings (mode, port, token) by cycling the listener.
func (h *Host) Restart(ctx context.Context) error {
	if err := h.Stop(); err != nil {
		return err
	}
	return h.Start(ctx)
}
