package ai

import (
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/tars/pkg/llm"
)

// langIsEnglish reports whether the app UI language selects English prompts.
func langIsEnglish(lang string) bool {
	return strings.HasPrefix(lang, "en")
}

// langPick returns en when lang is English, otherwise ko (the default).
func langPick(lang, ko, en string) string {
	if langIsEnglish(lang) {
		return en
	}
	return ko
}

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
	c = ApplyContextSelection(c)
	system := buildSystem(c)
	user := buildUser(c)
	return []llm.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

func buildSystem(c Context) string {
	lang := c.Options.Language
	if langIsEnglish(lang) {
		var b strings.Builder
		b.WriteString("You are an inline editor for a fiction writer. ")
		b.WriteString("Carry out the writer's request so it fits the flow of the manuscript. ")
		b.WriteString("Respond in English. Output pure prose only, without markdown headers.\n\n")
		switch c.Options.Tone {
		case TonePresetMy:
			if strings.TrimSpace(c.StyleNotes) != "" {
				b.WriteString("The writer's style notes (must follow):\n")
				b.WriteString(c.StyleNotes)
				b.WriteString("\n\n")
			}
		case TonePresetCool:
			b.WriteString("Keep this output in a cold, detached tone.\n\n")
		case TonePresetSensory:
			b.WriteString("Keep this output in a sensory tone, rich in sight, sound, and touch.\n\n")
		case TonePresetDry:
			b.WriteString("Keep this output dry and factual, trimming adjectives.\n\n")
		case TonePresetTense:
			b.WriteString("Keep this output tense, using short, clipped sentences.\n\n")
		case TonePresetLyrical:
			b.WriteString("Keep this output lyrical, with a living cadence.\n\n")
		case TonePresetHumor:
			b.WriteString("Keep this output light and witty.\n\n")
		}
		if c.Options.ShortForm {
			b.WriteString("Keep the output short: one paragraph, at most 500 characters.\n")
		}
		return b.String()
	}
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
		b.WriteString("출력은 한 문단 이내, 500자 이내로 짧게 작성하세요.\n")
	}
	return b.String()
}

func buildUser(c Context) string {
	var b strings.Builder
	lang := c.Options.Language

	capPlotDescriptions(&c.Plot, plotMaxChars)

	// Plan 18 project meta: a one-line summary of the user-configured genres,
	// length target, and default POV. Sits above everything else so the model
	// frames the entire context within the writer's stated intent.
	if meta := renderProjectMeta(c.Project, lang); meta != "" {
		b.WriteString(langPick(lang, "## 작품 설정\n", "## Project Settings\n"))
		b.WriteString(meta)
		b.WriteString("\n\n")
	}

	overview := strings.TrimSpace(c.Outline)
	if overview != "" {
		b.WriteString(langPick(lang, "## 작품 개요\n", "## Work Overview\n"))
		b.WriteString(overview)
		b.WriteString("\n\n")
	}

	synopsis := strings.TrimSpace(c.Project.Synopsis)
	if synopsis != "" {
		b.WriteString(langPick(lang, "## 작품 시놉시스\n", "## Synopsis\n"))
		b.WriteString(synopsis)
		b.WriteString("\n\n")
	}

	if len(c.Hierarchical.NearbyLeafSummaries) > 0 {
		b.WriteString(langPick(lang, "## 직전·직후 씬 발췌\n", "## Adjacent Scene Excerpts\n"))
		for _, ss := range c.Hierarchical.NearbyLeafSummaries {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", ss.Label, ss.Body))
		}
		b.WriteString("\n")
	}

	if len(c.RelatedScenes) > 0 {
		b.WriteString(langPick(lang, "## 관련 과거 씬\n", "## Related Past Scenes\n"))
		for _, ss := range c.RelatedScenes {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", ss.Label, ss.Body))
		}
		b.WriteString("\n")
	}

	if strings.TrimSpace(c.SceneText) != "" {
		b.WriteString(fmt.Sprintf(langPick(lang, "## 현재 씬: %s\n", "## Current Scene: %s\n"), c.SceneLabel))
		b.WriteString(c.SceneText)
		b.WriteString("\n\n")
	}

	if strings.TrimSpace(c.SelectionText) != "" {
		b.WriteString(langPick(lang, "## 선택 영역\n", "## Selected Text\n"))
		b.WriteString(c.SelectionText)
		b.WriteString("\n\n")
	}

	if len(c.Entities) > 0 {
		b.WriteString(langPick(lang, "## 세계관 요소\n", "## World Elements\n"))
		for _, e := range c.Entities {
			b.WriteString(fmt.Sprintf("- @%s — %s", e.Name, kindLabel(e.Kind, lang)))
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
	if hasPlot(c.Plot) {
		b.WriteString(langPick(lang, "## 플롯\n", "## Plot\n"))
		writeScene := func(tag string, s *plot.SceneBeats) {
			if s == nil || len(s.Beats) == 0 {
				return
			}
			b.WriteString(tag)
			b.WriteString("\n")
			for _, bt := range s.Beats {
				line := fmt.Sprintf("  · [%s] #%d %s", bt.ThreadName, bt.Ordinal, bt.Label)
				if strings.TrimSpace(bt.Description) != "" {
					line += " — " + bt.Description
				}
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		writeScene(langPick(lang, "[이전 씬]", "[prev scene]"), c.Plot.Prev)
		writeScene(langPick(lang, "[현재 씬]", "[current scene]"), &c.Plot.Current)
		writeScene(langPick(lang, "[다음 씬]", "[next scene]"), c.Plot.Next)
		b.WriteString("\n")
	}
	if len(c.Relationships) > 0 {
		b.WriteString(langPick(lang, "## 관계\n", "## Relationships\n"))
		for _, r := range c.Relationships {
			arrow := "→"
			if r.Bidirectional {
				arrow = "↔"
			}
			line := fmt.Sprintf("- %s %s %s: %s", r.From, arrow, r.To, r.Label)
			if strings.TrimSpace(r.Notes) != "" {
				line += " — " + r.Notes
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(c.Notes) > 0 {
		b.WriteString(langPick(lang, "## 작가 주석\n", "## Author Notes\n"))
		for _, n := range c.Notes {
			b.WriteString("- ")
			b.WriteString(n.Body)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if c.Options.Tone != TonePresetMy && strings.TrimSpace(c.StyleNotes) != "" {
		b.WriteString(langPick(lang, "## 작가 메모\n", "## Author Memo\n"))
		b.WriteString(c.StyleNotes)
		b.WriteString("\n\n")
	}
	b.WriteString(langPick(lang, "## 작가의 지시\n", "## Writer's Instruction\n"))
	b.WriteString(strings.TrimSpace(c.UserPrompt))
	return b.String()
}

// renderProjectMeta returns a one-line "장르: X, Y · 분량: Z · 시점: W"
// with empty pieces omitted. Returns empty string if all three are empty.
// Unmapped LengthTarget / DefaultPOV values pass through as-is.
func renderProjectMeta(m ProjectMeta, lang string) string {
	parts := []string{}
	if len(m.Genres) > 0 {
		parts = append(parts, langPick(lang, "장르: ", "Genres: ")+strings.Join(m.Genres, ", "))
	}
	if m.LengthTarget != "" {
		parts = append(parts, langPick(lang, "분량: ", "Length: ")+mapLengthTarget(m.LengthTarget, lang))
	}
	if m.DefaultPOV != "" {
		parts = append(parts, langPick(lang, "시점: ", "POV: ")+mapDefaultPOV(m.DefaultPOV, lang))
	}
	return strings.Join(parts, " · ")
}

func mapLengthTarget(v, lang string) string {
	if langIsEnglish(lang) {
		switch v {
		case "flash":
			return "flash fiction"
		case "short":
			return "short story"
		case "novella":
			return "novella"
		case "novel":
			return "novel"
		case "series":
			return "series"
		default:
			return v
		}
	}
	switch v {
	case "flash":
		return "플래시"
	case "short":
		return "단편"
	case "novella":
		return "중편"
	case "novel":
		return "장편"
	case "series":
		return "시리즈"
	default:
		return v
	}
}

func mapDefaultPOV(v, lang string) string {
	if langIsEnglish(lang) {
		switch v {
		case "first":
			return "first person"
		case "third_limited":
			return "third person limited"
		case "omniscient":
			return "omniscient"
		default:
			return v
		}
	}
	switch v {
	case "first":
		return "1인칭"
	case "third_limited":
		return "3인칭 제한"
	case "omniscient":
		return "전지적"
	default:
		return v
	}
}

func kindLabel(k, lang string) string {
	if langIsEnglish(lang) {
		return k
	}
	switch k {
	case "character":
		return "인물"
	case "place":
		return "장소"
	case "item":
		return "아이템"
	case "concept":
		return "개념"
	}
	return k
}

const plotMaxChars = 2000

func hasPlot(s plot.Spine) bool {
	if len(s.Current.Beats) > 0 {
		return true
	}
	if s.Prev != nil && len(s.Prev.Beats) > 0 {
		return true
	}
	if s.Next != nil && len(s.Next.Beats) > 0 {
		return true
	}
	return false
}

// capPlotDescriptions zeroes out beat descriptions (keeping labels + thread
// names) once the running size of the plot section exceeds maxChars.
//
// Note: buildUser passes a value copy of the Context (c Context), so mutation
// here is local and does not affect the Context returned by Build.
func capPlotDescriptions(s *plot.Spine, maxChars int) {
	total := 0
	trim := func(sb *plot.SceneBeats) {
		if sb == nil {
			return
		}
		for i := range sb.Beats {
			head := len(sb.Beats[i].ThreadName) + len(sb.Beats[i].Label) + 12
			total += head
			if total+len(sb.Beats[i].Description) > maxChars {
				sb.Beats[i].Description = ""
			} else {
				total += len(sb.Beats[i].Description)
			}
		}
	}
	trim(s.Prev)
	trim(&s.Current)
	trim(s.Next)
}
