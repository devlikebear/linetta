package draft

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	cliomigrations "github.com/devlikebear/linetta/engine/internal/clio/migrations"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func openRepo(t *testing.T) *Repo {
	t.Helper()
	st, err := store.OpenWith(context.Background(), filepath.Join(t.TempDir(), "clio.db"), cliomigrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := NewRepo(st)
	tick := int64(1000)
	r.now = func() time.Time { tick++; return time.UnixMilli(tick) }
	return r
}

func TestCreateGet(t *testing.T) {
	ctx := context.Background()
	r := openRepo(t)
	d, err := r.Create(ctx, "x", "post", "")
	if err != nil || d.ID == 0 || d.ContentVersion != 1 || d.Platform != "x" || d.Body != "" {
		t.Fatalf("%+v %v", d, err)
	}
	got, err := r.Get(ctx, d.ID)
	if err != nil || got != d {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := r.Get(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestUpdateVersioning(t *testing.T) {
	ctx := context.Background()
	r := openRepo(t)
	d, _ := r.Create(ctx, "x", "post", "a")
	u, err := r.Update(ctx, d.ID, 1, "b", "threads", "reply")
	if err != nil || u.ContentVersion != 2 || u.Body != "b" || u.Platform != "threads" || u.Kind != "reply" || u.UpdatedAt <= d.UpdatedAt {
		t.Fatalf("%+v %v", u, err)
	}
	if _, err := r.Update(ctx, d.ID, 1, "stale", "x", "post"); !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
	got, _ := r.Get(ctx, d.ID)
	if got.Body != "b" || got.ContentVersion != 2 {
		t.Fatalf("conflict must not write: %+v", got)
	}
	if _, err := r.Update(ctx, 999, 1, "x", "x", "post"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestListOrderAndDelete(t *testing.T) {
	ctx := context.Background()
	r := openRepo(t)
	a, _ := r.Create(ctx, "x", "post", "a")
	b, _ := r.Create(ctx, "x", "post", "b")
	if _, err := r.Update(ctx, a.ID, 1, "a2", "x", "post"); err != nil {
		t.Fatal(err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Fatalf("%+v %v", list, err)
	}
	if err := r.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	list, _ = r.List(ctx)
	if len(list) != 1 {
		t.Fatalf("%+v", list)
	}
}
