package engineapp

import (
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

// Everything a writer can copy into a bug report, or a client can read over RPC,
// has to come back without the recovery key while one is stored.
func TestStoredVaultKeyStaysOutOfRPCResponses(t *testing.T) {
	const key = "ABCD-EFGH-JKMN-PQRS"
	app := openApp(t)
	st, err := settings.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetVaultKey(key); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := st.VaultKey(); got != key || !ok || err != nil {
		t.Fatalf("the key is not stored: %q, %v, %v", got, ok, err)
	}
	for _, method := range []string{"diagnostics.version", "diagnostics.get", "settings.get"} {
		result, rpcErr := call(t, app, method, "")
		if rpcErr != nil {
			t.Fatalf("%s: %+v", method, rpcErr)
		}
		if len(result) == 0 {
			t.Fatalf("%s: empty result", method)
		}
		if strings.Contains(string(result), key) {
			t.Fatalf("%s carries the vault key: %s", method, result)
		}
	}
}
