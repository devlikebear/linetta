//go:build mobile

package engineapp

import (
	"context"
	"strings"
	"testing"
)

// iOS publishing is out of scope: the methods refuse and diagnostics says so,
// which is what hides the Settings entry and the publish actions.
func TestPublishIsUnavailableOnMobile(t *testing.T) {
	app := openTestApp(t, t.TempDir())
	raw, err := app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"diagnostics.version","params":null}`))
	if err != nil || !strings.Contains(string(raw), `"publish_available":false`) {
		t.Fatalf("diagnostics.version: %s %v", raw, err)
	}
	for _, method := range []string{"publish.connect_start", "publish.work_status", "publish.episode_publish"} {
		raw, err := app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":{}}`))
		if err != nil || !strings.Contains(string(raw), "not available in this build") {
			t.Fatalf("%s: %s %v", method, raw, err)
		}
	}
}
