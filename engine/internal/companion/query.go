package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
func (s *Service) runQueries(ctx context.Context, projectID string, qs []Query, lang string) string {
	var b strings.Builder
	b.WriteString(pickLang(lang, "## 조회 결과\n", "## Query Results\n", "## 照会結果\n"))
	for _, q := range qs {
		b.WriteString("### " + q.Tool + "\n")
		b.WriteString(s.runOneQuery(ctx, projectID, q, lang))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func (s *Service) runOneQuery(ctx context.Context, projectID string, q Query, lang string) string {
	switch q.Tool {
	case "search_entities":
		ents, err := s.entities.Search(ctx, projectID, q.Args["query"], 20)
		if err != nil {
			return pickLang(lang, "(오류: ", "(error: ", "（エラー: ") + err.Error() + ")"
		}
		if len(ents) == 0 {
			return pickLang(lang, "(결과 없음)", "(no results)", "（結果なし）")
		}
		var sb strings.Builder
		for _, e := range ents {
			sb.WriteString(fmt.Sprintf("- [%s] (%s) %s", e.ID, kindLabel(e.Kind, lang), e.Name))
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
			return pickLang(lang, "(오류: node_id 필요)", "(error: node_id required)", "（エラー: node_id が必要）")
		}
		n, err := s.nodes.Get(ctx, id)
		if err != nil {
			return pickLang(lang, "(오류: ", "(error: ", "（エラー: ") + err.Error() + ")"
		}
		txt := trimRunesLocal(plainTextFromDoc(n.ContentDoc), sceneTextMaxRunes)
		if txt == "" {
			return pickLang(lang, "(본문 없음)", "(no scene text)", "（本文なし）")
		}
		return txt
	case "search_manuscript":
		query := strings.TrimSpace(q.Args["query"])
		if query == "" {
			return pickLang(lang, "(오류: query 필요)", "(error: query required)", "（エラー: query が必要）")
		}
		if s.manuscript == nil {
			return pickLang(lang, "(오류: 본문 검색을 사용할 수 없음)", "(error: manuscript search unavailable)", "（エラー: 本文検索が利用不可）")
		}
		hits, err := s.manuscript.Query(ctx, projectID, query, parseQueryLimit(q.Args["limit"], 5, 20))
		if err != nil {
			return pickLang(lang, "(오류: ", "(error: ", "（エラー: ") + err.Error() + ")"
		}
		if len(hits) == 0 {
			return pickLang(lang, "(검색 결과 없음)", "(no search hits)", "（検索結果なし）")
		}
		var sb strings.Builder
		for _, h := range hits {
			label := node.DisplayBreadcrumb(h.Breadcrumb, lang)
			if label == "" {
				label = h.NodeID
			}
			sb.WriteString("- [" + h.NodeID + "] " + label)
			if h.Snippet != "" {
				sb.WriteString(": " + h.Snippet)
			}
			sb.WriteString("\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	case "list_scenes":
		all, err := s.nodes.ListByProject(ctx, projectID)
		if err != nil {
			return pickLang(lang, "(오류: ", "(error: ", "（エラー: ") + err.Error() + ")"
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
			sb.WriteString("- [" + n.ID + "] " + node.DisplayBreadcrumb(node.BreadcrumbLabel(byID, n), lang) + "\n")
		}
		if sb.Len() == 0 {
			return pickLang(lang, "(씬 없음)", "(no scenes)", "（シーンなし）")
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
			return pickLang(lang, "(오류: node_id 또는 thread_id 필요)", "(error: node_id or thread_id required)", "（エラー: node_id または thread_id が必要）")
		}
		if err != nil {
			return pickLang(lang, "(오류: ", "(error: ", "（エラー: ") + err.Error() + ")"
		}
		if len(bs) == 0 {
			return pickLang(lang, "(비트 없음)", "(no beats)", "（ビートなし）")
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
			return pickLang(lang, "(기억 없음)", "(no memories)", "（記憶なし）")
		}
		return "- " + strings.Join(hits, "\n- ")
	default:
		return pickLang(lang, "(오류: 알 수 없는 도구 ", "(error: unknown tool ", "（エラー: 不明なツール ") + q.Tool + ")"
	}
}

func parseQueryLimit(raw string, fallback, max int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return fallback
	}
	if n > max {
		return max
	}
	return n
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
			if t["type"] == "hardBreak" {
				sb.WriteString("\n")
			}
			if c, ok := t["content"].([]interface{}); ok {
				for _, ch := range c {
					walk(ch)
				}
			}
			if k, _ := t["type"].(string); k == "paragraph" || k == "heading" {
				sb.WriteString("\n\n")
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
