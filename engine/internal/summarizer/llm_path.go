package summarizer

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/devlikebear/tars/pkg/llm"
)

// This file is the summarizer's entire LLM surface. The MCP-first pivot (#47)
// replaces provider-generated summaries with agent-written ones
// (linetta_write_summary), so this file — and the provider plumbing it uses —
// is deleted wholesale in the removal phase. The short-scene plain-text path
// in summarizer.go stays.

const systemPrompt = "다음 본문을 본문과 같은 언어로 3~5문장으로 요약하라. 등장인물·장소·핵심 사건은 반드시 보존하라. 새 정보 추가 금지. (Summarize the passage below in 3-5 sentences, in the same language as the passage. Preserve characters, places, and key events. Do not add new information.)"
const containerSystemPrompt = "다음은 소설의 하위 단위 요약들이다. 이 단위 전체를 요약들과 같은 언어로 3~5문장으로 요약하라. 등장인물·장소·핵심 사건은 반드시 보존하라. 새 정보 추가 금지. (The lines below are summaries of a fiction unit's children. Summarize the whole unit in 3-5 sentences, in the same language as those summaries. Preserve characters, places, and key events. Do not add new information.)"

// summarizeViaLLM sends one summarize request to the configured provider and
// returns the trimmed summary. ok is false when the provider is unavailable,
// the request fails, or the model returns nothing; failures are logged and
// recorded against nodeID, and an empty response is silently skipped —
// preserving the summarizer's long-standing best-effort behavior.
func (s *Summarizer) summarizeViaLLM(ctx context.Context, nodeID, label, system, userText string) (string, bool) {
	rp := s.src.Resolve()
	provider := rp.Provider
	client, err := s.factory(rp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: factory(%s): %v\n", provider, err)
		s.recordError(ctx, nodeID, err.Error())
		return "", false
	}
	msgs := []llm.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: userText},
	}
	resp, err := client.Chat(ctx, msgs, llm.ChatOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: Chat%s %s: %v\n", label, nodeID, err)
		s.recordError(ctx, nodeID, err.Error())
		return "", false
	}
	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return "", false
	}
	return summary, true
}
