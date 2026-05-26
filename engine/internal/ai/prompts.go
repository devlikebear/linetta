package ai

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/pkg/llm"
)

// PresetID identifies a built-in prompt template.
type PresetID string

const (
	PresetRewrite PresetID = "rewrite"
	PresetExpand  PresetID = "expand"
	PresetCompact PresetID = "compact"
	PresetFree    PresetID = "free" // verbatim user prompt, no template
)

// PresetSeed returns the user-prompt seed for each preset. The UI calls this
// when the writer clicks a chip so the PROMPT textarea gets prefilled.
func PresetSeed(p PresetID) string {
	switch p {
	case PresetRewrite:
		return "이 문단을 다른 톤으로 다시 써줘."
	case PresetExpand:
		return "이 장면을 더 감각적으로 확장해줘."
	case PresetCompact:
		return "이 씬을 한 문단으로 요약해줘."
	}
	return ""
}

// BuildMessages converts a Context into the two-message system+user pair the
// engine sends to tars. The system message governs tone and length; the user
// message contains the structured context.
//
// Why msg.Content (string) and not msg.ContentBlocks: both claude-code-cli and
// openai-codex providers in tars/pkg/llm read the plain `Content` field; the
// openai-codex provider only puts system messages into the Responses API's
// `instructions` field when `msg.Content` is non-empty, and claude-code-cli's
// system-prompt assembler ignores ContentBlocks entirely. ContentBlocks is for
// multimodal inputs (images, PDFs) which we don't send.
func BuildMessages(c Context) []llm.ChatMessage {
	system := buildSystem(c)
	user := buildUser(c)
	return []llm.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

func buildSystem(c Context) string {
	var b strings.Builder
	b.WriteString("당신은 한국어 소설 작가의 인라인 편집기입니다. ")
	b.WriteString("작가가 요청한 작업을 본문 흐름에 맞게 수행하세요. ")
	b.WriteString("출력은 마크다운 헤더 없이 순수 본문만 작성합니다.\n\n")
	if c.Options.TonePreset && strings.TrimSpace(c.StyleNotes) != "" {
		b.WriteString("작가의 스타일 노트(반드시 따를 것):\n")
		b.WriteString(c.StyleNotes)
		b.WriteString("\n\n")
	}
	if c.Options.ShortForm {
		b.WriteString("출력은 한 문단 이내로 짧게 작성하세요.\n")
	}
	return b.String()
}

func buildUser(c Context) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## 현재 씬: %s\n", c.SceneLabel))
	b.WriteString(c.SceneText)
	b.WriteString("\n\n")
	if strings.TrimSpace(c.PrevSummary) != "" {
		b.WriteString("## 직전 씬 발췌\n")
		b.WriteString(c.PrevSummary)
		b.WriteString("\n\n")
	}
	if len(c.Entities) > 0 {
		b.WriteString("## 등장 인물·장소\n")
		for _, e := range c.Entities {
			b.WriteString(fmt.Sprintf("- @%s — %s", e.Name, kindLabel(e.Kind)))
			if e.Role != "" {
				b.WriteString(" / " + e.Role)
			}
			if e.Summary != "" {
				b.WriteString(": " + e.Summary)
			}
			if len(e.Attributes) > 0 {
				b.WriteString(" (")
				first := true
				for k, v := range e.Attributes {
					if !first {
						b.WriteString(", ")
					}
					first = false
					b.WriteString(k + ":" + v)
				}
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if !c.Options.TonePreset && strings.TrimSpace(c.StyleNotes) != "" {
		b.WriteString("## 작가 메모\n")
		b.WriteString(c.StyleNotes)
		b.WriteString("\n\n")
	}
	b.WriteString("## 작가의 지시\n")
	b.WriteString(strings.TrimSpace(c.UserPrompt))
	return b.String()
}

func kindLabel(k string) string {
	switch k {
	case "character":
		return "인물"
	case "place":
		return "장소"
	case "item":
		return "물건"
	case "concept":
		return "개념"
	}
	return k
}
