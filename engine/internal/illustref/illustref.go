// Package illustref is how a scene document refers to an illustration: the
// editor's "illustration" node, which holds only the picture's id and a
// caption, and the one-line placeholder that stands for that node wherever a
// scene travels as plain text.
//
// The placeholder is what keeps a picture in place when an agent rewrites a
// scene. linetta_read_scene renders the node as the line, linetta_write_scene
// turns the line back into the node; an agent that keeps the line keeps the
// picture, one that drops it takes the picture out of the scene (the gallery
// still has it).
package illustref

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// NodeType is the editor node's name.
const NodeType = "illustration"

// Index answers which illustrations a work's gallery holds, so a write can
// tell a picture of this work from an id an agent made up.
type Index interface {
	IDsInProject(ctx context.Context, projectID string) (map[string]bool, error)
}

// KeepProjects applies KeepKnown against idx for projectID. With no index it
// returns the document as it is.
func KeepProjects(ctx context.Context, idx Index, projectID, rawDoc string) (string, error) {
	if idx == nil || !strings.Contains(rawDoc, `"`+NodeType+`"`) {
		return rawDoc, nil
	}
	known, err := idx.IDsInProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	return KeepKnown(rawDoc, func(id string) bool { return known[id] })
}

// placeholderRE matches one whole placeholder line. Ids are the gallery's
// UUIDs; anything else is left as text.
var placeholderRE = regexp.MustCompile(`^\[\[illustration:([0-9A-Za-z-]{1,64})(?:\|([^\n]*))?\]\]$`)

// Placeholder is the line that stands for an illustration in plain text.
// The caption loses what would end the line or the placeholder early.
func Placeholder(id, caption string) string {
	caption = strings.Join(strings.Fields(caption), " ")
	caption = strings.ReplaceAll(caption, "]]", "] ]")
	if caption == "" {
		return "[[illustration:" + id + "]]"
	}
	return "[[illustration:" + id + "|" + caption + "]]"
}

// Parse reports whether line, trimmed, is exactly one placeholder.
func Parse(line string) (id, caption string, ok bool) {
	m := placeholderRE.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", "", false
	}
	return m[1], strings.TrimSpace(m[2]), true
}

// Node is the document node for an illustration.
func Node(id, caption string) map[string]any {
	return map[string]any{"type": NodeType, "attrs": map[string]any{"id": id, "caption": caption}}
}

// Attrs returns the id and caption of n when n is an illustration node.
func Attrs(n map[string]any) (id, caption string, ok bool) {
	if t, _ := n["type"].(string); t != NodeType {
		return "", "", false
	}
	attrs, _ := n["attrs"].(map[string]any)
	id, _ = attrs["id"].(string)
	caption, _ = attrs["caption"].(string)
	return id, caption, id != ""
}

// IDs lists the illustration ids a stored document refers to, in document
// order, each once.
func IDs(rawDoc string) ([]string, error) {
	if !strings.Contains(rawDoc, `"`+NodeType+`"`) {
		return nil, nil
	}
	var doc any
	if err := json.Unmarshal([]byte(rawDoc), &doc); err != nil {
		return nil, fmt.Errorf("illustref: parse doc: %w", err)
	}
	seen := map[string]bool{}
	var out []string
	walk(doc, func(n map[string]any) {
		if id, _, ok := Attrs(n); ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	})
	return out, nil
}

// RewriteIDs maps every illustration id in a stored document through ids and
// reports whether anything changed. An id the map does not hold stays as it
// is.
func RewriteIDs(rawDoc string, ids map[string]string) (string, bool, error) {
	if !strings.Contains(rawDoc, `"`+NodeType+`"`) {
		return rawDoc, false, nil
	}
	var doc any
	if err := json.Unmarshal([]byte(rawDoc), &doc); err != nil {
		return "", false, fmt.Errorf("illustref: parse doc: %w", err)
	}
	changed := false
	walk(doc, func(n map[string]any) {
		id, _, ok := Attrs(n)
		if !ok {
			return
		}
		if next, found := ids[id]; found && next != id {
			n["attrs"].(map[string]any)["id"] = next
			changed = true
		}
	})
	if !changed {
		return rawDoc, false, nil
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return "", false, err
	}
	return string(out), true, nil
}

// KeepKnown turns every illustration node whose id known rejects back into a
// paragraph holding its placeholder line, so a made-up or foreign id shows up
// as the text the agent wrote rather than as a picture of something else.
func KeepKnown(rawDoc string, known func(id string) bool) (string, error) {
	if !strings.Contains(rawDoc, `"`+NodeType+`"`) {
		return rawDoc, nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(rawDoc), &doc); err != nil {
		return "", fmt.Errorf("illustref: parse doc: %w", err)
	}
	changed := false
	content, _ := doc["content"].([]any)
	for i, c := range content {
		n, _ := c.(map[string]any)
		id, caption, ok := Attrs(n)
		if !ok || known(id) {
			continue
		}
		content[i] = map[string]any{
			"type":    "paragraph",
			"content": []any{map[string]any{"type": "text", "text": Placeholder(id, caption)}},
		}
		changed = true
	}
	if !changed {
		return rawDoc, nil
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func walk(v any, visit func(map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		visit(t)
		if content, ok := t["content"].([]any); ok {
			for _, c := range content {
				walk(c, visit)
			}
		}
	case []any:
		for _, c := range t {
			walk(c, visit)
		}
	}
}
