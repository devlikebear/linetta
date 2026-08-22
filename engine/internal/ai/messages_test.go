package ai

import (
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/storycontext"
)

// The adapter is the only ai-side prompt logic left after the storycontext
// extraction: it must wrap storycontext.Render verbatim into the system+user
// chat-message pair, applying the context selection exactly once.
func TestBuildMessagesWrapsRender(t *testing.T) {
	c := storycontext.Context{
		SceneLabel: "씬 1",
		SceneText:  "본문 텍스트",
		UserPrompt: "이어서 써줘",
		Options:    storycontext.Options{Language: "ko"},
	}
	wantSystem, wantUser := storycontext.Render(c)

	msgs := BuildMessages(c)
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("roles = %q, %q", msgs[0].Role, msgs[1].Role)
	}
	if msgs[0].Content != wantSystem {
		t.Errorf("system mismatch:\n got %q\nwant %q", msgs[0].Content, wantSystem)
	}
	if msgs[1].Content != wantUser {
		t.Errorf("user mismatch:\n got %q\nwant %q", msgs[1].Content, wantUser)
	}
	if !strings.Contains(msgs[1].Content, "본문 텍스트") {
		t.Errorf("scene text missing from user message: %q", msgs[1].Content)
	}
}

// Selection must be applied inside the adapter path: a disabled section that
// Render would drop must not reappear in the messages.
func TestBuildMessagesAppliesSelection(t *testing.T) {
	off := false
	c := storycontext.Context{
		SceneLabel: "씬 1",
		SceneText:  "지워질 본문",
		UserPrompt: "요청",
		Options: storycontext.Options{
			Language: "ko",
			Context:  storycontext.ContextSelection{CurrentScene: &off},
		},
	}
	msgs := BuildMessages(c)
	for _, m := range msgs {
		if strings.Contains(m.Content, "지워질 본문") {
			t.Errorf("disabled current scene leaked into %q message", m.Role)
		}
	}
}
