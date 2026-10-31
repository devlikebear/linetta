package vault

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/vaultbundle"
)

const workID = "P"

// device is one library with its own home and settings, on one fake site.
type device struct {
	svc        *Service
	st         *store.Store
	db         *sql.DB
	home       string
	settings   *settings.Store
	secrets    settings.SecretStore
	waits      *[]time.Duration
	reconciled *[]string
	progress   *[]Progress
}

func newDevice(t *testing.T, site *fakeSite) *device {
	t.Helper()
	home := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	secrets := settings.NewMemorySecretStore()
	cfg, err := settings.NewForHomeWithSecretStore(home, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetPublishToken(testToken); err != nil {
		t.Fatal(err)
	}
	client := NewClient(site.srv.URL, "test")
	var waits []time.Duration
	client.wait = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	var mu sync.Mutex
	var reconciled []string
	var progress []Progress
	clock := int64(1_700_000_000_000)
	svc := NewService(Deps{
		Client: client, Settings: cfg, Store: st, Home: home,
		Index: manuscript.NewIndexer(st.DB()),
		Now: func() int64 {
			clock += 1000
			return clock
		},
		Notify: func(p Progress) {
			mu.Lock()
			defer mu.Unlock()
			progress = append(progress, p)
		},
		AfterPull: func(_ context.Context, projectID string) error {
			reconciled = append(reconciled, projectID)
			return nil
		},
	})
	return &device{svc: svc, st: st, db: st.DB(), home: home, settings: cfg, secrets: secrets, waits: &waits, reconciled: &reconciled, progress: &progress}
}

func exec(t *testing.T, db *sql.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.Exec(statement, args...); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

func doc(text string) string {
	return fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":%q}]}]}`, text)
}

// seed makes a work with a picture too large for one blob, and files outside
// the database.
func (d *device) seed(t *testing.T, id string, picture []byte) {
	t.Helper()
	exec(t, d.db, `INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at) VALUES (?, '비밀 작품', '[]', 'long', '3rd', 1, 2)`, id)
	exec(t, d.db, `INSERT INTO nodes (id, project_id, ordinal, kind, label, content_doc, created_at, updated_at)
		VALUES (?, ?, 0, 'leaf', '1화', ?, 1, 2)`, id+"-n1", id, doc("숨겨진 첫 문장"))
	exec(t, d.db, `INSERT INTO illustrations (id, project_id, prompt, provider, model, mime, byte_size, data, created_at)
		VALUES (?, ?, 'p', 'p', 'm', 'image/png', ?, ?, 1)`, id+"-i1", id, len(picture), picture)
	writeFile(t, filepath.Join(vaultbundle.SkillDir(d.home, id), "style.md"), "기기의 스킬")
	writeFile(t, filepath.Join(vaultbundle.MemoryDir(d.home, id), "notes.md"), "기기의 메모리")
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (d *device) edit(t *testing.T, id, text string) {
	t.Helper()
	exec(t, d.db, `UPDATE nodes SET content_doc = ? WHERE id = ?`, doc(text), id+"-n1")
}

func (d *device) text(t *testing.T, id string) string {
	t.Helper()
	var body string
	if err := d.db.QueryRow(`SELECT content_doc FROM nodes WHERE id = ?`, id+"-n1").Scan(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func (d *device) hash(t *testing.T, id string) string {
	t.Helper()
	bundle, err := vaultbundle.Export(context.Background(), d.db, d.home, id, time.UnixMilli(1))
	if err != nil {
		t.Fatal(err)
	}
	return bundle.Manifest.ContentHash()
}

func (d *device) state(t *testing.T, id string) string {
	t.Helper()
	status, err := d.svc.WorkStatus(context.Background(), id)
	if err != nil {
		t.Fatalf("WorkStatus: %v", err)
	}
	return status.State
}

func (d *device) snapshots(t *testing.T) []string {
	t.Helper()
	var found []string
	_ = filepath.WalkDir(filepath.Join(d.home, "backups"), func(path string, entry fs.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(entry.Name(), "library-pre-vault-") && strings.HasSuffix(path, ".db") {
			found = append(found, path)
		}
		return nil
	})
	return found
}

func reasonOf(err error) string {
	var re *rpc.ReasonError
	if errors.As(err, &re) {
		return re.Reason
	}
	return ""
}

func wantReason(t *testing.T, err error, want string) {
	t.Helper()
	if got := reasonOf(err); got != want {
		t.Fatalf("error = %v (reason %q), want reason %q", err, got, want)
	}
}

// devices sets up two machines with one key, with the work pushed from the first
// and pulled to the second.
func devices(t *testing.T) (*fakeSite, *device, *device) {
	t.Helper()
	ctx := context.Background()
	site := newFakeSite(t)
	a, b := newDevice(t, site), newDevice(t, site)
	created, err := a.svc.Setup(ctx, SetupInput{Consented: true})
	if err != nil {
		t.Fatal(err)
	}
	a.seed(t, workID, bytes.Repeat([]byte("그림"), 1<<20))
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: created.RecoveryKey}); err != nil {
		t.Fatal(err)
	}
	item, _ := site.onlyItem(t)
	if _, err := b.svc.Pull(ctx, PullInput{ItemID: item}); err != nil {
		t.Fatal(err)
	}
	return site, a, b
}

func TestPushAndPullCarryAWorkToAnotherDevice(t *testing.T) {
	ctx := context.Background()
	site := newFakeSite(t)
	a, b := newDevice(t, site), newDevice(t, site)
	created, err := a.svc.Setup(ctx, SetupInput{Consented: true})
	if err != nil {
		t.Fatal(err)
	}
	picture := make([]byte, MaxPlaintext+1234)
	for i := range picture {
		picture[i] = byte(i * 7)
	}
	a.seed(t, workID, picture)
	pushed, err := a.svc.Push(ctx, workID)
	if err != nil {
		t.Fatal(err)
	}
	if pushed.HeadRevision != 1 || pushed.ItemID == workID {
		t.Fatalf("push = %+v", pushed)
	}

	site.mu.Lock()
	for id, blob := range site.blobs {
		for _, secret := range []string{"비밀 작품", "숨겨진 첫 문장", "기기의 스킬", workID + "-n1"} {
			if bytes.Contains(blob, []byte(secret)) {
				t.Fatalf("blob %s shows %q to the server", id, secret)
			}
		}
	}
	site.mu.Unlock()

	if _, err := b.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: created.RecoveryKey}); err != nil {
		t.Fatal(err)
	}
	overview, err := b.svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Items) != 1 || overview.Items[0].Title != "비밀 작품" || overview.Items[0].OnThisDevice {
		t.Fatalf("overview = %+v", overview)
	}
	pulled, err := b.svc.Pull(ctx, PullInput{ItemID: pushed.ItemID})
	if err != nil {
		t.Fatal(err)
	}
	if pulled.ProjectID != workID || !pulled.FilesRestored || pulled.SnapshotPath != "" {
		t.Fatalf("pull = %+v", pulled)
	}
	if a.hash(t, workID) != b.hash(t, workID) {
		t.Fatal("the two libraries differ")
	}
	var got []byte
	if err := b.db.QueryRow(`SELECT data FROM illustrations WHERE id = ?`, workID+"-i1").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, picture) {
		t.Fatal("a picture split across blobs came back different")
	}
	if a.state(t, workID) != StateSynced || b.state(t, workID) != StateSynced {
		t.Fatalf("states: %s, %s", a.state(t, workID), b.state(t, workID))
	}
	body, err := os.ReadFile(filepath.Join(vaultbundle.SkillDir(b.home, workID), "style.md"))
	if err != nil || string(body) != "기기의 스킬" {
		t.Fatalf("skill file: %q, %v", body, err)
	}
	hits, err := manuscript.NewSearcher(b.db, nil, manuscript.NewIndexer(b.db)).Query(ctx, workID, "숨겨진", 5)
	if err != nil || len(hits) == 0 {
		t.Fatalf("search after pull: %v, %v", hits, err)
	}
	if len(*b.reconciled) != 1 || (*b.reconciled)[0] != workID {
		t.Fatalf("site reconciled for %v", *b.reconciled)
	}
	if entries, _ := os.ReadDir(filepath.Join(b.home, incomingDir)); len(entries) != 0 {
		t.Fatalf("download cache left behind: %v", entries)
	}
	if last := (*b.progress)[len(*b.progress)-1]; last.Phase != PhaseApply || last.Done != last.Total {
		t.Fatalf("last progress = %+v", last)
	}
}

// The searcher fills an empty index by itself, so only a work that was already
// indexed shows whether a pull keeps the index in step with the text.
func TestPullReplacesWhatSearchFinds(t *testing.T) {
	ctx := context.Background()
	_, a, b := devices(t)
	searcher := manuscript.NewSearcher(b.db, nil, manuscript.NewIndexer(b.db))
	if hits, err := searcher.Query(ctx, workID, "숨겨진", 5); err != nil || len(hits) == 0 {
		t.Fatalf("before: %v, %v", hits, err)
	}
	a.edit(t, workID, "완전히 새로운 고양이")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.svc.Pull(ctx, PullInput{ProjectID: workID}); err != nil {
		t.Fatal(err)
	}
	if hits, err := searcher.Query(ctx, workID, "고양이", 5); err != nil || len(hits) == 0 {
		t.Fatalf("new text: %v, %v", hits, err)
	}
	if hits, err := searcher.Query(ctx, workID, "숨겨진", 5); err != nil || len(hits) != 0 {
		t.Fatalf("old text is still found: %v, %v", hits, err)
	}
}

func TestSecondPushSendsOnlyWhatChanged(t *testing.T) {
	ctx := context.Background()
	site, a, _ := devices(t)
	if out, err := a.svc.Push(ctx, workID); err != nil || !out.Unchanged {
		t.Fatalf("nothing changed: %+v, %v", out, err)
	}
	puts, _, commits := site.counts()
	a.edit(t, workID, "고친 문장")
	if a.state(t, workID) != StateLocalChanges {
		t.Fatal("an edit did not show as a local change")
	}
	out, err := a.svc.Push(ctx, workID)
	if err != nil {
		t.Fatal(err)
	}
	if out.HeadRevision != 2 {
		t.Fatalf("head = %d", out.HeadRevision)
	}
	morePuts, _, _ := site.counts()
	if sent := morePuts - puts; sent > 3 { // the nodes table, the hash-free manifest, nothing else
		t.Fatalf("one changed scene sent %d blobs", sent)
	}
	if _, _, after := site.counts(); after <= commits {
		t.Fatal("no commit")
	}
}

// An interrupted push leaves no record here. What was sent is on the server, and
// the retry sends only the rest.
func TestInterruptedPushResumes(t *testing.T) {
	ctx := context.Background()
	site := newFakeSite(t)
	a := newDevice(t, site)
	if _, err := a.svc.Setup(ctx, SetupInput{Consented: true}); err != nil {
		t.Fatal(err)
	}
	a.seed(t, workID, []byte("작은 그림"))
	allowed := 5
	site.failPut = func(string) int {
		if allowed == 0 {
			return http.StatusForbidden
		}
		allowed--
		return 0
	}
	if _, err := a.svc.Push(ctx, workID); err == nil {
		t.Fatal("expected the push to stop")
	}
	if len(site.items) != 0 {
		t.Fatal("a partial push made a revision")
	}
	if _, linked, _ := getLink(ctx, a.db, workID); linked {
		t.Fatal("a partial push recorded a link")
	}
	site.failPut = nil
	first := map[string]bool{}
	for id := range site.blobs {
		first[id] = true
	}
	if len(first) != 5 {
		t.Fatalf("server has %d blobs", len(first))
	}
	for id := range site.putsByID {
		delete(site.putsByID, id)
	}
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	for id, n := range site.putsByID {
		if first[id] {
			t.Fatalf("blob %s was sent again, %d times", id, n)
		}
	}
}

func TestCommitLoopFollowsAShortMissingList(t *testing.T) {
	ctx := context.Background()
	site := newFakeSite(t)
	site.missedCap = 2
	a := newDevice(t, site)
	if _, err := a.svc.Setup(ctx, SetupInput{Consented: true}); err != nil {
		t.Fatal(err)
	}
	a.seed(t, workID, []byte("그림"))
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	item, stored := site.onlyItem(t)
	if item == "" || stored.head != 1 {
		t.Fatalf("head = %d", stored.head)
	}
}

func TestInterruptedPullResumesAndLeavesTheLibraryAlone(t *testing.T) {
	ctx := context.Background()
	site, a, b := devices(t)
	a.edit(t, workID, "새 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	before := b.text(t, workID)
	allowed := 4
	site.failGet = func(string) int {
		if allowed == 0 {
			return http.StatusNotFound
		}
		allowed--
		return 0
	}
	if _, err := b.svc.Pull(ctx, PullInput{ProjectID: workID}); err == nil {
		t.Fatal("expected the pull to stop")
	}
	if b.text(t, workID) != before {
		t.Fatal("a failed download changed the manuscript")
	}
	if got := b.snapshots(t); len(got) != 0 {
		t.Fatalf("snapshot taken before the download finished: %v", got)
	}
	site.failGet = nil
	_, gets, _ := site.counts()
	if _, err := b.svc.Pull(ctx, PullInput{ProjectID: workID}); err != nil {
		t.Fatal(err)
	}
	_, after, _ := site.counts()
	blobsInRevision := len(site.items[mustItem(t, site)].revisions[1].BlobIDs)
	if fetched := after - gets; fetched > blobsInRevision-3 {
		t.Fatalf("resumed pull fetched %d of %d blobs", fetched, blobsInRevision)
	}
	if b.hash(t, workID) != a.hash(t, workID) {
		t.Fatal("pull did not land")
	}
}

func mustItem(t *testing.T, site *fakeSite) string {
	t.Helper()
	id, _ := site.onlyItem(t)
	return id
}

func TestConflictIsCaughtBeforeUploading(t *testing.T) {
	ctx := context.Background()
	site, a, b := devices(t)
	a.edit(t, workID, "A의 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if b.state(t, workID) != StateRemoteChanges {
		t.Fatalf("state = %s", b.state(t, workID))
	}
	b.edit(t, workID, "B의 문장")
	if b.state(t, workID) != StateConflict {
		t.Fatalf("state = %s", b.state(t, workID))
	}
	puts, _, _ := site.counts()
	_, err := b.svc.Push(ctx, workID)
	wantReason(t, err, rpc.ReasonVaultConflict)
	if after, _, _ := site.counts(); after != puts {
		t.Fatalf("a refused push uploaded %d blobs", after-puts)
	}
	_, err = b.svc.Pull(ctx, PullInput{ProjectID: workID})
	wantReason(t, err, rpc.ReasonVaultLocalChanges)
	if !strings.Contains(b.text(t, workID), "B의 문장") {
		t.Fatal("a refused pull changed the manuscript")
	}
}

func TestKeepLocalLeavesTheServerVersionAsARestorePoint(t *testing.T) {
	ctx := context.Background()
	_, a, b := devices(t)
	a.edit(t, workID, "A의 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	b.edit(t, workID, "B의 문장")
	out, err := b.svc.ResolveConflict(ctx, ResolveInput{ProjectID: workID, Choice: ChoiceKeepLocal})
	if err != nil {
		t.Fatal(err)
	}
	if out.Push == nil || out.Push.HeadRevision != 3 {
		t.Fatalf("resolve = %+v", out)
	}
	if b.state(t, workID) != StateSynced {
		t.Fatalf("state = %s", b.state(t, workID))
	}
	if _, err := a.svc.Pull(ctx, PullInput{ProjectID: workID, Revision: 3}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.text(t, workID), "B의 문장") {
		t.Fatal("device A did not get B's version")
	}
	if _, err := a.svc.Pull(ctx, PullInput{ProjectID: workID, Revision: 2}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.text(t, workID), "A의 문장") {
		t.Fatal("the overwritten version is not a restore point")
	}
}

func TestTakeRemoteKeepsThisDevicesVersionAsANewWork(t *testing.T) {
	ctx := context.Background()
	_, a, b := devices(t)
	a.edit(t, workID, "A의 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	b.edit(t, workID, "B의 문장")
	writeFile(t, filepath.Join(vaultbundle.SkillDir(b.home, workID), "style.md"), "B의 스킬")

	out, err := b.svc.ResolveConflict(ctx, ResolveInput{ProjectID: workID, Choice: ChoiceTakeRemote, TitleSuffix: " (이 기기)"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Pull == nil || out.Pull.CopyProjectID == "" || out.Pull.CopyProjectID == workID {
		t.Fatalf("resolve = %+v", out)
	}
	if !strings.Contains(b.text(t, workID), "A의 문장") {
		t.Fatal("the work did not take the server's version")
	}
	copyID := out.Pull.CopyProjectID
	var title, body string
	if err := b.db.QueryRow(`SELECT title FROM projects WHERE id = ?`, copyID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "비밀 작품 (이 기기)" {
		t.Fatalf("copy title = %q", title)
	}
	if err := b.db.QueryRow(`SELECT content_doc FROM nodes WHERE project_id = ?`, copyID).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "B의 문장") {
		t.Fatalf("copy holds %s", body)
	}
	skill, err := os.ReadFile(filepath.Join(vaultbundle.SkillDir(b.home, copyID), "style.md"))
	if err != nil || string(skill) != "B의 스킬" {
		t.Fatalf("copy's skill: %q, %v", skill, err)
	}
	if rows, err := manuscript.NewIndexer(b.db).ProjectRowCount(ctx, copyID); err != nil || rows == 0 {
		t.Fatalf("the copy is not indexed: %d, %v", rows, err)
	}
	if b.state(t, workID) != StateSynced {
		t.Fatalf("state = %s", b.state(t, workID))
	}
}

func TestPullSnapshotsTheLibraryAndTheFilesItReplaces(t *testing.T) {
	ctx := context.Background()
	_, a, b := devices(t)
	writeFile(t, filepath.Join(vaultbundle.MemoryDir(a.home, workID), "notes.md"), "A의 메모리")
	writeFile(t, filepath.Join(vaultbundle.MemoryDir(b.home, workID), "notes.md"), "기기의 메모리")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	// B's memory now differs from what it pulled: treat it as its last hash so
	// the pull goes ahead, and the file has to come from the snapshot.
	exec(t, b.db, `UPDATE vault_items SET last_hash = ?`, b.hash(t, workID))
	out, err := b.svc.Pull(ctx, PullInput{ProjectID: workID})
	if err != nil {
		t.Fatal(err)
	}
	if out.SnapshotPath == "" {
		t.Fatal("no snapshot")
	}
	if info, err := os.Stat(out.SnapshotPath); err != nil || info.Size() == 0 {
		t.Fatalf("snapshot: %v", err)
	}
	saved := filepath.Join(strings.TrimSuffix(out.SnapshotPath, ".db")+"-files", "memory", "notes.md")
	body, err := os.ReadFile(saved)
	if err != nil || string(body) != "기기의 메모리" {
		t.Fatalf("saved memory: %q, %v", body, err)
	}
	now, _ := os.ReadFile(filepath.Join(vaultbundle.MemoryDir(b.home, workID), "notes.md"))
	if string(now) != "A의 메모리" {
		t.Fatalf("memory now = %q", now)
	}
}

func TestPullLeavesDeviceRowsAndThePublishedIdentityAlone(t *testing.T) {
	ctx := context.Background()
	_, a, b := devices(t)
	exec(t, b.db, `INSERT INTO publish_works (project_id, remote_url, slug) VALUES (?, 'https://works.test/w', 'my-work')`, workID)
	exec(t, b.db, `INSERT INTO publish_episodes (node_id, project_id, remote_url, number, title, content_hash, remote_status, moderation_reason, published_at)
		VALUES (?, ?, 'u', 1, 't', 'h', 'public', '', 'x')`, workID+"-n1", workID)
	a.edit(t, workID, "새 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.svc.Pull(ctx, PullInput{ProjectID: workID}); err != nil {
		t.Fatal(err)
	}
	var works, episodes int
	if err := b.db.QueryRow(`SELECT (SELECT COUNT(*) FROM publish_works), (SELECT COUNT(*) FROM publish_episodes WHERE node_id = ?)`, workID+"-n1").Scan(&works, &episodes); err != nil {
		t.Fatal(err)
	}
	if works != 1 || episodes != 1 {
		t.Fatalf("publish rows after pull: %d works, %d episodes", works, episodes)
	}
}

func TestTamperedBlobLeavesTheManuscriptAndRecovers(t *testing.T) {
	ctx := context.Background()
	site, a, b := devices(t)
	a.edit(t, workID, "새 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	before := b.text(t, workID)
	stored := site.items[mustItem(t, site)]
	head := stored.revisions[len(stored.revisions)-1]
	var victim string
	for _, id := range head.BlobIDs {
		if id != head.ManifestBlob {
			victim = id
		}
	}
	original := bytes.Clone(site.blobs[victim])
	site.blobs[victim][len(original)-1] ^= 1

	_, err := b.svc.Pull(ctx, PullInput{ProjectID: workID})
	wantReason(t, err, rpc.ReasonVaultDamaged)
	if b.text(t, workID) != before {
		t.Fatal("a damaged download changed the manuscript")
	}
	if got := b.snapshots(t); len(got) != 0 {
		t.Fatalf("snapshot taken for a pull that could not happen: %v", got)
	}
	site.blobs[victim] = original
	if _, err := b.svc.Pull(ctx, PullInput{ProjectID: workID}); err != nil {
		t.Fatalf("the repaired blob stayed stuck in the cache: %v", err)
	}
}

// A row that cannot go in rolls back the whole transaction, after the snapshot.
func TestFailedApplyRollsBack(t *testing.T) {
	ctx := context.Background()
	_, a, b := devices(t)
	exec(t, a.db, `INSERT INTO nodes (id, project_id, ordinal, kind, label, created_at, updated_at) VALUES ('taken', ?, 1, 'leaf', '2화', 1, 2)`, workID)
	a.edit(t, workID, "새 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	exec(t, b.db, `INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at) VALUES ('other', '다른 작품', '[]', 'long', '3rd', 1, 1)`)
	exec(t, b.db, `INSERT INTO nodes (id, project_id, ordinal, kind, label, created_at, updated_at) VALUES ('taken', 'other', 0, 'leaf', '남의 회차', 1, 1)`)
	before, _, _ := getLink(ctx, b.db, workID)
	text := b.text(t, workID)

	_, err := b.svc.Pull(ctx, PullInput{ProjectID: workID})
	wantReason(t, err, rpc.ReasonVaultDamaged)
	if b.text(t, workID) != text {
		t.Fatal("a failed apply changed the manuscript")
	}
	after, _, _ := getLink(ctx, b.db, workID)
	if after != before {
		t.Fatalf("link moved: %+v", after)
	}
	var owner string
	if err := b.db.QueryRow(`SELECT project_id FROM nodes WHERE id = 'taken'`).Scan(&owner); err != nil || owner != "other" {
		t.Fatalf("owner = %q, %v", owner, err)
	}
}

// The link goes in with the rows, so a library is never left holding the new
// text with the old revision or the other way round.
func TestFailingToRecordTheLinkRollsBackTheWork(t *testing.T) {
	ctx := context.Background()
	site, a, b := devices(t)
	a.edit(t, workID, "A의 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	b.edit(t, workID, "B의 문장")
	exec(t, b.db, `DELETE FROM vault_items`)
	exec(t, b.db, `INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at) VALUES ('other', '다른 작품', '[]', 'long', '3rd', 1, 1)`)
	exec(t, b.db, `INSERT INTO vault_items (project_id, item_id) VALUES ('other', ?)`, mustItem(t, site))

	if _, err := b.svc.ResolveConflict(ctx, ResolveInput{ProjectID: workID, Choice: ChoiceTakeRemote, TitleSuffix: " 사본"}); err == nil {
		t.Fatal("expected the unique item id to refuse the link")
	}
	if !strings.Contains(b.text(t, workID), "B의 문장") {
		t.Fatal("the work changed without its link")
	}
}

func TestNewerSchemaIsRefusedBeforeTheSnapshot(t *testing.T) {
	ctx := context.Background()
	_, a, b := devices(t)
	exec(t, a.db, `INSERT INTO schema_migrations (version, applied_at) SELECT MAX(version) + 1, 1 FROM schema_migrations`)
	a.edit(t, workID, "미래의 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	_, err := b.svc.Pull(ctx, PullInput{ProjectID: workID})
	wantReason(t, err, rpc.ReasonVaultNewerVersion)
	if got := b.snapshots(t); len(got) != 0 {
		t.Fatalf("snapshot: %v", got)
	}
}

// The commit landed and the answer did not. The next push must not see its own
// revision as another device's.
func TestLostCommitAnswerIsNotAConflict(t *testing.T) {
	ctx := context.Background()
	site, a, _ := devices(t)
	a.edit(t, workID, "새 문장")
	site.failCommitAfterSaving = true
	// The first commit is refused for missing blobs, the second lands.
	if _, err := a.svc.Push(ctx, workID); err == nil {
		t.Fatal("expected the lost answer to surface")
	}
	out, err := a.svc.Push(ctx, workID)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Unchanged || out.HeadRevision != 2 {
		t.Fatalf("push = %+v", out)
	}
	if a.state(t, workID) != StateSynced {
		t.Fatalf("state = %s", a.state(t, workID))
	}
}

func TestBusySiteIsRetriedAndAStuckOneIsNot(t *testing.T) {
	ctx := context.Background()
	site := newFakeSite(t)
	a := newDevice(t, site)
	if _, err := a.svc.Setup(ctx, SetupInput{Consented: true}); err != nil {
		t.Fatal(err)
	}
	a.seed(t, workID, []byte("그림"))
	busy := 2
	site.retryAfter = "3"
	site.failPut = func(string) int {
		if busy > 0 {
			busy--
			return http.StatusServiceUnavailable
		}
		return 0
	}
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if len(*a.waits) != 2 || (*a.waits)[0] != 3*time.Second {
		t.Fatalf("waits = %v", *a.waits)
	}

	site.failGet = func(string) int { return http.StatusServiceUnavailable }
	site.mu.Lock()
	site.gets = 0
	site.mu.Unlock()
	_, err := a.svc.Pull(ctx, PullInput{ProjectID: workID})
	wantReason(t, err, rpc.ReasonVaultUnavailable)
	if _, gets, _ := site.counts(); gets != maxAttempts {
		t.Fatalf("stuck blob tried %d times, want %d", gets, maxAttempts)
	}
}

func TestRestoringAnOlderRevisionShowsAsALocalChange(t *testing.T) {
	ctx := context.Background()
	_, a, _ := devices(t)
	first := a.text(t, workID)
	a.edit(t, workID, "두 번째 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	out, err := a.svc.Pull(ctx, PullInput{ProjectID: workID, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.Revision != 1 || out.HeadRevision != 2 {
		t.Fatalf("pull = %+v", out)
	}
	if a.text(t, workID) != first {
		t.Fatal("not restored")
	}
	if a.state(t, workID) != StateLocalChanges {
		t.Fatalf("state = %s", a.state(t, workID))
	}
	if pushed, err := a.svc.Push(ctx, workID); err != nil || pushed.HeadRevision != 3 {
		t.Fatalf("push = %+v, %v", pushed, err)
	}
	_, err = a.svc.Pull(ctx, PullInput{ProjectID: workID, Revision: 99})
	wantReason(t, err, rpc.ReasonVaultRevisionGone)
}

// Restoring the site from a backup can take an item back a few revisions.
func TestServerGoingBackwardsIsAConflict(t *testing.T) {
	ctx := context.Background()
	site, a, _ := devices(t)
	a.edit(t, workID, "두 번째 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	text := a.text(t, workID)
	stored := site.items[mustItem(t, site)]
	stored.head = 1
	stored.revisions = stored.revisions[:1]

	if got := a.state(t, workID); got != StateConflict {
		t.Fatalf("state = %s", got)
	}
	_, err := a.svc.Pull(ctx, PullInput{ProjectID: workID})
	wantReason(t, err, rpc.ReasonVaultConflict)
	if a.text(t, workID) != text {
		t.Fatal("manuscript changed")
	}
	out, err := a.svc.ResolveConflict(ctx, ResolveInput{ProjectID: workID, Choice: ChoiceKeepLocal})
	if err != nil || out.Push.HeadRevision != 2 {
		t.Fatalf("resolve = %+v, %v", out, err)
	}
}

func TestLibraryThatLostItsLinkFindsTheItem(t *testing.T) {
	ctx := context.Background()
	site, a, _ := devices(t)
	exec(t, a.db, `DELETE FROM vault_items`)
	if got := a.state(t, workID); got != StateSynced {
		t.Fatalf("state = %s", got)
	}
	exec(t, a.db, `DELETE FROM vault_items`)
	if out, err := a.svc.Push(ctx, workID); err != nil || !out.Unchanged {
		t.Fatalf("push = %+v, %v", out, err)
	}
	site.onlyItem(t)
	exec(t, a.db, `DELETE FROM vault_items`)
	a.edit(t, workID, "다른 문장")
	_, err := a.svc.Push(ctx, workID)
	wantReason(t, err, rpc.ReasonVaultConflict)
	site.onlyItem(t)
}

func TestItemHoldingAnotherWorkIsRefused(t *testing.T) {
	ctx := context.Background()
	site, a, b := devices(t)
	a.seed(t, "Q", []byte("다른 그림"))
	if _, err := a.svc.Push(ctx, "Q"); err != nil {
		t.Fatal(err)
	}
	link, _, _ := getLink(ctx, b.db, workID)
	other, _, _ := getLink(ctx, a.db, "Q")
	site.items[link.ItemID] = site.items[other.ItemID]
	text := b.text(t, workID)
	_, err := b.svc.Pull(ctx, PullInput{ProjectID: workID})
	wantReason(t, err, rpc.ReasonVaultDamaged)
	if b.text(t, workID) != text {
		t.Fatal("manuscript changed")
	}
}

func TestIDsFromTheServerNeverBecomePaths(t *testing.T) {
	ctx := context.Background()
	site, _, b := devices(t)
	stored := site.items[mustItem(t, site)]
	stored.revisions[0].ManifestBlob = "../../../escaped"
	exec(t, b.db, `UPDATE vault_items SET base_revision = 1`)
	_, err := b.svc.Pull(ctx, PullInput{ProjectID: workID})
	wantReason(t, err, rpc.ReasonVaultDamaged)
	if _, err := os.Stat(filepath.Join(b.home, "escaped")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("a blob id escaped the cache")
	}
	svc := b.svc
	if _, err := svc.cacheDir("../../x-1234567890123456"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("cacheDir: %v", err)
	}
}

func TestOneOperationAtATime(t *testing.T) {
	ctx := context.Background()
	_, a, _ := devices(t)
	end, err := a.svc.begin()
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.svc.Push(ctx, workID)
	wantReason(t, err, rpc.ReasonVaultInProgress)
	_, err = a.svc.Pull(ctx, PullInput{ProjectID: workID})
	wantReason(t, err, rpc.ReasonVaultInProgress)
	end()
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
}

func TestSetupNeedsConsentAndShowsTheKeyOnce(t *testing.T) {
	ctx := context.Background()
	site := newFakeSite(t)
	a := newDevice(t, site)
	_, err := a.svc.Setup(ctx, SetupInput{})
	wantReason(t, err, rpc.ReasonVaultConsentRequired)
	if _, _, requests := 0, 0, site.requests; requests != 0 {
		t.Fatalf("%d requests before consent", requests)
	}
	first, err := a.svc.Setup(ctx, SetupInput{Consented: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.RecoveryKey == "" || !first.KeyStored {
		t.Fatalf("first setup = %+v", first)
	}
	if _, err := ParseRecoveryKey(first.RecoveryKey); err != nil {
		t.Fatal(err)
	}
	second, err := a.svc.Setup(ctx, SetupInput{Consented: true})
	if err != nil {
		t.Fatal(err)
	}
	if second.RecoveryKey != "" || !second.KeyStored {
		t.Fatalf("second setup = %+v", second)
	}
	stored, _, _ := a.settings.VaultKey()
	if stored != first.RecoveryKey {
		t.Fatal("the second setup changed the key")
	}
}

func TestSetupOnAnAccountWithItems(t *testing.T) {
	ctx := context.Background()
	site, a, _ := devices(t)
	key, _, _ := a.settings.VaultKey()
	fresh := newDevice(t, site)

	_, err := fresh.svc.Setup(ctx, SetupInput{Consented: true})
	wantReason(t, err, rpc.ReasonVaultKeyRequired)
	if ok, _ := fresh.settings.VaultKeyExists(); ok {
		t.Fatal("a new key was stored for an account that has one")
	}
	_, err = fresh.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: "not a key"})
	wantReason(t, err, rpc.ReasonVaultBadKey)
	wrong := NewRecoveryKey().Text()
	_, err = fresh.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: wrong})
	wantReason(t, err, rpc.ReasonVaultWrongKey)
	if ok, _ := fresh.settings.VaultKeyExists(); ok {
		t.Fatal("a key that opens nothing was stored")
	}
	if _, err := fresh.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: key}); err != nil {
		t.Fatal(err)
	}
}

// A device left with another account's key has to be able to swap it, and only
// for one the server proves right.
func TestReplacingAStaleKey(t *testing.T) {
	ctx := context.Background()
	site, a, _ := devices(t)
	right, _, _ := a.settings.VaultKey()
	stale := newDevice(t, site)
	staleKey := NewRecoveryKey().Text()
	if err := stale.settings.SetVaultKey(staleKey); err != nil {
		t.Fatal(err)
	}
	overview, err := stale.svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !overview.KeyMismatch || overview.Items[0].Title != "" {
		t.Fatalf("overview = %+v", overview)
	}
	item := overview.Items[0].ItemID
	_, err = stale.svc.Pull(ctx, PullInput{ItemID: item})
	wantReason(t, err, rpc.ReasonVaultWrongKey)

	_, err = stale.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: right})
	wantReason(t, err, rpc.ReasonVaultKeyExists)
	_, err = stale.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: NewRecoveryKey().Text(), Replace: true})
	wantReason(t, err, rpc.ReasonVaultWrongKey)
	if got, _, _ := stale.settings.VaultKey(); got != staleKey {
		t.Fatal("a failed replace lost the stored key")
	}
	if _, err := stale.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: right, Replace: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := stale.svc.Pull(ctx, PullInput{ItemID: item}); err != nil {
		t.Fatal(err)
	}
}

// The cached head came from the right key, so answering from it would approve
// any key at all.
func TestKeyCheckDoesNotTrustTheCache(t *testing.T) {
	ctx := context.Background()
	_, a, _ := devices(t)
	right, _, _ := a.settings.VaultKey()
	if overview, err := a.svc.Status(ctx); err != nil || overview.Items[0].Title == "" {
		t.Fatalf("status: %+v, %v", overview, err)
	}
	_, err := a.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: NewRecoveryKey().Text(), Replace: true})
	wantReason(t, err, rpc.ReasonVaultWrongKey)
	if got, _, _ := a.settings.VaultKey(); got != right {
		t.Fatal("a wrong key replaced the right one")
	}
}

// A library on a newer schema fills in columns the bundle never had. That is not
// the writer changing anything.
func TestPullFromAnOlderSchemaDoesNotLookLikeALocalChange(t *testing.T) {
	ctx := context.Background()
	site := newFakeSite(t)
	a, b := newDevice(t, site), newDevice(t, site)
	created, err := a.svc.Setup(ctx, SetupInput{Consented: true})
	if err != nil {
		t.Fatal(err)
	}
	a.seed(t, workID, []byte("그림"))
	exec(t, a.db, `ALTER TABLE projects DROP COLUMN synopsis`)
	exec(t, a.db, `DELETE FROM schema_migrations WHERE version = (SELECT MAX(version) FROM schema_migrations)`)
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: created.RecoveryKey}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.svc.Pull(ctx, PullInput{ItemID: mustItem(t, site)}); err != nil {
		t.Fatal(err)
	}
	if got := b.state(t, workID); got != StateSynced {
		t.Fatalf("state = %s", got)
	}
}

// The text is in by the time the files are written, so failing there reports and
// does not pretend the pull failed.
func TestFilesThatCannotLandDoNotFailThePull(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a read-only directory stops neither Windows nor root")
	}
	ctx := context.Background()
	_, a, b := devices(t)
	a.edit(t, workID, "새 문장")
	if _, err := a.svc.Push(ctx, workID); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(vaultbundle.SkillDir(b.home, workID))
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	out, err := b.svc.Pull(ctx, PullInput{ProjectID: workID})
	if err != nil {
		t.Fatal(err)
	}
	if out.FilesRestored {
		t.Fatal("reported files that did not land")
	}
	if !strings.Contains(b.text(t, workID), "새 문장") {
		t.Fatal("the text did not land")
	}
}

func TestStatusStaysOffTheNetworkUntilConnected(t *testing.T) {
	ctx := context.Background()
	site := newFakeSite(t)
	a := newDevice(t, site)
	if err := a.settings.DeletePublishToken(); err != nil {
		t.Fatal(err)
	}
	overview, err := a.svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Connected || site.requests != 0 {
		t.Fatalf("overview = %+v, %d requests", overview, site.requests)
	}
	_, err = a.svc.Push(ctx, workID)
	if reasonOf(err) != rpc.ReasonVaultNotConnected {
		t.Fatalf("push: %v", err)
	}
}

func TestStatusForAnAccountThatCannotUseTheVault(t *testing.T) {
	ctx := context.Background()
	site := newFakeSite(t)
	site.mode = ModeNone
	a := newDevice(t, site)
	overview, err := a.svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !overview.Connected || overview.Account == nil || overview.Account.Mode != ModeNone || len(overview.Items) != 0 {
		t.Fatalf("overview = %+v", overview)
	}
}

func TestRefusalsAreNotRetried(t *testing.T) {
	ctx := context.Background()
	site := newFakeSite(t)
	client := NewClient(site.srv.URL, "test")
	client.wait = func(context.Context, time.Duration) error {
		t.Fatal("a refusal was retried")
		return nil
	}
	site.failPut = func(string) int { return http.StatusForbidden }
	if err := client.PutBlob(ctx, testToken, strings.Repeat("a", 64), []byte("x")); err == nil {
		t.Fatal("expected a refusal")
	}
	if puts, _, _ := site.counts(); puts != 1 {
		t.Fatalf("%d attempts", puts)
	}
}

func TestManifestFromANewerAppIsRefused(t *testing.T) {
	keys := newKeys(t)
	id, sealed, err := keys.Seal([]byte(`{"format":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openManifest(keys, id, sealed); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("got %v", err)
	}
}

func TestSiteRefusalsBecomeReasons(t *testing.T) {
	for code, want := range map[string]string{
		CodeNotEnabled:    rpc.ReasonVaultNotEnabled,
		CodeTermsOutdated: rpc.ReasonVaultTermsOutdated,
		CodeQuotaExceeded: rpc.ReasonVaultQuotaExceeded,
		CodeTooLarge:      rpc.ReasonVaultTooLarge,
		CodeRateLimited:   rpc.ReasonVaultRateLimited,
		CodeUnauthorized:  rpc.ReasonVaultUnauthorized,
		"anything_else":   rpc.ReasonVaultRejected,
	} {
		wantReason(t, classify(&APIError{Status: 403, Code: code}), want)
	}
	wantReason(t, classify(&APIError{Status: 503, Code: CodeBusy}), rpc.ReasonVaultUnavailable)
	if reasonOf(classify(errors.New("disk full"))) != "" {
		t.Fatal("a local failure was dressed up as a vault reason")
	}
}
