package ai

import (
	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/devlikebear/tars/pkg/llm"
)

// BuildMessages wraps storycontext.Render into the two-message system+user
// pair the engine sends to tars. This adapter is the only place the rendered
// brief meets an LLM message type; it goes away with this package in the
// MCP-first pivot's removal phase.
//
// Why msg.Content (string) and not msg.ContentBlocks: both claude-code-cli and
// openai-codex providers in tars/pkg/llm read the plain `Content` field; the
// openai-codex provider only puts system messages into the Responses API's
// `instructions` field when `msg.Content` is non-empty, and claude-code-cli's
// system-prompt assembler ignores ContentBlocks entirely. ContentBlocks is for
// multimodal inputs (images, PDFs) which we don't send.
func BuildMessages(c storycontext.Context) []llm.ChatMessage {
	system, user := storycontext.Render(c)
	return []llm.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}
