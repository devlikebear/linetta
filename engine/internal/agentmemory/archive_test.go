package agentmemory

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func archiveRepo(t *testing.T) (context.Context, *Repo, string) {
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
	return ctx, NewRepo(st.DB()), p.ID
}

func TestArchiveMovesTheLineOutOfTheNotes(t *testing.T) {
	ctx, repo, id := archiveRepo(t)
	if _, err := repo.Save(ctx, ScopeWorkNotes, id, "곰배는 조연\n표두는 엑스트라", 1); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc, err := repo.Archive(ctx, id, "곰배", "msg-1", 2)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if doc.Body != "표두는 엑스트라" || doc.CharsUsed != len([]rune("표두는 엑스트라")) {
		t.Errorf("doc = %+v", doc)
	}
	list, err := repo.ListArchived(ctx, id, 0)
	if err != nil {
		t.Fatalf("ListArchived: %v", err)
	}
	if len(list) != 1 || list[0].Body != "곰배는 조연" || list[0].SourceMessageID != "msg-1" || list[0].ArchivedAt != 2 {
		t.Fatalf("list = %+v", list)
	}
}

func TestArchiveRefusesWhatItCannotIdentifyAndChangesNothing(t *testing.T) {
	ctx, repo, id := archiveRepo(t)
	if _, err := repo.Save(ctx, ScopeWorkNotes, id, "곰배는 조연\n곰배는 두목", 1); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := repo.Archive(ctx, id, "곰배", "", 2); !errors.Is(err, ErrAmbiguous) {
		t.Errorf("ambiguous err = %v", err)
	}
	if _, err := repo.Archive(ctx, id, "없는 줄", "", 2); !errors.Is(err, ErrNoMatch) {
		t.Errorf("no match err = %v", err)
	}
	doc, err := repo.Load(ctx, ScopeWorkNotes, id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Body != "곰배는 조연\n곰배는 두목" {
		t.Errorf("a refused archive changed the notes: %q", doc.Body)
	}
	if list, _ := repo.ListArchived(ctx, id, 0); len(list) != 0 {
		t.Errorf("a refused archive stored %+v", list)
	}
}

func TestSearchArchivedFindsShortAndLongWordsInThisWorkOnly(t *testing.T) {
	ctx, repo, id := archiveRepo(t)
	for _, line := range []string{"곰배는 흑풍채 두목", "표두는 엑스트라"} {
		if err := repo.Remember(ctx, id, line, 1); err != nil {
			t.Fatalf("Remember: %v", err)
		}
	}
	for query, want := range map[string]int{"흑풍채": 1, "곰배": 1, "두목 곰배": 1, "없는단어": 0, "": 0} {
		hits, err := repo.SearchArchived(ctx, id, query, 0)
		if err != nil {
			t.Fatalf("SearchArchived(%q): %v", query, err)
		}
		if len(hits) != want {
			t.Errorf("SearchArchived(%q) = %+v", query, hits)
		}
	}
	if hits, _ := repo.SearchArchived(ctx, "other", "흑풍채", 0); len(hits) != 0 {
		t.Errorf("another work's search found %+v", hits)
	}
}

func TestRestoreArchivedPutsTheLineBackOrRefusesWhenFull(t *testing.T) {
	ctx, repo, id := archiveRepo(t)
	if err := repo.Remember(ctx, id, "곰배는 조연", 1); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	list, _ := repo.ListArchived(ctx, id, 0)

	full := strings.Repeat("가", ScopeWorkNotes.Budget())
	if _, err := repo.Save(ctx, ScopeWorkNotes, id, full, 2); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := repo.RestoreArchived(ctx, id, list[0].ID, 3); !errors.Is(err, ErrOverBudget) {
		t.Fatalf("full notes err = %v", err)
	}
	if after, _ := repo.ListArchived(ctx, id, 0); len(after) != 1 {
		t.Fatalf("a refused restore lost the line: %+v", after)
	}

	if _, err := repo.Save(ctx, ScopeWorkNotes, id, "표두", 4); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc, err := repo.RestoreArchived(ctx, id, list[0].ID, 5)
	if err != nil {
		t.Fatalf("RestoreArchived: %v", err)
	}
	if doc.Body != "표두\n곰배는 조연" {
		t.Errorf("Body = %q", doc.Body)
	}
	if after, _ := repo.ListArchived(ctx, id, 0); len(after) != 0 {
		t.Errorf("the line stayed archived: %+v", after)
	}
	if _, err := repo.RestoreArchived(ctx, id, list[0].ID, 6); !errors.Is(err, ErrNoArchivedLine) {
		t.Errorf("second restore err = %v", err)
	}
}

func TestDeleteArchivedForgetsTheLineAndItsIndexEntry(t *testing.T) {
	ctx, repo, id := archiveRepo(t)
	if err := repo.Remember(ctx, id, "곰배는 조연", 1); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	list, _ := repo.ListArchived(ctx, id, 0)
	if err := repo.DeleteArchived(ctx, "other", list[0].ID); !errors.Is(err, ErrNoArchivedLine) {
		t.Fatalf("another work deleted it: %v", err)
	}
	if err := repo.DeleteArchived(ctx, id, list[0].ID); err != nil {
		t.Fatalf("DeleteArchived: %v", err)
	}
	if hits, _ := repo.SearchArchived(ctx, id, "곰배는", 0); len(hits) != 0 {
		t.Errorf("deleted line still found: %+v", hits)
	}
}

func TestImportArchivedKeepsOneRowPerLine(t *testing.T) {
	ctx, repo, id := archiveRepo(t)
	for range 2 {
		if err := repo.ImportArchived(ctx, id, "곰배는 조연", 1); err != nil {
			t.Fatalf("ImportArchived: %v", err)
		}
	}
	if list, _ := repo.ListArchived(ctx, id, 0); len(list) != 1 {
		t.Errorf("list = %+v", list)
	}
}

func TestRememberScreensAndNamesAnUnknownWork(t *testing.T) {
	ctx, repo, id := archiveRepo(t)
	if err := repo.Remember(ctx, id, "", 1); !errors.Is(err, ErrEmptyText) {
		t.Errorf("empty err = %v", err)
	}
	if err := repo.Remember(ctx, id, "## 제목", 1); err == nil {
		t.Error("a heading must be screened out")
	}
	if err := repo.Remember(ctx, "no-such-work", "곰배", 1); !errors.Is(err, ErrUnknownProject) {
		t.Errorf("unknown work err = %v", err)
	}
}
