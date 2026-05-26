// Package export converts Tiptap JSON documents to markdown.
// Mentions are rendered as plain `@label`. The whole-project export
// (project.go) walks the node tree and produces a single document with
// H1/H2/H3 headings derived from depth.
package export

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// DocToMarkdown parses a Tiptap JSON document and returns its markdown rendering.
// Returns an error only when the input is not valid JSON.
func DocToMarkdown(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return "", fmt.Errorf("export: parse doc: %w", err)
	}
	var sb strings.Builder
	walk(node, &sb, false)
	return collapseBlankLines(sb.String()), nil
}

var multiBlankRE = regexp.MustCompile(`\n{3,}`)

func collapseBlankLines(s string) string {
	return multiBlankRE.ReplaceAllString(s, "\n\n")
}

// walk recurses through the node tree. `inBlockquote` is true while inside a
// blockquote so paragraph children prefix `> ` per line.
func walk(v any, sb *strings.Builder, inBlockquote bool) {
	switch t := v.(type) {
	case []any:
		for _, c := range t {
			walk(c, sb, inBlockquote)
		}
	case map[string]any:
		kind, _ := t["type"].(string)
		switch kind {
		case "doc":
			if content, ok := t["content"].([]any); ok {
				walk(content, sb, inBlockquote)
			}
		case "paragraph":
			var inner strings.Builder
			if content, ok := t["content"].([]any); ok {
				walk(content, &inner, false)
			}
			body := inner.String()
			if inBlockquote {
				body = prefixLines(body, "> ")
			}
			sb.WriteString(body)
			sb.WriteString("\n\n")
		case "heading":
			level := 1
			if attrs, ok := t["attrs"].(map[string]any); ok {
				if l, ok := attrs["level"].(float64); ok {
					level = int(l)
				}
			}
			if level < 1 {
				level = 1
			}
			if level > 6 {
				level = 6
			}
			sb.WriteString(strings.Repeat("#", level))
			sb.WriteString(" ")
			if content, ok := t["content"].([]any); ok {
				walk(content, sb, false)
			}
			sb.WriteString("\n\n")
		case "blockquote":
			if content, ok := t["content"].([]any); ok {
				walk(content, sb, true)
			}
		case "horizontalRule":
			sb.WriteString("---\n\n")
		case "hardBreak":
			sb.WriteString("  \n")
		case "mention":
			label := ""
			if attrs, ok := t["attrs"].(map[string]any); ok {
				if s, ok := attrs["label"].(string); ok {
					label = s
				}
			}
			if label != "" {
				sb.WriteString("@")
				sb.WriteString(label)
			}
		case "text":
			text, _ := t["text"].(string)
			bold, italic := false, false
			if marks, ok := t["marks"].([]any); ok {
				for _, m := range marks {
					mm, _ := m.(map[string]any)
					switch mm["type"] {
					case "bold":
						bold = true
					case "italic":
						italic = true
					}
				}
			}
			if italic {
				text = "_" + text + "_"
			}
			if bold {
				text = "**" + text + "**"
			}
			sb.WriteString(text)
		default:
			// Unknown node — recurse children only.
			if content, ok := t["content"].([]any); ok {
				walk(content, sb, inBlockquote)
			}
		}
	}
}

// prefixLines applies `prefix` to every line of `s`. The trailing newline (if any)
// is preserved so the caller can append its own `\n\n` block boundary.
func prefixLines(s, prefix string) string {
	if s == "" {
		return prefix
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
