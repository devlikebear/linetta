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
	switch c.Options.Tone {
	case TonePresetMy:
		if strings.TrimSpace(c.StyleNotes) != "" {
			b.WriteString("작가의 스타일 노트(반드시 따를 것):\n")
			b.WriteString(c.StyleNotes)
			b.WriteString("\n\n")
		}
	case TonePresetCool:
		b.WriteString("이번 출력은 차갑고 거리감 있는 톤으로 유지하라.\n\n")
	case TonePresetSensory:
		b.WriteString("이번 출력은 시각·청각·촉각 묘사를 적극 활용한 감각적 톤으로 유지하라.\n\n")
	case TonePresetDry:
		b.WriteString("이번 출력은 형용사를 줄이고 사실 위주의 건조한 톤으로 유지하라.\n\n")
	case TonePresetTense:
		b.WriteString("이번 출력은 짧은 문장과 끊김으로 긴장감을 살린 톤으로 유지하라.\n\n")
	case TonePresetLyrical:
		b.WriteString("이번 출력은 율격이 살아있는 서정적인 톤으로 유지하라.\n\n")
	case TonePresetHumor:
		b.WriteString("이번 출력은 가볍고 위트 있는 톤으로 유지하라.\n\n")
	}
	if c.Options.ShortForm {
		b.WriteString("출력은 한 문단 이내로 짧게 작성하세요.\n")
	}
	return b.String()
}

func buildUser(c Context) string {
	var b strings.Builder

	// Plan 16 layer-1 hierarchical sections (project synopsis → nearby parts/
	// chapters → same-chapter peers → directly-adjacent leaves) precede the
	// current scene so the model sees coarse-to-fine context.
	if strings.TrimSpace(c.Hierarchical.ProjectSynopsis) != "" {
		b.WriteString("## 작품 전반\n")
		b.WriteString(c.Hierarchical.ProjectSynopsis)
		b.WriteString("\n\n")
	}

	if len(c.Hierarchical.OtherPartSummaries) > 0 || len(c.Hierarchical.OtherChapterSummaries) > 0 {
		b.WriteString("## 인근 줄거리\n")
		for _, pt := range c.Hierarchical.OtherPartSummaries {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", pt.Label, pt.Body))
		}
		for _, ch := range c.Hierarchical.OtherChapterSummaries {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", ch.Label, ch.Body))
		}
		b.WriteString("\n")
	}

	if len(c.Hierarchical.SameChapterSummaries) > 0 {
		b.WriteString("## 같은 장 다른 씬\n")
		for _, ss := range c.Hierarchical.SameChapterSummaries {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", ss.Label, ss.Body))
		}
		b.WriteString("\n")
	}

	if len(c.Hierarchical.NearbyLeafSummaries) > 0 {
		b.WriteString("## 직전·직후 씬 발췌\n")
		for _, ss := range c.Hierarchical.NearbyLeafSummaries {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", ss.Label, ss.Body))
		}
		b.WriteString("\n")
	}

	if len(c.RelatedScenes) > 0 {
		b.WriteString("## 관련 과거 씬\n")
		for _, ss := range c.RelatedScenes {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", ss.Label, ss.Body))
		}
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("## 현재 씬: %s\n", c.SceneLabel))
	b.WriteString(c.SceneText)
	b.WriteString("\n\n")

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
			// Plan 16 layer-2 entity dossier: indented one-line summaries of the
			// most recent scenes where the entity appeared elsewhere.
			for _, line := range e.Recent {
				b.WriteString("  · ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	if len(c.ActiveThreads) > 0 {
		b.WriteString("## 활성 스토리라인\n")
		for _, t := range c.ActiveThreads {
			line := fmt.Sprintf("- [%s] %s", t.Color, t.Name)
			if t.Summary != "" {
				line += " — " + t.Summary
			}
			b.WriteString(line)
			b.WriteString("\n")
			for _, bt := range t.RecentBeats {
				b.WriteString(fmt.Sprintf("  · #%d %s\n", bt.Ordinal, bt.Label))
			}
		}
		b.WriteString("\n")
	}
	if len(c.Notes) > 0 {
		b.WriteString("## 작가 주석\n")
		for _, n := range c.Notes {
			b.WriteString("- ")
			b.WriteString(n.Body)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if c.Options.Tone != TonePresetMy && strings.TrimSpace(c.StyleNotes) != "" {
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
