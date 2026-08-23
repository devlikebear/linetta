//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)

// call sends one JSONRPC request through the app and returns the raw result.
func call(t *testing.T, app *App, method string, params string) (json.RawMessage, *rpcError) {
	t.Helper()
	if params == "" {
		params = "null"
	}
	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, method, params)
	raw, err := app.Handle(context.Background(), []byte(req))
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode %s response: %v", method, err)
	}
	return envelope.Result, envelope.Error
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type mcpStatus struct {
	Running  bool   `json:"running"`
	Mode     string `json:"mode"`
	Port     int    `json:"port"`
	TokenSet bool   `json:"token_set"`
}

func openApp(t *testing.T) *App {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	app, err := Open(context.Background(), Options{Home: home})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func portFree(t *testing.T, port int) bool {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// waitPortFree polls until the OS has actually released the port. Shutdown
// returning does not guarantee the socket is instantly rebindable — on a
// loaded machine the teardown lags — so asserting instantaneously is a flake,
// not a stronger check. A port that never frees still fails here.
func waitPortFree(t *testing.T, port int) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if portFree(t, port) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A fresh install must not open a port. MCP is opt-in.
func TestMCPDefaultsToNoListener(t *testing.T) {
	app := openApp(t)
	result, rpcErr := call(t, app, "mcp.status", "")
	if rpcErr != nil {
		t.Fatalf("mcp.status: %+v", rpcErr)
	}
	var st mcpStatus
	if err := json.Unmarshal(result, &st); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if st.Running {
		t.Fatal("a fresh engine must not be serving MCP")
	}
	if st.Mode != "off" {
		t.Fatalf("mode = %q, want off", st.Mode)
	}
}

// Enabling without consent must be refused with a reason code the UI can
// localize, not a generic internal error.
func TestMCPEnableWithoutConsentIsRefused(t *testing.T) {
	app := openApp(t)
	if _, rpcErr := call(t, app, "settings.set", `{"mcp_mode":"read_only"}`); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	_, rpcErr := call(t, app, "mcp.enable", "")
	if rpcErr == nil {
		t.Fatal("enable without consent should fail")
	}
	if got := string(rpcErr.Data); got != `{"reason":"mcp_consent_required"}` {
		t.Fatalf("error data = %s, want an mcp_consent_required reason", got)
	}
}

// The full loop: consent + mode, enable, verify the port is really bound, then
// confirm Close releases it — a leaked listener would block the next launch.
func TestMCPEnableBindsAndCloseReleasesPort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	app, err := Open(context.Background(), Options{Home: home})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	free := freeTestPort(t)
	patch := fmt.Sprintf(`{"mcp_mode":"read_only","mcp_port":%d,"mcp_consent_version":1,"mcp_consented_at":1}`, free)
	if _, rpcErr := call(t, app, "settings.set", patch); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}

	result, rpcErr := call(t, app, "mcp.enable", "")
	if rpcErr != nil {
		t.Fatalf("mcp.enable: %+v", rpcErr)
	}
	var st mcpStatus
	if err := json.Unmarshal(result, &st); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !st.Running || st.Port != free {
		t.Fatalf("status = %+v, want running on port %d", st, free)
	}
	if !st.TokenSet {
		t.Error("enabling must ensure a bearer token exists")
	}
	if portFree(t, free) {
		t.Fatal("mcp.enable reported running but nothing is listening")
	}

	if err := app.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !waitPortFree(t, free) {
		t.Fatal("Close must release the MCP port")
	}
}

// mcp.disable is the kill switch: the listener goes away immediately.
func TestMCPDisableStopsListener(t *testing.T) {
	app := openApp(t)
	free := freeTestPort(t)
	patch := fmt.Sprintf(`{"mcp_mode":"full","mcp_port":%d,"mcp_consent_version":1,"mcp_consented_at":1}`, free)
	if _, rpcErr := call(t, app, "settings.set", patch); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	if _, rpcErr := call(t, app, "mcp.enable", ""); rpcErr != nil {
		t.Fatalf("mcp.enable: %+v", rpcErr)
	}
	if portFree(t, free) {
		t.Fatal("expected a bound port after enable")
	}
	if _, rpcErr := call(t, app, "mcp.disable", ""); rpcErr != nil {
		t.Fatalf("mcp.disable: %+v", rpcErr)
	}
	if !waitPortFree(t, free) {
		t.Fatal("mcp.disable must drop the listener")
	}
}

func freeTestPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}
