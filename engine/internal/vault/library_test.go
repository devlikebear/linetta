package vault

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/vaultbundle"
)

func (d *device) skill(t *testing.T, name, body string) {
	t.Helper()
	writeFile(t, filepath.Join(vaultbundle.GlobalSkillsDir(d.home), name, "SKILL.md"),
		"---\nname: "+name+"\ndescription: 설명\nauthor: writer\n---\n"+body)
}

func (d *device) readSkill(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultbundle.GlobalSkillsDir(d.home), name, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (d *device) profile(t *testing.T, body string, when int64) {
	t.Helper()
	if _, err := agentmemory.NewRepo(d.db).Save(context.Background(), agentmemory.ScopeWriterProfile, "", body, when); err != nil {
		t.Fatal(err)
	}
}

func (d *device) profileBody(t *testing.T) string {
	t.Helper()
	doc, err := agentmemory.NewRepo(d.db).Load(context.Background(), agentmemory.ScopeWriterProfile, "")
	if err != nil {
		t.Fatal(err)
	}
	return doc.Body
}

func (d *device) setItems(t *testing.T, omit ...string) ItemsView {
	t.Helper()
	view, err := d.svc.SetItems(context.Background(), omit)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func (d *device) push(t *testing.T, id string) PushResult {
	t.Helper()
	out, err := d.svc.Push(context.Background(), id)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	return out
}

func (d *device) pull(t *testing.T, in PullInput) PullResult {
	t.Helper()
	out, err := d.svc.Pull(context.Background(), in)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	return out
}

func (d *device) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := d.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (d *device) syncLibrary(t *testing.T) {
	t.Helper()
	if err := d.svc.SyncLibrary(context.Background()); err != nil {
		t.Fatalf("SyncLibrary: %v", err)
	}
}

func TestOmittedImagesDoNotLeaveAndAreNotRemoved(t *testing.T) {
	site, a, b := devices(t)
	if b.count(t, `SELECT COUNT(*) FROM illustrations WHERE project_id = ?`, workID) != 1 {
		t.Fatal("the picture did not arrive to start with")
	}
	a.setItems(t, vaultbundle.CategoryImages)
	a.push(t, workID)

	// What went up has no picture part.
	site.mu.Lock()
	item := site.items[linkOf(t, a, TargetWorks, workID).ItemID]
	head := item.revisions[len(item.revisions)-1]
	site.mu.Unlock()
	keys := keysOf(t, a)
	m, err := openManifest(keys, head.ManifestBlob, site.blobs[head.ManifestBlob])
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Bundle.Omitted, []string{vaultbundle.CategoryImages}) {
		t.Fatalf("omitted = %v", m.Bundle.Omitted)
	}
	for _, part := range m.Bundle.Parts {
		if vaultbundle.PartCategory(part.Name) == vaultbundle.CategoryImages {
			t.Errorf("part %s went up", part.Name)
		}
	}

	a.edit(t, workID, "삽화 없는 다음 장면")
	a.push(t, workID)
	b.pull(t, PullInput{ProjectID: workID})

	if !strings.Contains(b.text(t, workID), "삽화 없는 다음 장면") {
		t.Error("the text did not arrive")
	}
	if b.count(t, `SELECT COUNT(*) FROM illustrations WHERE project_id = ?`, workID) != 1 {
		t.Error("taking a bundle without pictures removed the picture")
	}
	if got := b.state(t, workID); got != StateSynced {
		t.Errorf("the pulled work reads as %s", got)
	}
}

func TestSelectionFollowsTheLibraryItemToOtherDevices(t *testing.T) {
	_, a, b := devices(t)
	a.setItems(t, vaultbundle.CategoryHistory)

	b.syncLibrary(t)

	selection, err := getSelection(context.Background(), b.db)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(selection.Omit, []string{vaultbundle.CategoryHistory}) {
		t.Fatalf("omit = %v", selection.Omit)
	}
}

func TestChangedSelectionIsOneNewRevisionNotAConflict(t *testing.T) {
	_, a, b := devices(t)
	a.setItems(t, vaultbundle.CategoryImages)
	if got := a.state(t, workID); got != StateLocalChanges {
		t.Fatalf("the device that changed it reads %s", got)
	}

	// The other device learns the choice before the upload lands.
	b.syncLibrary(t)
	if got := b.state(t, workID); got != StateSynced {
		t.Fatalf("the other device reads %s", got)
	}
	a.push(t, workID)
	if got := b.state(t, workID); got != StateRemoteChanges {
		t.Fatalf("after the upload the other device reads %s", got)
	}
	b.pull(t, PullInput{ProjectID: workID})
	if got := b.state(t, workID); got != StateSynced {
		t.Fatalf("after pulling the other device reads %s", got)
	}
}

func TestOlderRestorePointWithoutPicturesKeepsThem(t *testing.T) {
	_, a, b := devices(t)
	a.setItems(t, vaultbundle.CategoryImages)
	first := a.push(t, workID)
	a.setItems(t)
	a.edit(t, workID, "그림이 다시 실린 판")
	a.push(t, workID)
	b.pull(t, PullInput{ProjectID: workID})

	b.pull(t, PullInput{ProjectID: workID, Revision: first.HeadRevision})

	if b.count(t, `SELECT COUNT(*) FROM illustrations WHERE project_id = ?`, workID) != 1 {
		t.Error("going back to a revision without pictures removed the picture")
	}
	if strings.Contains(b.text(t, workID), "그림이 다시 실린 판") {
		t.Error("the restore did not go back")
	}
}

func TestLibraryTravelsBetweenDevices(t *testing.T) {
	site := newFakeSite(t)
	ctx := context.Background()
	a := newDevice(t, site)
	key := a.makeKey(t)
	a.skill(t, "style", "짧게 쓴다")
	a.profile(t, "존댓말", 100)
	a.seed(t, workID, []byte("그림"))
	a.push(t, workID)

	// A new device: the library lands with the key, before any work.
	b := newDevice(t, site)
	if _, err := b.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: key}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.readSkill(t, "style"), "짧게 쓴다") {
		t.Error("the skill did not arrive with the key")
	}
	if got := b.profileBody(t); got != "존댓말" {
		t.Errorf("profile = %q", got)
	}

	// The library item is not a work.
	overview, err := b.svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Items) != 1 || overview.Items[0].Title != "비밀 작품" {
		t.Fatalf("items = %+v", overview.Items)
	}

	// And back: a newer edit goes the other way.
	b.skill(t, "style", "길게 쓴다")
	b.syncLibrary(t)
	a.syncLibrary(t)
	if !strings.Contains(a.readSkill(t, "style"), "길게 쓴다") {
		t.Error("the edit did not come back")
	}

	// A skill deleted on one device does not come back from the other.
	if err := os.RemoveAll(filepath.Join(vaultbundle.GlobalSkillsDir(a.home), "style")); err != nil {
		t.Fatal(err)
	}
	a.syncLibrary(t)
	b.syncLibrary(t)
	a.syncLibrary(t)
	for _, d := range []*device{a, b} {
		if _, err := os.Stat(filepath.Join(vaultbundle.GlobalSkillsDir(d.home), "style")); !os.IsNotExist(err) {
			t.Errorf("the deleted skill is still there: %v", err)
		}
	}
	versions, err := agentskills.NewHistory(b.db).List(ctx, agentskills.ScopeWriter, "", "style", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) == 0 || versions[0].Reason != agentskills.ReasonDeleted {
		t.Errorf("history = %+v", versions)
	}
}

func TestLibraryHoldsNothingReadableOnTheServer(t *testing.T) {
	site := newFakeSite(t)
	a := newDevice(t, site)
	a.makeKey(t)
	a.skill(t, "style", "숨겨진 스킬 문장")
	a.profile(t, "숨겨진 프로필 문장", 100)
	a.syncLibrary(t)
	if len(site.items) != 1 {
		t.Fatalf("items = %d", len(site.items))
	}
	for id, blob := range site.blobs {
		for _, secret := range []string{"숨겨진", "style", testToken} {
			if strings.Contains(string(blob), secret) {
				t.Errorf("blob %s holds %q", id, secret)
			}
		}
	}
}

func TestEmptyLibraryMakesNoItem(t *testing.T) {
	site := newFakeSite(t)
	a := newDevice(t, site)
	a.makeKey(t)
	a.syncLibrary(t)
	if len(site.items) != 0 {
		t.Fatalf("items = %d", len(site.items))
	}
}

func TestLibraryOfTwoDevicesMergesRatherThanConflicts(t *testing.T) {
	site := newFakeSite(t)
	ctx := context.Background()
	a := newDevice(t, site)
	key := a.makeKey(t)
	a.skill(t, "one", "첫째")
	a.syncLibrary(t)
	b := newDevice(t, site)
	if _, err := b.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: key}); err != nil {
		t.Fatal(err)
	}

	a.skill(t, "two", "둘째")
	b.skill(t, "three", "셋째")
	a.syncLibrary(t)
	b.syncLibrary(t)
	a.syncLibrary(t)

	for _, d := range []*device{a, b} {
		for _, name := range []string{"one", "two", "three"} {
			d.readSkill(t, name)
		}
	}
}

func TestSelectionRejectsWhatCannotBeOff(t *testing.T) {
	site := newFakeSite(t)
	a := newDevice(t, site)
	for _, omit := range [][]string{{""}, {"manuscript"}, {"images", "nope"}} {
		if _, err := a.svc.SetItems(context.Background(), omit); err == nil {
			t.Errorf("%v was accepted", omit)
		}
	}
}

func TestItemsShowSizesAndChoice(t *testing.T) {
	site := newFakeSite(t)
	a := newDevice(t, site)
	a.seed(t, workID, []byte("그림 바이트"))
	view := a.setItems(t, vaultbundle.CategoryHistory)
	byID := map[string]ItemCategory{}
	for _, category := range view.Categories {
		byID[category.ID] = category
	}
	if len(view.Categories) != 7 || !view.Categories[0].Required || view.Categories[0].Bytes == 0 {
		t.Fatalf("categories = %+v", view.Categories)
	}
	if byID[vaultbundle.CategoryHistory].Enabled || !byID[vaultbundle.CategoryImages].Enabled || byID[vaultbundle.CategoryImages].Bytes == 0 {
		t.Fatalf("categories = %+v", view.Categories)
	}
}

func TestSelectionWithNoKeyIsKeptForLater(t *testing.T) {
	site := newFakeSite(t)
	a := newDevice(t, site)
	a.setItems(t, vaultbundle.CategoryImages)
	if site.requests != 0 {
		t.Fatalf("%d requests with no key", site.requests)
	}
	a.makeKey(t)
	a.seed(t, workID, []byte("그림"))
	a.push(t, workID)
	if len(site.items) != 2 {
		t.Fatalf("items = %d, want the work and the library", len(site.items))
	}
}

func TestRefusedProfileShowsInTheOverview(t *testing.T) {
	site := newFakeSite(t)
	ctx := context.Background()
	a := newDevice(t, site)
	key := a.makeKey(t)
	// A row over budget, as a hand edit or a shrunk budget leaves it.
	exec(t, a.db, `INSERT INTO agent_memory (scope, project_id, body, updated_at) VALUES ('writer_profile', NULL, ?, 100)`,
		strings.Repeat("가", agentmemory.ScopeWriterProfile.Budget()+1))
	a.syncLibrary(t)

	b := newDevice(t, site)
	b.profile(t, "이 기기의 프로필", 50)
	if _, err := b.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: key}); err != nil {
		t.Fatal(err)
	}
	if got := b.profileBody(t); got != "이 기기의 프로필" {
		t.Errorf("profile = %q", got)
	}
	overview, err := b.svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if overview.LibraryWarning != LibraryWarningProfileRefused {
		t.Fatalf("warning = %q", overview.LibraryWarning)
	}
}

func TestSameKeyFindsTheSameLibraryItem(t *testing.T) {
	first, err := vectorKeysFor(t).Derive()
	if err != nil {
		t.Fatal(err)
	}
	second, err := vectorKeysFor(t).Derive()
	if err != nil {
		t.Fatal(err)
	}
	if first.LibraryItemID() != second.LibraryItemID() || !itemIDPattern.MatchString(first.LibraryItemID()) {
		t.Fatalf("ids %q and %q", first.LibraryItemID(), second.LibraryItemID())
	}
	if other, _ := NewRecoveryKey().Derive(); other.LibraryItemID() == first.LibraryItemID() {
		t.Fatal("two keys share a library item")
	}
}

func vectorKeysFor(t *testing.T) RecoveryKey {
	t.Helper()
	key, err := ParseRecoveryKey(vectorRecoveryKey)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func linkOf(t *testing.T, d *device, target, id string) Link {
	t.Helper()
	link, ok, err := getLink(context.Background(), d.db, target, id)
	if err != nil || !ok {
		t.Fatalf("link: %v %v", ok, err)
	}
	return link
}

func keysOf(t *testing.T, d *device) *Keys {
	t.Helper()
	keys, err := d.svc.keys()
	if err != nil {
		t.Fatal(err)
	}
	return keys
}
