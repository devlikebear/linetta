// Package search provides the app-wide project/node search used by the desktop UI.
package search

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/store"
)

const DefaultLimit = 20

// Result is a single app-wide search hit.
type Result struct {
	ProjectID    string `json:"project_id"`
	ProjectTitle string `json:"project_title"`
	NodeID       string `json:"node_id"`
	NodeLabel    string `json:"node_label"`
	NodeTitle    string `json:"node_title"`
	NodeKind     string `json:"node_kind"`
	Preview      string `json:"preview"`
	UpdatedAt    int64  `json:"updated_at"`
}

// Repo searches projects and nodes in SQLite.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Query searches visible projects by project title, node label/title, and leaf
// Tiptap content. It deliberately stays SQLite-only; FTS can be added later if
// the corpus outgrows LIKE scans.
func (r *Repo) Query(ctx context.Context, q string, limit int) ([]Result, error) {
	query := strings.TrimSpace(q)
	if query == "" {
		return []Result{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = DefaultLimit
	}

	pattern := "%" + escapeLike(strings.ToLower(query)) + "%"
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT p.id, p.title, n.id, n.label, n.title, n.kind, COALESCE(n.content_doc, ''), n.updated_at,
       CASE
         WHEN lower(n.label) LIKE ? ESCAPE '\' THEN 0
         WHEN lower(n.title) LIKE ? ESCAPE '\' THEN 1
         WHEN lower(p.title) LIKE ? ESCAPE '\' THEN 2
         ELSE 3
       END AS rank
  FROM nodes n
  JOIN projects p ON p.id = n.project_id
 WHERE p.archived_at IS NULL
   AND (
        lower(p.title) LIKE ? ESCAPE '\'
     OR lower(n.label) LIKE ? ESCAPE '\'
     OR lower(n.title) LIKE ? ESCAPE '\'
     OR lower(COALESCE(n.content_doc, '')) LIKE ? ESCAPE '\'
   )
 ORDER BY rank ASC, n.updated_at DESC, n.ordinal ASC
 LIMIT ?`,
		pattern, pattern, pattern,
		pattern, pattern, pattern, pattern,
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Result{}
	for rows.Next() {
		var (
			result     Result
			contentDoc string
			rank       int
		)
		if err := rows.Scan(
			&result.ProjectID,
			&result.ProjectTitle,
			&result.NodeID,
			&result.NodeLabel,
			&result.NodeTitle,
			&result.NodeKind,
			&contentDoc,
			&result.UpdatedAt,
			&rank,
		); err != nil {
			return nil, err
		}
		result.Preview = preview(result, contentDoc, query)
		out = append(out, result)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '%', '_':
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func preview(result Result, contentDoc string, query string) string {
	plain := docToPlainText(contentDoc)
	if plain != "" {
		return excerpt(plain, query, 140)
	}
	for _, candidate := range []string{result.NodeTitle, result.NodeLabel, result.ProjectTitle} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	// An untitled, empty scene in an untitled work has nothing to preview. The
	// engine does not know the reader's language, so it says nothing rather
	// than a Korean placeholder (#45); the result still carries its own title.
	return ""
}

func docToPlainText(raw string) string {
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
		if text, ok := x["text"].(string); ok {
			if b.Len() > 0 {
				b.WriteRune(' ')
			}
			b.WriteString(text)
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

func excerpt(text string, query string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return strings.TrimSpace(text)
	}

	idx := indexFoldRunes(text, query)
	if idx < 0 {
		return strings.TrimSpace(string(runes[:limit])) + "..."
	}
	queryLen := len([]rune(query))
	start := idx - 40
	if start < 0 {
		start = 0
	}
	end := idx + queryLen + 80
	if end > len(runes) {
		end = len(runes)
	}
	prefix := ""
	if start > 0 {
		prefix = "..."
	}
	suffix := ""
	if end < len(runes) {
		suffix = "..."
	}
	return prefix + strings.TrimSpace(string(runes[start:end])) + suffix
}

func indexFoldRunes(text string, query string) int {
	haystack := []rune(strings.ToLower(text))
	needle := []rune(strings.ToLower(strings.TrimSpace(query)))
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		ok := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}
