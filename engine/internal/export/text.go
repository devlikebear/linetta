package export

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/node"
)

// TextPayload is the wire shape for platform-editor plain-text copy.
type TextPayload struct {
	Text      string `json:"text"`
	CharCount int    `json:"char_count"`
}

// DocToPlainText renders a Tiptap JSON document as plain manuscript text.
// Paragraphs are separated by one blank line; marks and markdown syntax are
// discarded, and mentions render as their visible label.
func DocToPlainText(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", fmt.Errorf("export text: parse doc: %w", err)
	}
	blocks := plainBlocks(root)
	return strings.Trim(strings.Join(blocks, "\n\n"), "\n\r"), nil
}

// ExportNodeText renders one leaf or all leaf descendants of a container.
// Descendant scenes are joined by two blank lines for platform pasting.
func ExportNodeText(ctx context.Context, nr *node.Repo, nodeID string) (TextPayload, error) {
	root, err := nr.Get(ctx, nodeID)
	if err != nil {
		return TextPayload{}, err
	}

	leaves := []node.Node{}
	if root.Kind == node.KindLeaf {
		leaves = append(leaves, root)
	} else {
		flat, err := nr.ListByProject(ctx, root.ProjectID)
		if err != nil {
			return TextPayload{}, err
		}
		children := map[string][]node.Node{}
		for _, n := range flat {
			if n.ParentID == nil {
				continue
			}
			children[*n.ParentID] = append(children[*n.ParentID], n)
		}
		var walk func(parentID string)
		walk = func(parentID string) {
			for _, child := range children[parentID] {
				if child.Kind == node.KindLeaf {
					leaves = append(leaves, child)
					continue
				}
				walk(child.ID)
			}
		}
		walk(root.ID)
	}

	parts := make([]string, 0, len(leaves))
	for _, leaf := range leaves {
		if leaf.ContentDoc == nil {
			continue
		}
		text, err := DocToPlainText([]byte(*leaf.ContentDoc))
		if err != nil {
			return TextPayload{}, fmt.Errorf("export text node %s: %w", leaf.ID, err)
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	text := strings.Join(parts, "\n\n\n")
	return TextPayload{Text: text, CharCount: countCopiedChars(text)}, nil
}

func plainBlocks(v any) []string {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, child := range t {
			out = append(out, plainBlocks(child)...)
		}
		return out
	case map[string]any:
		kind, _ := t["type"].(string)
		switch kind {
		case "doc", "blockquote":
			if content, ok := t["content"].([]any); ok {
				return plainBlocks(content)
			}
		case "paragraph", "heading":
			if content, ok := t["content"].([]any); ok {
				return []string{plainInline(content)}
			}
			return []string{""}
		case "horizontalRule":
			return nil
		default:
			if content, ok := t["content"].([]any); ok {
				return plainBlocks(content)
			}
		}
	}
	return nil
}

func plainInline(v any) string {
	switch t := v.(type) {
	case []any:
		var sb strings.Builder
		for _, child := range t {
			sb.WriteString(plainInline(child))
		}
		return sb.String()
	case map[string]any:
		kind, _ := t["type"].(string)
		switch kind {
		case "text":
			text, _ := t["text"].(string)
			return text
		case "mention":
			if attrs, ok := t["attrs"].(map[string]any); ok {
				if label, ok := attrs["label"].(string); ok {
					return label
				}
			}
		case "hardBreak":
			return "\n"
		default:
			if content, ok := t["content"].([]any); ok {
				return plainInline(content)
			}
		}
	}
	return ""
}

func countCopiedChars(text string) int {
	return utf8.RuneCountInString(strings.NewReplacer("\n", "", "\r", "").Replace(text))
}
