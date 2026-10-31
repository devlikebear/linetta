//go:build !mobile

package engineapp

import (
	"strings"
	"testing"
)

// Every method answers, and none reaches the site on a device that has not
// connected. A method missing from Handle would fail here as not found.
func TestVaultMethodsAreRegisteredAndSendNothingUntilConnected(t *testing.T) {
	app := openApp(t)
	result, rpcErr := call(t, app, "vault.status", "")
	if rpcErr != nil {
		t.Fatalf("vault.status: %+v", rpcErr)
	}
	if !strings.Contains(string(result), `"connected":false`) {
		t.Fatalf("vault.status = %s", result)
	}
	for method, params := range map[string]string{
		"vault.work_status":      `{"project_id":"p"}`,
		"vault.setup":            `{"consented":true}`,
		"vault.push":             `{"project_id":"p"}`,
		"vault.pull":             `{"project_id":"p"}`,
		"vault.resolve_conflict": `{"project_id":"p","choice":"keep_local"}`,
	} {
		_, rpcErr := call(t, app, method, params)
		if rpcErr == nil {
			t.Fatalf("%s: expected a refusal", method)
		}
		if got := string(rpcErr.Data); got != `{"reason":"vault_not_connected"}` {
			t.Errorf("%s: data = %s", method, got)
		}
	}
}

func TestVaultAvailableReachesDiagnostics(t *testing.T) {
	result, rpcErr := call(t, openApp(t), "diagnostics.get", "")
	if rpcErr != nil {
		t.Fatalf("diagnostics.get: %+v", rpcErr)
	}
	if !strings.Contains(string(result), `"vault_available":true`) {
		t.Fatalf("diagnostics.get = %s", result)
	}
}

func TestVaultParamsAreChecked(t *testing.T) {
	app := openApp(t)
	for method, params := range map[string]string{
		"vault.push":             `{}`,
		"vault.pull":             `{}`,
		"vault.resolve_conflict": `{"project_id":"p","choice":"take_remote"}`,
	} {
		if _, rpcErr := call(t, app, method, params); rpcErr == nil || len(rpcErr.Data) != 0 {
			t.Errorf("%s with %s: %+v", method, params, rpcErr)
		}
	}
}
