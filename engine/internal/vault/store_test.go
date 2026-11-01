package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newDirStore(t *testing.T, folder, device string) dirStore {
	t.Helper()
	return dirStore{folder: folder, device: device, now: time.Now}
}

func blobName(n int) string { return strings.Repeat(fmt.Sprintf("%x", n%16), 64) }

const contractItem = "item-0123456789abcdef"

// The same answers from both, because everything above the interface relies on
// them: resuming an upload on missing blobs, and refusing a stale base.
func TestStoresKeepTheSameContract(t *testing.T) {
	for name, open := range map[string]func(t *testing.T) Store{
		"works": func(t *testing.T) Store {
			site := newFakeSite(t)
			return worksStore{client: NewClient(site.srv.URL, "test"), token: testToken}
		},
		"folder": func(t *testing.T) Store { return newDirStore(t, t.TempDir(), "aaaa") },
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := open(t)
			manifest, blob := blobName(1), blobName(2)

			items, err := s.Items(ctx)
			if err != nil || len(items) != 0 {
				t.Fatalf("items %v, %v", items, err)
			}
			if _, err := s.Commit(ctx, contractItem, 0, manifest, []string{blob}); !isCode(err, CodeBlobsMissing) {
				t.Fatalf("commit before blobs: %v", err)
			}
			var api *APIError
			if !errors.As(err, &api) {
				_, err = s.Commit(ctx, contractItem, 0, manifest, []string{blob})
				if !errors.As(err, &api) {
					t.Fatal("not an API error")
				}
			}
			if !slices.Contains(api.Missing, blob) {
				t.Fatalf("missing = %v", api.Missing)
			}
			if err := s.PutBlob(ctx, blob, []byte("sealed")); err != nil {
				t.Fatal(err)
			}
			if err := s.PutBlob(ctx, manifest, []byte("manifest")); err != nil {
				t.Fatal(err)
			}
			revision, err := s.Commit(ctx, contractItem, 0, manifest, []string{blob})
			if err != nil || revision != 1 {
				t.Fatalf("commit %d, %v", revision, err)
			}
			if _, err := s.Commit(ctx, contractItem, 0, manifest, []string{blob}); !isCode(err, CodeConflict) {
				t.Fatalf("stale commit: %v", err)
			}
			got, err := s.GetBlob(ctx, blob)
			if err != nil || string(got) != "sealed" {
				t.Fatalf("blob %q, %v", got, err)
			}
			items, err = s.Items(ctx)
			if err != nil || len(items) != 1 || items[0].ItemID != contractItem || items[0].HeadRevision != 1 {
				t.Fatalf("items %+v, %v", items, err)
			}
			revisions, err := s.Revisions(ctx, contractItem)
			if err != nil || len(revisions) != 1 || revisions[0].ManifestBlob != manifest {
				t.Fatalf("revisions %+v, %v", revisions, err)
			}
			if _, err := s.Revisions(ctx, "item-does-not-exist-0"); !isCode(err, CodeNotFound) {
				t.Fatalf("revisions of nothing: %v", err)
			}
			account, err := s.Account(ctx)
			if err != nil || account.Mode != ModeFull {
				t.Fatalf("account %+v, %v", account, err)
			}
			if err := s.DeleteItem(ctx, contractItem); err != nil {
				t.Fatal(err)
			}
			if err := s.DeleteItem(ctx, contractItem); !isCode(err, CodeNotFound) {
				t.Fatalf("second delete: %v", err)
			}
		})
	}
}

// folderDevice is one library syncing through a folder, as a second device would
// see it. Sharing the folder stands in for whatever carries it.
func folderDevice(t *testing.T, folder, name string) *device {
	t.Helper()
	home := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg, err := settings.NewForHomeWithSecretStore(home, settings.NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	clock := int64(1_700_000_000_000)
	svc := NewService(Deps{
		Target:    TargetFolder,
		OpenStore: func() (Store, error) { return newDirStore(t, folder, name), nil },
		Settings:  cfg, Store: st, Home: home,
		Index: manuscript.NewIndexer(st.DB()),
		Now: func() int64 {
			clock += 1000
			return clock
		},
	})
	return &device{svc: svc, st: st, db: st.DB(), home: home, settings: cfg}
}

func folderPair(t *testing.T) (string, *device, *device) {
	t.Helper()
	ctx := context.Background()
	folder := t.TempDir()
	a, b := folderDevice(t, folder, "aaaa"), folderDevice(t, folder, "bbbb")
	key := a.makeKey(t)
	a.seed(t, workID, []byte("그림 바이트"))
	a.syncAll(t, SyncPolicy{})
	if _, err := b.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: key}); err != nil {
		t.Fatal(err)
	}
	b.syncAll(t, SyncPolicy{})
	return folder, a, b
}

func TestFolderCarriesAWorkBetweenDevices(t *testing.T) {
	_, a, b := folderPair(t)
	if !strings.Contains(b.text(t, workID), "숨겨진 첫 문장") {
		t.Fatal("the work did not arrive")
	}
	a.edit(t, workID, "폴더를 건너온 문장")
	a.syncAll(t, SyncPolicy{})
	report := b.syncAll(t, SyncPolicy{})
	if !slices.Equal(report.Pulled, []string{workID}) || !strings.Contains(b.text(t, workID), "폴더를 건너온 문장") {
		t.Fatalf("report = %+v", report)
	}
}

func TestFolderHoldsNothingReadable(t *testing.T) {
	folder, a, _ := folderPair(t)
	a.skill(t, "style", "숨겨진 스킬 문장")
	a.syncAll(t, SyncPolicy{})
	err := filepath.WalkDir(filepath.Join(folder, dirRoot), func(file string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		for _, secret := range []string{"숨겨진", "비밀 작품", "style"} {
			if strings.Contains(string(data), secret) {
				t.Errorf("%s holds %q", file, secret)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTwoDevicesWritingOneNumberEndInAConflict(t *testing.T) {
	folder, a, b := folderPair(t)
	a.edit(t, workID, "여기서 고침")
	b.edit(t, workID, "저기서 고침")

	// Each sees revision 1 as the head and writes a 2, which a slow sync allows.
	hide := filepath.Join(t.TempDir(), "hidden")
	itemDir := filepath.Join(folder, dirRoot, dirItems, linkOf(t, a, TargetFolder, workID).ItemID)
	a.syncAll(t, SyncPolicy{})
	written := func() []string {
		entries, err := listRevisionFiles(itemDir)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, entry := range entries {
			names = append(names, entry.name)
		}
		return names
	}
	first := written()[0]
	if err := os.Rename(filepath.Join(itemDir, first), hide); err != nil {
		t.Fatal(err)
	}
	b.syncAll(t, SyncPolicy{})
	if err := os.Rename(hide, filepath.Join(itemDir, first)); err != nil {
		t.Fatal(err)
	}
	if got := written(); len(got) != 3 || !strings.HasPrefix(got[0], "00000002-") || !strings.HasPrefix(got[1], "00000002-") {
		t.Fatalf("files %v", got)
	}

	// Both devices name the same head. The winner is in step; the loser is not.
	states := map[string]string{}
	for name, d := range map[string]*device{"a": a, "b": b} {
		states[name] = stateOf(t, d.syncAll(t, SyncPolicy{}), workID).State
	}
	want := map[string]string{"a": StateConflict, "b": StateSynced}
	if strings.Contains(written()[0], "aaaa") {
		want = map[string]string{"a": StateSynced, "b": StateConflict}
	}
	if states["a"] != want["a"] || states["b"] != want["b"] {
		t.Fatalf("states %v, want %v (head %s)", states, want, written()[0])
	}
}

func TestRevisionArrivingBeforeItsBlobsWaits(t *testing.T) {
	folder, a, b := folderPair(t)
	a.edit(t, workID, "조각보다 먼저 온 리비전")
	a.syncAll(t, SyncPolicy{})

	aside := t.TempDir()
	blobs := filepath.Join(folder, dirRoot, dirBlobs)
	var moved [][2]string
	revisions, err := newDirStore(t, folder, "x").readRevisions(linkOf(t, a, TargetFolder, workID).ItemID)
	if err != nil {
		t.Fatal(err)
	}
	id := revisions[0].ManifestBlob
	from := filepath.Join(blobs, id[:2], id)
	to := filepath.Join(aside, id)
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	moved = append(moved, [2]string{to, from})

	report := b.syncAll(t, SyncPolicy{})
	if len(report.Pulled) != 0 {
		t.Fatalf("report = %+v", report)
	}
	if got := stateOf(t, report, workID).Problem; got != rpc.ReasonVaultPending {
		t.Fatalf("problem = %q", got)
	}
	for _, pair := range moved {
		if err := os.Rename(pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	report = b.syncAll(t, SyncPolicy{})
	if !slices.Equal(report.Pulled, []string{workID}) {
		t.Fatalf("report = %+v", report)
	}
}

func TestInterruptedCommitLeavesOnlyUnusedBlobs(t *testing.T) {
	folder, a, b := folderPair(t)
	store := newDirStore(t, folder, "cccc")
	orphan := blobName(9)
	if err := store.PutBlob(context.Background(), orphan, []byte("x")); err != nil {
		t.Fatal(err)
	}
	report := b.syncAll(t, SyncPolicy{})
	if got := stateOf(t, report, workID).State; got != StateSynced {
		t.Fatalf("state = %s", got)
	}
	_ = a
}

func TestSweepKeepsWhatARevisionUsesAndRecentBlobs(t *testing.T) {
	folder, a, _ := folderPair(t)
	s := newDirStore(t, folder, "cccc")
	old, recent := blobName(9), blobName(10)
	for _, id := range []string{old, recent} {
		if err := s.PutBlob(context.Background(), id, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	file, _ := s.blobPath(old)
	stale := time.Now().Add(-2 * dirDay)
	if err := os.Chtimes(file, stale, stale); err != nil {
		t.Fatal(err)
	}
	revisions, err := s.readRevisions(linkOf(t, a, TargetFolder, workID).ItemID)
	if err != nil {
		t.Fatal(err)
	}
	used, _ := s.blobPath(revisions[0].ManifestBlob)
	if err := os.Chtimes(used, stale, stale); err != nil {
		t.Fatal(err)
	}

	if err := s.sweepBlobs(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("an old unused blob stayed")
	}
	if recentFile, _ := s.blobPath(recent); !fileExists(recentFile) {
		t.Error("a recent blob went: a commit may be on its way to it")
	}
	if !fileExists(used) {
		t.Error("a blob a revision uses went")
	}
}

func fileExists(file string) bool {
	_, err := os.Stat(file)
	return err == nil
}

func TestFolderPrunesLikeTheSite(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	s := dirStore{folder: t.TempDir(), device: "aaaa", now: func() time.Time { return now }}
	if err := s.prepare(); err != nil {
		t.Fatal(err)
	}
	if err := s.PutBlob(ctx, blobName(1), []byte("m")); err != nil {
		t.Fatal(err)
	}
	commit := func(n int, age time.Duration) {
		t.Helper()
		s.now = func() time.Time { return now.Add(-age) }
		if _, err := s.Commit(ctx, contractItem, n, blobName(1), nil); err != nil {
			t.Fatal(err)
		}
	}
	commit(0, 9*dirDay)
	commit(1, 3*dirDay)
	commit(2, 3*dirDay)
	commit(3, 2*dirDay)
	for n := 4; n < 11; n++ {
		commit(n, 0)
	}
	s.now = func() time.Time { return now }
	revisions, err := s.Revisions(ctx, contractItem)
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, r := range revisions {
		got = append(got, r.Revision)
	}
	if !slices.Equal(got, []int{11, 10, 9, 8, 7, 4, 3}) {
		t.Fatalf("kept %v", got)
	}
}

func TestFolderRefusesANewerLayout(t *testing.T) {
	folder := t.TempDir()
	writeFile(t, filepath.Join(folder, dirRoot, dirFormatFile), `{"format":2}`)
	if _, err := newDirStore(t, folder, "aaaa").Items(context.Background()); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("got %v", err)
	}
}

func TestMissingFolderSaysSo(t *testing.T) {
	for _, folder := range []string{"", filepath.Join(t.TempDir(), "unplugged")} {
		_, err := newDirStore(t, folder, "aaaa").Items(context.Background())
		wantReason(t, err, rpc.ReasonSyncTargetMissing)
	}
}

func TestFolderIgnoresFilesOtherAppsLeave(t *testing.T) {
	folder, a, _ := folderPair(t)
	item := linkOf(t, a, TargetFolder, workID).ItemID
	writeFile(t, filepath.Join(folder, dirRoot, dirItems, item, "00000099-aaaa-00000000 (conflicted copy).json"), "x")
	writeFile(t, filepath.Join(folder, dirRoot, dirItems, ".DS_Store"), "x")
	items, err := newDirStore(t, folder, "aaaa").Items(context.Background())
	if err != nil || len(items) != 1 || items[0].HeadRevision != 1 {
		t.Fatalf("items %+v, %v", items, err)
	}
}

func TestRevisionFileHoldsWhatTheLayoutSays(t *testing.T) {
	folder, a, _ := folderPair(t)
	item := linkOf(t, a, TargetFolder, workID).ItemID
	entries, err := listRevisionFiles(filepath.Join(folder, dirRoot, dirItems, item))
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries %v, %v", entries, err)
	}
	raw, err := os.ReadFile(filepath.Join(folder, dirRoot, dirItems, item, entries[0].name))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"revision", "manifest_blob", "blob_ids", "created", "bytes"} {
		if _, ok := fields[field]; !ok {
			t.Errorf("no %s in %s", field, raw)
		}
	}
}
