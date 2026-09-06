//go:build !mobile

package mcphost

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newSkillDeps builds the two collaborators the skill tools read from — a
// Store over a throwaway home and a History over a throwaway database — plus
// the activity log and a real work to scope against. The handlers are driven
// directly, with no server, the way tools_memory_test.go drives editMemory.
func newSkillDeps(t *testing.T) (context.Context, ToolDeps, string, string) {
	t.Helper()
	ctx, d, projectID, home, _ := newSkillDepsWithStore(t)
	return ctx, d, projectID, home
}

// newSkillDepsWithStore is newSkillDeps plus the open database itself, for
// the one test that has to break the history behind a live store.
func newSkillDepsWithStore(t *testing.T) (context.Context, ToolDeps, string, string, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := project.NewRepo(st).Create(ctx, 42, project.NewInput{
		Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	home := t.TempDir()
	d := ToolDeps{
		Skills:       agentskills.NewStore(home),
		SkillHistory: agentskills.NewHistory(st.DB()),
		Activity:     NewActivityRepo(st.DB()),
		Projects:     project.NewRepo(st),
		Source:       SourceAgent,
		Clock:        func() int64 { return 42 },
	}
	return ctx, d, p.ID, home, st
}

// mustCreateSkill is the happy path every other test starts from.
func mustCreateSkill(t *testing.T, ctx context.Context, d ToolDeps, in editSkillInput) editSkillOutput {
	t.Helper()
	res, out, err := d.editSkill(ctx, nil, in)
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("editSkill %+v: err=%v res=%q", in, err, firstText(res))
	}
	return out
}

func TestEditSkillCreatesAndReadSkillReadsItBack(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	out := mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "fight-scenes",
		Description: "액션 장면을 쓸 때", Body: "# 액션\n\n짧은 문장으로.",
	})
	if out.Name != "fight-scenes" || out.Scope != "writer" {
		t.Errorf("out = %+v", out)
	}
	if out.BodyRunes == 0 || out.BodyBudget != agentskills.MaxBodyRunes {
		t.Errorf("out must report the body budget so the agent can manage its own space: %+v", out)
	}
	if !out.Enabled {
		t.Error("a new skill defaults to enabled")
	}

	res, got, err := d.readSkill(ctx, nil, readSkillInput{Scope: "writer", Name: "fight-scenes"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("readSkill: err=%v res=%q", err, firstText(res))
	}
	if got.Body != "# 액션\n\n짧은 문장으로." {
		t.Errorf("Body = %q", got.Body)
	}
	if got.Description != "액션 장면을 쓸 때" {
		t.Errorf("Description = %q", got.Description)
	}
}

func TestEditSkillCreatesAWorkScopedSkill(t *testing.T) {
	ctx, d, projectID, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "work", ProjectID: projectID, Name: "minjun-voice",
		Description: "민준의 말투", Body: "존댓말을 쓴다.",
	})
	res, got, err := d.readSkill(ctx, nil, readSkillInput{
		Scope: "work", ProjectID: projectID, Name: "minjun-voice"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("readSkill: err=%v res=%q", err, firstText(res))
	}
	if got.ProjectID != projectID || got.Scope != "work" {
		t.Errorf("out = %+v", got)
	}

	// The same name in the writer scope is a different skill entirely.
	res, _, err = d.readSkill(ctx, nil, readSkillInput{Scope: "writer", Name: "minjun-voice"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Error("a work skill must not be reachable through the writer scope")
	}
}

func TestEditSkillPatchesByFind(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing",
		Description: "속도 조절", Body: "짧은 문장. 긴 문장.",
	})
	res, out, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "patch", Scope: "writer", Name: "pacing", Find: "긴 문장", Replace: "중간 길이 문장",
	})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("patch: err=%v res=%q", err, firstText(res))
	}
	if out.Body != "짧은 문장. 중간 길이 문장." {
		t.Fatalf("Body = %q — find/replace must edit in place, not rewrite the document", out.Body)
	}
}

func TestEditSkillPatchesWholesale(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing",
		Description: "속도 조절", Body: "옛 내용",
	})
	res, out, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "patch", Scope: "writer", Name: "pacing", Body: "새 내용",
	})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("patch: err=%v res=%q", err, firstText(res))
	}
	if out.Body != "새 내용" {
		t.Errorf("Body = %q", out.Body)
	}
	if out.Description != "속도 조절" {
		t.Errorf("a body-only patch must leave the description alone; got %q", out.Description)
	}
}

// A patch that only flips enabled, or only rewrites the one-line description,
// is a legitimate edit: the description is what decides whether the skill is
// ever reached for, and disabling is how an agent retires a skill without
// destroying it.
func TestEditSkillPatchesMetadataOnly(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing",
		Description: "속도 조절", Body: "본문",
	})
	off := false
	res, out, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "patch", Scope: "writer", Name: "pacing", Description: "더 정확한 설명", Enabled: &off,
	})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("patch: err=%v res=%q", err, firstText(res))
	}
	if out.Enabled {
		t.Error("Enabled = true after a patch that turned it off")
	}
	if out.Description != "더 정확한 설명" || out.Body != "본문" {
		t.Errorf("out = %+v", out)
	}
}

func TestEditSkillDeletes(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing",
		Description: "속도 조절", Body: "본문",
	})
	res, _, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "delete", Scope: "writer", Name: "pacing"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("delete: err=%v res=%q", err, firstText(res))
	}
	res, _, err = d.readSkill(ctx, nil, readSkillInput{Scope: "writer", Name: "pacing"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Error("the deleted skill is still readable")
	}
}

// Recoverable failures come back as a tool RESULT with a nil Go error, so the
// model reads the message and retries. A transport error would end the turn.
func TestEditSkillFailuresAreToolErrorsNotTransportErrors(t *testing.T) {
	ctx, d, projectID, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing", Description: "속도", Body: "본문"})

	cases := map[string]editSkillInput{
		"unknown scope":     {Action: "create", Scope: "nonsense", Name: "x", Description: "d", Body: "b"},
		"unknown action":    {Action: "rewrite", Scope: "writer", Name: "pacing", Body: "b"},
		"unknown skill":     {Action: "patch", Scope: "writer", Name: "no-such-skill", Body: "b"},
		"delete unknown":    {Action: "delete", Scope: "writer", Name: "no-such-skill"},
		"not a slug":        {Action: "create", Scope: "writer", Name: "Fight Scenes", Description: "d", Body: "b"},
		"empty name":        {Action: "create", Scope: "writer", Name: "  ", Description: "d", Body: "b"},
		"over the cap":      {Action: "create", Scope: "writer", Name: "huge", Description: "d", Body: strings.Repeat("가", agentskills.MaxBodyRunes+1)},
		"invisible char":    {Action: "create", Scope: "writer", Name: "sneaky", Description: "d", Body: "안녕​"},
		"work, no work":     {Action: "create", Scope: "work", Name: "x", Description: "d", Body: "b"},
		"writer + work id":  {Action: "create", Scope: "writer", Name: "x", ProjectID: projectID, Description: "d", Body: "b"},
		"unknown work":      {Action: "create", Scope: "work", ProjectID: "no-such-work", Name: "x", Description: "d", Body: "b"},
		"create, no body":   {Action: "create", Scope: "writer", Name: "empty-body", Description: "d"},
		"create, no descr":  {Action: "create", Scope: "writer", Name: "no-descr", Body: "b"},
		"create over exist": {Action: "create", Scope: "writer", Name: "pacing", Description: "d", Body: "b"},
		"patch, nothing":    {Action: "patch", Scope: "writer", Name: "pacing"},
		"find not found":    {Action: "patch", Scope: "writer", Name: "pacing", Find: "없음", Replace: "x"},
		"find and body":     {Action: "patch", Scope: "writer", Name: "pacing", Find: "본", Replace: "x", Body: "b"},
	}
	for name, in := range cases {
		res, _, err := d.editSkill(ctx, nil, in)
		if err != nil {
			t.Errorf("%s: got a transport error %v; want a tool error result", name, err)
		}
		if res == nil || !res.IsError {
			t.Errorf("%s: want an error result, got %+v", name, res)
		}
	}
}

// find must name exactly one place. Two matches is the agent being vague, and
// silently editing the first would corrupt a document it cannot see.
func TestEditSkillRefusesAnAmbiguousFind(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing", Description: "속도", Body: "짧게. 짧게."})
	res, _, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "patch", Scope: "writer", Name: "pacing", Find: "짧게", Replace: "길게"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("an ambiguous find must be refused, not applied to the first match")
	}
	if msg := firstText(res); !strings.Contains(msg, "2") {
		t.Errorf("the message must say how many times it matched; got %q", msg)
	}
}

func TestEditSkillRefusesTheFortyFirstSkill(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	for i := range agentskills.MaxSkillsPerScope {
		mustCreateSkill(t, ctx, d, editSkillInput{
			Action: "create", Scope: "writer", Name: fmt.Sprintf("skill-%d", i),
			Description: "d", Body: "b"})
	}
	res, _, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "create", Scope: "writer", Name: "one-too-many", Description: "d", Body: "b"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("the 41st skill must be refused")
	}
	if msg := firstText(res); !strings.Contains(msg, "40") {
		t.Errorf("the message must name the cap; got %q", msg)
	}
}

// A writer skill is global — it steers every work. A client the writer pinned
// to one work must not be able to write one, or the pin means nothing.
func TestEditSkillRefusesAWriterSkillOnAPinnedServer(t *testing.T) {
	ctx, d, projectID, _ := newSkillDeps(t)
	d.Source = "" // external, the only kind of client a pin applies to
	d.Settings = newRestrictedSettings(t, projectID)

	res, _, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "create", Scope: "writer", Name: "global-steer", Description: "d", Body: "b"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("a pinned client must not be able to write a global writer skill")
	}
	if msg := firstText(res); !strings.Contains(msg, "restricted") || !strings.Contains(msg, "work") {
		t.Errorf("the message must say it is scoped and what to use instead; got %q", msg)
	}
	if _, err := d.Skills.Read(agentskills.ScopeWriter, "", "global-steer"); err == nil {
		t.Error("the refused create still wrote a skill to disk")
	}

	// The pinned client keeps its own work's skills.
	res, _, err = d.editSkill(ctx, nil, editSkillInput{
		Action: "create", Scope: "work", ProjectID: projectID, Name: "minjun-voice",
		Description: "민준의 말투", Body: "존댓말"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("pinned work skill: err=%v res=%q", err, firstText(res))
	}
}

// The built-in agent is never pinned: mcp_project_id restricts external
// clients, and the panel's scope is whichever work it is open on.
func TestEditSkillLetsTheBuiltInAgentWriteAWriterSkillWhileAPinIsSet(t *testing.T) {
	ctx, d, projectID, _ := newSkillDeps(t)
	d.Settings = newRestrictedSettings(t, projectID)
	if d.Source != SourceAgent {
		t.Fatalf("Source = %q, want the agent", d.Source)
	}
	res, out, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "create", Scope: "writer", Name: "global-steer", Description: "d", Body: "본문"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("agent writer skill: err=%v res=%q", err, firstText(res))
	}
	if out.Body != "본문" {
		t.Errorf("out = %+v", out)
	}
}

func TestEditSkillNotifiesWithItsSource(t *testing.T) {
	ctx, d, projectID, _ := newSkillDeps(t)
	var gotMethod string
	var gotParams any
	d.Notify = func(method string, params any) { gotMethod, gotParams = method, params }
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "work", ProjectID: projectID, Name: "minjun-voice",
		Description: "민준의 말투", Body: "존댓말"})
	if gotMethod != "skills.changed" {
		t.Fatalf("method = %q, want skills.changed", gotMethod)
	}
	p, ok := gotParams.(skillChangedPayload)
	if !ok {
		t.Fatalf("payload type %T", gotParams)
	}
	if p.Source != SourceAgent || p.Scope != "work" || p.ProjectID != projectID || p.Name != "minjun-voice" {
		t.Errorf("payload = %+v", p)
	}
}

// A failed edit must not tell the UI something changed.
func TestEditSkillDoesNotNotifyOnFailure(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	notified := false
	d.Notify = func(string, any) { notified = true }
	if _, _, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "create", Scope: "nonsense", Name: "x", Description: "d", Body: "b"}); err != nil {
		t.Fatalf("editSkill: %v", err)
	}
	if notified {
		t.Error("a refused edit must not emit skills.changed")
	}
}

func TestEditSkillScopesTheActivityEntry(t *testing.T) {
	ctx, d, projectID, _ := newSkillDeps(t)
	h := record(d, "linetta_edit_skill", d.editSkill)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "linetta_edit_skill"}}
	if _, _, err := h(ctx, req, editSkillInput{
		Action: "create", Scope: "work", ProjectID: projectID, Name: "minjun-voice",
		Description: "민준의 말투", Body: "존댓말"}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	rows, err := d.Activity.List(ctx, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 activity row, got %d", len(rows))
	}
	if rows[0].ProjectID != projectID || rows[0].TargetID != "minjun-voice" ||
		rows[0].Tool != "linetta_edit_skill" || !rows[0].OK {
		t.Errorf("row = %+v — the writer must be able to see which skill was touched", rows[0])
	}
}

// Every write is versioned, including the deletion — the row marked deleted
// carries the last body, so a writer can restore straight from it.
func TestEditSkillRecordsAVersionForEveryWrite(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing", Description: "속도", Body: "첫 본문"})
	if _, _, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "patch", Scope: "writer", Name: "pacing", Body: "둘째 본문"}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if _, _, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "delete", Scope: "writer", Name: "pacing"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	versions, err := d.SkillHistory.List(ctx, agentskills.ScopeWriter, "", "pacing", 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("want 3 versions (created, edited, deleted), got %d", len(versions))
	}
	if versions[0].Reason != agentskills.ReasonDeleted {
		t.Errorf("newest reason = %q, want %q", versions[0].Reason, agentskills.ReasonDeleted)
	}
	if versions[0].Skill.Body != "둘째 본문" {
		t.Errorf("the deleted row must carry the last body so it is restorable; got %q", versions[0].Skill.Body)
	}
	if versions[2].Reason != agentskills.ReasonCreated {
		t.Errorf("oldest reason = %q, want %q", versions[2].Reason, agentskills.ReasonCreated)
	}
}

// Store.Read deliberately does not guard — a writer must be able to open a
// broken skill to repair it. This tool is a path to a model, so it guards.
func TestReadSkillRefusesABodyTheGuardRejects(t *testing.T) {
	ctx, d, _, home := newSkillDeps(t)
	dir := filepath.Join(home, "skills", "tainted")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := agentskills.Render(agentskills.Skill{
		Name: "tainted", Description: "d", Author: agentskills.AuthorWriter, Enabled: true,
		Body: "보이지 않는 글자​가 있다",
	})
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(raw), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	// The store itself still hands it over — that is what makes it fixable.
	if _, err := d.Skills.Read(agentskills.ScopeWriter, "", "tainted"); err != nil {
		t.Fatalf("Store.Read must not guard: %v", err)
	}
	res, _, err := d.readSkill(ctx, nil, readSkillInput{Scope: "writer", Name: "tainted"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("a body the guard rejects must not reach a model through this tool")
	}
	// agentmemory.ScreenInvisible is shared by two document types, and this
	// is the first path that puts its message in front of a model. It must
	// name the code point to fix and must not call a skill a memory.
	msg := firstText(res)
	if !strings.Contains(msg, "U+200B") {
		t.Errorf("the refusal must name the offending code point; got %q", msg)
	}
	if strings.Contains(msg, "in a memory") {
		t.Errorf("the refusal names the wrong document type — this is a skill, not a memory; got %q", msg)
	}
	if !strings.Contains(msg, "skill") {
		t.Errorf("the refusal must say which document it is about; got %q", msg)
	}
}

func TestReadSkillFailuresAreToolErrorsNotTransportErrors(t *testing.T) {
	ctx, d, projectID, _ := newSkillDeps(t)
	cases := map[string]readSkillInput{
		"unknown scope":    {Scope: "nonsense", Name: "x"},
		"unknown skill":    {Scope: "writer", Name: "no-such-skill"},
		"not a slug":       {Scope: "writer", Name: "Fight Scenes"},
		"empty name":       {Scope: "writer", Name: "  "},
		"work, no work":    {Scope: "work", Name: "x"},
		"writer + work id": {Scope: "writer", Name: "x", ProjectID: projectID},
		"unknown work":     {Scope: "work", ProjectID: "no-such-work", Name: "x"},
	}
	for name, in := range cases {
		res, _, err := d.readSkill(ctx, nil, in)
		if err != nil {
			t.Errorf("%s: got a transport error %v; want a tool error result", name, err)
		}
		if res == nil || !res.IsError {
			t.Errorf("%s: want an error result, got %+v", name, res)
		}
	}
}

func TestReadSkillScopesTheActivityEntry(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing", Description: "속도", Body: "본문"})
	h := record(d, "linetta_read_skill", d.readSkill)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "linetta_read_skill"}}
	if _, _, err := h(ctx, req, readSkillInput{Scope: "writer", Name: "pacing"}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	rows, err := d.Activity.List(ctx, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 activity row, got %d", len(rows))
	}
	if rows[0].TargetID != "pacing" || rows[0].Tool != "linetta_read_skill" || !rows[0].OK {
		t.Errorf("row = %+v", rows[0])
	}
}

// A read_only server hands out reading but not writing. That is the point of
// the split, so it is asserted rather than left to registration order.
func TestSkillToolsSitOnTheRightSideOfTheReadWriteSplit(t *testing.T) {
	if !contains(WriteToolNames, "linetta_edit_skill") {
		t.Error("linetta_edit_skill must be in WriteToolNames — a read_only server must not hand out a way to rewrite the writer's skills")
	}
	if contains(ReadToolNames, "linetta_edit_skill") {
		t.Error("linetta_edit_skill must not also be a read tool")
	}
	if !contains(ReadToolNames, "linetta_read_skill") {
		t.Error("linetta_read_skill must be in ReadToolNames — reading a skill is safe on a read_only server")
	}
	if contains(WriteToolNames, "linetta_read_skill") {
		t.Error("linetta_read_skill must not also be a write tool")
	}
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// Nothing in this build has a skills store when the database is closed. The
// tools refuse rather than panicking, the way linetta_edit_memory does.
func TestSkillToolsRefuseWhenTheStoreIsMissing(t *testing.T) {
	ctx := context.Background()
	var d ToolDeps
	res, _, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "create", Scope: "writer", Name: "x", Description: "d", Body: "b"})
	if err != nil || res == nil || !res.IsError {
		t.Errorf("editSkill: err=%v res=%+v", err, res)
	}
	res, _, err = d.readSkill(ctx, nil, readSkillInput{Scope: "writer", Name: "x"})
	if err != nil || res == nil || !res.IsError {
		t.Errorf("readSkill: err=%v res=%+v", err, res)
	}
}

// A patch carrying replace but no find is a half-formed edit: the agent named
// the new text and forgot to say what it replaces. Before this it "succeeded"
// — replace was dropped on the floor, any description alongside it was
// applied, and the agent was told the edit landed.
func TestEditSkillRefusesReplaceWithNoFind(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing", Description: "속도", Body: "옛 본문"})

	// With a description alongside it, which is what made this silent: the
	// description gave the call something to change, so it reported success.
	res, _, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "patch", Scope: "writer", Name: "pacing",
		Replace: "새 본문", Description: "더 정확한 설명"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("replace with no find must be refused, not silently dropped")
	}
	if msg := firstText(res); !strings.Contains(msg, "find") {
		t.Errorf("the message must name what is missing; got %q", msg)
	}
	// And nothing may have changed on the way to that refusal.
	got, err := d.Skills.Read(agentskills.ScopeWriter, "", "pacing")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Body != "옛 본문" || got.Description != "속도" {
		t.Errorf("the refused patch still wrote: %+v", got)
	}

	// Without a description it is the same refusal, not the older "nothing to
	// change" — which was actively misleading, because replace WAS given.
	res, _, err = d.editSkill(ctx, nil, editSkillInput{
		Action: "patch", Scope: "writer", Name: "pacing", Replace: "새 본문"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("replace with no find must be refused")
	}
	if msg := firstText(res); !strings.Contains(msg, "find") {
		t.Errorf("the message must name what is missing; got %q", msg)
	}
}

// Whether a patch changes anything is decided on the rendered document, so
// every no-op is refused the same way — a find equal to its replace, a body
// identical to the one on disk, and a re-asserted enabled flag alike. The
// alternative is a version row in the writer's history that reverts to
// itself.
func TestEditSkillRefusesEveryNoOpPatchAlike(t *testing.T) {
	ctx, d, _, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing", Description: "속도", Body: "짧은 문장."})
	on := true

	cases := map[string]editSkillInput{
		"find equals replace":  {Action: "patch", Scope: "writer", Name: "pacing", Find: "짧은", Replace: "짧은"},
		"same body wholesale":  {Action: "patch", Scope: "writer", Name: "pacing", Body: "짧은 문장."},
		"same description":     {Action: "patch", Scope: "writer", Name: "pacing", Description: "속도"},
		"enabled re-asserted":  {Action: "patch", Scope: "writer", Name: "pacing", Enabled: &on},
		"nothing at all":       {Action: "patch", Scope: "writer", Name: "pacing"},
		"all of them together": {Action: "patch", Scope: "writer", Name: "pacing", Body: "짧은 문장.", Description: "속도", Enabled: &on},
	}
	for name, in := range cases {
		res, _, err := d.editSkill(ctx, nil, in)
		if err != nil {
			t.Errorf("%s: got a transport error %v; want a tool error result", name, err)
			continue
		}
		if res == nil || !res.IsError {
			t.Errorf("%s: a no-op patch must be refused, got %+v", name, res)
		}
	}

	// One version row: the create. No refused no-op added one.
	versions, err := d.SkillHistory.List(ctx, agentskills.ScopeWriter, "", "pacing", 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(versions) != 1 {
		t.Errorf("want only the created row, got %d versions — a no-op patch wrote history", len(versions))
	}
}

// The disk write succeeded and is not rolled back — undoing it would lose the
// agent's work. But the result has to say the version row is missing, or the
// only trace of an unrevertible edit is a log line nobody reads.
func TestEditSkillReportsAWriteItCouldNotVersion(t *testing.T) {
	ctx, d, _, _, st := newSkillDepsWithStore(t)
	out := mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "pacing", Description: "속도", Body: "본문"})
	if !out.Versioned {
		t.Fatal("a normal write is versioned")
	}

	// Close the database behind the live store: the filesystem write still
	// lands, History.Record cannot.
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	res, out, err := d.editSkill(ctx, nil, editSkillInput{
		Action: "create", Scope: "writer", Name: "unversioned", Description: "d", Body: "본문"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("the write landed on disk, so it must not be reported as a failure: %q", firstText(res))
	}
	if out.Versioned {
		t.Error("versioned must be false when the version row could not be recorded")
	}
	// The skill really is on disk — that is why this is reported rather than
	// rolled back.
	if _, err := d.Skills.Read(agentskills.ScopeWriter, "", "unversioned"); err != nil {
		t.Errorf("the skill must still be on disk: %v", err)
	}
}

// #97's ruling for the global writer profile, applied to skills: a pinned
// client may READ what is global — linetta_get_story_context already hands
// it the writer profile — and only the write is refused (346fb7d). One rule,
// not two.
func TestReadSkillAllowsAWriterSkillOnAPinnedServer(t *testing.T) {
	ctx, d, projectID, _ := newSkillDeps(t)
	mustCreateSkill(t, ctx, d, editSkillInput{
		Action: "create", Scope: "writer", Name: "global-steer", Description: "d", Body: "전역 본문"})

	d.Source = "" // external, the only kind of client a pin applies to
	d.Settings = newRestrictedSettings(t, projectID)

	res, got, err := d.readSkill(ctx, nil, readSkillInput{Scope: "writer", Name: "global-steer"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("a pinned client reads a global skill, as it reads the global writer profile in the "+
			"brief: err=%v res=%q", err, firstText(res))
	}
	if got.Body != "전역 본문" {
		t.Errorf("Body = %q", got.Body)
	}
	// The write half of the same rule still refuses; that pairing is what
	// makes this one rule rather than an oversight.
	res, _, err = d.editSkill(ctx, nil, editSkillInput{
		Action: "patch", Scope: "writer", Name: "global-steer", Body: "고쳐 쓴 본문"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Error("a pinned client must still not be able to WRITE a global skill")
	}
}
