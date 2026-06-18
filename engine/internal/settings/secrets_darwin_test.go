//go:build darwin

package settings

import "testing"

func TestKeychainRoundTrip(t *testing.T) {
	k := keychainSecretStore{service: "devlikebear.linetta.test"}
	const name = "roundtrip-key"
	_ = k.Delete(name) // clean any leftover from a prior failed run

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
