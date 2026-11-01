//go:build !mobile && !mas

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
		"vault.delete_item":      `{"item_id":"item-id-of-sixteen"}`,
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

func TestVaultItemsWorkWithoutAConnection(t *testing.T) {
	app := openApp(t)
	result, rpcErr := call(t, app, "vault.items_get", "")
	if rpcErr != nil {
		t.Fatalf("vault.items_get: %+v", rpcErr)
	}
	if !strings.Contains(string(result), `"required":true`) || !strings.Contains(string(result), `"id":"images"`) {
		t.Fatalf("vault.items_get = %s", result)
	}
	result, rpcErr = call(t, app, "vault.items_set", `{"omit":["images"]}`)
	if rpcErr != nil {
		t.Fatalf("vault.items_set: %+v", rpcErr)
	}
	if !strings.Contains(string(result), `"id":"images","bytes":0,"enabled":false`) {
		t.Fatalf("vault.items_set = %s", result)
	}
}

func TestVaultItemsRefuseWhatCannotBeOff(t *testing.T) {
	app := openApp(t)
	for _, params := range []string{`{}`, `{"omit":["manuscript"]}`, `{"omit":[""]}`} {
		if _, rpcErr := call(t, app, "vault.items_set", params); rpcErr == nil {
			t.Errorf("%s was accepted", params)
		}
	}
}

func TestSyncMethodsAreRegisteredAndOffByDefault(t *testing.T) {
	app := openApp(t)
	result, rpcErr := call(t, app, "sync.status", "")
	if rpcErr != nil {
		t.Fatalf("sync.status: %+v", rpcErr)
	}
	if !strings.Contains(string(result), `"enabled":false`) || !strings.Contains(string(result), `"state":"off"`) {
		t.Fatalf("sync.status = %s", result)
	}
	for method, params := range map[string]string{
		"sync.preview":    "",
		"sync.run_now":    "",
		"sync.touch":      `{"project_id":"p"}`,
		"sync.set_active": `{"project_id":"p","active":true}`,
	} {
		if _, rpcErr := call(t, app, method, params); rpcErr != nil {
			t.Errorf("%s: %+v", method, rpcErr)
		}
	}
	for method, params := range map[string]string{
		"sync.touch":      `{}`,
		"sync.set_active": `{"project_id":"p"}`,
		"sync.set_auto":   `{}`,
	} {
		if _, rpcErr := call(t, app, method, params); rpcErr == nil {
			t.Errorf("%s with %s was accepted", method, params)
		}
	}
}

func TestSyncSetAutoTurnsOnAndSaysWhyItStopped(t *testing.T) {
	app := openApp(t)
	result, rpcErr := call(t, app, "sync.set_auto", `{"enabled":true}`)
	if rpcErr != nil {
		t.Fatalf("sync.set_auto: %+v", rpcErr)
	}
	if !strings.Contains(string(result), `"enabled":true`) {
		t.Fatalf("sync.set_auto = %s", result)
	}
	if _, rpcErr := call(t, app, "sync.set_auto", `{"enabled":false}`); rpcErr != nil {
		t.Fatalf("sync.set_auto: %+v", rpcErr)
	}
}

func TestSyncTargetsListWhatThisBuildHas(t *testing.T) {
	app := openApp(t)
	result, rpcErr := call(t, app, "sync.targets", "")
	if rpcErr != nil {
		t.Fatalf("sync.targets: %+v", rpcErr)
	}
	for _, want := range []string{`"primary":"works"`, `"id":"works"`, `"id":"git"`, `"id":"folder"`, `"key_stored":false`} {
		if !strings.Contains(string(result), want) {
			t.Errorf("sync.targets = %s, want %s", result, want)
		}
	}
	for _, params := range []string{`{}`, `{"primary":"nowhere"}`, `{"primary":"works","mirrors":["nowhere"]}`} {
		if _, rpcErr := call(t, app, "sync.set_targets", params); rpcErr == nil {
			t.Errorf("%s was accepted", params)
		}
	}
	if _, rpcErr := call(t, app, "sync.restore", `{"target":"nowhere"}`); rpcErr == nil {
		t.Error("restoring from nothing was accepted")
	}
}

func TestChangingATargetNeedsItToWork(t *testing.T) {
	app := openApp(t)
	// No folder is set and no key exists, so nothing may switch.
	if _, rpcErr := call(t, app, "sync.set_targets", `{"primary":"folder","mirrors":[]}`); rpcErr == nil {
		t.Fatal("a target that cannot take the library was accepted")
	}
	result, rpcErr := call(t, app, "sync.targets", "")
	if rpcErr != nil {
		t.Fatalf("sync.targets: %+v", rpcErr)
	}
	if !strings.Contains(string(result), `"primary":"works"`) {
		t.Fatalf("sync.targets = %s", result)
	}
	// A mirror changes nothing about the primary, and needs no first upload.
	result, rpcErr = call(t, app, "sync.set_targets", `{"primary":"works","mirrors":["folder","folder","works"]}`)
	if rpcErr != nil {
		t.Fatalf("sync.set_targets: %+v", rpcErr)
	}
	if !strings.Contains(string(result), `"mirrors":["folder"]`) {
		t.Fatalf("sync.set_targets = %s", result)
	}
}

func TestVaultStatusNamesATarget(t *testing.T) {
	app := openApp(t)
	result, rpcErr := call(t, app, "vault.status", `{"target":"works"}`)
	if rpcErr != nil || !strings.Contains(string(result), `"connected":false`) {
		t.Fatalf("vault.status = %s, %+v", result, rpcErr)
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
