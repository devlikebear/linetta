package mention

import (
	"encoding/json"
	"reflect"
	"sort"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/entity"
)

// Auto-linking for agent-written prose (#72).
//
// This is the Go port of the editor's scene-scan matcher
// (apps/desktop/src/lib/editor/autoMention.ts) — keep the two in step, or a
// writer's scan after an agent write would find matches the engine missed
// (or vice versa). The editor applies it only on the writer's click, because
// auto-linking rewrites prose the writer typed; here it runs on
// linetta_write_scene bodies, which are machine-written, snapshotted, and
// version-guarded from start to finish — a different consent regime.

// Candidate is one linkable surface: an entity's name or alias.
type Candidate struct {
	EntityID string
	Name     string // the entity's canonical name, for reporting
	Surface  string // the text to match; also the mention label
}

// Linked reports one entity's link count after AutoLinkDoc.
type Linked struct {
	EntityID string `json:"entity_id"`
	Name     string `json:"name"`
	Count    int    `json:"count"`
}

// BuildCandidates mirrors the editor's rules: names and aliases, trimmed,
// surfaces under two runes skipped (single characters over-match badly in
// prose), longest surface first so "삼도천 나루" wins over an alias "삼도천".
func BuildCandidates(entities []entity.Entity) []Candidate {
	seen := map[string]bool{}
	out := []Candidate{}
	for _, e := range entities {
		surfaces := append([]string{e.Name}, e.Aliases...)
		for _, raw := range surfaces {
			surface := trimSpace(raw)
			if utf8.RuneCountInString(surface) < 2 {
				continue
			}
			key := e.ID + "\n" + surface
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, Candidate{EntityID: e.ID, Name: e.Name, Surface: surface})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Surface) > len(out[j].Surface) })
	return out
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// AutoLinkDoc converts registered names in a Tiptap doc's text nodes into
// mention atoms. Existing mention atoms are left alone, so running it twice
// (or the writer running scene-scan afterwards) finds nothing new. Returns
// the possibly-rewritten doc and what got linked.
func AutoLinkDoc(rawDoc []byte, candidates []Candidate) ([]byte, []Linked, error) {
	if len(rawDoc) == 0 || len(candidates) == 0 {
		return rawDoc, nil, nil
	}
	var root interface{}
	if err := json.Unmarshal(rawDoc, &root); err != nil {
		return nil, nil, err
	}
	counts := map[string]*Linked{}
	converted, applied := convertNode(root, candidates, counts)
	if applied == 0 {
		return rawDoc, nil, nil
	}
	out, err := json.Marshal(converted)
	if err != nil {
		return nil, nil, err
	}
	linked := make([]Linked, 0, len(counts))
	for _, l := range counts {
		linked = append(linked, *l)
	}
	sort.Slice(linked, func(i, j int) bool { return linked[i].Name < linked[j].Name })
	return out, linked, nil
}

func convertNode(v interface{}, candidates []Candidate, counts map[string]*Linked) (interface{}, int) {
	cur, ok := v.(map[string]interface{})
	if !ok {
		return v, 0
	}
	kind, _ := cur["type"].(string)
	if kind == "mention" {
		return cur, 0
	}
	if kind == "text" {
		if text, ok := cur["text"].(string); ok {
			nodes, applied := convertTextNode(cur, text, candidates, counts)
			if applied == 0 {
				return cur, 0
			}
			if len(nodes) == 1 {
				return nodes[0], applied
			}
			return nodes, applied
		}
	}
	rawContent, ok := cur["content"].([]interface{})
	if !ok {
		return cur, 0
	}
	applied := 0
	content := make([]interface{}, 0, len(rawContent))
	for _, child := range rawContent {
		converted, n := convertNode(child, candidates, counts)
		applied += n
		if list, ok := converted.([]interface{}); ok {
			content = append(content, list...)
		} else {
			content = append(content, converted)
		}
	}
	if applied == 0 {
		return cur, 0
	}
	next := map[string]interface{}{}
	for k, val := range cur {
		next[k] = val
	}
	next["content"] = content
	return next, applied
}

func convertTextNode(node map[string]interface{}, text string, candidates []Candidate, counts map[string]*Linked) ([]interface{}, int) {
	nodes := []interface{}{}
	applied := 0
	index := 0

	pushText := func(value string) {
		if value == "" {
			return
		}
		copied := map[string]interface{}{}
		for k, v := range node {
			copied[k] = v
		}
		copied["text"] = value
		nodes = append(nodes, copied)
	}

	for index < len(text) {
		c, end := matchAt(text, index, candidates)
		if c == nil {
			next := nextMatchIndex(text, index+1, candidates)
			pushText(text[index:next])
			index = next
			continue
		}
		nodes = append(nodes, map[string]interface{}{
			"type":  "mention",
			"attrs": map[string]interface{}{"id": c.EntityID, "label": c.Surface},
		})
		applied++
		if l, ok := counts[c.EntityID]; ok {
			l.Count++
		} else {
			counts[c.EntityID] = &Linked{EntityID: c.EntityID, Name: c.Name, Count: 1}
		}
		index = end
	}

	return mergeAdjacentText(nodes), applied
}

// matchAt mirrors the editor: a leading "@" is consumed together with the
// name, so an agent that writes "@호루" leaves no stray at-sign behind.
func matchAt(text string, index int, candidates []Candidate) (*Candidate, int) {
	offset := index
	if text[index] == '@' {
		offset = index + 1
	}
	rest := text[offset:]
	for i := range candidates {
		c := &candidates[i]
		if len(rest) >= len(c.Surface) && rest[:len(c.Surface)] == c.Surface {
			return c, offset + len(c.Surface)
		}
	}
	return nil, 0
}

func nextMatchIndex(text string, from int, candidates []Candidate) int {
	for i := from; i < len(text); i++ {
		if c, _ := matchAt(text, i, candidates); c != nil {
			return i
		}
	}
	return len(text)
}

func mergeAdjacentText(nodes []interface{}) []interface{} {
	out := []interface{}{}
	for _, raw := range nodes {
		node, ok := raw.(map[string]interface{})
		if ok && node["type"] == "text" && len(out) > 0 {
			if prev, ok := out[len(out)-1].(map[string]interface{}); ok && prev["type"] == "text" &&
				reflect.DeepEqual(prev["marks"], node["marks"]) {
				prevText, _ := prev["text"].(string)
				nodeText, _ := node["text"].(string)
				prev["text"] = prevText + nodeText
				continue
			}
		}
		out = append(out, raw)
	}
	return out
}
