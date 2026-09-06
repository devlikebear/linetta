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
	notify := func(m string, _ any) { gotMethod = m }
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

// A name can be deleted and reused. The old version row still addresses that
// name, but the skill standing there now is a DIFFERENT document, and
// restoring over it would destroy work the writer never asked to lose. So
// the restore is refused, with the sentence that says how to go through with
// it deliberately.
func TestRestoreRefusesWhenADifferentSkillNowStandsAtThatName(t *testing.T) {
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

	_, err = RestoreSkill(st, hist, clockAt(4000), nil)(ctx, json.RawMessage(`{"id":`+quote(old.ID)+`}`))
	me := wantInvalidParams(t, err)
	if !strings.Contains(me.Message, "recycled") {
		t.Errorf("the message must name the skill; got %q", me.Message)
	}
	live, err := st.Read(agentskills.ScopeWriter, "", "recycled")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if live.Body != "새 본문\n" {
		t.Errorf("the skill standing at that name must be untouched; got %q", live.Body)
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

// ---- helpers --------------------------------------------------------------

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

func (failingHistory) Get(context.Context, string) (agentskills.Version, error) {
	return agentskills.Version{}, errors.New("the database is not readable")
}
