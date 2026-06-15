package manuscript

import (
	"context"
	"database/sql"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/node"
)

const DefaultLimit = 5
const MaxLimit = 20

type Hit struct {
	NodeID     string `json:"node_id"`
	Breadcrumb string `json:"breadcrumb"`
	Snippet    string `json:"snippet"`
	UpdatedAt  int64  `json:"updated_at,omitempty"`
}

type Searcher struct {
	db      *sql.DB
	nodes   *node.Repo
	indexer *Indexer
}

func NewSearcher(db *sql.DB, nodes *node.Repo, indexer *Indexer) *Searcher {
	return &Searcher{db: db, nodes: nodes, indexer: indexer}
}

func (s *Searcher) Query(ctx context.Context, projectID, q string, limit int) ([]Hit, error) {
	if s == nil || s.db == nil {
		return []Hit{}, nil
	}
	query := strings.TrimSpace(q)
	if query == "" {
		return []Hit{}, nil
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	if err := s.rebuildIfEmpty(ctx, projectID); err != nil {
		return nil, err
	}

	match, ok := buildTrigramMatch(query)
	if !ok {
		return s.queryLike(ctx, projectID, query, limit)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT node_id,
       snippet(manuscript_fts, 0, '...', '...', '...', 12),
       bm25(manuscript_fts) AS rank
  FROM manuscript_fts
 WHERE manuscript_fts MATCH ? AND project_id = ?
 ORDER BY rank
 LIMIT ?`, match, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanHits(ctx, projectID, rows)
}

func (s *Searcher) rebuildIfEmpty(ctx context.Context, projectID string) error {
	if s.indexer == nil {
		return nil
	}
	count, err := s.indexer.ProjectRowCount(ctx, projectID)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return s.indexer.Rebuild(ctx, projectID)
}

func (s *Searcher) queryLike(ctx context.Context, projectID, q string, limit int) ([]Hit, error) {
	pattern := "%" + escapeLike(q) + "%"
	rows, err := s.db.QueryContext(ctx, `
SELECT node_id, plain, 0.0 AS rank
  FROM manuscript_fts
 WHERE project_id = ? AND plain LIKE ? ESCAPE '\'
 ORDER BY rowid
 LIMIT ?`, projectID, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanHits(ctx, projectID, rows)
}

type hitScanner interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func (s *Searcher) scanHits(ctx context.Context, projectID string, rows hitScanner) ([]Hit, error) {
	var rank float64
	hits := []Hit{}
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.NodeID, &h.Snippet, &rank); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.addBreadcrumbs(ctx, projectID, hits)
	return hits, nil
}

func (s *Searcher) addBreadcrumbs(ctx context.Context, projectID string, hits []Hit) {
	if s.nodes == nil || len(hits) == 0 {
		return
	}
	all, err := s.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return
	}
	byID := map[string]node.Node{}
	for _, n := range all {
		byID[n.ID] = n
	}
	for i := range hits {
		if n, ok := byID[hits[i].NodeID]; ok {
			hits[i].Breadcrumb = node.BreadcrumbLabel(byID, n)
			hits[i].UpdatedAt = n.UpdatedAt
		}
	}
}

func buildTrigramMatch(q string) (string, bool) {
	terms := strings.Fields(q)
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		term = sanitizeFTSTerm(term)
		if utf8.RuneCountInString(term) < 3 {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	if len(quoted) == 0 {
		return "", false
	}
	return strings.Join(quoted, " AND "), true
}

func sanitizeFTSTerm(term string) string {
	term = strings.TrimSpace(term)
	term = strings.Trim(term, `"'()[]{}:,^*`)
	switch strings.ToUpper(term) {
	case "AND", "OR", "NOT":
		return ""
	default:
		return term
	}
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
