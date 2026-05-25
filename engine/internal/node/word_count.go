// Package node owns Node domain logic for the Tiptap-content recursive tree.
package node

import (
	"encoding/json"
	"unicode/utf8"
)

// CountChars walks a Tiptap doc (raw JSON) and returns the total visible
// character count including spaces — what Korean writing UIs label as "자".
// Returns 0 for any malformed or empty input.
func CountChars(rawDoc []byte) int {
	if len(rawDoc) == 0 {
		return 0
	}
	var any interface{}
	if err := json.Unmarshal(rawDoc, &any); err != nil {
		return 0
	}
	return walk(any)
}

func walk(v interface{}) int {
	switch t := v.(type) {
	case map[string]interface{}:
		// A text node has {"type":"text","text":"..."} — count utf8 chars in text.
		if kind, _ := t["type"].(string); kind == "text" {
			if s, ok := t["text"].(string); ok {
				return utf8.RuneCountInString(s)
			}
			return 0
		}
		// Otherwise, recurse into "content" if present.
		if content, ok := t["content"].([]interface{}); ok {
			n := 0
			for _, c := range content {
				n += walk(c)
			}
			return n
		}
		return 0
	case []interface{}:
		n := 0
		for _, c := range t {
			n += walk(c)
		}
		return n
	}
	return 0
}
