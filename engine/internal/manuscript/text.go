package manuscript

import (
	"encoding/json"
	"strings"
)

func docToPlainText(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return ""
	}
	var b strings.Builder
	collectText(decoded, &b)
	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), " "))
}

func collectText(v any, b *strings.Builder) {
	switch x := v.(type) {
	case map[string]any:
		if x["type"] == "mention" {
			if attrs, ok := x["attrs"].(map[string]any); ok {
				if label, ok := attrs["label"].(string); ok {
					writeToken(b, label)
				}
			}
			return
		}
		if text, ok := x["text"].(string); ok {
			writeToken(b, text)
		}
		if content, ok := x["content"].([]any); ok {
			for _, child := range content {
				collectText(child, b)
			}
		}
	case []any:
		for _, child := range x {
			collectText(child, b)
		}
	}
}

func writeToken(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteRune(' ')
	}
	b.WriteString(text)
}
