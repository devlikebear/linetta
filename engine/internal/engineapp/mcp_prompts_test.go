//go:build !mobile

package engineapp

import (
	"encoding/json"
	"testing"
)

// #133: /linetta:draft_current_scene in an external client and the panel's
// "draft the current scene" chip put the same sentence in front of the agent,
// in the app's language, because both come from one prompts/get.
func TestSlashCommandAndPanelChipSayTheSameThing(t *testing.T) {
	app, c, _, _ := startWritableMCP(t)
	for _, lang := range []string{"ko", "en", "ja"} {
		if _, rpcErr := call(t, app, "settings.set", `{"language":"`+lang+`"}`); rpcErr != nil {
			t.Fatalf("settings.set: %+v", rpcErr)
		}
		raw, rpcErr := call(t, app, "agent.prompts", "")
		if rpcErr != nil {
			t.Fatalf("agent.prompts: %+v", rpcErr)
		}
		var panel struct {
			Prompts []struct {
				Name, Title, Text string
				Slots             []string
			}
		}
		if err := json.Unmarshal(raw, &panel); err != nil {
			t.Fatal(err)
		}
		chips := map[string]string{}
		for _, p := range panel.Prompts {
			chips[p.Name] = p.Text
		}

		// A new session, as the host reads the language per connection.
		c.sessionID = ""
		c.initialize()
		listed := c.rpc("prompts/list", map[string]any{})
		prompts, _ := listed["result"].(map[string]any)["prompts"].([]any)
		if len(prompts) != len(panel.Prompts) {
			t.Fatalf("%s: client lists %d prompts, panel shows %d", lang, len(prompts), len(panel.Prompts))
		}
		for _, name := range []string{"draft_current_scene", "check_continuity", "suggest_next_scene"} {
			got := c.rpc("prompts/get", map[string]any{"name": name})
			msgs, _ := got["result"].(map[string]any)["messages"].([]any)
			if len(msgs) != 1 {
				t.Fatalf("%s %s: messages %v", lang, name, got)
			}
			text := msgs[0].(map[string]any)["content"].(map[string]any)["text"]
			if text == "" || text != chips[name] {
				t.Errorf("%s %s: client gets %q, chip says %q", lang, name, text, chips[name])
			}
		}
	}
}
