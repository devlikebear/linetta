// Package search provides the app-wide project/node search used by the desktop UI.
package search

import (
	"context"
	"database/sql"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/ftsquery"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
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

// Query searches visible projects: names first (a label, then a title, then a
// work's title), then the prose. The prose comes from manuscript_fts rather
// than the stored document, because the document is JSON and its key names
// are not the writer's words: "paragraph" would match every scene there is.
func (r *Repo) Query(ctx context.Context, q string, limit int) ([]Result, error) {
	query := strings.TrimSpace(q)
	if query == "" {
		return []Result{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = DefaultLimit
	}
	out, err := r.byName(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	if len(out) >= limit {
		return out, nil
	}
	seen := make(map[string]bool, len(out))
	for _, result := range out {
		seen[result.NodeID] = true
	}
	// Enough to still fill the page when every name match is also a prose match.
	prose, err := r.byProse(ctx, query, limit+len(out))
	if err != nil {
		return nil, err
	}
	for _, result := range prose {
		if len(out) == limit {
			break
		}
		if !seen[result.NodeID] {
			out = append(out, result)
		}
	}
	return out, nil
}

func (r *Repo) byName(ctx context.Context, query string, limit int) ([]Result, error) {
	pattern := "%" + ftsquery.EscapeLike(strings.ToLower(query)) + "%"
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT p.id, p.title, n.id, n.label, n.title, n.kind, COALESCE(n.content_doc, ''), n.updated_at,
       CASE
         WHEN lower(n.label) LIKE ? ESCAPE '\' THEN 0
         WHEN lower(n.title) LIKE ? ESCAPE '\' THEN 1
         ELSE 2
       END AS rank
  FROM nodes n
  JOIN projects p ON p.id = n.project_id
 WHERE p.archived_at IS NULL
   AND (
        lower(p.title) LIKE ? ESCAPE '\'
     OR lower(n.label) LIKE ? ESCAPE '\'
     OR lower(n.title) LIKE ? ESCAPE '\'
   )
 ORDER BY rank ASC, n.updated_at DESC, n.ordinal ASC
 LIMIT ?`,
		pattern, pattern,
		pattern, pattern, pattern,
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
			&result.ProjectID, &result.ProjectTitle, &result.NodeID, &result.NodeLabel,
			&result.NodeTitle, &result.NodeKind, &contentDoc, &result.UpdatedAt, &rank,
		); err != nil {
			return nil, err
		}
		result.Preview = preview(result, manuscript.DocToPlainText(contentDoc), []string{query})
		out = append(out, result)
	}
	return out, rows.Err()
}

// byProse finds the scenes holding every word of query. A word too short for
// the trigram index goes through LIKE over the same plain text, the way
// linetta_search_manuscript does it.
func (r *Repo) byProse(ctx context.Context, query string, limit int) ([]Result, error) {
	const columns = `
SELECT p.id, p.title, n.id, n.label, n.title, n.kind, f.plain, n.updated_at
  FROM manuscript_fts f
  JOIN nodes n ON n.id = f.node_id
  JOIN projects p ON p.id = n.project_id
 WHERE p.archived_at IS NULL AND `
	terms := ftsquery.Terms(query)
	var (
		rows *sql.Rows
		err  error
	)
	if match, ok := ftsquery.TrigramMatch(terms); ok {
		rows, err = r.s.DB().QueryContext(ctx, columns+`f.manuscript_fts MATCH ?
 ORDER BY bm25(manuscript_fts)
 LIMIT ?`, match, limit)
	} else {
		likes := make([]string, 0, len(terms))
		args := make([]any, 0, len(terms)+1)
		for _, term := range terms {
			likes = append(likes, `f.plain LIKE ? ESCAPE '\'`)
			args = append(args, "%"+ftsquery.EscapeLike(term)+"%")
		}
		rows, err = r.s.DB().QueryContext(ctx, columns+strings.Join(likes, " AND ")+`
 ORDER BY n.updated_at DESC, n.ordinal ASC
 LIMIT ?`, append(args, limit)...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	needles := append([]string{query}, terms...)
	out := []Result{}
	for rows.Next() {
		var (
			result Result
			plain  string
		)
		if err := rows.Scan(
			&result.ProjectID, &result.ProjectTitle, &result.NodeID, &result.NodeLabel,
			&result.NodeTitle, &result.NodeKind, &plain, &result.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result.Preview = preview(result, strings.Join(strings.Fields(plain), " "), needles)
		out = append(out, result)
	}
	return out, rows.Err()
}

func preview(result Result, plain string, needles []string) string {
	if plain != "" {
		return excerpt(plain, needles, 140)
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

// excerpt cuts text to the first needle found, in order, or to its start when
// none is there: a scene found through two separate words may hold neither
// the phrase nor, under case folding, either spelling.
func excerpt(text string, needles []string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return strings.TrimSpace(text)
	}

	idx, width := -1, 0
	for _, needle := range needles {
		if idx = indexFoldRunes(text, needle); idx >= 0 {
			width = len([]rune(needle))
			break
		}
	}
	if idx < 0 {
		return strings.TrimSpace(string(runes[:limit])) + "..."
	}
	start := idx - 40
	if start < 0 {
		start = 0
	}
	end := idx + width + 80
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
