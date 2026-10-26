package illustration

import (
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

func TestRepresentative_setReplaceClear(t *testing.T) {
	f := fixture(t)
	a, err := f.repo.Add(f.ctx, 1, NewInput{ProjectID: f.project, Prompt: "p", Provider: "x", Model: "m", Data: encodeJPEG(t, 4, 4)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.repo.Add(f.ctx, 2, NewInput{ProjectID: f.project, Prompt: "p", Provider: "x", Model: "m", Data: encodeJPEG(t, 6, 6)})
	if err != nil {
		t.Fatal(err)
	}
	if prev, err := f.repo.SetRepresentative(f.ctx, 3, f.scene, a.ID); err != nil || prev != "" {
		t.Fatalf("set = %q, %v", prev, err)
	}
	if prev, err := f.repo.SetRepresentative(f.ctx, 4, f.scene, b.ID); err != nil || prev != a.ID {
		t.Fatalf("replace = %q, %v", prev, err)
	}
	reps, _ := f.repo.Representatives(f.ctx, f.project)
	if len(reps) != 1 || reps[f.scene] != b.ID {
		t.Fatalf("representatives = %v", reps)
	}
	// A representative picture cannot be deleted; the one it replaced can.
	if err := f.repo.Delete(f.ctx, b.ID); !errors.Is(err, ErrIsRepresentative) {
		t.Fatalf("delete representative = %v", err)
	}
	if err := f.repo.Delete(f.ctx, a.ID); err != nil {
		t.Fatalf("delete replaced = %v", err)
	}
	if prev, err := f.repo.SetRepresentative(f.ctx, 5, f.scene, ""); err != nil || prev != b.ID {
		t.Fatalf("clear = %q, %v", prev, err)
	}
	if reps, _ := f.repo.Representatives(f.ctx, f.project); len(reps) != 0 {
		t.Fatalf("after clear = %v", reps)
	}
	if err := f.repo.Delete(f.ctx, b.ID); err != nil {
		t.Fatalf("delete after clear = %v", err)
	}
}

func TestRepresentative_refusesOtherWorksAndUnknowns(t *testing.T) {
	f := fixture(t)
	mine, err := f.repo.Add(f.ctx, 1, NewInput{ProjectID: f.project, Prompt: "p", Provider: "x", Model: "m", Data: encodeJPEG(t, 4, 4)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SetRepresentative(f.ctx, 2, "no-node", mine.ID); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("unknown node = %v", err)
	}
	if _, err := f.repo.SetRepresentative(f.ctx, 2, f.scene, "no-picture"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown picture = %v", err)
	}
	// Another work in the same library cannot take this work's picture.
	p2, err := project.NewRepo(f.s).Create(f.ctx, 3, project.NewInput{Title: "다른 작품", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SetRepresentative(f.ctx, 4, *p2.LastOpenedNodeID, mine.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another work's scene = %v", err)
	}
	if reps, _ := f.repo.Representatives(f.ctx, p2.ID); len(reps) != 0 {
		t.Fatalf("other work = %v", reps)
	}
}

func TestRepresentative_goesWithItsNode(t *testing.T) {
	f := fixture(t)
	pic, err := f.repo.Add(f.ctx, 1, NewInput{ProjectID: f.project, Prompt: "p", Provider: "x", Model: "m", Data: encodeJPEG(t, 4, 4)})
	if err != nil {
		t.Fatal(err)
	}
	nodes := node.NewRepo(f.s)
	extra, err := nodes.CreateRoot(f.ctx, f.project, node.KindLeaf, "씬 2", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SetRepresentative(f.ctx, 3, extra.ID, pic.ID); err != nil {
		t.Fatal(err)
	}
	if err := nodes.Delete(f.ctx, extra.ID, 4); err != nil {
		t.Fatal(err)
	}
	if reps, _ := f.repo.Representatives(f.ctx, f.project); len(reps) != 0 {
		t.Fatalf("a deleted node kept its picture: %v", reps)
	}
	if err := f.repo.Delete(f.ctx, pic.ID); err != nil {
		t.Fatalf("the picture stays protected after its node went: %v", err)
	}
}
