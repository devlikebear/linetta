package companion

import (
	"errors"
	"fmt"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
)

func addHistory(t *testing.T, repo *HistoryRepo, m HistoryMessage) {
	t.Helper()
	if m.Intent == "" {
		m.Intent = HistoryAgentIntent
	}
	if m.Status == "" {
		m.Status = HistoryStatusDone
	}
	if err := repo.Append(t.Context(), m); err != nil {
		t.Fatalf("append %s: %v", m.ID, err)
	}
}

func hitIDs(hits []HistoryHit) []string {
	out := []string{}
	for _, h := range hits {
		out = append(out, h.MessageID)
	}
	return out
}

func sameIDs(got []string, want ...string) bool {
	return fmt.Sprint(got) == fmt.Sprint(want)
}

func ftsRows(t *testing.T, repo *HistoryRepo) int {
	t.Helper()
	var n int
	if err := repo.db.QueryRowContext(t.Context(), `SELECT COUNT(1) FROM companion_messages_fts`).Scan(&n); err != nil {
		t.Fatalf("count index: %v", err)
	}
	return n
}

func TestSearchFindsAWordInsideItsParticles(t *testing.T) {
	ctx, st, _, p := seedWork(t)
	repo := NewHistoryRepo(st.DB())
	addHistory(t, repo, HistoryMessage{ID: "m1", ProjectID: p.ID, RunID: "r1", Role: "user", Content: "흑풍채가 나오는 건 좋아", CreatedAt: 1})

	hits, err := repo.Search(ctx, HistorySearchQuery{ProjectID: p.ID, Query: "흑풍채"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !sameIDs(hitIDs(hits), "m1") {
		t.Fatalf("hits = %v", hitIDs(hits))
	}
	if hits[0].Snippet == "" || hits[0].Role != "user" || hits[0].RunID != "r1" {
		t.Errorf("hit = %+v", hits[0])
	}
}

func TestSearchAnswersATwoCharacterNameByScanning(t *testing.T) {
	ctx, st, _, p := seedWork(t)
	repo := NewHistoryRepo(st.DB())
	addHistory(t, repo, HistoryMessage{ID: "old", ProjectID: p.ID, Role: "user", Content: "곰배는 조연으로", CreatedAt: 1})
	addHistory(t, repo, HistoryMessage{ID: "new", ProjectID: p.ID, Role: "assistant", Content: "곰배를 다시 넣었어요", CreatedAt: 2})
	addHistory(t, repo, HistoryMessage{ID: "other", ProjectID: p.ID, Role: "user", Content: "표두는 엑스트라", CreatedAt: 3})

	hits, err := repo.Search(ctx, HistorySearchQuery{ProjectID: p.ID, Query: "곰배"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !sameIDs(hitIDs(hits), "new", "old") {
		t.Errorf("hits = %v, want the newest first", hitIDs(hits))
	}
}

func TestSearchKeepsToOneWorkAndToThisAgentsWords(t *testing.T) {
	ctx, st, _, p := seedWork(t)
	other, err := project.NewRepo(st).Create(ctx, 2, project.NewInput{Title: "다른 작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	repo := NewHistoryRepo(st.DB())
	addHistory(t, repo, HistoryMessage{ID: "mine", ProjectID: p.ID, Role: "user", Content: "무형검 이야기", CreatedAt: 1})
	addHistory(t, repo, HistoryMessage{ID: "theirs", ProjectID: other.ID, Role: "user", Content: "무형검 이야기", CreatedAt: 2})
	addHistory(t, repo, HistoryMessage{ID: "legacy", ProjectID: p.ID, Role: "user", Intent: "chat", Content: "무형검 이야기", CreatedAt: 3})
	addHistory(t, repo, HistoryMessage{ID: "tool", ProjectID: p.ID, Role: "tool", Content: `{"summary":"무형검 이야기"}`, CreatedAt: 4})

	for _, q := range []string{"무형검", "무형"} {
		hits, err := repo.Search(ctx, HistorySearchQuery{ProjectID: p.ID, Query: q})
		if err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
		if !sameIDs(hitIDs(hits), "mine") {
			t.Errorf("Search(%q) = %v", q, hitIDs(hits))
		}
	}
}

func TestSearchLooksOnlyBeforeTheBoundaryAndSkipsTheRunningTurn(t *testing.T) {
	ctx, st, _, p := seedWork(t)
	repo := NewHistoryRepo(st.DB())
	// Equal timestamps: a user row reads before the reply, whichever landed first.
	addHistory(t, repo, HistoryMessage{ID: "reply", ProjectID: p.ID, RunID: "r1", Role: "assistant", Content: "무형검 답", CreatedAt: 5})
	addHistory(t, repo, HistoryMessage{ID: "ask", ProjectID: p.ID, RunID: "r1", Role: "user", Content: "무형검 질문", CreatedAt: 5})
	addHistory(t, repo, HistoryMessage{ID: "later", ProjectID: p.ID, RunID: "r2", Role: "user", Content: "무형검 나중", CreatedAt: 6})

	hits, err := repo.Search(ctx, HistorySearchQuery{ProjectID: p.ID, Query: "무형검", BeforeID: "reply"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !sameIDs(hitIDs(hits), "ask") {
		t.Errorf("before reply = %v", hitIDs(hits))
	}

	hits, err = repo.Search(ctx, HistorySearchQuery{ProjectID: p.ID, Query: "무형검", ExcludeRun: "r1"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !sameIDs(hitIDs(hits), "later") {
		t.Errorf("excluding r1 = %v", hitIDs(hits))
	}

	if _, err := repo.Search(ctx, HistorySearchQuery{ProjectID: p.ID, Query: "무형검", BeforeID: "nope"}); !errors.Is(err, ErrHistoryMessageNotFound) {
		t.Errorf("unknown boundary err = %v", err)
	}
}

func TestSearchCapsItsLimit(t *testing.T) {
	ctx, st, _, p := seedWork(t)
	repo := NewHistoryRepo(st.DB())
	for i := range 15 {
		addHistory(t, repo, HistoryMessage{ID: fmt.Sprintf("m%d", i), ProjectID: p.ID, Role: "user", Content: "무형검", CreatedAt: int64(i)})
	}
	hits, err := repo.Search(ctx, HistorySearchQuery{ProjectID: p.ID, Query: "무형검", Limit: 99})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != maxHistorySearchLimit {
		t.Errorf("got %d hits", len(hits))
	}
}

func TestWindowReadsAroundAMessageInReadingOrder(t *testing.T) {
	ctx, st, _, p := seedWork(t)
	repo := NewHistoryRepo(st.DB())
	addHistory(t, repo, HistoryMessage{ID: "a", ProjectID: p.ID, Role: "user", Content: "하나", CreatedAt: 1})
	addHistory(t, repo, HistoryMessage{ID: "b", ProjectID: p.ID, Role: "assistant", Content: "둘", CreatedAt: 2})
	addHistory(t, repo, HistoryMessage{ID: "tool", ProjectID: p.ID, Role: "tool", Content: "{}", CreatedAt: 3})
	addHistory(t, repo, HistoryMessage{ID: "d", ProjectID: p.ID, Role: "assistant", Content: "셋", CreatedAt: 5})
	addHistory(t, repo, HistoryMessage{ID: "c", ProjectID: p.ID, Role: "user", Content: "넷", CreatedAt: 5})
	addHistory(t, repo, HistoryMessage{ID: "e", ProjectID: p.ID, Role: "user", Content: "다섯", CreatedAt: 6})

	msgs, err := repo.Window(ctx, p.ID, "c", 2, 1)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	got := []string{}
	for _, m := range msgs {
		got = append(got, m.ID)
	}
	if !sameIDs(got, "a", "b", "c", "d") {
		t.Errorf("window = %v", got)
	}

	if _, err := repo.Window(ctx, p.ID, "tool", 1, 1); !errors.Is(err, ErrHistoryMessageNotFound) {
		t.Errorf("a tool row must not anchor a window, err = %v", err)
	}
}

func TestWindowRefusesAnotherWorksMessage(t *testing.T) {
	ctx, st, _, p := seedWork(t)
	other, err := project.NewRepo(st).Create(ctx, 2, project.NewInput{Title: "다른 작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	repo := NewHistoryRepo(st.DB())
	addHistory(t, repo, HistoryMessage{ID: "theirs", ProjectID: other.ID, Role: "user", Content: "비밀", CreatedAt: 1})
	if _, err := repo.Window(ctx, p.ID, "theirs", 1, 1); !errors.Is(err, ErrHistoryMessageNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestIndexFollowsClearAndLeavesLegacyRowsAlone(t *testing.T) {
	ctx, st, _, p := seedWork(t)
	repo := NewHistoryRepo(st.DB())
	addHistory(t, repo, HistoryMessage{ID: "m1", ProjectID: p.ID, RunID: "r1", Role: "user", Content: "무형검", CreatedAt: 1})
	addHistory(t, repo, HistoryMessage{ID: "legacy", ProjectID: p.ID, Role: "user", Intent: "chat", Content: "무형검", CreatedAt: 2})
	if got := ftsRows(t, repo); got != 1 {
		t.Fatalf("indexed %d rows, want 1", got)
	}
	if err := repo.MarkRunStatus(ctx, "r1", HistoryStatusCancelled); err != nil {
		t.Fatalf("MarkRunStatus: %v", err)
	}
	if got := ftsRows(t, repo); got != 1 {
		t.Fatalf("a status change moved the index to %d rows", got)
	}
	if err := repo.Clear(ctx, HistoryQuery{ProjectID: p.ID, Scope: HistoryViewProject, Intent: HistoryAgentIntent}); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got := ftsRows(t, repo); got != 0 {
		t.Errorf("a cleared conversation left %d indexed rows", got)
	}
	if n, _ := repo.ProjectMessageCount(ctx, p.ID); n != 1 {
		t.Errorf("Clear left %d rows, want the legacy one", n)
	}
}

// A work's deletion reaches companion_messages through the foreign key, not
// through Clear. What the writer deleted must not linger in the index.
func TestIndexFollowsTheWorksDeletion(t *testing.T) {
	ctx, st, _, p := seedWork(t)
	repo := NewHistoryRepo(st.DB())
	addHistory(t, repo, HistoryMessage{ID: "m1", ProjectID: p.ID, Role: "user", Content: "무형검", CreatedAt: 1})
	if _, err := st.DB().ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, p.ID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if got := ftsRows(t, repo); got != 0 {
		t.Errorf("a deleted work left %d indexed rows", got)
	}
}
