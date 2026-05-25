package mention

import "encoding/json"

// Collect walks a Tiptap doc (raw JSON) and returns every mention atom in DFS
// order, skipping malformed entries. Returns nil-equivalent empty slice on bad
// input.
func Collect(rawDoc []byte) []Found {
	if len(rawDoc) == 0 {
		return nil
	}
	var any interface{}
	if err := json.Unmarshal(rawDoc, &any); err != nil {
		return nil
	}
	var out []Found
	pos := 0
	walk(any, &pos, &out)
	return out
}

func walk(v interface{}, pos *int, out *[]Found) {
	switch t := v.(type) {
	case map[string]interface{}:
		kind, _ := t["type"].(string)
		if kind == "mention" {
			attrs, _ := t["attrs"].(map[string]interface{})
			id, _ := attrs["id"].(string)
			label, _ := attrs["label"].(string)
			if id != "" && label != "" {
				*out = append(*out, Found{EntityID: id, Position: *pos, Surface: label})
			}
			*pos++ // mention atoms count as one position
			return
		}
		if kind == "text" {
			if s, ok := t["text"].(string); ok {
				*pos += len([]rune(s))
			}
			return
		}
		if content, ok := t["content"].([]interface{}); ok {
			for _, c := range content {
				walk(c, pos, out)
			}
		}
	case []interface{}:
		for _, c := range t {
			walk(c, pos, out)
		}
	}
}
