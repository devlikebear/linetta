package manuscript

import (
	"encoding/json"
	"strings"
)

// DocToPlainText renders a Tiptap document as the prose the writer sees. Inline
// nodes run together, so a particle stays on the mention before it and a word
// split by a mark stays whole; only a block boundary or a line break is a space.
func DocToPlainText(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return ""
	}
	var b strings.Builder
	collectText(decoded, &b)
	return strings.Join(strings.Fields(b.String()), " ")
}

func collectText(v any, b *strings.Builder) {
	switch x := v.(type) {
	case map[string]any:
		switch x["type"] {
		case "mention":
			if attrs, ok := x["attrs"].(map[string]any); ok {
				if label, ok := attrs["label"].(string); ok {
					b.WriteString(label)
				}
			}
			return
		case "hardBreak":
			b.WriteRune(' ')
			return
		}
		if text, ok := x["text"].(string); ok {
			b.WriteString(text)
		}
		if content, ok := x["content"].([]any); ok {
			for _, child := range content {
				collectText(child, b)
			}
			b.WriteRune(' ')
		}
	case []any:
		for _, child := range x {
			collectText(child, b)
		}
	}
}
