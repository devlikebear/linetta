package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// SkillStore and SkillHistory are the slices of *agentskills.Store and
// *agentskills.History these handlers need. Interfaces, not the concrete
// types, for the same reason MemoryStore is one (memory.go): handlers must
// never link tars/pkg/llm, and keeping the dependency abstract is how that
// stays true by construction rather than by anyone remembering.
type SkillStore interface {
	List(scope agentskills.Scope, projectID string) ([]agentskills.Skill, []agentskills.Diagnostic, error)
	Read(scope agentskills.Scope, projectID, name string) (agentskills.Skill, error)
	// ReadRaw is Read's unparsed twin, and it is in this interface for one
	// reason: a SKILL.md whose frontmatter the writer broke does not parse,
	// so Read cannot return it, and a skill that is listed as broken and
	// cannot be opened cannot be repaired from inside the app. See
	// ReadSkill.
	ReadRaw(scope agentskills.Scope, projectID, name string) (string, int64, error)
	Write(s agentskills.Skill, now int64) (agentskills.Skill, error)
	Delete(scope agentskills.Scope, projectID, name string) error
}

// SkillHistory is the version log behind the same skills.
type SkillHistory interface {
	Record(ctx context.Context, s agentskills.Skill, reason string, now int64) error
	List(ctx context.Context, scope agentskills.Scope, projectID, name string, limit int) ([]agentskills.Version, error)
	// Newest is the last row recorded for one skill. RestoreSkill needs it
	// and List cannot stand in for it: List is a window, and a check written
	// against a window stops being true once the table outgrows it.
	Newest(ctx context.Context, scope agentskills.Scope, projectID, name string) (agentskills.Version, error)
	Get(ctx context.Context, id string) (agentskills.Version, error)
}

// skillChangedPayload mirrors mcphost's skillChangedPayload
// (mcphost/tools_write.go) field for field: a listener must not have to
// handle two shapes for the same notification method.
//
// Source is always "writer" from this file. mcphost's vocabulary is
// external/agent, and a save made by the person at the keyboard is neither.
type skillChangedPayload struct {
	Scope     string `json:"scope"`
	ProjectID string `json:"project_id,omitempty"`
	Name      string `json:"name"`
	Source    string `json:"source"`
}

// sourceWriter is that third value, named once so the four places that emit
// it cannot drift.
const sourceWriter = "writer"

// ---- error mapping --------------------------------------------------------

// skillErr maps a store failure onto the two codes the writer's UI can tell
// apart, following handlers/settings.go's convention.
//
// Nearly everything this package refuses is something the writer can act on
// — an unknown scope, a name that is not a slug, a body over the cap, a
// zero-width space pasted in from somewhere, forty skills already — and each
// of those errors carries a sentence that says what to change. Those are
// CodeInvalidParams, with the underlying message passed through, because an
// opaque 500 would strand a writer who is one keystroke from fixing it.
//
// CodeInternalError is reserved for a genuine read or write failure on disk:
// an unreadable directory, a permissions problem. Those surface as an
// *fs.PathError somewhere in the chain, which is what this switches on —
// agentskills' own sentinels never wrap one (ErrNotFound is synthesized from
// a stat that failed with fs.ErrNotExist, not passed through), so the two
// classes cannot be confused.
func skillErr(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
	}
	return &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
}

// historyErr is the same split for the version log, where the storage is a
// database rather than a directory: only a missing id is the caller's
// problem, and everything else is the server's.
func historyErr(err error) error {
	if errors.Is(err, agentskills.ErrVersionNotFound) {
		return &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
	}
	return &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
}

func badParams(err error) error {
	return &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
}

func refuse(format string, args ...any) error {
	return &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: fmt.Sprintf(format, args...)}
}

// refuseScopeMismatch catches a scope paired with the wrong work id BEFORE
// the call that would otherwise catch it.
//
// Both storage layers already enforce this rule — Store.Dir and
// History.projectArg — but they return it as an ordinary error, mixed in
// with everything else that can go wrong down there, and the history's
// errors come from a database where the other failures are the server's
// fault rather than the caller's. Sorting the two apart afterwards means
// guessing. Asking here, where the difference is still obvious, means
// skills.history can answer a genuine SQLite failure with the internal
// error it is.
//
// The wording mirrors agentskills' own, so a writer does not read two
// different sentences about the same mistake.
func refuseScopeMismatch(scope agentskills.Scope, projectID string) error {
	switch scope {
	case agentskills.ScopeWriter:
		if projectID != "" {
			return refuse("writer skills are global; they take no work id (got %q)", projectID)
		}
	case agentskills.ScopeWork:
		if projectID == "" {
			return refuse("work skills need the work they belong to")
		}
	}
	return nil
}

// ---- skills.list ----------------------------------------------------------

// skillSummary is one row of the Settings list: the whole skill except its
// body. Forty bodies at MaxBodyRunes each is a third of a megabyte of text
// the pane does not draw — skills.read fetches the one the writer opens.
// BodyRunes and BodyBudget stay, because the list shows how full each skill
// is before any of them is opened.
type skillSummary struct {
	Name        string             `json:"name"`
	Scope       agentskills.Scope  `json:"scope"`
	ProjectID   string             `json:"project_id,omitempty"`
	Description string             `json:"description"`
	Author      agentskills.Author `json:"author"`
	Enabled     bool               `json:"enabled"`
	UpdatedAt   int64              `json:"updated_at"`
	BodyRunes   int                `json:"body_runes"`
	BodyBudget  int                `json:"body_budget"`
}

func summarize(s agentskills.Skill) skillSummary {
	return skillSummary{
		Name: s.Name, Scope: s.Scope, ProjectID: s.ProjectID,
		Description: s.Description, Author: s.Author, Enabled: s.Enabled,
		UpdatedAt: s.UpdatedAt, BodyRunes: s.BodyRunes, BodyBudget: agentskills.MaxBodyRunes,
	}
}

type listSkillsParams struct {
	ProjectID string `json:"project_id"`
}

type listSkillsResult struct {
	Skills      []skillSummary           `json:"skills"`
	Diagnostics []agentskills.Diagnostic `json:"diagnostics"`
}

// ListSkills returns a handler for skills.list: BOTH scopes in one call,
// plus the diagnostics.
//
// The diagnostics are the point of the method, not a detail of it. A
// SKILL.md the writer broke by hand — bad frontmatter, a body over the cap,
// a name that disagrees with its folder — is never listed, because it must
// not reach a prompt, and would otherwise be invisible: the writer would see
// a skill missing from Settings with nothing anywhere saying why. Task 3
// made the store report those instead of dropping them; this is the surface
// that delivers them.
//
// No work picked yet is a legitimate UI state, not an error — the Settings
// pane opens before the writer has opened a work, and this is the first call
// it makes. The store is right to refuse ScopeWork with no work id (a
// work-scoped skill has no directory to live in without one), so this layer,
// which is the one that knows the difference, simply does not ask. The
// writer scope comes back on its own. That is #97's Critical in this
// surface's shape: there, the handler asked anyway and the profile it had
// already read successfully was thrown away with the refusal.
func ListSkills(store SkillStore) rpc.Handler {
	return func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listSkillsParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, badParams(err)
			}
		}
		projectID := strings.TrimSpace(p.ProjectID)

		out := listSkillsResult{
			Skills:      make([]skillSummary, 0),
			Diagnostics: make([]agentskills.Diagnostic, 0),
		}
		writerSkills, writerDiags, err := store.List(agentskills.ScopeWriter, "")
		if err != nil {
			return nil, skillErr(err)
		}
		for _, s := range writerSkills {
			out.Skills = append(out.Skills, summarize(s))
		}
		out.Diagnostics = append(out.Diagnostics, writerDiags...)

		if projectID != "" {
			workSkills, workDiags, err := store.List(agentskills.ScopeWork, projectID)
			if err != nil {
				return nil, skillErr(err)
			}
			for _, s := range workSkills {
				out.Skills = append(out.Skills, summarize(s))
			}
			out.Diagnostics = append(out.Diagnostics, workDiags...)
		}
		return json.Marshal(out)
	}
}

// ---- skills.read ----------------------------------------------------------

type skillTargetParams struct {
	Scope     string `json:"scope"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
}

// target resolves the three fields every method but list and restore takes.
func (p skillTargetParams) target() (agentskills.Scope, string, string, error) {
	scope, err := agentskills.ParseScope(p.Scope)
	if err != nil {
		return "", "", "", badParams(err)
	}
	return scope, strings.TrimSpace(p.ProjectID), strings.TrimSpace(p.Name), nil
}

// skillReadResult is the document plus, when it did not parse, the reason.
//
// ParseError is empty for every skill that is well-formed, which is nearly
// all of them, so it is omitempty: a pane that does not know about it reads
// exactly what it read before. When it IS set, the fields around it are the
// best that can be said about a file that is not a skill — see ReadSkill —
// and the pane has to show it, because a writer handed a body with no
// explanation would save it straight back and wonder why the skill still
// does not appear.
type skillReadResult struct {
	agentskills.Skill
	ParseError string `json:"parse_error,omitempty"`
}

// ReadSkill returns a handler for skills.read: the full document, body and
// all.
//
// It deliberately does NOT guard, and that is the whole reason the method
// exists separately from the list. Store.Read made the same reversal for the
// same reason (see its doc comment): a skill the guard refuses is visible in
// Settings as a diagnostic and has to be FIXABLE, and an 8050-rune body
// cannot be trimmed by someone who cannot read it. So this is the repair
// path — it opens what the list refused, and skills.write, which does guard,
// is what refuses to store it again unrepaired.
//
// Not guarding was not enough, and this is the second half of the fix.
// Store.Read still PARSES, so a SKILL.md whose frontmatter the writer broke
// by hand — the commonest way to break one, and the one Settings shows as a
// diagnostic — came back as "agentskills: no frontmatter" and nothing else.
// The skill was visible and unopenable: exactly the state Task 3's rule
// exists to forbid, one level up. So a parse failure falls back to
// Store.ReadRaw and the file is returned as what it is:
//
//   - body: the file's text, verbatim, every byte of it including whatever
//     is left of the frontmatter. The writer is looking at the same thing
//     their own editor would show them, which is what lets them fix it.
//   - name: the one that was asked for (the folder's), since the file's own
//     may be unreadable or absent.
//   - description: empty. It is a required field, so skills.write will
//     refuse the save until the writer supplies one — which is right: a
//     skill with no description is invisible to the agent that has to pick
//     it.
//   - parse_error: what is wrong, in the same sentence the diagnostic gave.
//
// A file that is not there stays a refusal, and a genuine read failure
// (permissions, an unreadable directory) stays a 500 — skillErr's split is
// unchanged. Only "there is a file here and it is not a skill" is new, and
// it is a repair, not an error.
func ReadSkill(store SkillStore) rpc.Handler {
	return func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p skillTargetParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, badParams(err)
		}
		scope, projectID, name, err := p.target()
		if err != nil {
			return nil, err
		}
		s, err := store.Read(scope, projectID, name)
		if err == nil {
			return json.Marshal(skillReadResult{Skill: s})
		}
		if errors.Is(err, agentskills.ErrNotFound) {
			return nil, skillErr(err)
		}
		parseErr := err

		raw, updatedAt, rawErr := store.ReadRaw(scope, projectID, name)
		if rawErr != nil {
			// Nothing readable is there at all — a SKILL.md that is a
			// directory, a file over the size ceiling, a permissions
			// problem. Report what the FIRST read said, which is the
			// sentence about the document rather than about the byte
			// stream, and let skillErr pick the code from it.
			return nil, skillErr(parseErr)
		}
		return json.Marshal(skillReadResult{
			Skill: agentskills.Skill{
				Name: name, Scope: scope, ProjectID: projectID,
				Author:    agentskills.AuthorWriter,
				Enabled:   true,
				Body:      raw,
				UpdatedAt: updatedAt,
				BodyRunes: len([]rune(raw)),
			},
			ParseError: parseErr.Error(),
		})
	}
}

// ---- what is standing at a name -------------------------------------------

// skillPresence is what the two writing methods found at a name before they
// wrote over it. There are four states, not two, and collapsing any of them
// together is how a file this handler could not read got replaced by one it
// could.
type skillPresence int

const (
	// presenceAbsent: nothing is at that name. A write is a create.
	presenceAbsent skillPresence = iota
	// presenceSkill: a SKILL.md that reads back as a skill. A write is an
	// edit, and the skill it returns is the live one.
	presenceSkill
	// presenceUnparseable: a file IS there and its bytes ARE readable — the
	// only thing that failed is Parse. That is a SKILL.md whose frontmatter
	// the writer broke by hand, which is exactly the file they opened
	// Settings to repair, so a write over it is an edit too. Nothing can be
	// said about its fields; the Skill returned alongside is the zero value.
	presenceUnparseable
	// presenceUnreadable: something is at that name and inspectBeforeOverwrite
	// could not read it (over the size ceiling, a permissions problem — see
	// its doc comment). It is ALWAYS returned together with a non-nil error,
	// and every caller in this file returns that error before ever looking at
	// presence. This value exists so a caller that skipped the error check
	// anyway cannot read "unreadable" as presenceAbsent's zero value and treat
	// a file it could not see as empty ground to create over — the exact
	// overwrite this task exists to prevent, reached through a different door.
	presenceUnreadable
)

// inspectBeforeOverwrite reports what is at a name, and refuses on behalf of
// its caller when the answer is "something, and I cannot tell you what".
//
// This is the narrow version of a fallback that used to be one line of
// `default:` on the read's error — every failure that was not ErrNotFound
// counted as "there is a broken skill here, overwrite it". Only ONE failure
// means that. The rest mean the handler does not know what is on disk, and
// the difference is not academic: a SKILL.md over the 64 KiB ceiling
// (maxSkillFileBytes) fails BOTH Read and ReadRaw, so skills.read refuses to
// open it — and the widened fallback let skills.write replace it anyway. A
// 600 KB document became an 81-byte one, with a version row holding six
// runes as its only trace. Before that widening the write was refused, and
// it has to be again.
//
// ReadRaw is what tells the two apart, and it is the right question to ask:
// it resolves the same path through the same checks and reads through the
// same ceiling as Read, and does nothing else. If it comes back with bytes,
// the file is genuinely there and genuinely readable and Parse is what
// failed. If it comes back with an error, nobody here knows what is at that
// path, and a writer's file that cannot be read must not be silently
// replaced by one that can.
//
// The refusal carries ReadRaw's own error — the sentence about the byte
// stream ("over the 65536-byte file limit", a permissions problem), which is
// the one that says why the save cannot go through — and skillErr picks the
// code from it, so a file the writer can fix is still a refusal they can act
// on and a genuine IO failure is still a 500.
func inspectBeforeOverwrite(store SkillStore, scope agentskills.Scope, projectID, name string) (agentskills.Skill, skillPresence, error) {
	cur, err := store.Read(scope, projectID, name)
	switch {
	case err == nil:
		return cur, presenceSkill, nil
	case errors.Is(err, agentskills.ErrNotFound):
		return agentskills.Skill{}, presenceAbsent, nil
	}
	if _, _, rawErr := store.ReadRaw(scope, projectID, name); rawErr != nil {
		return agentskills.Skill{}, presenceUnreadable, skillErr(fmt.Errorf(
			"something is at %q that cannot be read, so it must not be written over: %w", name, rawErr))
	}
	return agentskills.Skill{}, presenceUnparseable, nil
}

// ---- skills.write ---------------------------------------------------------

type writeSkillParams struct {
	Scope       string `json:"scope"`
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
	// Enabled is a pointer so "the pane said nothing" stays distinguishable
	// from "the pane said false". A plain bool would make every save that
	// omitted the field switch the skill off, which is precisely the value
	// nobody would notice was wrong until the agent stopped using a skill.
	// Absent means: on for a new skill, unchanged for one that exists.
	Enabled *bool `json:"enabled"`
}

// skillWriteResult is the stored skill plus one field the document itself
// does not carry.
//
// Versioned says whether the version row behind this change landed. Almost
// always true; false means the SKILL.md on disk did change but the history
// did not, so this one edit cannot be reverted and a backup carrying only
// the database would not contain it. It is not an error — the file HAS
// changed, and failing after the fact would send the writer repairing
// something that is not broken — but it must not be silence either, so it
// is reported the same way mcphost's editSkillOutput reports it. No
// omitempty: false is the whole point of the field.
type skillWriteResult struct {
	agentskills.Skill
	Versioned bool `json:"versioned"`
}

// WriteSkill returns a handler for skills.write: create-or-update, which is
// what a Settings editor with a Save button needs (the pane does not know,
// and should not have to know, whether the file exists yet — Store.Write is
// an upsert underneath).
//
// A rename is deliberately NOT possible through this method. The params
// carry one name, and it is both the skill written and the skill addressed;
// there is no old-name/new-name pair and there should not be one. A skill's
// identity is (scope, work, name) — that is what skill_snapshots keys its
// history on — so renaming through a write would silently orphan every
// version the skill has, leaving the history stranded under a name nothing
// on disk answers to any more. A rename is a delete plus a create, and the
// writer performing it that way gets exactly what it means: the old name's
// history ends with a row marked deleted, and the new name starts fresh.
//
// The sequence follows mcphost's editSkill, which is the same write from the
// other side: guard (inside Store.Write, before any directory is created),
// write, record the version, notify. A nil notify is tolerated so callers
// with no live connection do not have to fake one.
func WriteSkill(store SkillStore, history SkillHistory, clock func() int64, notify func(method string, params any)) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p writeSkillParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, badParams(err)
		}
		scope, err := agentskills.ParseScope(p.Scope)
		if err != nil {
			return nil, badParams(err)
		}
		projectID := strings.TrimSpace(p.ProjectID)
		name := strings.TrimSpace(p.Name)

		// Whether this is a create or an edit decides the reason the version
		// row carries, and what an absent enabled flag falls back to.
		//
		// A file that is there but does not PARSE counts as an edit, and the
		// save goes through: a SKILL.md with broken frontmatter is exactly
		// the file a writer opens Settings to fix, and what they are
		// overwriting it with is the repaired version of the same document.
		// A file that cannot be READ at all is a different answer and
		// inspectBeforeOverwrite refuses it there — see its doc comment.
		enabled := true
		reason := agentskills.ReasonCreated
		cur, presence, err := inspectBeforeOverwrite(store, scope, projectID, name)
		if err != nil {
			return nil, err
		}
		switch presence {
		case presenceSkill:
			enabled, reason = cur.Enabled, agentskills.ReasonEdited
		case presenceUnparseable:
			// The broken file's own enabled flag is unreadable, so the
			// repaired skill comes back ON, the way a new one does — a
			// repair that silently switched a skill off is the value nobody
			// notices is wrong until the agent stops using it.
			reason = agentskills.ReasonEdited
		}
		if p.Enabled != nil {
			enabled = *p.Enabled
		}

		now := clock()
		saved, err := store.Write(agentskills.Skill{
			Name: name, Scope: scope, ProjectID: projectID,
			Description: strings.TrimSpace(p.Description),
			// The author is whoever wrote it LAST (skill.go), and this save
			// came from the person at the keyboard — even if an agent wrote
			// the first draft of it.
			Author:  agentskills.AuthorWriter,
			Enabled: enabled, Body: p.Body,
		}, now)
		if err != nil {
			return nil, skillErr(err)
		}

		versioned := recordVersion(ctx, history, saved, reason, now)
		notifySkillChanged(notify, scope, projectID, name)
		return json.Marshal(skillWriteResult{Skill: saved, Versioned: versioned})
	}
}

// ---- skills.delete --------------------------------------------------------

// deleteSkillResult is `{}` plus the same Versioned flag skills.write
// carries, and for a sharper reason: a deletion whose version row did not
// land is not merely unrevertible, it is the body gone with nothing left
// holding it.
type deleteSkillResult struct {
	Versioned bool `json:"versioned"`
}

// DeleteSkill returns a handler for skills.delete.
//
// The skill is read before it is removed, so the version row marked deleted
// carries the last body rather than an empty one — that is what makes a
// deletion restorable straight from the row that records it, instead of the
// writer having to know to reach for the row before it (History.Record's doc
// comment is explicit about this, and skills.restore depends on it).
//
// That read is BEST EFFORT, and this is the correction of the first
// version, which treated it as a precondition. A SKILL.md whose frontmatter
// the writer broke by hand does not parse, so the read failed, so the
// deletion was refused — and the writer was left with a file that Settings
// listed as broken and offered no way to remove. A delete has no business
// parsing anything: the caller asked for the name to be gone. So the read
// only decides what the version row can SAY; whether the skill goes is
// Store.Delete's call, and it is the one that refuses a name that is not
// there (a writer who deleted the wrong thing has to be told, not
// silently succeeded at).
//
// When the file could not be read, no version row is recorded and the
// result says versioned:false. That is the honest answer: there was no
// readable document to snapshot, so this deletion cannot be undone from the
// history — and a row carrying an empty body would be worse, since it would
// appear in the version list as a restore point that restores nothing.
func DeleteSkill(store SkillStore, history SkillHistory, clock func() int64, notify func(method string, params any)) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p skillTargetParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, badParams(err)
		}
		scope, projectID, name, err := p.target()
		if err != nil {
			return nil, err
		}
		cur, readErr := store.Read(scope, projectID, name)
		if err := store.Delete(scope, projectID, name); err != nil {
			return nil, skillErr(err)
		}
		now := clock()
		versioned := false
		if readErr == nil {
			versioned = recordVersion(ctx, history, cur, agentskills.ReasonDeleted, now)
		}
		notifySkillChanged(notify, scope, projectID, name)
		return json.Marshal(deleteSkillResult{Versioned: versioned})
	}
}

// ---- skills.history -------------------------------------------------------

// skillVersion is one row of a skill's history on the wire.
// agentskills.Version carries no JSON tags — it is a storage type — so the
// shape the pane reads is declared here rather than leaking Go field names
// into the protocol.
//
// The body travels with each row on purpose. skills.restore takes only an
// id, so nothing else in this surface can show the writer what a version
// CONTAINS before they revert to it, and a version list you cannot preview
// is a list of timestamps. History.List clamps its own limit (200), and a
// body is bounded by MaxBodyRunes, so the worst case is bounded too.
type skillVersion struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Scope       agentskills.Scope  `json:"scope"`
	ProjectID   string             `json:"project_id,omitempty"`
	Description string             `json:"description"`
	Author      agentskills.Author `json:"author"`
	Body        string             `json:"body"`
	BodyRunes   int                `json:"body_runes"`
	Reason      string             `json:"reason"`
	CreatedAt   int64              `json:"created_at"`
}

func wireVersion(v agentskills.Version) skillVersion {
	return skillVersion{
		ID: v.ID, Name: v.Skill.Name, Scope: v.Skill.Scope, ProjectID: v.Skill.ProjectID,
		Description: v.Skill.Description, Author: v.Skill.Author,
		Body: v.Skill.Body, BodyRunes: v.Skill.BodyRunes,
		Reason: v.Reason, CreatedAt: v.CreatedAt,
	}
}

type skillHistoryParams struct {
	Scope     string `json:"scope"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Limit     int    `json:"limit"`
}

type skillVersionsResult struct {
	Versions []skillVersion `json:"versions"`
}

// SkillVersions returns a handler for skills.history. The limit goes
// straight through to History.List, which defaults it (0 means "the caller
// said nothing") and clamps it — skill_snapshots has no retention pass yet,
// so that clamp is the only thing standing between a client-supplied number
// and the whole table.
func SkillVersions(history SkillHistory) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p skillHistoryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, badParams(err)
		}
		scope, err := agentskills.ParseScope(p.Scope)
		if err != nil {
			return nil, badParams(err)
		}
		projectID := strings.TrimSpace(p.ProjectID)
		if err := refuseScopeMismatch(scope, projectID); err != nil {
			return nil, err
		}
		versions, err := history.List(ctx, scope, projectID, strings.TrimSpace(p.Name), p.Limit)
		if err != nil {
			// Everything the caller could have got wrong was refused above,
			// so anything left is the database failing — an internal error,
			// not a bad request. Routing it through skillErr (which reads a
			// non-filesystem error as "the writer can fix this") answered a
			// broken SQLite file with -32602 and a message about skills.
			return nil, historyErr(err)
		}
		out := skillVersionsResult{Versions: make([]skillVersion, 0, len(versions))}
		for _, v := range versions {
			out.Versions = append(out.Versions, wireVersion(v))
		}
		return json.Marshal(out)
	}
}

// ---- skills.restore -------------------------------------------------------

type restoreSkillParams struct {
	ID string `json:"id"`
}

// RestoreSkill returns a handler for skills.restore: it puts a version's
// body back.
//
// The target is taken from the VERSION, never from the caller: the row
// carries its own (scope, work, name), so a version can only ever be written
// back to the skill it came from. There is no parameter that could aim it
// somewhere else.
//
// Two states are reachable here and neither may silently write the wrong
// thing:
//
//   - The skill has since been DELETED. Restoring recreates it. That is the
//     whole reason a deletion records the last body (History.Record's doc
//     comment), and refusing here would make a deletion the one change in
//     this system that cannot be undone. The version row this restore lands
//     is marked created, because that is what just happened — the skill did
//     not exist a moment ago.
//
//   - A skill stands at that name that THIS LOG HAS NEVER SEEN. Writing
//     over it would destroy a document nothing else holds a copy of, so it
//     is refused. See refuseUnrecordedSkill for how that is told apart from
//     the ordinary case, and for what changed from the first version of
//     this check.
//
// The enabled flag is NOT restored, because skill_snapshots has no column
// for it: scanVersion leaves Skill.Enabled at its zero value, so writing a
// version back verbatim would switch a skill off as a side effect of
// reverting its text. The live skill's flag is kept instead, and a recreated
// one comes back on.
//
// The author IS restored, unlike skills.write's: this puts a document back
// as it was, and if the agent wrote that draft then the agent is still who
// wrote it.
func RestoreSkill(store SkillStore, history SkillHistory, clock func() int64, notify func(method string, params any)) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p restoreSkillParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, badParams(err)
		}
		id := strings.TrimSpace(p.ID)
		if id == "" {
			return nil, refuse("a version id is required")
		}
		version, err := history.Get(ctx, id)
		if err != nil {
			return nil, historyErr(err)
		}
		want := version.Skill
		scope, projectID, name := want.Scope, want.ProjectID, want.Name

		enabled := true
		reason := agentskills.ReasonCreated
		live, presence, err := inspectBeforeOverwrite(store, scope, projectID, name)
		if err != nil {
			return nil, err
		}
		if presence != presenceAbsent {
			reason = agentskills.ReasonEdited
			if presence == presenceSkill {
				enabled = live.Enabled
			}
			if err := refuseUnrecordedSkill(ctx, history, version, live, presence); err != nil {
				return nil, err
			}
		}
		want.Enabled = enabled

		now := clock()
		saved, err := store.Write(want, now)
		if err != nil {
			return nil, skillErr(err)
		}
		versioned := recordVersion(ctx, history, saved, reason, now)
		notifySkillChanged(notify, scope, projectID, name)
		return json.Marshal(skillWriteResult{Skill: saved, Versioned: versioned})
	}
}

// refuseUnrecordedSkill refuses a restore that would write over a document
// this history does not hold a copy of.
//
// The question a restore has to answer before it overwrites anything is not
// "is the skill standing here the same one this version came from" — that is
// unanswerable, because skill_snapshots has no lineage column and a name
// that was deleted and recreated looks identical in the log whether the
// recreation was a restore of the same skill or somebody's brand new one.
// The answerable question, and the one that actually matters, is: if this
// restore overwrites what is there, can it be got back?
//
// That question is answered by COMPARING, not by reading a row's reason.
// Every change through this surface and through mcphost's editSkill records
// a row carrying the whole document, and Store.Write renders exactly what
// the row stores (Render and Parse round-trip Description, Author and Body
// byte for byte), so a skill whose text still matches the newest row for its
// name IS held by the log: overwriting it loses nothing, and the writer who
// reverts too far reverts back. A skill whose text does NOT match that row
// is standing there unrecorded — hand-edited in the skills folder (which the
// store is deliberately built to support), restored from a backup, or left
// by a write whose version row did not land — and nothing anywhere holds it.
//
// What is compared, and what is not:
//
//   - Body, Description and Author: the three document fields
//     skill_snapshots stores and Store.Write round-trips.
//   - NOT Enabled. The table has no column for it (History.Record's INSERT
//     is the list), so scanVersion always reads it back as false and a
//     comparison including it — or a comparison of Render(row) against the
//     file, which amounts to the same thing — would call every enabled
//     skill in the app unrecorded. Toggling the flag goes through
//     skills.write, which records a row with the same body anyway.
//   - NOT Name, Scope or ProjectID: they are the key the row was fetched by.
//   - NOT UpdatedAt: an mtime, not part of the document.
//
// This replaces a rule that asked "is the last recorded row a deletion?",
// which was a proxy for the real question and answered it wrongly at two
// reachable states, both unrecoverable:
//
//   - After a delete-then-restore cycle the last row is `created`, so the
//     rule was disarmed, and a folder replaced BY HAND afterwards was
//     overwritten though it existed in no row.
//   - Deleting a skill whose SKILL.md does not parse records nothing
//     (DeleteSkill's versioned:false), leaving a stale `created` as the last
//     row — so the rule was disarmed at exactly the name the app had just
//     emptied, and whatever the writer put there next was overwritten.
//
// Comparing content closes both, because it does not care which row kind
// came last. It also stops the rule from firing on a hand-recreated file
// whose byte-identical copy IS sitting in the deletion row, which the old
// message asserted was unrecorded when it was not.
//
// The unparseable case is a refusal on its own, and necessarily so: the log
// only ever recorded documents that parsed (they came from Store.Write,
// which renders), so text that does not parse cannot be a copy of any row.
// Nothing is trapped by that — skills.read hands the file back verbatim
// (ReadRaw), skills.write saves a repair over it, and skills.delete removes
// it — and the message says so.
//
// A history read that fails is the one case that is NOT the caller's fault,
// and it is not silently allowed either: without the log this check cannot
// be made, and a restore that cannot be checked must not proceed over a live
// document. It comes back as the internal error it is (historyErr), which a
// retry clears; the alternative, allowing it, is the destructive direction
// for a reason that has nothing to do with the request.
func refuseUnrecordedSkill(ctx context.Context, history SkillHistory, version agentskills.Version, live agentskills.Skill, presence skillPresence) error {
	s := version.Skill
	if presence == presenceUnparseable {
		return refuse("the file standing at %q is not a readable skill, so nothing in the history is a copy "+
			"of it and restoring over it would lose its text for good; open it to keep what it says, then "+
			"save over it or delete it", s.Name)
	}
	newest, err := history.Newest(ctx, s.Scope, s.ProjectID, s.Name)
	if err != nil && !errors.Is(err, agentskills.ErrVersionNotFound) {
		return historyErr(err)
	}
	// A name with no history at all cannot be reached from here (the version
	// being restored is itself one of its rows), but if it ever were, a skill
	// standing at it is recorded by definition nowhere.
	if err == nil && recordsSkill(newest.Skill, live) {
		return nil
	}
	return refuse("the skill standing at %q is not the one this history last recorded there, so nothing "+
		"holds a copy of it and restoring over it would lose it for good; open it to keep what it says, "+
		"then delete it if you mean to bring this version back", s.Name)
}

// recordsSkill reports whether row is a copy of the document standing on
// disk. See refuseUnrecordedSkill for which fields participate and why the
// enabled flag cannot.
//
// Body and Description are compared after normalizeLineEndings, not
// byte-for-byte. splitFrontmatter (skill.go's doc comment) deliberately
// preserves a body's own line endings through Parse and Render — the body is
// content this package does not own — so a SKILL.md a writer opens and
// re-saves in a CRLF-by-default editor (Notepad, and most editors on
// Windows, which Linetta ships to, and this folder is one a writer points
// other tools at) comes back with every "\n" turned into "\r\n" and not one
// keystroke of actual writer intent behind the difference. Comparing raw
// bytes read that as "an unrecorded document is standing here" and refused
// every future restore at that name until the writer deleted it — for a
// rewrite nobody made on purpose. The same editors routinely add or drop
// exactly one trailing newline on save, which normalizeLineEndings also
// absorbs.
//
// Author is compared as-is: it is one of two fixed values (AuthorWriter,
// AuthorAgent — skill.go), never free text a line-ending change could touch.
//
// Nothing else is normalized. Any other difference — including a single
// changed word, with matching line endings — still fails this comparison,
// and the restore still refuses; that direction is what this check exists
// to protect.
func recordsSkill(row, live agentskills.Skill) bool {
	return normalizeLineEndings(row.Body) == normalizeLineEndings(live.Body) &&
		normalizeLineEndings(row.Description) == normalizeLineEndings(live.Description) &&
		row.Author == live.Author
}

// normalizeLineEndings collapses CRLF to LF and trims trailing whitespace
// (including the run of newlines many editors add or remove on save), so
// recordsSkill can tell "the same document, re-saved by a different tool"
// from "a different document". See recordsSkill's doc comment for why this
// exists and what it deliberately does not touch — everything other than
// line endings and trailing whitespace still participates in the
// comparison untouched.
func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, " \t\n")
}

// ---- shared write tail ----------------------------------------------------

// recordVersion lands the version row behind a change that has ALREADY
// happened on disk, and reports whether it did.
//
// It never turns a failure into an error result, for the reason mcphost's
// editSkill gives: the SKILL.md has changed, and refusing after the fact
// would send the caller off repairing something that is not broken, or
// rewriting work that already landed. What it must not be is silent — the
// state it leaves (a file newer than its history, unrevertible, and absent
// from a backup that carries only the database) is worse than a refusal
// anyone could read. So the boolean travels back in the result.
func recordVersion(ctx context.Context, history SkillHistory, s agentskills.Skill, reason string, now int64) bool {
	if history == nil {
		return false
	}
	return history.Record(ctx, s, reason, now) == nil
}

func notifySkillChanged(notify func(method string, params any), scope agentskills.Scope, projectID, name string) {
	if notify == nil {
		return
	}
	notify("skills.changed", skillChangedPayload{
		Scope: string(scope), ProjectID: projectID, Name: name, Source: sourceWriter,
	})
}
