package manuscript

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/ftsquery"
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
	// ParagraphFrom and ParagraphTo are where in the scene the match is,
	// 1-based and inclusive, numbered as linetta_read_scene numbered=true
	// prints them. Both are 0 when the match could not be placed.
	ParagraphFrom int `json:"paragraph_from,omitempty"`
	ParagraphTo   int `json:"paragraph_to,omitempty"`
}

type Searcher struct {
	db       *sql.DB
	nodes    *node.Repo
	indexer  *Indexer
	semantic Semantic
}

// SetSemantic adds meaning search (#384) to Query once the writer has it on.
func (s *Searcher) SetSemantic(sem Semantic) { s.semantic = sem }

// How Query finds scenes: by both (auto), by the words alone, or by meaning
// alone. Meaning falls back to words while it cannot answer.
const (
	ModeAuto    = "auto"
	ModeWords   = "words"
	ModeMeaning = "meaning"
)

// semanticPool is how many paragraphs meaning search ranks for one query:
// enough that a scene's best paragraph is in it for every scene that could
// make the list.
const semanticPool = 200

// rrfK damps the reciprocal rank fusion of the two lists: a scene first in
// one list and absent from the other still ranks with scenes both put high.
const rrfK = 60

func NewSearcher(db *sql.DB, nodes *node.Repo, indexer *Indexer) *Searcher {
	return &Searcher{db: db, nodes: nodes, indexer: indexer}
}

// Query is QueryMode in auto mode.
func (s *Searcher) Query(ctx context.Context, projectID, q string, limit int) ([]Hit, error) {
	hits, _, err := s.QueryMode(ctx, projectID, q, limit, ModeAuto)
	return hits, err
}

// QueryMode returns the scenes for q, best first. By words: the scenes that
// contain every word of q; the trigram index cannot see a word under three
// characters, and most Korean names are that short, so a query with one is
// answered by queryLike instead. By meaning: the scenes whose paragraphs are
// closest to q. Auto merges the two rankings (RRF), so a name that matches
// exactly is not pushed down by paragraphs that only mean something alike.
// semantic reports whether meaning search took part.
func (s *Searcher) QueryMode(ctx context.Context, projectID, q string, limit int, mode string) (hits []Hit, semantic bool, err error) {
	if s == nil || s.db == nil {
		return []Hit{}, false, nil
	}
	if strings.TrimSpace(q) == "" {
		return []Hit{}, false, nil
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	var matches []SemanticMatch
	if mode != ModeWords && s.semantic != nil {
		var ok bool
		matches, ok, err = s.semantic.Search(ctx, projectID, strings.TrimSpace(q), semanticPool)
		if err != nil || !ok {
			// Meaning search not ready, or failing: the words still answer.
			matches = nil
		}
	}
	if matches == nil {
		hits, err := s.queryWords(ctx, projectID, q, limit)
		return hits, false, err
	}
	var words []Hit
	if mode != ModeMeaning {
		if words, err = s.queryWords(ctx, projectID, q, MaxLimit); err != nil {
			return nil, false, err
		}
	}
	return s.merge(ctx, projectID, words, matches, limit), true, nil
}

// merge ranks scenes by both lists. A scene the words found keeps its
// snippet and range; one only meaning found shows its best paragraph.
func (s *Searcher) merge(ctx context.Context, projectID string, words []Hit, matches []SemanticMatch, limit int) []Hit {
	score := map[string]float64{}
	best := map[string]SemanticMatch{}
	var order []string
	add := func(id string, rank int) {
		if _, seen := score[id]; !seen {
			order = append(order, id)
		}
		score[id] += 1 / float64(rrfK+rank+1)
	}
	for i, h := range words {
		add(h.NodeID, i)
	}
	rank := 0
	for _, m := range matches {
		if _, seen := best[m.NodeID]; seen {
			continue
		}
		best[m.NodeID] = m
		add(m.NodeID, rank)
		rank++
	}
	sort.SliceStable(order, func(i, j int) bool { return score[order[i]] > score[order[j]] })
	if len(order) > limit {
		order = order[:limit]
	}
	byID := map[string]Hit{}
	for _, h := range words {
		byID[h.NodeID] = h
	}
	out := make([]Hit, 0, len(order))
	var fresh []int
	for _, id := range order {
		h, ok := byID[id]
		m, meant := best[id]
		if !ok {
			h = Hit{NodeID: id, Snippet: clipRunes(m.Text, semanticSnippetRunes)}
			fresh = append(fresh, len(out))
		}
		if h.ParagraphFrom == 0 && meant {
			h.ParagraphFrom, h.ParagraphTo = m.Para, m.Para
		}
		out = append(out, h)
	}
	if len(fresh) > 0 {
		s.addBreadcrumbs(ctx, projectID, out)
	}
	return out
}

// semanticSnippetRunes is how much of a paragraph meaning search shows, about
// what a word hit's snippet holds.
const semanticSnippetRunes = 60

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// queryWords is the word search, best match first.
func (s *Searcher) queryWords(ctx context.Context, projectID, q string, limit int) ([]Hit, error) {
	query := strings.TrimSpace(q)
	if err := s.rebuildIfEmpty(ctx, projectID); err != nil {
		return nil, err
	}

	terms := ftsquery.Terms(query)
	hits, err := s.queryScenes(ctx, projectID, terms, limit)
	if err != nil {
		return nil, err
	}
	s.locate(ctx, terms, hits)
	return hits, nil
}

func (s *Searcher) queryScenes(ctx context.Context, projectID string, terms []string, limit int) ([]Hit, error) {
	match, ok := ftsquery.TrigramMatch(terms)
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
		filterArgs = append(filterArgs, "%"+ftsquery.EscapeLike(term)+"%")
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
		hits[i].Snippet = ftsquery.LikeSnippet(hits[i].Snippet, append([]string{phrase}, terms...), likeSnippetRadius)
	}
	return hits, nil
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
