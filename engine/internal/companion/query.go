package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/node"
)

const queryFence = "linetta-query"
const sceneTextMaxRunes = 1200

type Query struct {
	Tool string            `json:"tool"`
	Args map[string]string `json:"args"`
}

type QueryRequest struct {
	Queries []Query `json:"queries"`
}

// ParseQuery extracts a linetta-query block. (zero,false,nil)=no block;
// (parsed,true,nil)=ok; (zero,true,err)=malformed.
func ParseQuery(full string) (QueryRequest, bool, error) {
	blocks := extractFencedBlocks(full, queryFence)
	if len(blocks) == 0 {
		return QueryRequest{}, false, nil
	}
	var qr QueryRequest
	if err := json.Unmarshal([]byte(strings.TrimSpace(blocks[0])), &qr); err != nil {
		return QueryRequest{}, true, fmt.Errorf("invalid query JSON: %w", err)
	}
	if len(qr.Queries) == 0 {
		return QueryRequest{}, true, fmt.Errorf("query block has no queries")
	}
	return qr, true, nil
}

// runQueries executes each read tool, returning a human-readable result block.
func (s *Service) runQueries(ctx context.Context, projectID string, qs []Query) string {
	var b strings.Builder
	b.WriteString("## 조회 결과\n")
	for _, q := range qs {
		b.WriteString("### " + q.Tool + "\n")
		b.WriteString(s.runOneQuery(ctx, projectID, q))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func (s *Service) runOneQuery(ctx context.Context, projectID string, q Query) string {
	switch q.Tool {
	case "search_entities":
		ents, err := s.entities.Search(ctx, projectID, q.Args["query"], 20)
		if err != nil {
			return "(오류: " + err.Error() + ")"
		}
		if len(ents) == 0 {
			return "(결과 없음)"
		}
		var sb strings.Builder
		for _, e := range ents {
			sb.WriteString(fmt.Sprintf("- [%s] (%s) %s", e.ID, kindLabel(e.Kind), e.Name))
			if e.Role != "" {
				sb.WriteString(" / " + e.Role)
			}
			if e.Summary != "" {
				sb.WriteString(": " + e.Summary)
			}
			sb.WriteString("\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	case "get_scene_text":
		id := q.Args["node_id"]
		if id == "" {
			return "(오류: node_id 필요)"
		}
		n, err := s.nodes.Get(ctx, id)
		if err != nil {
			return "(오류: " + err.Error() + ")"
		}
		txt := trimRunesLocal(plainTextFromDoc(n.ContentDoc), sceneTextMaxRunes)
		if txt == "" {
			return "(본문 없음)"
		}
		return txt
	case "list_scenes":
		all, err := s.nodes.ListByProject(ctx, projectID)
		if err != nil {
			return "(오류: " + err.Error() + ")"
		}
		byID := map[string]node.Node{}
		for _, n := range all {
			byID[n.ID] = n
		}
		var sb strings.Builder
		for _, n := range all {
			if n.Kind != "leaf" {
				continue
			}
			sb.WriteString("- [" + n.ID + "] " + node.BreadcrumbLabel(byID, n) + "\n")
		}
		if sb.Len() == 0 {
			return "(씬 없음)"
		}
		return strings.TrimRight(sb.String(), "\n")
	case "list_beats":
		var bs []beat.Beat
		var err error
		if nid := q.Args["node_id"]; nid != "" {
			bs, err = s.beats.ListByNode(ctx, nid)
		} else if tid := q.Args["thread_id"]; tid != "" {
			bs, err = s.beats.ListByThread(ctx, tid)
		} else {
			return "(오류: node_id 또는 thread_id 필요)"
		}
		if err != nil {
			return "(오류: " + err.Error() + ")"
		}
		if len(bs) == 0 {
			return "(비트 없음)"
		}
		var sb strings.Builder
		for _, bt := range bs {
			sb.WriteString(fmt.Sprintf("- [%s] #%d %s", bt.ID, bt.Ordinal, bt.Label))
			if bt.Description != "" {
				sb.WriteString(" — " + bt.Description)
			}
			sb.WriteString("\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	case "recall_memory":
		hits := s.Recall(projectID, q.Args["query"], recallLimit)
		if len(hits) == 0 {
			return "(기억 없음)"
		}
		return "- " + strings.Join(hits, "\n- ")
	default:
		return "(오류: 알 수 없는 도구 " + q.Tool + ")"
	}
}

func plainTextFromDoc(raw *string) string {
	if raw == nil || *raw == "" {
		return ""
	}
	var v interface{}
	if err := json.Unmarshal([]byte(*raw), &v); err != nil {
		return ""
	}
	var sb strings.Builder
	var walk func(x interface{})
	walk = func(x interface{}) {
		switch t := x.(type) {
		case map[string]interface{}:
			if t["type"] == "mention" {
				if attrs, ok := t["attrs"].(map[string]interface{}); ok {
					if label, ok := attrs["label"].(string); ok {
						sb.WriteString(label)
					}
				}
				return
			}
			if t["type"] == "text" {
				if s, ok := t["text"].(string); ok {
					sb.WriteString(s)
				}
			}
			if c, ok := t["content"].([]interface{}); ok {
				for _, ch := range c {
					walk(ch)
				}
			}
			if k, _ := t["type"].(string); k == "paragraph" || k == "heading" {
				sb.WriteString("\n")
			}
		case []interface{}:
			for _, ch := range t {
				walk(ch)
			}
		}
	}
	walk(v)
	return strings.TrimSpace(sb.String())
}

func trimRunesLocal(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
