package vault

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func (d *device) syncAll(t *testing.T, policy SyncPolicy) SyncReport {
	t.Helper()
	out, err := d.svc.SyncAll(context.Background(), policy)
	if err != nil {
		t.Fatalf("SyncAll: %v", err)
	}
	return out
}

func stateOf(t *testing.T, report SyncReport, id string) WorkSync {
	t.Helper()
	for _, line := range report.Works {
		if line.ProjectID == id {
			return line
		}
	}
	t.Fatalf("no line for %s in %+v", id, report.Works)
	return WorkSync{}
}

func is(id string) func(string) bool { return func(got string) bool { return got == id } }

func TestAutoUploadsAWorkNeverUploaded(t *testing.T) {
	site := newFakeSite(t)
	a := newDevice(t, site)
	a.makeKey(t)
	a.seed(t, workID, []byte("그림"))

	report := a.syncAll(t, SyncPolicy{})

	if !slices.Equal(report.Pushed, []string{workID}) || stateOf(t, report, workID).State != StateSynced {
		t.Fatalf("report = %+v", report)
	}
}

func TestAutoLeavesAWorkThatIsStillBeingEdited(t *testing.T) {
	site := newFakeSite(t)
	a := newDevice(t, site)
	a.makeKey(t)
	a.seed(t, workID, []byte("그림"))
	unsettled := SyncPolicy{Settled: func(string) bool { return false }}

	report := a.syncAll(t, unsettled)
	if len(report.Pushed) != 0 || len(site.items) != 0 {
		t.Fatalf("a work being written went up: %+v", report)
	}

	a.syncAll(t, SyncPolicy{})
	a.edit(t, workID, "새 문장")
	report = a.syncAll(t, unsettled)
	if line := stateOf(t, report, workID); line.State != StateLocalChanges || len(report.Pushed) != 0 {
		t.Fatalf("report = %+v", report)
	}
	report = a.syncAll(t, SyncPolicy{})
	if !slices.Equal(report.Pushed, []string{workID}) {
		t.Fatalf("report = %+v", report)
	}
}

func TestAutoDoesNothingWhenInStep(t *testing.T) {
	site, a, _ := devices(t)
	commits := site.commits
	report := a.syncAll(t, SyncPolicy{})
	if len(report.Pushed)+len(report.Pulled) != 0 || site.commits != commits {
		t.Fatalf("report = %+v, commits %d → %d", report, commits, site.commits)
	}
	if got := stateOf(t, report, workID).State; got != StateSynced {
		t.Fatalf("state = %s", got)
	}
}

func TestAutoTakesARemoteChangeWhenTheWorkIsClosed(t *testing.T) {
	_, a, b := devices(t)
	a.edit(t, workID, "다른 기기의 문장")
	a.push(t, workID)

	report := b.syncAll(t, SyncPolicy{})

	if !slices.Equal(report.Pulled, []string{workID}) || !strings.Contains(b.text(t, workID), "다른 기기의 문장") {
		t.Fatalf("report = %+v", report)
	}
}

func TestAutoLeavesAnOpenWorkAndSaysThereIsANewVersion(t *testing.T) {
	_, a, b := devices(t)
	a.edit(t, workID, "다른 기기의 문장")
	a.push(t, workID)
	before := b.text(t, workID)

	report := b.syncAll(t, SyncPolicy{Open: is(workID)})

	if got := stateOf(t, report, workID).State; got != StateRemoteChanges {
		t.Fatalf("state = %s", got)
	}
	if b.text(t, workID) != before || len(report.Pulled) != 0 {
		t.Fatal("an open work was replaced")
	}
}

func TestAutoLeavesAWorkAnAgentIsWritingBothWays(t *testing.T) {
	_, a, b := devices(t)
	a.edit(t, workID, "다른 기기의 문장")
	a.push(t, workID)
	before := b.text(t, workID)
	report := b.syncAll(t, SyncPolicy{Busy: is(workID)})
	if b.text(t, workID) != before || len(report.Pulled) != 0 {
		t.Fatal("a work in an agent's hands was replaced")
	}

	b.pull(t, PullInput{ProjectID: workID})
	b.edit(t, workID, "에이전트가 쓰는 중")
	report = b.syncAll(t, SyncPolicy{Busy: is(workID)})
	if len(report.Pushed) != 0 {
		t.Fatal("a work in an agent's hands went up")
	}
}

func TestAutoLeavesAConflictAlone(t *testing.T) {
	site, a, b := devices(t)
	a.edit(t, workID, "여기서 고침")
	a.push(t, workID)
	b.edit(t, workID, "저기서 고침")
	commits := site.commits
	before := b.text(t, workID)

	report := b.syncAll(t, SyncPolicy{})

	if got := stateOf(t, report, workID).State; got != StateConflict {
		t.Fatalf("state = %s", got)
	}
	if site.commits != commits || b.text(t, workID) != before {
		t.Fatal("a conflict was touched")
	}
}

func TestAutoDownloadsAWorkOnlyOnTheServer(t *testing.T) {
	site := newFakeSite(t)
	ctx := context.Background()
	a, b := newDevice(t, site), newDevice(t, site)
	key := a.makeKey(t)
	a.seed(t, workID, []byte("그림"))
	a.push(t, workID)
	if _, err := b.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: key}); err != nil {
		t.Fatal(err)
	}

	report := b.syncAll(t, SyncPolicy{})

	if !slices.Equal(report.Pulled, []string{workID}) {
		t.Fatalf("report = %+v", report)
	}
	if !strings.Contains(b.text(t, workID), "숨겨진 첫 문장") {
		t.Fatal("the work did not arrive")
	}
}

func TestAutoStopsOnAnAccountThatCannotUpload(t *testing.T) {
	for mode, want := range map[string]string{
		ModeTermsOutdated: rpc.ReasonVaultTermsOutdated,
		ModeReadOnly:      rpc.ReasonVaultNotEnabled,
		ModeNone:          rpc.ReasonVaultNotEnabled,
	} {
		site, a, _ := devices(t)
		a.edit(t, workID, "고침")
		commits := site.commits
		site.mode = mode
		_, err := a.svc.SyncAll(context.Background(), SyncPolicy{})
		wantReason(t, err, want)
		if site.commits != commits {
			t.Fatalf("%s: something went up", mode)
		}
	}
}

func TestAutoStopsWithoutAKeyOrConnection(t *testing.T) {
	site := newFakeSite(t)
	a := newDevice(t, site)
	_, err := a.svc.SyncAll(context.Background(), SyncPolicy{})
	wantReason(t, err, rpc.ReasonVaultNoKey)
	if err := a.settings.DeletePublishToken(); err != nil {
		t.Fatal(err)
	}
	_, err = a.svc.SyncAll(context.Background(), SyncPolicy{})
	wantReason(t, err, rpc.ReasonVaultNotConnected)
	if site.requests != 0 {
		t.Fatalf("%d requests", site.requests)
	}
}

func TestAutoSkipsARunWhileAManualOneHoldsTheLock(t *testing.T) {
	_, a, _ := devices(t)
	end, err := a.svc.begin()
	if err != nil {
		t.Fatal(err)
	}
	defer end()
	_, err = a.svc.SyncAll(context.Background(), SyncPolicy{})
	wantReason(t, err, rpc.ReasonVaultInProgress)
}

func TestAutoPullBacksUpOnceADay(t *testing.T) {
	_, a, b := devices(t)
	if got := len(b.snapshots(t)); got != 0 {
		// The pull that set the device up made no copy: the work was new here.
		t.Fatalf("%d snapshots to start with", got)
	}
	for i, text := range []string{"첫째", "둘째", "셋째"} {
		a.edit(t, workID, text)
		a.push(t, workID)
		b.syncAll(t, SyncPolicy{})
		if !strings.Contains(b.text(t, workID), text) {
			t.Fatalf("round %d did not arrive", i)
		}
	}
	if got := len(b.snapshots(t)); got != 1 {
		t.Fatalf("%d snapshots", got)
	}
}

func TestManualPullStillBacksUpEveryTime(t *testing.T) {
	_, a, b := devices(t)
	for _, text := range []string{"첫째", "둘째"} {
		a.edit(t, workID, text)
		a.push(t, workID)
		b.pull(t, PullInput{ProjectID: workID})
	}
	if got := len(b.snapshots(t)); got != 2 {
		t.Fatalf("%d snapshots", got)
	}
}

func TestAnArchivedWorkStillTravels(t *testing.T) {
	_, a, b := devices(t)
	exec(t, a.db, `UPDATE projects SET archived_at = 5 WHERE id = ?`, workID)
	a.syncAll(t, SyncPolicy{})
	b.syncAll(t, SyncPolicy{})
	if b.count(t, `SELECT COUNT(*) FROM projects WHERE id = ? AND archived_at IS NOT NULL`, workID) != 1 {
		t.Fatal("archiving did not reach the other device")
	}
}

func (d *device) deleteWork(t *testing.T, id string) {
	t.Helper()
	if err := d.svc.QueueDeletion(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	exec(t, d.db, `DELETE FROM projects WHERE id = ?`, id)
}

func TestDeletingAWorkArchivesItOnOtherDevices(t *testing.T) {
	site, a, b := devices(t)
	before := b.text(t, workID)
	a.deleteWork(t, workID)

	report := a.syncAll(t, SyncPolicy{})
	if len(report.Pulled) != 0 {
		t.Fatalf("the deleted work came back: %+v", report)
	}
	if a.count(t, `SELECT COUNT(*) FROM vault_deletions`) != 0 {
		t.Fatal("the marker stayed queued")
	}

	report = b.syncAll(t, SyncPolicy{})
	if !slices.Equal(report.Archived, []string{workID}) {
		t.Fatalf("report = %+v", report)
	}
	if b.count(t, `SELECT COUNT(*) FROM projects WHERE id = ? AND archived_at IS NOT NULL`, workID) != 1 {
		t.Fatal("the work did not move to the archive")
	}
	if b.text(t, workID) != before {
		t.Fatal("the manuscript changed")
	}

	// It settles: nothing moves on the next runs, on either device.
	commits := site.commits
	report = b.syncAll(t, SyncPolicy{})
	if len(report.Archived)+len(report.Pushed)+len(report.Pulled) != 0 || site.commits != commits {
		t.Fatalf("report = %+v", report)
	}
	report = a.syncAll(t, SyncPolicy{})
	if len(report.Pulled) != 0 || a.count(t, `SELECT COUNT(*) FROM projects WHERE id = ?`, workID) != 0 {
		t.Fatalf("the deleted work came back: %+v", report)
	}
}

func TestDeletionMarkerLeavesRestorePointsAndCannotBePulled(t *testing.T) {
	ctx := context.Background()
	site, a, b := devices(t)
	a.deleteWork(t, workID)
	a.syncAll(t, SyncPolicy{})

	overview, err := a.svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Items) != 1 || !overview.Items[0].Deleted || overview.Items[0].Title != "비밀 작품" {
		t.Fatalf("items = %+v", overview.Items)
	}
	item, _ := site.onlyItem(t)
	_, err = a.svc.Pull(ctx, PullInput{ItemID: item})
	wantReason(t, err, rpc.ReasonVaultItemNotFound)

	// An older revision is still there to fetch.
	if _, err := a.svc.Pull(ctx, PullInput{ItemID: item, Revision: 1}); err != nil {
		t.Fatalf("restoring from before the deletion: %v", err)
	}
	if !strings.Contains(a.text(t, workID), "숨겨진 첫 문장") {
		t.Fatal("the restore point did not come back")
	}
	_ = b
}

func TestDeletingOfflineWaitsForTheConnection(t *testing.T) {
	site, a, b := devices(t)
	a.deleteWork(t, workID)
	site.mu.Lock()
	site.failGetAll = true
	site.mu.Unlock()
	if _, err := a.svc.SyncAll(context.Background(), SyncPolicy{}); err == nil {
		t.Fatal("offline run succeeded")
	}
	if a.count(t, `SELECT COUNT(*) FROM vault_deletions`) != 1 {
		t.Fatal("the deletion was lost")
	}
	site.mu.Lock()
	site.failGetAll = false
	site.mu.Unlock()
	a.syncAll(t, SyncPolicy{})
	if a.count(t, `SELECT COUNT(*) FROM vault_deletions`) != 0 {
		t.Fatal("the deletion was not sent")
	}
	report := b.syncAll(t, SyncPolicy{})
	if !slices.Equal(report.Archived, []string{workID}) {
		t.Fatalf("report = %+v", report)
	}
}

func TestDeletingAWorkNeverUploadedQueuesNothing(t *testing.T) {
	site := newFakeSite(t)
	a := newDevice(t, site)
	a.seed(t, workID, []byte("그림"))
	a.deleteWork(t, workID)
	if a.count(t, `SELECT COUNT(*) FROM vault_deletions`) != 0 {
		t.Fatal("a local work queued a marker")
	}
}

func TestEditingAnArchivedDeletedWorkBringsTheItemBack(t *testing.T) {
	_, a, b := devices(t)
	a.deleteWork(t, workID)
	a.syncAll(t, SyncPolicy{})
	b.syncAll(t, SyncPolicy{})

	b.edit(t, workID, "되살린 문장")
	report := b.syncAll(t, SyncPolicy{})
	if !slices.Equal(report.Pushed, []string{workID}) {
		t.Fatalf("report = %+v", report)
	}
	report = a.syncAll(t, SyncPolicy{})
	if !slices.Equal(report.Pulled, []string{workID}) || !strings.Contains(a.text(t, workID), "되살린 문장") {
		t.Fatalf("report = %+v", report)
	}
}

func TestWorkTakenOffTheServerIsNotUploadedBack(t *testing.T) {
	ctx := context.Background()
	site, a, b := devices(t)
	item, _ := site.onlyItem(t)
	if err := a.svc.DeleteItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	for _, d := range []*device{a, b} {
		report := d.syncAll(t, SyncPolicy{})
		if got := stateOf(t, report, workID).State; got != StateRemoved || len(report.Pushed) != 0 {
			t.Fatalf("report = %+v", report)
		}
	}
	if len(site.items) != 0 {
		t.Fatalf("items = %d", len(site.items))
	}
	// A manual upload is the writer changing their mind.
	a.push(t, workID)
	if a.count(t, `SELECT COUNT(*) FROM sync_removed`) != 0 {
		t.Fatal("the mark stayed")
	}
	a.edit(t, workID, "다시 올림")
	if report := a.syncAll(t, SyncPolicy{}); !slices.Equal(report.Pushed, []string{workID}) {
		t.Fatalf("report = %+v", report)
	}
}

func TestOnlyRunTouchesOneWorkAndNotTheLibrary(t *testing.T) {
	site, a, b := devices(t)
	a.seed(t, "other", []byte("그림"))
	a.push(t, "other")
	a.edit(t, workID, "다른 기기의 문장")
	a.push(t, workID)
	a.skill(t, "style", "글")
	a.syncLibrary(t)
	b.syncLibrary(t)
	if err := os.RemoveAll(b.home + "/skills/style"); err != nil {
		t.Fatal(err)
	}
	commits := site.commits

	report := b.syncAll(t, SyncPolicy{Only: workID})

	if !slices.Equal(report.Pulled, []string{workID}) || len(report.Works) != 1 {
		t.Fatalf("report = %+v", report)
	}
	if b.count(t, `SELECT COUNT(*) FROM projects WHERE id = 'other'`) != 0 || site.commits != commits {
		t.Fatal("the run went past its one work")
	}
}

func TestAutoSyncSendsTheLibrary(t *testing.T) {
	site := newFakeSite(t)
	a := newDevice(t, site)
	a.makeKey(t)
	a.skill(t, "style", "글")
	a.syncAll(t, SyncPolicy{})
	if len(site.items) != 1 {
		t.Fatalf("items = %d", len(site.items))
	}
}

func TestAWorkFromANewerAppDoesNotStopTheOthers(t *testing.T) {
	site, a, b := devices(t)
	a.seed(t, "other", []byte("그림"))
	a.push(t, "other")

	// The head of the first work is written by something this build cannot read.
	keys := keysOf(t, a)
	id, sealed, err := keys.Seal([]byte(`{"format":99}`))
	if err != nil {
		t.Fatal(err)
	}
	site.mu.Lock()
	site.blobs[id] = sealed
	item := site.items[linkOf(t, a, workID).ItemID]
	item.head++
	item.revisions = append(item.revisions, Revision{Revision: item.head, ManifestBlob: id, Bytes: 1})
	site.mu.Unlock()

	report := b.syncAll(t, SyncPolicy{})

	if !slices.Contains(report.Pulled, "other") {
		t.Fatalf("report = %+v", report)
	}
	if got := stateOf(t, report, workID).Problem; got != rpc.ReasonVaultNewerVersion {
		t.Fatalf("problem = %q", got)
	}
}
