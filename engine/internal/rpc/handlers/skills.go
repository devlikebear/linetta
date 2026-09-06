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
		// A file that is there but does NOT read as a skill counts as an
		// edit, and the save goes through. The first version of this
		// refused it — "writing over something unreadable is how a
		// half-broken file becomes a lost one" — and that was backwards: a
		// SKILL.md with broken frontmatter is exactly the file a writer
		// opens Settings to fix, refusing the save left them with a skill
		// they could see, could not repair and could not remove, and what
		// they are overwriting it with is the repaired version of the same
		// document. Anything genuinely wrong with the path (a permissions
		// problem, a directory where the SKILL.md belongs) fails in
		// Store.Write a few lines below, with its own message, so nothing
		// is silently lost by not pre-judging it here.
		enabled := true
		reason := agentskills.ReasonCreated
		switch cur, err := store.Read(scope, projectID, name); {
		case err == nil:
			enabled, reason = cur.Enabled, agentskills.ReasonEdited
		case errors.Is(err, agentskills.ErrNotFound):
			// A create. The defaults above are already right.
		default:
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
		switch cur, err := store.Read(scope, projectID, name); {
		case err == nil:
			enabled, reason = cur.Enabled, agentskills.ReasonEdited
			if err := refuseUnrecordedSkill(ctx, history, version); err != nil {
				return nil, err
			}
		case errors.Is(err, agentskills.ErrNotFound):
			// Gone. This restore recreates it; see above.
		default:
			// Something is at that name and it is not a readable skill —
			// broken frontmatter, or a path problem. Either way the
			// restore is a repair of it, and it has to pass the same check
			// as any other live document.
			reason = agentskills.ReasonEdited
			if err := refuseUnrecordedSkill(ctx, history, version); err != nil {
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
// Every change made through this surface or through mcphost records a row
// carrying the whole body, and this restore records one too, so the answer
// is normally yes: reverting is undoable by reverting again, and the writer
// browsing the version list can see both documents' rows sitting in it. The
// case where the answer is NO is precise and detectable: the last thing this
// log recorded for the name is its DELETION, and yet a skill is standing
// there. Nothing recorded it, so nothing holds it, and overwriting it
// destroys it for good. That is a file the writer created by hand in the
// skills folder (which the store is deliberately built to support), one
// restored from a backup, or one whose version row did not land.
//
// This replaces a check that asked "is there a deletion row newer than this
// version, by created_at", which was wrong in both directions:
//
//   - It refused far too much. After any delete-then-restore cycle, every
//     version older than the deletion became permanently unrestorable, with
//     a message asserting a different skill stood at the name when it was
//     the same one, restored a moment earlier.
//
//   - It let the destructive case through. created_at is milliseconds, the
//     comparison was strictly greater, and a deletion recorded in the same
//     millisecond as the version being restored therefore did not count —
//     so a genuinely unrecorded skill at that name was silently overwritten.
//     Past 200 rows the check could not see the deletion at all.
//
// Both directions came from the same root: a millisecond clock is not an
// ordering, and a windowed List is not a history. This asks History.Newest
// instead, which reads one row ordered by rowid — a total order, no ties,
// no window, unchanged by however many versions the skill accumulates.
//
// A history read that FAILS is not a refusal: the version's own target is
// still valid, and blocking a restore because the log could not be read
// would make an unrelated database problem look like a rejected request.
// The same goes for a name with no history at all, which cannot happen from
// here (the version being restored is itself a row) but must not become a
// refusal if it ever does.
func refuseUnrecordedSkill(ctx context.Context, history SkillHistory, version agentskills.Version) error {
	s := version.Skill
	newest, err := history.Newest(ctx, s.Scope, s.ProjectID, s.Name)
	if err != nil {
		return nil
	}
	if newest.Reason != agentskills.ReasonDeleted {
		return nil
	}
	return refuse("a skill is standing at %q that this history has no copy of: the last thing recorded "+
		"for that name is its deletion, so restoring over it would lose it for good; delete it first "+
		"if you mean to bring this version back", s.Name)
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
