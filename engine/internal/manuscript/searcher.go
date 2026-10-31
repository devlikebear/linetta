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

// likeSnippetRadius is how many runes of context a LIKE hit keeps on each side
// of its match, about what the FTS path's snippet() returns.
const likeSnippetRadius = 20

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

// Query returns the scenes that contain every word of q, best match first. The
// trigram index cannot see a word under three characters, and most Korean names
// are that short, so a query with one is answered by queryLike instead.
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

	terms := searchTerms(query)
	match, ok := buildTrigramMatch(terms)
	if !ok {
		return s.queryLike(ctx, projectID, terms, limit)
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

// queryLike scans for every term, puts scenes holding the whole phrase first,
// then orders by how often the terms occur. lower() and LIKE both fold ASCII
// only, so the occurrence count agrees with what LIKE matched.
func (s *Searcher) queryLike(ctx context.Context, projectID string, terms []string, limit int) ([]Hit, error) {
	phrase := strings.Join(terms, " ")
	filters := make([]string, 0, len(terms))
	counts := make([]string, 0, len(terms))
	filterArgs := make([]any, 0, len(terms))
	countArgs := make([]any, 0, 2*len(terms))
	for _, term := range terms {
		filters = append(filters, `plain LIKE ? ESCAPE '\'`)
		filterArgs = append(filterArgs, "%"+escapeLike(term)+"%")
		counts = append(counts, `(length(plain) - length(replace(lower(plain), lower(?), ''))) / length(?)`)
		countArgs = append(countArgs, term, term)
	}
	args := []any{projectID}
	args = append(args, filterArgs...)
	args = append(args, phrase)
	args = append(args, countArgs...)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
SELECT node_id, plain, 0.0 AS rank
  FROM manuscript_fts
 WHERE project_id = ? AND `+strings.Join(filters, " AND ")+`
 ORDER BY instr(lower(plain), lower(?)) > 0 DESC, `+strings.Join(counts, " + ")+` DESC, rowid
 LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits, err := s.scanHits(ctx, projectID, rows)
	if err != nil {
		return nil, err
	}
	for i := range hits {
		hits[i].Snippet = likeSnippet(hits[i].Snippet, append([]string{phrase}, terms...))
	}
	return hits, nil
}

// likeSnippet cuts plain down to the first needle found and the runes around
// it. Needles are tried in order, so a whole phrase wins over its words.
func likeSnippet(plain string, needles []string) string {
	lower := strings.ToLower(plain)
	start, width := 0, 0
	for _, needle := range needles {
		if i := strings.Index(lower, strings.ToLower(needle)); i >= 0 {
			start = utf8.RuneCountInString(lower[:i])
			width = utf8.RuneCountInString(needle)
			break
		}
	}
	runes := []rune(plain)
	from := max(start-likeSnippetRadius, 0)
	to := min(start+width+likeSnippetRadius, len(runes))
	snippet := string(runes[from:to])
	if from > 0 {
		snippet = "..." + snippet
	}
	if to < len(runes) {
		snippet += "..."
	}
	return snippet
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

// searchTerms splits q into words. When nothing survives sanitizing, q itself
// is the one term, so a query of only punctuation still finds itself.
func searchTerms(q string) []string {
	terms := []string{}
	for _, field := range strings.Fields(q) {
		if term := sanitizeFTSTerm(field); term != "" {
			terms = append(terms, term)
		}
	}
	if len(terms) == 0 {
		return []string{q}
	}
	return terms
}

// buildTrigramMatch reports false when any term is too short for the trigram
// index: dropping it would search for less than was asked.
func buildTrigramMatch(terms []string) (string, bool) {
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		if utf8.RuneCountInString(term) < 3 {
			return "", false
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
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
