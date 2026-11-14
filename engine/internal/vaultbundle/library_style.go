package vaultbundle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/style"
)

// The writer's default style (#178) lives in the library, not in any work, so
// it travels in the library item beside the writer profile and under the same
// category. Rules, notes and the pending draft are parts of their own: a rule
// changed on one device and the notes on another both stand. The two parts of
// the writer_style row share its one updated_at as their time.
//
// A work's own opt-out, project_style_rules.ignore_writer_style, travels with
// the work.

// styleRules is the "style/rules" part. Its time is in FileTimes, outside the
// hash.
type styleRules struct {
	AvoidPhrases     []string `json:"avoid_phrases"`
	MaxSentenceChars int      `json:"max_sentence_chars"`
}

// styleDraft is the "style/draft" part. Its time, created_at, is in FileTimes.
type styleDraft struct {
	Body   string `json:"body"`
	Author string `json:"author"`
}

func isStylePart(name string) bool {
	return name == libraryStyleRulesPart || name == libraryStyleNotesPart || name == libraryStyleDraftPart
}

// exportWriterStyle adds the parts the writer has: none for rules that are
// empty, notes that are blank or no draft, so clearing one reads as a delete.
func exportWriterStyle(ctx context.Context, db *sql.DB, bundle *LibraryBundle) error {
	var phrases, notes string
	var rules styleRules
	var updated int64
	err := db.QueryRowContext(ctx,
		"SELECT avoid_phrases, max_sentence_chars, notes, updated_at FROM writer_style WHERE id = 1").
		Scan(&phrases, &rules.MaxSentenceChars, &notes, &updated)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		if json.Unmarshal([]byte(phrases), &rules.AvoidPhrases) != nil || rules.AvoidPhrases == nil {
			rules.AvoidPhrases = []string{} // what GetWriterRules reads it as
		}
		if len(rules.AvoidPhrases) > 0 || rules.MaxSentenceChars > 0 {
			data, err := json.Marshal(rules)
			if err != nil {
				return err
			}
			bundle.addStyle(libraryStyleRulesPart, data, updated)
		}
		if notes != "" {
			bundle.addStyle(libraryStyleNotesPart, []byte(notes), updated)
		}
	}
	var draft styleDraft
	var created int64
	err = db.QueryRowContext(ctx, "SELECT body, author, created_at FROM writer_style_draft WHERE id = 1").
		Scan(&draft.Body, &draft.Author, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	bundle.addStyle(libraryStyleDraftPart, data, created)
	return nil
}

func (b *LibraryBundle) addStyle(name string, data []byte, when int64) {
	b.add(name, data)
	b.Meta.FileTimes[name] = when
	b.Meta.Format = LibraryFormat
}

// style takes or clears one part of the writer's default style. What would not
// pass the style package's own limits here is refused whole, as the profile
// is, and this device's stands.
func (a libraryApplier) style(name string, verdict mergeVerdict, out *LibraryApplied) error {
	data, when := a.remote.Parts[name], a.remote.Meta.FileTimes[name]
	if verdict == removeLocal {
		data, when = nil, a.now.UnixMilli()
	}
	var err error
	switch name {
	case libraryStyleRulesPart:
		err = a.styleRules(data, when)
	case libraryStyleNotesPart:
		err = a.styleNotes(data, when)
	case libraryStyleDraftPart:
		err = a.styleDraft(data, when)
	}
	if errors.Is(err, errStyleRefused) {
		out.StyleRefused = true
		return nil
	}
	if err != nil {
		return err
	}
	if verdict == takeRemote {
		out.Taken = append(out.Taken, name)
	} else {
		out.Removed = append(out.Removed, name)
	}
	return nil
}

var errStyleRefused = errors.New("vaultbundle: the writer's style does not fit here")

// The row's time only moves forward: the other part of it may be newer here.
const upsertWriterStyle = `INSERT INTO writer_style (id, avoid_phrases, max_sentence_chars, updated_at) VALUES (1,?,?,?)
ON CONFLICT(id) DO UPDATE SET avoid_phrases=excluded.avoid_phrases, max_sentence_chars=excluded.max_sentence_chars,
  updated_at=MAX(writer_style.updated_at, excluded.updated_at)`

func (a libraryApplier) styleRules(data []byte, when int64) error {
	rules := styleRules{AvoidPhrases: []string{}}
	if data != nil {
		if err := json.Unmarshal(data, &rules); err != nil {
			return errStyleRefused
		}
		phrases, err := style.NormalizePhrases(rules.AvoidPhrases)
		if err != nil || rules.MaxSentenceChars < 0 || rules.MaxSentenceChars > style.MaxSentenceLimit {
			return errStyleRefused
		}
		rules.AvoidPhrases = phrases
	}
	raw, err := json.Marshal(rules.AvoidPhrases)
	if err != nil {
		return err
	}
	_, err = a.db.ExecContext(a.ctx, upsertWriterStyle, string(raw), rules.MaxSentenceChars, when)
	return err
}

func (a libraryApplier) styleNotes(data []byte, when int64) error {
	notes := string(data)
	if !utf8.ValidString(notes) || utf8.RuneCountInString(notes) > style.MaxWriterNotesRunes {
		return errStyleRefused
	}
	_, err := a.db.ExecContext(a.ctx, `INSERT INTO writer_style (id, notes, updated_at) VALUES (1,?,?)
ON CONFLICT(id) DO UPDATE SET notes=excluded.notes, updated_at=MAX(writer_style.updated_at, excluded.updated_at)`, notes, when)
	return err
}

func (a libraryApplier) styleDraft(data []byte, when int64) error {
	if data == nil {
		_, err := a.db.ExecContext(a.ctx, "DELETE FROM writer_style_draft WHERE id = 1")
		return err
	}
	var draft styleDraft
	if err := json.Unmarshal(data, &draft); err != nil || draft.Body == "" ||
		utf8.RuneCountInString(draft.Body) > style.MaxWriterNotesRunes {
		return errStyleRefused
	}
	_, err := a.db.ExecContext(a.ctx, `INSERT INTO writer_style_draft (id, body, author, created_at) VALUES (1,?,?,?)
ON CONFLICT(id) DO UPDATE SET body=excluded.body, author=excluded.author, created_at=excluded.created_at`,
		draft.Body, draft.Author, when)
	return err
}
