package post

import (
	"context"
	"path/filepath"
	"testing"

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
	return NewRepo(st)
}

func np(body, platform, kind string) NewPost {
	return NewPost{Body: body, Platform: platform, Kind: kind, Source: SourceImport}
}

func TestInsertManySkipsDuplicates(t *testing.T) {
	ctx := context.Background()
	r := openRepo(t)
	in, sk, err := r.InsertMany(ctx, []NewPost{np("a", "x", "post"), np("b", "x", "post"), np("a", "x", "post")})
	if err != nil || in != 2 || sk != 1 {
		t.Fatalf("got %d %d %v", in, sk, err)
	}
	in, sk, err = r.InsertMany(ctx, []NewPost{np("a", "x", "reply"), np("a", "threads", "post"), np("c", "x", "post")})
	if err != nil || in != 2 || sk != 1 {
		t.Fatalf("got %d %d %v", in, sk, err)
	}
}

func TestCountsAndList(t *testing.T) {
	ctx := context.Background()
	r := openRepo(t)
	if _, _, err := r.InsertMany(ctx, []NewPost{np("a", "x", "post"), np("b", "x", "post"), np("c", "x", "reply"), np("d", "threads", "post")}); err != nil {
		t.Fatal(err)
	}
	counts, err := r.Counts(ctx)
	if err != nil || len(counts) != 3 {
		t.Fatalf("counts %v %v", counts, err)
	}
	if counts[1] != (ContextCount{"x", "post", 2}) {
		t.Fatalf("counts %v", counts)
	}
	all, _ := r.List(ctx, ListParams{})
	if len(all) != 4 || all[0].Body != "a" {
		t.Fatalf("all %v", all)
	}
	xs, _ := r.List(ctx, ListParams{Platform: "x", Limit: 2, Offset: 1})
	if len(xs) != 2 || xs[0].Body != "b" {
		t.Fatalf("page %v", xs)
	}
	replies, _ := r.List(ctx, ListParams{Kind: "reply"})
	if len(replies) != 1 {
		t.Fatalf("replies %v", replies)
	}
}

func TestCheckViolation(t *testing.T) {
	r := openRepo(t)
	for _, p := range []NewPost{
		{Body: "a", Platform: "mastodon", Kind: "post", Source: "import"},
		{Body: "a", Platform: "x", Kind: "dm", Source: "import"},
		{Body: "a", Platform: "x", Kind: "post", Source: "scraped"},
	} {
		if _, _, err := r.InsertMany(context.Background(), []NewPost{p}); err == nil {
			t.Fatalf("expected CHECK failure for %+v", p)
		}
	}
	// a failed batch rolls back whole
	_, _, err := r.InsertMany(context.Background(), []NewPost{np("ok", "x", "post"), {Body: "z", Platform: "bad", Kind: "post", Source: "import"}})
	if err == nil {
		t.Fatal("want error")
	}
	if c, _ := r.Counts(context.Background()); len(c) != 0 {
		t.Fatalf("rollback failed: %v", c)
	}
}
