//go:build !mobile

package mcphost

import "testing"

// mcp.changed has to say who wrote, for the same reason the activity log
// does. The workspace's conflict banner is the consumer that matters: without
// a source it tells a writer that "an external agent changed this scene"
// about a revision they asked their own built-in panel for — at a moment when
// the HTTP host may not even be running.
//
// The value comes from ToolDeps.Source, set when the server is composed, so
// an external client cannot claim to be the agent by anything it sends.
func TestNotifyChanged_namesWhoWrote(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		want   string
	}{
		{"the built-in panel", SourceAgent, SourceAgent},
		{"an HTTP client", SourceExternal, SourceExternal},
		{"deps that never set a source", "", SourceExternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got ChangedPayload
			d := ToolDeps{
				Source: tc.source,
				Notify: func(method string, params any) {
					if method != "mcp.changed" {
						t.Errorf("method = %q, want mcp.changed", method)
					}
					p, ok := params.(ChangedPayload)
					if !ok {
						t.Fatalf("params = %T, want ChangedPayload", params)
					}
					got = p
				},
			}
			d.notifyChanged("p1", "linetta_write_scene", []string{"n1"}, "batch-1")
			if got.Source != tc.want {
				t.Errorf("source = %q, want %q", got.Source, tc.want)
			}
			if got.ProjectID != "p1" || got.Tool != "linetta_write_scene" || got.BatchID != "batch-1" {
				t.Errorf("payload = %+v, want the rest of it unchanged", got)
			}
		})
	}
}
