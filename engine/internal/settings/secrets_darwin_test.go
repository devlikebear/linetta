//go:build darwin && cgo

package settings

import (
	"fmt"
	"testing"
	"time"
)

// This test talks to the real login keychain, on purpose: it is the only
// coverage the cgo backend has. It uses a per-run service name and deletes what
// it creates, so it never shares an item with the app's own service
// (keychainService) and never reads one a different test binary wrote — the
// read that makes macOS put up an access prompt and park the test.
func TestKeychainRoundTrip(t *testing.T) {
	k := keychainSecretStore{service: fmt.Sprintf("devlikebear.linetta.test.%d", time.Now().UnixNano())}
	const name = "roundtrip-key"
	t.Cleanup(func() { _ = k.Delete(name) })

	if _, ok, err := k.Get(name); err != nil || ok {
		t.Fatalf("expected absent: ok=%v err=%v", ok, err)
	}
	if err := k.Set(name, "s3cr3t"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, ok, err := k.Get(name)
	if err != nil || !ok || got != "s3cr3t" {
		t.Fatalf("get after set: got=%q ok=%v err=%v", got, ok, err)
	}
	if ok, err := k.Exists(name); err != nil || !ok {
		t.Fatalf("exists: ok=%v err=%v", ok, err)
	}
	if err := k.Set(name, "updated"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, _, _ := k.Get(name); got != "updated" {
		t.Fatalf("get after update: %q", got)
	}
	if err := k.Delete(name); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if ok, _ := k.Exists(name); ok {
		t.Fatalf("still exists after delete")
	}
}
