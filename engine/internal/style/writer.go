package style

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// The writer's default style (#178): checkable rules that sit under every
// work's own. Changing them changes every work at once — they are not a
// template copied into a new work. A work can opt out (Rules.IgnoreWriterStyle).
//
// Only the writer sets them, through the app's style.set_writer_rules. No MCP
// tool writes them.

// Where a rule came from.
const (
	SourceWriter = "writer"
	SourceWork   = "work"
)

// WriterRules are the writer's default checkable rules.
type WriterRules struct {
	AvoidPhrases []string `json:"avoid_phrases"`
	// MaxSentenceChars is the default sentence limit; 0 means none.
	MaxSentenceChars int   `json:"max_sentence_chars"`
	UpdatedAt        int64 `json:"updated_at"`
}

// Empty reports whether there is nothing to add to a work's rules.
func (w WriterRules) Empty() bool {
	return len(w.AvoidPhrases) == 0 && w.MaxSentenceChars == 0
}

// GetWriterRules returns the writer's rules, empty until some are saved.
func (r *Repo) GetWriterRules(ctx context.Context) (WriterRules, error) {
	out := WriterRules{AvoidPhrases: []string{}}
	var phrases string
	err := r.s.DB().QueryRowContext(ctx,
		"SELECT avoid_phrases, max_sentence_chars, updated_at FROM writer_style WHERE id = 1").
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

// SetWriterRules replaces the writer's rules, under the same limits and the
// same normalisation as a work's.
func (r *Repo) SetWriterRules(ctx context.Context, now int64, in WriterRules) (WriterRules, error) {
	phrases, err := NormalizePhrases(in.AvoidPhrases)
	if err != nil {
		return WriterRules{}, err
	}
	if in.MaxSentenceChars < 0 || in.MaxSentenceChars > MaxSentenceLimit {
		return WriterRules{}, ErrSentenceLimit
	}
	raw, err := json.Marshal(phrases)
	if err != nil {
		return WriterRules{}, err
	}
	out := WriterRules{AvoidPhrases: phrases, MaxSentenceChars: in.MaxSentenceChars, UpdatedAt: now}
	_, err = r.s.DB().ExecContext(ctx, `INSERT INTO writer_style (id, avoid_phrases, max_sentence_chars, updated_at) VALUES (1,?,?,?)
ON CONFLICT(id) DO UPDATE SET avoid_phrases=excluded.avoid_phrases, max_sentence_chars=excluded.max_sentence_chars, updated_at=excluded.updated_at`,
		string(raw), out.MaxSentenceChars, out.UpdatedAt)
	return out, err
}

// AdoptPhrases adds phrases to the writer's rules, keeping what is there. It
// carries the device-only "phrases to keep out of every work" setting that came
// before the writer's default style into it, once. Phrases over a limit are
// dropped rather than refusing the rest.
func (r *Repo) AdoptPhrases(ctx context.Context, now int64, phrases []string) error {
	cur, err := r.GetWriterRules(ctx)
	if err != nil {
		return err
	}
	merged := append([]string{}, cur.AvoidPhrases...)
	for _, p := range phrases {
		next, err := NormalizePhrases(append(append([]string{}, merged...), p))
		if err != nil {
			continue
		}
		merged = next
	}
	cur.AvoidPhrases = merged
	_, err = r.SetWriterRules(ctx, now, cur)
	return err
}

// EffectiveRules is what a check of the work runs against: its own rules with
// the writer's merged in, unless the work opts out.
func (r *Repo) EffectiveRules(ctx context.Context, projectID string) (Rules, error) {
	work, err := r.GetRules(ctx, projectID)
	if err != nil {
		return Rules{}, err
	}
	writer, err := r.GetWriterRules(ctx)
	if err != nil {
		return Rules{}, err
	}
	return Merge(work, writer), nil
}

// Merge layers a work's rules over the writer's.
//
//   - Phrases: the writer's and the work's together, case-insensitive
//     duplicates once. A phrase on both lists counts as the work's.
//   - Sentence limit: the work's own if it set one (0 being "no limit"),
//     otherwise the writer's.
//
// A work that opts out keeps only its own rules. WriterPhrases and
// SentenceLimitSource say where each rule came from.
func Merge(work Rules, writer WriterRules) Rules {
	out := work
	out.AvoidPhrases = append([]string{}, work.AvoidPhrases...)
	out.WriterPhrases = nil
	out.SentenceLimitSource = ""
	if work.OwnSentenceLimit {
		if work.MaxSentenceChars > 0 {
			out.SentenceLimitSource = SourceWork
		}
	} else {
		out.MaxSentenceChars = 0
	}
	if work.IgnoreWriterStyle {
		return out
	}
	have := map[string]bool{}
	for _, p := range work.AvoidPhrases {
		have[strings.ToLower(p)] = true
	}
	for _, p := range writer.AvoidPhrases {
		key := strings.ToLower(p)
		if have[key] {
			continue
		}
		have[key] = true
		out.AvoidPhrases = append(out.AvoidPhrases, p)
		out.WriterPhrases = append(out.WriterPhrases, p)
	}
	if !work.OwnSentenceLimit && writer.MaxSentenceChars > 0 {
		out.MaxSentenceChars = writer.MaxSentenceChars
		out.SentenceLimitSource = SourceWriter
	}
	return out
}

// phraseSource says whose phrase p is in rules a check runs against.
func (r Rules) phraseSource(p string) string {
	for _, w := range r.WriterPhrases {
		if strings.EqualFold(w, p) {
			return SourceWriter
		}
	}
	return SourceWork
}

func (r Rules) limitSource() string {
	if r.SentenceLimitSource != "" {
		return r.SentenceLimitSource
	}
	return SourceWork
}
