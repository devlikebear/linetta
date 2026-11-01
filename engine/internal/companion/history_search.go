package companion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/ftsquery"
)

// HistoryAgentIntent is the intent the built-in agent stamps on its rows. Only
// those are searched: the 1.0 companion's rows are words the writer never said
// to this agent.
const HistoryAgentIntent = "agent"

const (
	defaultHistorySearchLimit = 8
	maxHistorySearchLimit     = 10
	// historyLikeSnippetRadius is how many runes a LIKE hit keeps on each side
	// of its match, about what the index path's snippet returns.
	historyLikeSnippetRadius = 40
)

// ErrHistoryMessageNotFound means the id names no searchable message of this
// work, including a message of another work.
var ErrHistoryMessageNotFound = errors.New("companion: no such message in this work's conversation")

// HistorySearchQuery asks for messages of one work's agent conversation.
type HistorySearchQuery struct {
	ProjectID string
	Query     string
	// BeforeID keeps only messages earlier than this one, which is how a turn
	// looks past the start of its own replay.
	BeforeID string
	// ExcludeRun leaves one turn out, so the question being asked is not its
	// own first result.
	ExcludeRun string
	Limit      int
}

// HistoryHit is one message that matched, cut down to the part around it.
type HistoryHit struct {
	MessageID string
	RunID     string
	Role      string
	Snippet   string
	CreatedAt int64
}

// historyOrder is the order a conversation reads in, the one listHistorySQL
// produces. At equal timestamps a user row precedes the rest.
const historyOrder = `m.created_at, CASE m.role WHEN 'user' THEN 0 ELSE 1 END, m.rowid`

// searchableHistory is the same predicate the insert trigger indexes on, so an
// anchor is readable exactly when it can be found.
const searchableHistory = `m.intent = '` + HistoryAgentIntent + `' AND m.role IN ('user', 'assistant') AND trim(m.content) <> ''`

// historyAnchor is a message's place in that order.
type historyAnchor struct {
	createdAt int64
	roleRank  int
	rowID     int64
}

// anchor finds a message that also satisfies predicate.
func (r *HistoryRepo) anchor(ctx context.Context, projectID, messageID, predicate string) (historyAnchor, error) {
	var a historyAnchor
	err := r.db.QueryRowContext(ctx, `
SELECT m.created_at, CASE m.role WHEN 'user' THEN 0 ELSE 1 END, m.rowid
  FROM companion_messages m
 WHERE m.id = ? AND m.project_id = ? AND `+predicate,
		strings.TrimSpace(messageID), projectID).Scan(&a.createdAt, &a.roleRank, &a.rowID)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrHistoryMessageNotFound
	}
	return a, err
}

// Search returns the messages holding every word of q.Query, best match first.
// A word under three characters is answered by a scan, for the reason the
// manuscript search gives: most Korean names are that short.
func (r *HistoryRepo) Search(ctx context.Context, q HistorySearchQuery) ([]HistoryHit, error) {
	hits := []HistoryHit{}
	if r == nil {
		return hits, nil
	}
	query := strings.TrimSpace(q.Query)
	if query == "" {
		return hits, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultHistorySearchLimit
	}
	limit = min(limit, maxHistorySearchLimit)

	filter := ` AND ` + searchableHistory + ` AND (? = '' OR COALESCE(m.run_id, '') <> ?)`
	args := []any{q.ExcludeRun, q.ExcludeRun}
	if strings.TrimSpace(q.BeforeID) != "" {
		// Any row of the conversation can be a boundary, not only a searchable
		// one: a replay can begin at the tool row holding an ask card.
		a, err := r.anchor(ctx, q.ProjectID, q.BeforeID, `m.intent = '`+HistoryAgentIntent+`'`)
		if err != nil {
			return nil, err
		}
		filter += ` AND (` + historyOrder + `) < (?, ?, ?)`
		args = append(args, a.createdAt, a.roleRank, a.rowID)
	}

	terms := ftsquery.Terms(query)
	match, ok := ftsquery.TrigramMatch(terms)
	if !ok {
		return r.searchLike(ctx, q.ProjectID, terms, filter, args, limit)
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT m.id, COALESCE(m.run_id, ''), m.role, m.created_at,
       snippet(companion_messages_fts, 0, '', '', '...', 40)
  FROM companion_messages_fts
  JOIN companion_messages m ON m.id = companion_messages_fts.message_id
 WHERE companion_messages_fts MATCH ? AND companion_messages_fts.project_id = ?`+filter+`
 ORDER BY bm25(companion_messages_fts)
 LIMIT ?`, append(append([]any{match, q.ProjectID}, args...), limit)...)
	if err != nil {
		return nil, fmt.Errorf("search companion history: %w", err)
	}
	defer rows.Close()
	return scanHistoryHits(rows, nil)
}

// searchLike puts messages holding the whole phrase first, then the newest: a
// scan has no rank, and a later message is the likelier to still be true.
func (r *HistoryRepo) searchLike(ctx context.Context, projectID string, terms []string, filter string, filterArgs []any, limit int) ([]HistoryHit, error) {
	phrase := strings.Join(terms, " ")
	args := []any{projectID}
	likes := make([]string, 0, len(terms))
	for _, term := range terms {
		likes = append(likes, `m.content LIKE ? ESCAPE '\'`)
		args = append(args, "%"+ftsquery.EscapeLike(term)+"%")
	}
	args = append(args, filterArgs...)
	args = append(args, phrase, limit)
	rows, err := r.db.QueryContext(ctx, `
SELECT m.id, COALESCE(m.run_id, ''), m.role, m.created_at, m.content
  FROM companion_messages m
 WHERE m.project_id = ? AND `+strings.Join(likes, " AND ")+filter+`
 ORDER BY instr(lower(m.content), lower(?)) > 0 DESC, m.created_at DESC, m.rowid DESC
 LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("search companion history: %w", err)
	}
	defer rows.Close()
	needles := append([]string{phrase}, terms...)
	return scanHistoryHits(rows, func(content string) string {
		return ftsquery.LikeSnippet(content, needles, historyLikeSnippetRadius)
	})
}

func scanHistoryHits(rows *sql.Rows, cut func(string) string) ([]HistoryHit, error) {
	hits := []HistoryHit{}
	for rows.Next() {
		var h HistoryHit
		if err := rows.Scan(&h.MessageID, &h.RunID, &h.Role, &h.CreatedAt, &h.Snippet); err != nil {
			return nil, err
		}
		if cut != nil {
			h.Snippet = cut(h.Snippet)
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// Window returns the message and up to before/after searchable messages around
// it, in reading order. Tool rows are not counted: a heavy turn writes dozens,
// and a window of them would show nothing anyone said.
func (r *HistoryRepo) Window(ctx context.Context, projectID, messageID string, before, after int) ([]HistoryMessage, error) {
	if r == nil {
		return nil, ErrHistoryMessageNotFound
	}
	a, err := r.anchor(ctx, projectID, messageID, searchableHistory)
	if err != nil {
		return nil, err
	}
	key := []any{a.createdAt, a.roleRank, a.rowID}
	const columns = `SELECT m.id, m.project_id, COALESCE(m.node_id, ''), '', COALESCE(m.run_id, ''),
       m.role, m.scope, m.intent, m.status, m.content, m.created_at
  FROM companion_messages m
 WHERE m.project_id = ? AND ` + searchableHistory
	const descending = `m.created_at DESC, CASE m.role WHEN 'user' THEN 0 ELSE 1 END DESC, m.rowid DESC`

	earlier, err := r.list(ctx, columns+` AND (`+historyOrder+`) < (?, ?, ?) ORDER BY `+descending+` LIMIT ?`,
		append(append([]any{projectID}, key...), max(before, 0))...)
	if err != nil {
		return nil, fmt.Errorf("read companion history: %w", err)
	}
	rest, err := r.list(ctx, columns+` AND (`+historyOrder+`) >= (?, ?, ?) ORDER BY `+historyOrder+` LIMIT ?`,
		append(append([]any{projectID}, key...), max(after, 0)+1)...)
	if err != nil {
		return nil, fmt.Errorf("read companion history: %w", err)
	}
	out := make([]HistoryMessage, 0, len(earlier)+len(rest))
	for i := len(earlier) - 1; i >= 0; i-- {
		out = append(out, earlier[i])
	}
	return append(out, rest...), nil
}
