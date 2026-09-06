package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// Every test here runs against the REAL store and the REAL history, not a
// stub, and that is deliberate. #97 shipped a Critical because its handler
// stub could not reproduce the store's own validation: the suite stayed
// green while the production path was broken. A skill store is a directory
// under a temp dir and the history is one table in a temp database, so
// there is no cost to using the real ones — and they are the only things
// that reproduce ValidName, the guard, the cap and the frontmatter
// round-trip.
//
// A stub survives in exactly one place (failingHistory), for the one state
// the real collaborators cannot be talked into: a version row that will not
// land.
//
// None of this links tars/pkg/llm into the handlers test binary — neither
// agentskills, internal/store nor internal/project does, and
// scripts/validate-story-core-deps.sh checks ./internal/rpc/handlers with
// `go list -test -deps` on every run.
func realSkills(t *testing.T) (context.Context, *agentskills.Store, *agentskills.History, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{
		Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return ctx, agentskills.NewStore(t.TempDir()), agentskills.NewHistory(st.DB()), p.ID
}

// seedSkill writes one skill through the real store, the way the handler
// would, so a test can start from a populated directory.
func seedSkill(t *testing.T, st *agentskills.Store, s agentskills.Skill, now int64) agentskills.Skill {
	t.Helper()
	saved, err := st.Write(s, now)
	if err != nil {
		t.Fatalf("seed skill %q: %v", s.Name, err)
	}
	return saved
}

func methodError(t *testing.T, err error) *rpc.MethodError {
	t.Helper()
	var me *rpc.MethodError
	if !errors.As(err, &me) {
		t.Fatalf("error is %T, want *rpc.MethodError: %v", err, err)
	}
	return me
}

func wantInvalidParams(t *testing.T, err error) *rpc.MethodError {
	t.Helper()
	if err == nil {
		t.Fatal("want a refusal, got none")
	}
	me := methodError(t, err)
	if me.Code != rpc.CodeInvalidParams {
		t.Errorf("code = %d, want %d (a refusal the writer can act on, not a 500)",
			me.Code, rpc.CodeInvalidParams)
	}
	return me
}

// ---- skills.list ----------------------------------------------------------

// The Settings pane opens before the writer has picked a work, and this is
// the first call it makes. Against the real store the writer scope must
// still come back, with an empty work list beside it — the store refuses
// ScopeWork with no work id, so a handler that asked it anyway would take
// the writer's own skills down with it. This is #97's Critical, in this
// surface's shape.
func TestListSkillsWithNoWorkSelectedReturnsTheWriterScope(t *testing.T) {
	ctx, st, _, _ := realSkills(t)
	seedSkill(t, st, agentskills.Skill{
		Name: "fight-scenes", Scope: agentskills.ScopeWriter,
		Description: "싸움 장면을 쓸 때", Enabled: true, Body: "짧은 문장.\n",
	}, 1000)

	raw, err := ListSkills(st)(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("skills.list with no work must succeed: %v", err)
	}
	var got listSkillsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Skills) != 1 || got.Skills[0].Name != "fight-scenes" {
		t.Fatalf("skills = %+v, want the one writer skill", got.Skills)
	}
	if got.Skills[0].Scope != agentskills.ScopeWriter {
		t.Errorf("scope = %q, want %q", got.Skills[0].Scope, agentskills.ScopeWriter)
	}
	if len(got.Diagnostics) != 0 {
		t.Errorf("diagnostics = %+v, want none", got.Diagnostics)
	}
	// The two empty collections must serialize as [], not null: the pane
	// maps over them.
	if !strings.Contains(string(raw), `"diagnostics":[]`) {
		t.Errorf("diagnostics must be [] and not null; got %s", raw)
	}
}

func TestListSkillsWithAWorkReturnsBothScopes(t *testing.T) {
	ctx, st, _, projectID := realSkills(t)
	seedSkill(t, st, agentskills.Skill{
		Name: "fight-scenes", Scope: agentskills.ScopeWriter,
		Description: "싸움 장면", Enabled: true, Body: "짧은 문장.\n",
	}, 1000)
	seedSkill(t, st, agentskills.Skill{
		Name: "minjun-voice", Scope: agentskills.ScopeWork, ProjectID: projectID,
		Description: "민준의 말투", Enabled: true, Body: "존댓말.\n",
	}, 1000)

	raw, err := ListSkills(st)(ctx, json.RawMessage(`{"project_id":`+quote(projectID)+`}`))
	if err != nil {
		t.Fatalf("skills.list: %v", err)
	}
	var got listSkillsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Skills) != 2 {
		t.Fatalf("skills = %+v, want both scopes", got.Skills)
	}
	byName := map[string]agentskills.Scope{}
	for _, s := range got.Skills {
		byName[s.Name] = s.Scope
	}
	if byName["fight-scenes"] != agentskills.ScopeWriter || byName["minjun-voice"] != agentskills.ScopeWork {
		t.Errorf("scopes = %+v", byName)
	}
}

// A skill the guard refuses must be VISIBLE in Settings — that is Task 3's
// most important rule, and this is the surface that delivers it. A broken
// SKILL.md is a diagnostic, never a dropped file and never a failed call.
func TestListSkillsReportsABrokenSkillAsADiagnostic(t *testing.T) {
	ctx, st, _, _ := realSkills(t)
	seedSkill(t, st, agentskills.Skill{
		Name: "good", Scope: agentskills.ScopeWriter,
		Description: "쓸모 있는 스킬", Enabled: true, Body: "본문\n",
	}, 1000)
	// Written by hand, the way a writer breaks one: no frontmatter at all.
	dir, err := st.Dir(agentskills.ScopeWriter, "")
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	writeRawSkill(t, filepath.Join(dir, "broken"), "이건 프론트매터가 없다\n")

	raw, err := ListSkills(st)(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("one broken skill must not fail the listing: %v", err)
	}
	var got listSkillsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Skills) != 1 || got.Skills[0].Name != "good" {
		t.Errorf("the twelve that are fine must survive one that is not; got %+v", got.Skills)
	}
	if len(got.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want the broken one reported", got.Diagnostics)
	}
	if !strings.Contains(got.Diagnostics[0].Path, "broken") {
		t.Errorf("the diagnostic must name the file the writer has to open; got %+v", got.Diagnostics[0])
	}
}

// The summary is what Settings lists, so it must not carry forty bodies —
// but it must carry the size, because the pane draws the capacity line from
// it before anything is opened.
func TestListSkillsOmitsBodiesButKeepsTheirSize(t *testing.T) {
	ctx, st, _, _ := realSkills(t)
	body := strings.Repeat("가", 500)
	seedSkill(t, st, agentskills.Skill{
		Name: "long-one", Scope: agentskills.ScopeWriter,
		Description: "긴 본문", Enabled: true, Body: body,
	}, 1000)

	raw, err := ListSkills(st)(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("skills.list: %v", err)
	}
	if strings.Contains(string(raw), body) {
		t.Error("skills.list must not ship every body; that is what skills.read is for")
	}
	var got listSkillsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Skills[0].BodyRunes != 500 {
		t.Errorf("body_runes = %d, want 500", got.Skills[0].BodyRunes)
	}
	if got.Skills[0].BodyBudget != agentskills.MaxBodyRunes {
		t.Errorf("body_budget = %d, want %d", got.Skills[0].BodyBudget, agentskills.MaxBodyRunes)
	}
}

func TestListSkillsRefusesAnUnusableWorkID(t *testing.T) {
	ctx, st, _, _ := realSkills(t)
	_, err := ListSkills(st)(ctx, json.RawMessage(`{"project_id":"../../etc"}`))
	wantInvalidParams(t, err)
}

// ---- skills.read ----------------------------------------------------------

// Read is the repair path. A skill over the body cap is refused by Guard and
// therefore never LISTED — but the writer has to be able to open it,
// shorten it and save it, and an 8050-rune body cannot be trimmed by someone
// who cannot read it. So skills.read must not guard, and the shortened body
// must then be accepted by skills.write.
func TestReadSkillOpensAnOversizedSkillSoItCanBeRepaired(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	dir, err := st.Dir(agentskills.ScopeWriter, "")
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	tooLong := strings.Repeat("가", agentskills.MaxBodyRunes+50)
	writeRawSkill(t, filepath.Join(dir, "overlong"),
		"---\nname: overlong\ndescription: 너무 길다\nenabled: true\n---\n"+tooLong)

	// Not listed: it must never reach a prompt.
	raw, err := ListSkills(st)(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("skills.list: %v", err)
	}
	var listed listSkillsResult
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(listed.Skills) != 0 || len(listed.Diagnostics) != 1 {
		t.Fatalf("an over-cap skill is a diagnostic, not a listing; got %+v / %+v",
			listed.Skills, listed.Diagnostics)
	}

	// But readable, so it can be fixed.
	raw, err = ReadSkill(st)(ctx, json.RawMessage(`{"scope":"writer","name":"overlong"}`))
	if err != nil {
		t.Fatalf("an over-cap skill must still open for repair: %v", err)
	}
	var got agentskills.Skill
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.BodyRunes != agentskills.MaxBodyRunes+50 {
		t.Errorf("body_runes = %d, want the whole over-cap body", got.BodyRunes)
	}

	// And the shortened body saves.
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "overlong",
		"description": got.Description, "body": strings.Repeat("가", 100),
	})
	if _, err := WriteSkill(st, hist, func() int64 { return 2000 }, nil)(ctx, params); err != nil {
		t.Fatalf("the repaired skill must save: %v", err)
	}
}

func TestReadSkillOnAMissingNameIsARefusalNotA500(t *testing.T) {
	ctx, st, _, _ := realSkills(t)
	_, err := ReadSkill(st)(ctx, json.RawMessage(`{"scope":"writer","name":"nope"}`))
	me := wantInvalidParams(t, err)
	if !strings.Contains(me.Message, "nope") {
		t.Errorf("the message must name what was not found; got %q", me.Message)
	}
}

// The rule Task 3 is built around, one level up: a skill broken by hand is
// visible in Settings AND fixable there. Frontmatter is what a writer
// actually breaks — a fence deleted, a stray tab in the YAML — and such a
// file does not parse, so every one of these three methods used to answer
// "agentskills: no frontmatter" and nothing else. The skill was listed as
// broken and could not be opened, repaired or removed from inside the app.
//
// All three are checked here in one place, against one broken file each,
// because the bug was one bug: a delete and a write that had no business
// parsing, and a read that had to stop.
func TestABrokenSkillCanBeReadRepairedAndRemoved(t *testing.T) {
	broken := "name: no-fences\ndescription: 프론트매터가 깨졌다\n\n본문이라고 쓴 것\n"

	t.Run("read returns what is on disk", func(t *testing.T) {
		ctx, st, _, _ := realSkills(t)
		dir, err := st.Dir(agentskills.ScopeWriter, "")
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		writeRawSkill(t, filepath.Join(dir, "busted"), broken)

		raw, err := ReadSkill(st)(ctx, json.RawMessage(`{"scope":"writer","name":"busted"}`))
		if err != nil {
			t.Fatalf("a skill listed as broken must open, or it cannot be fixed: %v", err)
		}
		var got skillReadResult
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Body != broken {
			t.Errorf("body = %q, want the file verbatim — that is what the writer edits", got.Body)
		}
		if got.Name != "busted" {
			t.Errorf("name = %q, want the folder's name; the file's own is unreadable", got.Name)
		}
		if got.ParseError == "" {
			t.Error("parse_error must say what is wrong, or the writer saves it straight back")
		}
		if got.BodyRunes != len([]rune(broken)) {
			t.Errorf("body_runes = %d, want %d", got.BodyRunes, len([]rune(broken)))
		}
	})

	t.Run("write replaces it with a good one", func(t *testing.T) {
		ctx, st, hist, _ := realSkills(t)
		dir, err := st.Dir(agentskills.ScopeWriter, "")
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		writeRawSkill(t, filepath.Join(dir, "busted"), broken)

		params, _ := json.Marshal(map[string]any{
			"scope": "writer", "name": "busted", "description": "고쳤다", "body": "본문\n",
		})
		if _, err := WriteSkill(st, hist, clockAt(2000), nil)(ctx, params); err != nil {
			t.Fatalf("the repaired file must save over the broken one: %v", err)
		}
		fixed, err := st.Read(agentskills.ScopeWriter, "", "busted")
		if err != nil {
			t.Fatalf("the repaired skill must now parse: %v", err)
		}
		if fixed.Body != "본문\n" || fixed.Description != "고쳤다" {
			t.Errorf("stored skill = %+v", fixed)
		}
		// It is an edit of the file that was there, not a create.
		versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "busted", 10)
		if err != nil {
			t.Fatalf("history list: %v", err)
		}
		if len(versions) != 1 || versions[0].Reason != agentskills.ReasonEdited {
			t.Errorf("versions = %+v, want one row marked %q", versions, agentskills.ReasonEdited)
		}
	})

	t.Run("delete removes it", func(t *testing.T) {
		ctx, st, hist, _ := realSkills(t)
		dir, err := st.Dir(agentskills.ScopeWriter, "")
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		writeRawSkill(t, filepath.Join(dir, "busted"), broken)

		rawResult, err := DeleteSkill(st, hist, clockAt(2000), nil)(
			ctx, json.RawMessage(`{"scope":"writer","name":"busted"}`))
		if err != nil {
			t.Fatalf("the caller asked for the name to be gone; parsing is not delete's business: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "busted")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the directory must be gone; stat gave %v", err)
		}
		// Nothing readable was there to snapshot, and the result says so
		// rather than promising a restore point that restores nothing.
		var got deleteSkillResult
		if err := json.Unmarshal(rawResult, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Versioned {
			t.Error("versioned = true, but no readable body was there to record")
		}
		versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "busted", 10)
		if err != nil {
			t.Fatalf("history list: %v", err)
		}
		if len(versions) != 0 {
			t.Errorf("versions = %+v, want no row rather than an empty one", versions)
		}
	})
}

func TestReadSkillRefusesAnUnknownScope(t *testing.T) {
	ctx, st, _, _ := realSkills(t)
	_, err := ReadSkill(st)(ctx, json.RawMessage(`{"scope":"nonsense","name":"x"}`))
	wantInvalidParams(t, err)
}

func TestSkillHandlersRefuseMalformedParams(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	clock := func() int64 { return 1 }
	for name, h := range map[string]rpc.Handler{
		"skills.list":    ListSkills(st),
		"skills.read":    ReadSkill(st),
		"skills.write":   WriteSkill(st, hist, clock, nil),
		"skills.delete":  DeleteSkill(st, hist, clock, nil),
		"skills.history": SkillVersions(hist),
		"skills.restore": RestoreSkill(st, hist, clock, nil),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h(ctx, json.RawMessage(`{"scope":`))
			wantInvalidParams(t, err)
		})
	}
}

// ---- skills.write ---------------------------------------------------------

// The test that matters (Task 8's brief): a writer pasting text with a
// zero-width space gets a message naming the code point, not an opaque 500.
func TestWriteSkillNamesTheInvisibleCharacter(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "pasted", "description": "붙여넣기",
		"body": "보이지 않는 문자​가 있다",
	})
	_, err := WriteSkill(st, hist, func() int64 { return 1000 }, nil)(ctx, params)
	me := wantInvalidParams(t, err)
	if !strings.Contains(me.Message, "U+200B") {
		t.Errorf("the message must name the code point the writer has to delete; got %q", me.Message)
	}
}

func TestWriteSkillCreatesRecordsAndNotifies(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	var gotMethod string
	var gotPayload any
	notify := func(m string, p any) { gotMethod, gotPayload = m, p }

	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "fight-scenes", "description": "싸움 장면을 쓸 때",
		"body": "짧은 문장.\n", "enabled": true,
	})
	raw, err := WriteSkill(st, hist, func() int64 { return 1000 }, notify)(ctx, params)
	if err != nil {
		t.Fatalf("skills.write: %v", err)
	}
	var saved skillWriteResult
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if saved.Body != "짧은 문장.\n" || saved.UpdatedAt != 1000 {
		t.Errorf("saved = %+v", saved)
	}
	if !saved.Versioned {
		t.Error("versioned = false — the writer could not revert this edit")
	}
	// The author is whoever wrote it last, and this one came from the person
	// at the keyboard.
	if saved.Author != agentskills.AuthorWriter {
		t.Errorf("author = %q, want %q", saved.Author, agentskills.AuthorWriter)
	}

	if gotMethod != "skills.changed" {
		t.Fatalf("notify method = %q — another window would show a stale editor", gotMethod)
	}
	payload, ok := gotPayload.(skillChangedPayload)
	if !ok {
		t.Fatalf("payload is %T, want skillChangedPayload", gotPayload)
	}
	if payload.Source != "writer" {
		t.Errorf("source = %q; mcphost's vocabulary is external/agent, and a save from the "+
			"person at the keyboard is neither", payload.Source)
	}
	if payload.Name != "fight-scenes" || payload.Scope != string(agentskills.ScopeWriter) {
		t.Errorf("payload = %+v", payload)
	}

	versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "fight-scenes", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if len(versions) != 1 || versions[0].Reason != agentskills.ReasonCreated {
		t.Fatalf("versions = %+v, want one row marked %q", versions, agentskills.ReasonCreated)
	}
}

// The second write of the same name is an edit, not a create — and the
// history has to say so, or a version list reads as forty creations.
func TestWriteSkillOnAnExistingNameIsAnEdit(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	clock := func() int64 { return 1000 }
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "fight-scenes", "description": "설명", "body": "하나\n",
	})
	if _, err := WriteSkill(st, hist, clock, nil)(ctx, params); err != nil {
		t.Fatalf("first write: %v", err)
	}
	params, _ = json.Marshal(map[string]any{
		"scope": "writer", "name": "fight-scenes", "description": "설명", "body": "둘\n",
	})
	if _, err := WriteSkill(st, hist, func() int64 { return 2000 }, nil)(ctx, params); err != nil {
		t.Fatalf("second write: %v", err)
	}
	versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "fight-scenes", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions = %d, want 2", len(versions))
	}
	if versions[0].Reason != agentskills.ReasonEdited {
		t.Errorf("newest reason = %q, want %q", versions[0].Reason, agentskills.ReasonEdited)
	}
}

// enabled is a tri-state on the wire: the pane may not send it at all. A
// missing flag must not silently switch a skill off — on a create it
// defaults to on, and on an edit it leaves the stored value alone.
func TestWriteSkillLeavesEnabledAloneWhenItIsNotSent(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	clock := func() int64 { return 1000 }
	off, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "quiet", "description": "설명", "body": "본문\n", "enabled": false,
	})
	if _, err := WriteSkill(st, hist, clock, nil)(ctx, off); err != nil {
		t.Fatalf("write: %v", err)
	}
	silent, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "quiet", "description": "설명", "body": "고친 본문\n",
	})
	raw, err := WriteSkill(st, hist, clock, nil)(ctx, silent)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	var got skillWriteResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Enabled {
		t.Error("an edit that said nothing about enabled must not switch the skill back on")
	}

	fresh, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "brand-new", "description": "설명", "body": "본문\n",
	})
	raw, err = WriteSkill(st, hist, clock, nil)(ctx, fresh)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Enabled {
		t.Error("a new skill defaults to enabled")
	}
}

func TestWriteSkillRefusesABadName(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "Fight Scenes", "description": "설명", "body": "본문\n",
	})
	_, err := WriteSkill(st, hist, func() int64 { return 1 }, nil)(ctx, params)
	me := wantInvalidParams(t, err)
	if !strings.Contains(me.Message, "Fight Scenes") {
		t.Errorf("the message must name what to change; got %q", me.Message)
	}
}

func TestWriteSkillRefusesAnEmptyDescription(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "nameless", "description": "  ", "body": "본문\n",
	})
	_, err := WriteSkill(st, hist, func() int64 { return 1 }, nil)(ctx, params)
	wantInvalidParams(t, err)
}

func TestWriteSkillRefusesOverTheCap(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	for i := 0; i < agentskills.MaxSkillsPerScope; i++ {
		seedSkill(t, st, agentskills.Skill{
			Name: skillName(i), Scope: agentskills.ScopeWriter,
			Description: "설명", Enabled: true, Body: "본문\n",
		}, 1000)
	}
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "one-too-many", "description": "설명", "body": "본문\n",
	})
	_, err := WriteSkill(st, hist, func() int64 { return 1 }, nil)(ctx, params)
	me := wantInvalidParams(t, err)
	if !strings.Contains(me.Message, "delete one first") {
		t.Errorf("the message must say what to do about it; got %q", me.Message)
	}
}

// A write whose version row will not land still changed the file — refusing
// after the fact would send the writer repairing something that is not
// broken. But it must not be silent either: versioned=false is the field
// the pane reads to say "this edit cannot be reverted".
func TestWriteSkillReportsAVersionRowThatDidNotLand(t *testing.T) {
	ctx, st, _, _ := realSkills(t)
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "unversioned", "description": "설명", "body": "본문\n",
	})
	raw, err := WriteSkill(st, failingHistory{}, func() int64 { return 1000 }, nil)(ctx, params)
	if err != nil {
		t.Fatalf("the file changed, so the call must succeed: %v", err)
	}
	var got skillWriteResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Versioned {
		t.Error("versioned = true, but no version row landed — the writer would think this edit is revertible")
	}
}

// ---- skills.delete --------------------------------------------------------

func TestDeleteSkillRecordsTheLastBodyAndNotifies(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	seedSkill(t, st, agentskills.Skill{
		Name: "goner", Scope: agentskills.ScopeWriter,
		Description: "곧 지운다", Enabled: true, Body: "마지막 본문\n",
	}, 1000)

	var gotMethod string
	var gotPayload any
	notify := func(m string, p any) { gotMethod, gotPayload = m, p }
	if _, err := DeleteSkill(st, hist, func() int64 { return 2000 }, notify)(
		ctx, json.RawMessage(`{"scope":"writer","name":"goner"}`)); err != nil {
		t.Fatalf("skills.delete: %v", err)
	}
	if _, err := st.Read(agentskills.ScopeWriter, "", "goner"); !errors.Is(err, agentskills.ErrNotFound) {
		t.Errorf("the skill must be gone; Read gave %v", err)
	}
	if gotMethod != "skills.changed" {
		t.Errorf("notify method = %q", gotMethod)
	}
	if p, ok := gotPayload.(skillChangedPayload); !ok || p.Source != "writer" {
		t.Errorf("payload = %+v", gotPayload)
	}

	versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "goner", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if len(versions) != 1 || versions[0].Reason != agentskills.ReasonDeleted {
		t.Fatalf("versions = %+v, want one row marked %q", versions, agentskills.ReasonDeleted)
	}
	if versions[0].Skill.Body != "마지막 본문\n" {
		t.Errorf("the deleted row must carry the last body, or the deletion looks unrecoverable; got %q",
			versions[0].Skill.Body)
	}
}

func TestDeleteSkillOnAMissingNameIsARefusal(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	_, err := DeleteSkill(st, hist, func() int64 { return 1 }, nil)(
		ctx, json.RawMessage(`{"scope":"writer","name":"never-existed"}`))
	wantInvalidParams(t, err)
}

// ---- skills.history -------------------------------------------------------

func TestSkillVersionsAreNewestFirst(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	for i, body := range []string{"하나\n", "둘\n", "셋\n"} {
		params, _ := json.Marshal(map[string]any{
			"scope": "writer", "name": "versioned", "description": "설명", "body": body,
		})
		if _, err := WriteSkill(st, hist, clockAt(1000+int64(i)*1000), nil)(ctx, params); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	raw, err := SkillVersions(hist)(ctx, json.RawMessage(`{"scope":"writer","name":"versioned"}`))
	if err != nil {
		t.Fatalf("skills.history: %v", err)
	}
	var got skillVersionsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Versions) != 3 {
		t.Fatalf("versions = %d, want 3", len(got.Versions))
	}
	if got.Versions[0].Body != "셋\n" {
		t.Errorf("newest first: got %q", got.Versions[0].Body)
	}
	if got.Versions[0].ID == "" {
		t.Error("every row needs its id, or the writer cannot restore it")
	}
}

func TestSkillVersionsForAnUntouchedSkillIsAnEmptyArray(t *testing.T) {
	ctx, _, hist, _ := realSkills(t)
	raw, err := SkillVersions(hist)(ctx, json.RawMessage(`{"scope":"writer","name":"untouched"}`))
	if err != nil {
		t.Fatalf("skills.history: %v", err)
	}
	if !strings.Contains(string(raw), `"versions":[]`) {
		t.Errorf("versions must be [] and not null; got %s", raw)
	}
}

func TestSkillVersionsRefusesAWriterScopeWithAWorkID(t *testing.T) {
	ctx, _, hist, projectID := realSkills(t)
	_, err := SkillVersions(hist)(ctx,
		json.RawMessage(`{"scope":"writer","project_id":`+quote(projectID)+`,"name":"x"}`))
	wantInvalidParams(t, err)
}

// The brief's split, in the one method that was getting it wrong: a scope
// paired with the wrong work id is the caller's mistake (-32602, above), and
// a database that will not read is the server's (-32603). This used to send
// both through the same mapping, so an unreadable SQLite file came back as
// "your request was invalid" with a sentence about skills — and the pane
// would have shown the writer a refusal to act on that they could not act
// on.
func TestSkillVersionsReportsADatabaseFailureAsInternal(t *testing.T) {
	ctx, _, _, _ := realSkills(t)
	_, err := SkillVersions(failingHistory{})(ctx, json.RawMessage(`{"scope":"writer","name":"x"}`))
	if err == nil {
		t.Fatal("want an error")
	}
	me := methodError(t, err)
	if me.Code != rpc.CodeInternalError {
		t.Errorf("code = %d, want %d — a broken database is not a bad request",
			me.Code, rpc.CodeInternalError)
	}
}

// ---- skills.restore -------------------------------------------------------

func TestRestoreSkillPutsAnOlderBodyBack(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	first, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "reverted", "description": "설명", "body": "원래 본문\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(1000), nil)(ctx, first); err != nil {
		t.Fatalf("first write: %v", err)
	}
	second, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "reverted", "description": "설명", "body": "잘못 고친 본문\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(2000), nil)(ctx, second); err != nil {
		t.Fatalf("second write: %v", err)
	}
	versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "reverted", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	oldest := versions[len(versions)-1]

	var gotMethod string
	var gotPayload any
	notify := func(m string, p any) { gotMethod, gotPayload = m, p }
	raw, err := RestoreSkill(st, hist, clockAt(3000), notify)(
		ctx, json.RawMessage(`{"id":`+quote(oldest.ID)+`}`))
	if err != nil {
		t.Fatalf("skills.restore: %v", err)
	}
	var got skillWriteResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Body != "원래 본문\n" {
		t.Errorf("body = %q, want the restored one", got.Body)
	}
	if gotMethod != "skills.changed" {
		t.Errorf("notify method = %q", gotMethod)
	}
	// The payload too, not just the method: mcphost's vocabulary is
	// agent/external, and a revert made by the person at the keyboard is
	// neither. A listener that colours the two differently reads this field.
	p, ok := gotPayload.(skillChangedPayload)
	if !ok {
		t.Fatalf("payload = %+v (%T), want a skillChangedPayload", gotPayload, gotPayload)
	}
	// The literal, not the sourceWriter constant: asserting a constant
	// against itself passes however the constant is changed, which is no
	// assertion at all. "writer" is the value on the wire that the renderer
	// and mcphost both already agree on.
	if p.Source != "writer" || p.Name != "reverted" || p.Scope != "writer" {
		t.Errorf("payload = %+v, want a writer-sourced change to the writer-scope skill %q", p, "reverted")
	}
	back, err := st.Read(agentskills.ScopeWriter, "", "reverted")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if back.Body != "원래 본문\n" {
		t.Errorf("the file on disk is %q", back.Body)
	}
	// The restore is itself a version, so it can be undone in turn.
	versions, err = hist.List(ctx, agentskills.ScopeWriter, "", "reverted", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if len(versions) != 3 || versions[0].Reason != agentskills.ReasonEdited {
		t.Errorf("versions = %+v, want the restore recorded as an edit", versions)
	}
}

// A deleted skill's version row carries its last body precisely so it can be
// brought back. Restoring one recreates the skill, and the version row that
// records it says "created" — because that is what just happened.
func TestRestoreBringsBackADeletedSkill(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "goner", "description": "설명", "body": "마지막 본문\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(1000), nil)(ctx, params); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := DeleteSkill(st, hist, clockAt(2000), nil)(
		ctx, json.RawMessage(`{"scope":"writer","name":"goner"}`)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "goner", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	deleted := versions[0]
	if deleted.Reason != agentskills.ReasonDeleted {
		t.Fatalf("expected the newest row to be the deletion; got %+v", deleted)
	}

	raw, err := RestoreSkill(st, hist, clockAt(3000), nil)(
		ctx, json.RawMessage(`{"id":`+quote(deleted.ID)+`}`))
	if err != nil {
		t.Fatalf("restoring a deleted skill must recreate it: %v", err)
	}
	var got skillWriteResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Body != "마지막 본문\n" {
		t.Errorf("body = %q", got.Body)
	}
	// The history does not record the enabled flag at all (skill_snapshots
	// has no column for it), so a recreated skill must come back ON rather
	// than silently switched off by a zero value.
	if !got.Enabled {
		t.Error("a skill brought back from the dead must be enabled, not silently off")
	}
	versions, err = hist.List(ctx, agentskills.ScopeWriter, "", "goner", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if versions[0].Reason != agentskills.ReasonCreated {
		t.Errorf("newest reason = %q, want %q — the skill did not exist a moment ago",
			versions[0].Reason, agentskills.ReasonCreated)
	}
}

// The enabled flag is not in skill_snapshots, so a restore has nothing to
// say about it. It must therefore leave the live skill's flag alone rather
// than reading a column that does not exist as "off".
func TestRestoreKeepsTheLiveEnabledFlag(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	first, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "flagged", "description": "설명", "body": "원래\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(1000), nil)(ctx, first); err != nil {
		t.Fatalf("write: %v", err)
	}
	off, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "flagged", "description": "설명", "body": "고침\n", "enabled": false,
	})
	if _, err := WriteSkill(st, hist, clockAt(2000), nil)(ctx, off); err != nil {
		t.Fatalf("write: %v", err)
	}
	versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "flagged", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	oldest := versions[len(versions)-1]
	raw, err := RestoreSkill(st, hist, clockAt(3000), nil)(ctx, json.RawMessage(`{"id":`+quote(oldest.ID)+`}`))
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	var got skillWriteResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Enabled {
		t.Error("restoring a body must not switch a disabled skill back on — the history does not record the flag")
	}
}

// THE DESTRUCTIVE ONE. A deletion recorded in the same millisecond as the
// version being restored, and a skill standing at that name that this
// history never recorded — a file the writer made by hand in the skills
// folder, which the store is deliberately built to allow.
//
// The first version of this check compared created_at with a strict >, so a
// deletion sharing the version's millisecond did not count, and the restore
// wrote straight over a document nothing holds a copy of. A millisecond
// clock is not an ordering; the rowid is. Restoring must be refused here,
// and the file on disk must still be there afterwards.
func TestRestoreRefusesASkillTheHistoryNeverRecorded(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	original, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "recycled", "description": "설명", "body": "옛날 본문\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(1000), nil)(ctx, original); err != nil {
		t.Fatalf("write: %v", err)
	}
	versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "recycled", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	old := versions[0]

	// The same clock reading as the write above: the deletion lands in the
	// version's own millisecond, which is exactly what a fixed clock, a fast
	// path or an unlucky moment produces.
	if _, err := DeleteSkill(st, hist, clockAt(1000), nil)(
		ctx, json.RawMessage(`{"scope":"writer","name":"recycled"}`)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	deletion, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "recycled")
	if err != nil {
		t.Fatalf("newest: %v", err)
	}
	if deletion.CreatedAt != old.CreatedAt {
		t.Fatalf("this test is about a same-millisecond deletion; got %d vs %d",
			deletion.CreatedAt, old.CreatedAt)
	}

	// A different skill put at that name WITHOUT going through this surface,
	// so nothing recorded it: the history holds no copy, and overwriting it
	// would lose it for good.
	seedSkill(t, st, agentskills.Skill{
		Name: "recycled", Scope: agentskills.ScopeWriter,
		Description: "완전히 다른 스킬", Enabled: true, Body: "손으로 쓴 본문\n",
	}, 3000)

	_, err = RestoreSkill(st, hist, clockAt(4000), nil)(ctx, json.RawMessage(`{"id":`+quote(old.ID)+`}`))
	me := wantInvalidParams(t, err)
	if !strings.Contains(me.Message, "recycled") {
		t.Errorf("the message must name the skill; got %q", me.Message)
	}
	live, err := st.Read(agentskills.ScopeWriter, "", "recycled")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if live.Body != "손으로 쓴 본문\n" {
		t.Errorf("the skill standing at that name must be untouched; got %q", live.Body)
	}
}

// THE FALSE POSITIVE. Deleting a skill and restoring it back is an ordinary
// thing to do, and every version older than that deletion must still be
// restorable afterwards.
//
// The first version of this check refused them all, permanently, saying "a
// different skill now stands at that name" when it was the same skill,
// brought back a moment earlier by the writer sitting right there. A
// deletion row older than nothing at all is not evidence of a reused name.
func TestRestoreAllowsAnOlderVersionAfterADeleteAndRestoreCycle(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	for i, body := range []string{"원래 본문\n", "고친 본문\n"} {
		params, _ := json.Marshal(map[string]any{
			"scope": "writer", "name": "cycled", "description": "설명", "body": body,
		})
		if _, err := WriteSkill(st, hist, clockAt(1000+int64(i)*1000), nil)(ctx, params); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if _, err := DeleteSkill(st, hist, clockAt(3000), nil)(
		ctx, json.RawMessage(`{"scope":"writer","name":"cycled"}`)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	deletion, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "cycled")
	if err != nil {
		t.Fatalf("newest: %v", err)
	}
	// Bring it back, the way a writer who deleted the wrong thing does.
	if _, err := RestoreSkill(st, hist, clockAt(4000), nil)(
		ctx, json.RawMessage(`{"id":`+quote(deletion.ID)+`}`)); err != nil {
		t.Fatalf("restoring the deleted skill: %v", err)
	}

	versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "cycled", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	oldest := versions[len(versions)-1]
	if oldest.Skill.Body != "원래 본문\n" {
		t.Fatalf("expected the first write as the oldest row; got %q", oldest.Skill.Body)
	}
	raw, err := RestoreSkill(st, hist, clockAt(5000), nil)(ctx, json.RawMessage(`{"id":`+quote(oldest.ID)+`}`))
	if err != nil {
		t.Fatalf("a version older than the deletion must still be restorable: %v", err)
	}
	var got skillWriteResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Body != "원래 본문\n" {
		t.Errorf("body = %q, want the version that was asked for", got.Body)
	}
}

// A name that was deleted and then reused THROUGH THIS SURFACE is a restore
// that goes ahead, and this is a deliberate reversal of the first version's
// ruling.
//
// The old rule refused it to protect the document standing there — but that
// document is in the same log, one row down, carrying its whole body, and
// this restore records a row of its own. Nothing is lost: the writer who
// reverts too far reverts back. Refusing instead cost the far worse thing,
// since the same signal could not tell this apart from a delete-and-restore
// of the same skill, and every version older than any deletion became
// unrestorable forever. What is still refused is the case where the log
// genuinely cannot give the document back — see the test above.
func TestRestoreOverAReusedNameIsAllowedAndUndoable(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	original, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "recycled", "description": "설명", "body": "옛날 본문\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(1000), nil)(ctx, original); err != nil {
		t.Fatalf("write: %v", err)
	}
	first, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "recycled")
	if err != nil {
		t.Fatalf("newest: %v", err)
	}
	if _, err := DeleteSkill(st, hist, clockAt(2000), nil)(
		ctx, json.RawMessage(`{"scope":"writer","name":"recycled"}`)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	reused, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "recycled", "description": "완전히 다른 스킬", "body": "새 본문\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(3000), nil)(ctx, reused); err != nil {
		t.Fatalf("write: %v", err)
	}
	reusedRow, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "recycled")
	if err != nil {
		t.Fatalf("newest: %v", err)
	}

	if _, err := RestoreSkill(st, hist, clockAt(4000), nil)(
		ctx, json.RawMessage(`{"id":`+quote(first.ID)+`}`)); err != nil {
		t.Fatalf("the log holds both documents, so this must go through: %v", err)
	}
	live, err := st.Read(agentskills.ScopeWriter, "", "recycled")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if live.Body != "옛날 본문\n" {
		t.Errorf("body = %q, want the restored one", live.Body)
	}
	// And the document that was overwritten comes straight back, which is
	// the whole reason this is allowed rather than refused.
	if _, err := RestoreSkill(st, hist, clockAt(5000), nil)(
		ctx, json.RawMessage(`{"id":`+quote(reusedRow.ID)+`}`)); err != nil {
		t.Fatalf("undoing the restore: %v", err)
	}
	live, err = st.Read(agentskills.ScopeWriter, "", "recycled")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if live.Body != "새 본문\n" {
		t.Errorf("body = %q, want the overwritten document back", live.Body)
	}
}

// THE OVERWRITE. A SKILL.md over the 64 KiB read ceiling cannot be shown to
// the writer at all: skills.read refuses it, because ReadRaw hits the same
// ceiling Read does. The app therefore knows nothing about what that file
// holds — and a save or a restore at that name must not replace it.
//
// It could. The fallback that lets a broken SKILL.md be repaired was written
// as "any read failure that is not ErrNotFound means edit this", which is
// every read failure, not only a parse failure. A 600 KB document was
// replaced by an 81-byte one, with a version row carrying six runes as the
// only trace of it. Both writing methods take that door and both are covered
// here.
func TestASkillTooBigToReadIsNeverWrittenOver(t *testing.T) {
	// Well over the ceiling, and a real document rather than filler: this is
	// a skill someone grew past what this app will read, not a stray blob.
	huge := "---\nname: huge\ndescription: 아주 큰 문서\n---\n" + strings.Repeat("가", 200_000)

	t.Run("skills.write refuses it", func(t *testing.T) {
		ctx, st, hist, _ := realSkills(t)
		dir := writerDir(t, st)
		writeRawSkill(t, filepath.Join(dir, "huge"), huge)

		params, _ := json.Marshal(map[string]any{
			"scope": "writer", "name": "huge", "description": "짧게", "body": "짧은 본문\n",
		})
		_, err := WriteSkill(st, hist, clockAt(2000), nil)(ctx, params)
		me := wantInvalidParams(t, err)
		if !strings.Contains(me.Message, "huge") {
			t.Errorf("the message must name the skill; got %q", me.Message)
		}
		wantFileOnDisk(t, filepath.Join(dir, "huge", "SKILL.md"), huge)

		versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "huge", 10)
		if err != nil {
			t.Fatalf("history list: %v", err)
		}
		if len(versions) != 0 {
			t.Errorf("versions = %+v, want none — nothing was written", versions)
		}
	})

	// This subtest's name once claimed to prove skills.restore refuses via
	// inspectBeforeOverwrite's ReadRaw check specifically — the door that
	// stops "unreadable" from being silently written over. It didn't: a huge
	// body also refuses through refuseUnrecordedSkill's presenceUnparseable
	// branch, so the mutation that deletes the ReadRaw check entirely (making
	// inspectBeforeOverwrite report presenceUnparseable, no error, for every
	// file Read cannot open) still leaves restore refusing — just for the
	// wrong reason — and this subtest could not tell the difference. It now
	// asserts on inspectBeforeOverwrite directly, which is the only way to
	// pin that the ReadRaw check is what fires here, not a second refusal
	// downstream that happens to cover for it.
	t.Run("skills.restore refuses it, via the ReadRaw check specifically", func(t *testing.T) {
		ctx, st, hist, _ := realSkills(t)
		dir := writerDir(t, st)
		params, _ := json.Marshal(map[string]any{
			"scope": "writer", "name": "huge", "description": "설명", "body": "원래 본문\n",
		})
		if _, err := WriteSkill(st, hist, clockAt(1000), nil)(ctx, params); err != nil {
			t.Fatalf("write: %v", err)
		}
		row, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "huge")
		if err != nil {
			t.Fatalf("newest: %v", err)
		}
		// The writer grows the file past the ceiling in their own editor.
		writeRawSkill(t, filepath.Join(dir, "huge"), huge)

		// The door itself: inspectBeforeOverwrite must report this as
		// unreadable — an error, not presenceUnparseable with none — because
		// a file over the read ceiling fails ReadRaw too, and that failure is
		// what a caller ignoring refuseUnrecordedSkill's own refusal would
		// otherwise miss.
		_, presence, ibErr := inspectBeforeOverwrite(st, agentskills.ScopeWriter, "", "huge")
		if ibErr == nil {
			t.Fatalf("inspectBeforeOverwrite must refuse a file over the read ceiling itself "+
				"(via ReadRaw), not fall through to presenceUnparseable with no error; got presence=%v, err=nil",
				presence)
		}
		if presence != presenceUnreadable {
			t.Errorf("presence = %v, want presenceUnreadable", presence)
		}

		_, err = RestoreSkill(st, hist, clockAt(2000), nil)(ctx, json.RawMessage(`{"id":`+quote(row.ID)+`}`))
		wantInvalidParams(t, err)
		wantFileOnDisk(t, filepath.Join(dir, "huge", "SKILL.md"), huge)
	})
}

// A CRLF-only or trailing-newline-only rewrite of a store-written SKILL.md
// must not lock its own history out of every future restore.
//
// splitFrontmatter (skill.go) deliberately preserves a body's own line
// endings through Parse and Render, because the body is content this
// package does not own. Linetta ships Windows builds, and the skills folder
// is one a writer points other tools at — a file re-saved by Notepad, or
// any CRLF-by-default editor, turns every body "\n" into "\r\n" with no
// writer intent behind the change at all. Before recordsSkill normalized
// line endings, that rewrite made the live document compare unequal to the
// newest recorded row byte-for-byte, so refuseUnrecordedSkill read it as
// "an unrecorded document is standing here" and refused every restore at
// that name — falsely, since the log holds the same text. A save that only
// adds or drops the file's own trailing newline hit the same false refusal.
//
// The control case in the same test proves the fix did not go too far: a
// genuine one-word change to the live document — matching line endings and
// all — still refuses, because that IS a document the log has no copy of.
func TestRestoreOverALineEndingOrTrailingNewlineRewriteIsAllowed(t *testing.T) {
	rewriteFile := func(t *testing.T, path string, transform func(string) string) {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(transform(string(raw))), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	setup := func(t *testing.T) (ctx context.Context, st *agentskills.Store, hist *agentskills.History, dir, oldID string) {
		ctx, st, hist, _ = realSkills(t)
		dir = writerDir(t, st)
		old, err := json.Marshal(map[string]any{
			"scope": "writer", "name": "rewritten", "description": "설명", "body": "원래 본문\n",
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, err := WriteSkill(st, hist, clockAt(1000), nil)(ctx, old); err != nil {
			t.Fatalf("write v1: %v", err)
		}
		oldRow, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "rewritten")
		if err != nil {
			t.Fatalf("newest after v1: %v", err)
		}
		latest, err := json.Marshal(map[string]any{
			"scope": "writer", "name": "rewritten", "description": "설명", "body": "고친 본문\n",
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, err := WriteSkill(st, hist, clockAt(2000), nil)(ctx, latest); err != nil {
			t.Fatalf("write v2: %v", err)
		}
		return ctx, st, hist, dir, oldRow.ID
	}

	t.Run("a CRLF rewrite of the current document still allows restoring an older version", func(t *testing.T) {
		ctx, st, hist, dir, oldID := setup(t)
		path := filepath.Join(dir, "rewritten", "SKILL.md")
		// The writer's own editor re-saves the file with CRLF line endings —
		// no content change at all.
		rewriteFile(t, path, func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") })

		_, err := RestoreSkill(st, hist, clockAt(3000), nil)(ctx, json.RawMessage(`{"id":`+quote(oldID)+`}`))
		if err != nil {
			t.Fatalf("a CRLF-only rewrite must not block this restore: %v", err)
		}
		live, err := st.Read(agentskills.ScopeWriter, "", "rewritten")
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if live.Body != "원래 본문\n" {
			t.Errorf("body = %q, want the restored older version", live.Body)
		}
	})

	t.Run("a trailing-newline-only rewrite of the current document still allows restoring an older version", func(t *testing.T) {
		ctx, st, hist, dir, oldID := setup(t)
		path := filepath.Join(dir, "rewritten", "SKILL.md")
		// The writer's editor saves without a final newline — the body's
		// last "\n" is dropped, nothing else changes.
		rewriteFile(t, path, func(s string) string { return strings.TrimSuffix(s, "\n") })

		_, err := RestoreSkill(st, hist, clockAt(3000), nil)(ctx, json.RawMessage(`{"id":`+quote(oldID)+`}`))
		if err != nil {
			t.Fatalf("a trailing-newline-only rewrite must not block this restore: %v", err)
		}
		live, err := st.Read(agentskills.ScopeWriter, "", "rewritten")
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if live.Body != "원래 본문\n" {
			t.Errorf("body = %q, want the restored older version", live.Body)
		}
	})

	t.Run("a genuine one-word change to the current document still refuses", func(t *testing.T) {
		ctx, st, hist, dir, oldID := setup(t)
		path := filepath.Join(dir, "rewritten", "SKILL.md")
		// One word changed, line endings untouched: a document the log has
		// no copy of, unlike the two cases above.
		rewriteFile(t, path, func(s string) string {
			return strings.Replace(s, "고친 본문", "다른 본문", 1)
		})

		_, err := RestoreSkill(st, hist, clockAt(3000), nil)(ctx, json.RawMessage(`{"id":`+quote(oldID)+`}`))
		me := wantInvalidParams(t, err)
		if !strings.Contains(me.Message, "rewritten") {
			t.Errorf("the message must name the skill; got %q", me.Message)
		}
		live, err := st.Read(agentskills.ScopeWriter, "", "rewritten")
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if live.Body != "다른 본문\n" {
			t.Errorf("body = %q, want the hand-edited document untouched by the refused restore", live.Body)
		}
	})
}

// DOOR (a). "Is the last recorded row a deletion?" is disarmed by an
// ordinary delete-and-restore cycle: the restore records a `created` row, so
// the name's last row is no longer its deletion — and a folder the writer
// replaces BY HAND after that was overwritten by the next restore, though it
// existed in no row at all.
//
// Comparing what is on disk against the newest recorded document closes it,
// because the answer does not depend on which row kind came last.
func TestRestoreRefusesAHandReplacedSkillAfterADeleteAndRestoreCycle(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	for i, body := range []string{"원래 본문\n", "고친 본문\n"} {
		params, _ := json.Marshal(map[string]any{
			"scope": "writer", "name": "cycled", "description": "설명", "body": body,
		})
		if _, err := WriteSkill(st, hist, clockAt(1000+int64(i)*1000), nil)(ctx, params); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if _, err := DeleteSkill(st, hist, clockAt(3000), nil)(
		ctx, json.RawMessage(`{"scope":"writer","name":"cycled"}`)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	deletion, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "cycled")
	if err != nil {
		t.Fatalf("newest: %v", err)
	}
	if _, err := RestoreSkill(st, hist, clockAt(4000), nil)(
		ctx, json.RawMessage(`{"id":`+quote(deletion.ID)+`}`)); err != nil {
		t.Fatalf("bringing the deleted skill back: %v", err)
	}
	// The cycle is what disarms the old rule: the newest row is now a
	// creation, not the deletion.
	newest, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "cycled")
	if err != nil {
		t.Fatalf("newest: %v", err)
	}
	if newest.Reason != agentskills.ReasonCreated {
		t.Fatalf("this test is about a `created` last row; got %q", newest.Reason)
	}

	// And now the writer replaces the folder in their own file browser.
	// Nothing recorded this document; the history holds no copy of it.
	seedSkill(t, st, agentskills.Skill{
		Name: "cycled", Scope: agentskills.ScopeWriter,
		Description: "손으로 만든 스킬", Enabled: true, Body: "손으로 쓴 본문\n",
	}, 5000)

	versions, err := hist.List(ctx, agentskills.ScopeWriter, "", "cycled", 10)
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	oldest := versions[len(versions)-1]
	_, err = RestoreSkill(st, hist, clockAt(6000), nil)(ctx, json.RawMessage(`{"id":`+quote(oldest.ID)+`}`))
	me := wantInvalidParams(t, err)
	if !strings.Contains(me.Message, "cycled") {
		t.Errorf("the message must name the skill; got %q", me.Message)
	}
	live, err := st.Read(agentskills.ScopeWriter, "", "cycled")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if live.Body != "손으로 쓴 본문\n" {
		t.Errorf("body = %q, want the hand-made document untouched", live.Body)
	}
}

// DOOR (b). Deleting a skill whose SKILL.md does not parse records nothing —
// there was no readable document to snapshot, and DeleteSkill says so with
// versioned:false — which leaves an old `created` row as the last one for
// that name. The rule that read only that row was therefore disarmed at
// exactly the name the app had just emptied, and whatever the writer put
// there next was overwritten with no copy of it anywhere.
func TestRestoreRefusesAtANameWhoseBrokenSkillsDeleteRecordedNothing(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	dir := writerDir(t, st)
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "busted", "description": "설명", "body": "원래 본문\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(1000), nil)(ctx, params); err != nil {
		t.Fatalf("write: %v", err)
	}
	created, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "busted")
	if err != nil {
		t.Fatalf("newest: %v", err)
	}

	// The writer breaks the frontmatter in their own editor, then removes
	// the skill from Settings. The removal records nothing.
	writeRawSkill(t, filepath.Join(dir, "busted"), "name: no-fences\ndescription: 깨졌다\n\n본문\n")
	rawResult, err := DeleteSkill(st, hist, clockAt(2000), nil)(
		ctx, json.RawMessage(`{"scope":"writer","name":"busted"}`))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	var deleted deleteSkillResult
	if err := json.Unmarshal(rawResult, &deleted); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if deleted.Versioned {
		t.Fatal("this test is about a delete that recorded nothing")
	}
	newest, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "busted")
	if err != nil {
		t.Fatalf("newest: %v", err)
	}
	if newest.Reason != agentskills.ReasonCreated {
		t.Fatalf("this test is about a stale `created` last row; got %q", newest.Reason)
	}

	// The writer starts something new at the freed name, by hand.
	seedSkill(t, st, agentskills.Skill{
		Name: "busted", Scope: agentskills.ScopeWriter,
		Description: "완전히 다른 스킬", Enabled: true, Body: "새로 쓴 본문\n",
	}, 3000)

	_, err = RestoreSkill(st, hist, clockAt(4000), nil)(ctx, json.RawMessage(`{"id":`+quote(created.ID)+`}`))
	me := wantInvalidParams(t, err)
	if !strings.Contains(me.Message, "busted") {
		t.Errorf("the message must name the skill; got %q", me.Message)
	}
	live, err := st.Read(agentskills.ScopeWriter, "", "busted")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if live.Body != "새로 쓴 본문\n" {
		t.Errorf("body = %q, want the new document untouched", live.Body)
	}
}

// A file that does not parse can be a copy of no version row: every row came
// from Store.Write, which renders, and rendered output always parses. So the
// text sitting there is recorded nowhere and a restore must not write over
// it — the writer opens it (skills.read hands it back verbatim), keeps what
// it says, and then saves or deletes.
func TestRestoreRefusesOverAFileThatDoesNotParse(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	dir := writerDir(t, st)
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "halfway", "description": "설명", "body": "원래 본문\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(1000), nil)(ctx, params); err != nil {
		t.Fatalf("write: %v", err)
	}
	row, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "halfway")
	if err != nil {
		t.Fatalf("newest: %v", err)
	}
	const broken = "name: no-fences\ndescription: 깨졌다\n\n손으로 쓴 본문\n"
	writeRawSkill(t, filepath.Join(dir, "halfway"), broken)

	_, err = RestoreSkill(st, hist, clockAt(2000), nil)(ctx, json.RawMessage(`{"id":`+quote(row.ID)+`}`))
	me := wantInvalidParams(t, err)
	if !strings.Contains(me.Message, "halfway") {
		t.Errorf("the message must name the skill; got %q", me.Message)
	}
	wantFileOnDisk(t, filepath.Join(dir, "halfway", "SKILL.md"), broken)
}

// The check needs the log, and a log that cannot be read is the one failure
// here that is nobody's request. It must not become "allow the write": that
// is the destructive direction, taken for a reason that has nothing to do
// with what was asked. It comes back as the internal error it is, which a
// retry clears, and the document on disk is still there.
func TestRestoreReportsAHistoryItCannotReadRatherThanOverwriting(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	params, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "guarded", "description": "설명", "body": "원래 본문\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(1000), nil)(ctx, params); err != nil {
		t.Fatalf("write: %v", err)
	}
	row, err := hist.Newest(ctx, agentskills.ScopeWriter, "", "guarded")
	if err != nil {
		t.Fatalf("newest: %v", err)
	}
	second, _ := json.Marshal(map[string]any{
		"scope": "writer", "name": "guarded", "description": "설명", "body": "고친 본문\n",
	})
	if _, err := WriteSkill(st, hist, clockAt(2000), nil)(ctx, second); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err = RestoreSkill(st, unreadableNewest{hist}, clockAt(3000), nil)(
		ctx, json.RawMessage(`{"id":`+quote(row.ID)+`}`))
	if err == nil {
		t.Fatal("a restore that cannot be checked must not proceed")
	}
	if me := methodError(t, err); me.Code != rpc.CodeInternalError {
		t.Errorf("code = %d, want %d — a database that cannot be read is not a bad request",
			me.Code, rpc.CodeInternalError)
	}
	live, err := st.Read(agentskills.ScopeWriter, "", "guarded")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if live.Body != "고친 본문\n" {
		t.Errorf("body = %q, want the live document untouched", live.Body)
	}
}

func TestRestoreRefusesAnUnknownVersion(t *testing.T) {
	ctx, st, hist, _ := realSkills(t)
	_, err := RestoreSkill(st, hist, clockAt(1), nil)(ctx, json.RawMessage(`{"id":"no-such-version"}`))
	me := wantInvalidParams(t, err)
	if !strings.Contains(me.Message, "no-such-version") {
		t.Errorf("the message must name the id; got %q", me.Message)
	}
}

// ---- the three mutations, when they refuse --------------------------------

// A refused mutation must not tell the UI something changed. #97 shipped
// this test for its own surface (TestEditSkillDoesNotNotifyOnFailure is the
// mcphost twin) and it belongs here for the same reason: skills.changed is
// what makes every other window reload, and a notification for a save that
// did not happen sends them all to re-read a file nobody touched — or, on a
// delete that was refused, to drop a skill from the list that is still
// there.
//
// All three mutations are covered, each through a refusal it can actually
// reach.
func TestARefusedSkillMutationEmitsNothing(t *testing.T) {
	for name, tc := range map[string]struct {
		handler func(*agentskills.Store, *agentskills.History, func(string, any)) rpc.Handler
		params  string
	}{
		"write refuses an invisible character": {
			handler: func(st *agentskills.Store, h *agentskills.History, n func(string, any)) rpc.Handler {
				return WriteSkill(st, h, clockAt(1000), n)
			},
			params: `{"scope":"writer","name":"pasted","description":"설명","body":"보이지 않는 문자​가 있다"}`,
		},
		"delete refuses a name that is not there": {
			handler: func(st *agentskills.Store, h *agentskills.History, n func(string, any)) rpc.Handler {
				return DeleteSkill(st, h, clockAt(1000), n)
			},
			params: `{"scope":"writer","name":"never-existed"}`,
		},
		"restore refuses an unknown version": {
			handler: func(st *agentskills.Store, h *agentskills.History, n func(string, any)) rpc.Handler {
				return RestoreSkill(st, h, clockAt(1000), n)
			},
			params: `{"id":"no-such-version"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, st, hist, _ := realSkills(t)
			notified := ""
			notify := func(m string, _ any) { notified = m }
			if _, err := tc.handler(st, hist, notify)(ctx, json.RawMessage(tc.params)); err == nil {
				t.Fatal("this case must refuse, or it is testing nothing")
			}
			if notified != "" {
				t.Errorf("a refused mutation emitted %q; nothing changed", notified)
			}
		})
	}
}

// ---- helpers --------------------------------------------------------------

// writerDir is <home>/skills, where a test puts a SKILL.md the store never
// wrote.
func writerDir(t *testing.T, st *agentskills.Store) string {
	t.Helper()
	dir, err := st.Dir(agentskills.ScopeWriter, "")
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	return dir
}

// wantFileOnDisk asserts a file was left exactly as it was. It is the half
// of every "must not overwrite" case that the refusal alone does not prove.
func wantFileOnDisk(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s is %d bytes, want the original %d — it was written over",
			path, len(got), len(want))
	}
}

// unreadableNewest is a real history whose "what is standing here now" query
// fails: rows still come back by id, so a restore resolves its target and
// then finds it cannot check what it is about to overwrite.
type unreadableNewest struct{ *agentskills.History }

func (unreadableNewest) Newest(context.Context, agentskills.Scope, string, string) (agentskills.Version, error) {
	return agentskills.Version{}, errors.New("the database is not readable")
}

func clockAt(now int64) func() int64 { return func() int64 { return now } }

// writeRawSkill puts a SKILL.md on disk without going through Store.Write,
// which is the only way to produce the states a writer's own editor can:
// a file with no frontmatter, or a body the guard would have refused.
func writeRawSkill(t *testing.T, dir, raw string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(raw), 0o600); err != nil {
		t.Fatalf("write %s: %v", dir, err)
	}
}

func skillName(i int) string {
	return "skill-" + string(rune('a'+i/26)) + string(rune('a'+i%26))
}

// failingHistory is the one stub in this file: no real *History can be
// talked into refusing a write it accepts the shape of, and the state it
// leaves — a file newer than its history — is the one worth reporting.
type failingHistory struct{}

func (failingHistory) Record(context.Context, agentskills.Skill, string, int64) error {
	return errors.New("the database is not writable")
}

func (failingHistory) List(context.Context, agentskills.Scope, string, string, int) ([]agentskills.Version, error) {
	return nil, errors.New("the database is not readable")
}

func (failingHistory) Newest(context.Context, agentskills.Scope, string, string) (agentskills.Version, error) {
	return agentskills.Version{}, errors.New("the database is not readable")
}

func (failingHistory) Get(context.Context, string) (agentskills.Version, error) {
	return agentskills.Version{}, errors.New("the database is not readable")
}
