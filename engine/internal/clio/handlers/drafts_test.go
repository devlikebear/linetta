package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/clio/draft"
	cliomigrations "github.com/devlikebear/linetta/engine/internal/clio/migrations"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func draftRepo(t *testing.T) *draft.Repo {
	t.Helper()
	st, err := store.OpenWith(context.Background(), filepath.Join(t.TempDir(), "clio.db"), cliomigrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return draft.NewRepo(st)
}

func call(t *testing.T, h rpc.Handler, params string) (json.RawMessage, error) {
	t.Helper()
	return h(context.Background(), json.RawMessage(params))
}

func wantCode(t *testing.T, err error, code int) {
	t.Helper()
	var me *rpc.MethodError
	if !errors.As(err, &me) || me.Code != code {
		t.Fatalf("want code %d, got %v", code, err)
	}
}

func TestDraftsLifecycle(t *testing.T) {
	repo := draftRepo(t)
	out, err := call(t, CreateDraft(repo), `{"platform":"x","kind":"post","body":"첫 줄"}`)
	if err != nil {
		t.Fatal(err)
	}
	var d draft.Draft
	_ = json.Unmarshal(out, &d)
	if d.ID == 0 || d.ContentVersion != 1 || d.Body != "첫 줄" {
		t.Fatalf("%s", out)
	}
	out, err = call(t, UpdateDraft(repo), `{"id":1,"content_version":1,"body":"둘째","platform":"threads","kind":"post"}`)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(out, &d)
	if d.ContentVersion != 2 || d.Body != "둘째" || d.Platform != "threads" {
		t.Fatalf("%s", out)
	}
	_, err = call(t, UpdateDraft(repo), `{"id":1,"content_version":1,"body":"낡음","platform":"x","kind":"post"}`)
	wantCode(t, err, rpc.CodeContentConflict)

	out, err = call(t, ListDrafts(repo), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	var list []draft.Draft
	_ = json.Unmarshal(out, &list)
	if len(list) != 1 || list[0].Body != "둘째" {
		t.Fatalf("%s", out)
	}
	if _, err = call(t, GetDraft(repo), `{"id":1}`); err != nil {
		t.Fatal(err)
	}
	if _, err = call(t, DeleteDraft(repo), `{"id":1}`); err != nil {
		t.Fatal(err)
	}
	_, err = call(t, GetDraft(repo), `{"id":1}`)
	wantCode(t, err, rpc.CodeInvalidParams)
}

func TestDraftsValidation(t *testing.T) {
	repo := draftRepo(t)
	for _, p := range []string{`"x"`, `{"platform":"mastodon","kind":"post"}`, `{"platform":"x","kind":"dm"}`, `{}`} {
		_, err := call(t, CreateDraft(repo), p)
		wantCode(t, err, rpc.CodeInvalidParams)
	}
	_, err := call(t, UpdateDraft(repo), `{"id":1,"content_version":1,"body":"a","platform":"x","kind":"dm"}`)
	wantCode(t, err, rpc.CodeInvalidParams)
	_, err = call(t, UpdateDraft(repo), `{"id":1,"body":"a","platform":"x","kind":"post"}`)
	wantCode(t, err, rpc.CodeInvalidParams)
	_, err = call(t, DeleteDraft(repo), `{"id":0}`)
	wantCode(t, err, rpc.CodeInvalidParams)
}
