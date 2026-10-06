// Package style holds the part of a writer's style a machine can check: the
// rules, and a check that reports where a scene breaks them (#162).
//
// Nothing here calls a model. The work's style notes are prose and stay a
// matter of judgement for whoever reads them; this package answers only the
// questions that have one answer — is this phrase in the scene, is this
// sentence over the limit — and it reports, it never edits.
package style

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/store"
)

// Limits on the rules themselves. The check runs every phrase over every
// paragraph, and its report goes to an agent's context, so both are bounded.
const (
	MaxAvoidPhrases  = 100
	MaxPhraseRunes   = 80
	MaxSentenceLimit = 2000
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrTooManyPhrases  = errors.New("at most 100 phrases to avoid")
	ErrPhraseTooLong   = errors.New("a phrase to avoid is at most 80 characters")
	ErrSentenceLimit   = errors.New("sentence limit must be between 0 and 2000 characters")
)

// Rules are one work's checkable style rules.
type Rules struct {
	ProjectID string `json:"project_id"`
	// AvoidPhrases are matched as written, ignoring letter case.
	AvoidPhrases []string `json:"avoid_phrases"`
	// MaxSentenceChars is the longest a sentence may be, in characters.
	// 0 means no limit.
	MaxSentenceChars int   `json:"max_sentence_chars"`
	UpdatedAt        int64 `json:"updated_at"`
}

// Empty reports whether there is nothing to check against.
func (r Rules) Empty() bool {
	return len(r.AvoidPhrases) == 0 && r.MaxSentenceChars == 0
}

// Repo stores style rules in library.db, so they travel with the backup.
type Repo struct{ s *store.Store }

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// GetRules returns the work's rules, empty until the writer saves some.
func (r *Repo) GetRules(ctx context.Context, projectID string) (Rules, error) {
	out := Rules{ProjectID: projectID, AvoidPhrases: []string{}}
	var phrases string
	err := r.s.DB().QueryRowContext(ctx,
		"SELECT avoid_phrases, max_sentence_chars, updated_at FROM project_style_rules WHERE project_id = ?", projectID).
		Scan(&phrases, &out.MaxSentenceChars, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal([]byte(phrases), &out.AvoidPhrases); err != nil || out.AvoidPhrases == nil {
		out.AvoidPhrases = []string{}
	}
	return out, nil
}

// SetRules replaces the work's rules. Phrases are trimmed, blanks dropped and
// duplicates folded; anything over a limit is refused rather than cut, so the
// writer is never left checking against a list shorter than the one they typed.
func (r *Repo) SetRules(ctx context.Context, now int64, in Rules) (Rules, error) {
	var exists bool
	if err := r.s.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM projects WHERE id = ?)", in.ProjectID).Scan(&exists); err != nil {
		return Rules{}, err
	}
	if !exists {
		return Rules{}, ErrProjectNotFound
	}
	phrases, err := NormalizePhrases(in.AvoidPhrases)
	if err != nil {
		return Rules{}, err
	}
	if in.MaxSentenceChars < 0 || in.MaxSentenceChars > MaxSentenceLimit {
		return Rules{}, ErrSentenceLimit
	}
	raw, err := json.Marshal(phrases)
	if err != nil {
		return Rules{}, err
	}
	out := Rules{ProjectID: in.ProjectID, AvoidPhrases: phrases, MaxSentenceChars: in.MaxSentenceChars, UpdatedAt: now}
	_, err = r.s.DB().ExecContext(ctx, `INSERT INTO project_style_rules (project_id, avoid_phrases, max_sentence_chars, updated_at) VALUES (?,?,?,?)
ON CONFLICT(project_id) DO UPDATE SET avoid_phrases=excluded.avoid_phrases, max_sentence_chars=excluded.max_sentence_chars, updated_at=excluded.updated_at`,
		out.ProjectID, string(raw), out.MaxSentenceChars, out.UpdatedAt)
	return out, err
}

// NormalizePhrases trims, drops blanks, folds case-insensitive duplicates and
// enforces the limits. The result is never nil.
func NormalizePhrases(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, p := range in {
		p = strings.Join(strings.Fields(p), " ")
		if p == "" {
			continue
		}
		if utf8.RuneCountInString(p) > MaxPhraseRunes {
			return nil, ErrPhraseTooLong
		}
		key := strings.ToLower(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	if len(out) > MaxAvoidPhrases {
		return nil, ErrTooManyPhrases
	}
	return out, nil
}
