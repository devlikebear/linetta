package vault

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/linetta/engine/internal/atomicfile"
	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/restore"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/vaultbundle"
)

// A work as this device sees it against the vault.
const (
	StateNotUploaded   = "not_uploaded"
	StateSynced        = "synced"
	StateLocalChanges  = "local_changes"
	StateRemoteChanges = "remote_changes"
	StateConflict      = "conflict"
)

// Answers to a conflict.
const (
	ChoiceKeepLocal  = "keep_local"
	ChoiceTakeRemote = "take_remote"
)

// Progress phases.
const (
	PhaseUpload   = "upload"
	PhaseDownload = "download"
	PhaseApply    = "apply"
)

// incomingDir keeps downloaded blobs between attempts at a pull, per item. They
// are sealed, so nothing readable is left on disk if the pull is abandoned.
const incomingDir = "vault-incoming"

// maxCommitRounds bounds the commit loop. The site names at most 1000 missing
// blobs per answer and a revision holds at most 10000, so a healthy push needs
// about eleven; more than this is the site and the app disagreeing.
const maxCommitRounds = 16

// Settings is the part of the settings store the vault uses.
type Settings interface {
	PublishToken() string
	VaultKey() (string, bool, error)
	VaultKeyExists() (bool, error)
	SetVaultKey(key string) error
	DeleteVaultKey() error
}

// Deps is what the service runs on.
type Deps struct {
	Client   *Client
	Settings Settings
	Store    *store.Store
	Home     string
	Index    *manuscript.Indexer
	// Now is unix millis.
	Now func() int64
	// Notify reports progress. It may be nil.
	Notify func(Progress)
	// AfterPull brings what this device knows about the work on the public site
	// back in line. Failing is not failing the pull: the next status check
	// tries again. It may be nil.
	AfterPull func(ctx context.Context, projectID string) error
}

// Progress is one step of a push or a pull, in blobs.
type Progress struct {
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id"`
	Phase     string `json:"phase"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
}

// Service moves whole works between this library and the vault. One operation
// runs at a time: two would race over the same snapshot and the same link.
type Service struct {
	Deps
	busy atomic.Bool

	headsMu sync.Mutex
	heads   map[string]headDigest
}

// NewService returns a service over deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, heads: map[string]headDigest{}}
}

// headDigest is what a device needs from an item's head manifest. Each one costs
// two requests, and the head of an item only changes with its revision number.
type headDigest struct {
	revision  int
	title     string
	projectID string
	hash      string
}

type session struct {
	token string
	keys  *Keys
}

func (s *Service) db() *sql.DB { return s.Store.DB() }

func (s *Service) now() time.Time { return time.UnixMilli(s.Now()) }

func (s *Service) begin() (func(), error) {
	if !s.busy.CompareAndSwap(false, true) {
		return nil, reason(rpc.ReasonVaultInProgress, errors.New("vault: another operation is running"))
	}
	return func() { s.busy.Store(false) }, nil
}

func (s *Service) token() (string, error) {
	if token := s.Settings.PublishToken(); token != "" {
		return token, nil
	}
	return "", reason(rpc.ReasonVaultNotConnected, errors.New("vault: this device is not connected to Linetta Works"))
}

func (s *Service) keys() (*Keys, error) {
	text, ok, err := s.Settings.VaultKey()
	if err != nil {
		return nil, reason(rpc.ReasonVaultKeyUnavailable, err)
	}
	if !ok {
		return nil, reason(rpc.ReasonVaultNoKey, errors.New("vault: no recovery key on this device"))
	}
	key, err := ParseRecoveryKey(text)
	if err != nil {
		return nil, reason(rpc.ReasonVaultNoKey, errors.New("vault: the stored recovery key is damaged"))
	}
	return key.Derive()
}

func (s *Service) open() (session, error) {
	token, err := s.token()
	if err != nil {
		return session{}, err
	}
	keys, err := s.keys()
	if err != nil {
		return session{}, err
	}
	return session{token: token, keys: keys}, nil
}

func (s *Service) report(p Progress) {
	if s.Notify != nil {
		s.Notify(p)
	}
}

func newItemID() string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func (s *Service) title(ctx context.Context, projectID string) (string, bool, error) {
	var title string
	err := s.db().QueryRowContext(ctx, `SELECT title FROM projects WHERE id = ?`, projectID).Scan(&title)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return title, err == nil, err
}

func (s *Service) export(ctx context.Context, projectID string) (*vaultbundle.Bundle, error) {
	if _, ok, err := s.title(ctx, projectID); err != nil {
		return nil, err
	} else if !ok {
		return nil, reason(rpc.ReasonVaultWorkNotFound, fmt.Errorf("vault: project %s not found", projectID))
	}
	return vaultbundle.Export(ctx, s.db(), s.Home, projectID, s.now())
}

// head reads an item's head manifest, once per revision.
func (s *Service) head(ctx context.Context, sess session, item RemoteItem) (headDigest, error) {
	s.headsMu.Lock()
	cached, ok := s.heads[item.ItemID]
	s.headsMu.Unlock()
	if ok && cached.revision == item.HeadRevision {
		return cached, nil
	}
	digest, err := s.readHead(ctx, sess, item.ItemID)
	if err != nil {
		return headDigest{}, err
	}
	s.headsMu.Lock()
	s.heads[item.ItemID] = digest
	s.headsMu.Unlock()
	return digest, nil
}

func (s *Service) readHead(ctx context.Context, sess session, itemID string) (headDigest, error) {
	revisions, err := s.Client.Revisions(ctx, sess.token, itemID)
	if err != nil {
		return headDigest{}, err
	}
	if len(revisions) == 0 {
		return headDigest{}, reason(rpc.ReasonVaultItemNotFound, fmt.Errorf("vault: item %s has no revisions", itemID))
	}
	sealed, err := s.Client.GetBlob(ctx, sess.token, revisions[0].ManifestBlob)
	if err != nil {
		return headDigest{}, err
	}
	m, err := openManifest(sess.keys, revisions[0].ManifestBlob, sealed)
	if err != nil {
		return headDigest{}, err
	}
	return headDigest{revision: revisions[0].Revision, title: m.Title, projectID: m.Bundle.ProjectID, hash: m.Bundle.ContentHash()}, nil
}

// resolve finds the link and the server's item for a work. A work with no link
// here may still be on the server, from a library that lost the row: looking
// through the manifests keeps it from being uploaded as a second item, and the
// base of 0 then reads as a conflict unless the contents match.
func (s *Service) resolve(ctx context.Context, sess session, projectID string) (Link, RemoteItem, bool, error) {
	items, err := s.Client.Items(ctx, sess.token)
	if err != nil {
		return Link{}, RemoteItem{}, false, err
	}
	link, linked, err := getLink(ctx, s.db(), projectID)
	if err != nil {
		return Link{}, RemoteItem{}, false, err
	}
	if linked {
		for _, item := range items {
			if item.ItemID == link.ItemID {
				return link, item, true, nil
			}
		}
		return link, RemoteItem{}, false, nil
	}
	for _, item := range items {
		digest, err := s.head(ctx, sess, item)
		if errors.Is(err, ErrWrongKey) || errors.Is(err, ErrCorrupt) || errors.Is(err, ErrNewerFormat) {
			continue // an item this device cannot read does not stand in the way
		}
		if err != nil {
			return Link{}, RemoteItem{}, false, err
		}
		if digest.projectID == projectID {
			return Link{ProjectID: projectID, ItemID: item.ItemID}, item, true, nil
		}
	}
	return Link{ProjectID: projectID, ItemID: newItemID()}, RemoteItem{}, false, nil
}

// WorkStatus is one work against the vault.
type WorkStatus struct {
	ProjectID     string         `json:"project_id"`
	State         string         `json:"state"`
	ItemID        string         `json:"item_id,omitempty"`
	BaseRevision  int            `json:"base_revision"`
	HeadRevision  int            `json:"head_revision"`
	LastSyncedAt  int64          `json:"last_synced_at"`
	RestorePoints []RestorePoint `json:"restore_points"`
}

// RestorePoint is a revision the writer can go back to.
type RestorePoint struct {
	Revision int    `json:"revision"`
	Created  string `json:"created"`
	Bytes    int64  `json:"bytes"`
}

// WorkStatus compares a work with the vault.
func (s *Service) WorkStatus(ctx context.Context, projectID string) (WorkStatus, error) {
	out, err := s.workStatus(ctx, projectID)
	return out, classify(err)
}

func (s *Service) workStatus(ctx context.Context, projectID string) (WorkStatus, error) {
	sess, err := s.open()
	if err != nil {
		return WorkStatus{}, err
	}
	bundle, err := s.export(ctx, projectID)
	if err != nil {
		return WorkStatus{}, err
	}
	link, item, found, err := s.resolve(ctx, sess, projectID)
	if err != nil {
		return WorkStatus{}, err
	}
	out := WorkStatus{ProjectID: projectID, State: StateNotUploaded, RestorePoints: []RestorePoint{}, LastSyncedAt: link.LastSyncedAt}
	if !found {
		return out, nil
	}
	out.ItemID, out.BaseRevision, out.HeadRevision = link.ItemID, link.BaseRevision, item.HeadRevision
	revisions, err := s.Client.Revisions(ctx, sess.token, item.ItemID)
	if err != nil {
		return WorkStatus{}, err
	}
	for _, r := range revisions {
		out.RestorePoints = append(out.RestorePoints, RestorePoint{Revision: r.Revision, Created: r.Created, Bytes: r.Bytes})
	}
	hash := bundle.Manifest.ContentHash()
	localChanged := hash != link.LastHash
	switch {
	case item.HeadRevision == link.BaseRevision && !localChanged:
		out.State = StateSynced
	case item.HeadRevision == link.BaseRevision:
		out.State = StateLocalChanges
	case item.HeadRevision > link.BaseRevision && !localChanged:
		out.State = StateRemoteChanges
	default:
		// Both moved, or the server went backwards. Two devices can still have
		// arrived at the same text.
		same, err := s.matchesHead(ctx, sess, item, hash)
		if err != nil {
			return WorkStatus{}, err
		}
		if !same {
			out.State = StateConflict
			return out, nil
		}
		link.BaseRevision, link.LastHash, link.LastSyncedAt = item.HeadRevision, hash, s.Now()
		if err := putLink(ctx, s.db(), link); err != nil {
			return WorkStatus{}, err
		}
		out.State, out.BaseRevision, out.LastSyncedAt = StateSynced, link.BaseRevision, link.LastSyncedAt
	}
	return out, nil
}

func (s *Service) matchesHead(ctx context.Context, sess session, item RemoteItem, hash string) (bool, error) {
	digest, err := s.head(ctx, sess, item)
	if err != nil {
		return false, err
	}
	return digest.hash == hash, nil
}

// PushResult is what a push did. Unchanged means there was nothing to send.
type PushResult struct {
	ItemID       string `json:"item_id"`
	HeadRevision int    `json:"head_revision"`
	Unchanged    bool   `json:"unchanged"`
}

// Push sends a work's current contents as a new revision.
func (s *Service) Push(ctx context.Context, projectID string) (PushResult, error) {
	end, err := s.begin()
	if err != nil {
		return PushResult{}, err
	}
	defer end()
	sess, err := s.open()
	if err != nil {
		return PushResult{}, err
	}
	out, err := s.push(ctx, sess, projectID, false)
	return out, classify(err)
}

// push compares revisions before sending anything: the site reports missing
// blobs ahead of a conflict, so a commit alone would upload the whole work only
// to be refused. overwrite is the writer answering a conflict in this device's
// favour; the server's head stays behind as a restore point.
func (s *Service) push(ctx context.Context, sess session, projectID string, overwrite bool) (PushResult, error) {
	bundle, err := s.export(ctx, projectID)
	if err != nil {
		return PushResult{}, err
	}
	hash := bundle.Manifest.ContentHash()
	link, item, found, err := s.resolve(ctx, sess, projectID)
	if err != nil {
		return PushResult{}, err
	}
	upToDate := found && item.HeadRevision == link.BaseRevision && hash == link.LastHash
	if !found {
		// Never uploaded, or the item is gone from the server. Either way the
		// next revision is the first.
		link.BaseRevision = 0
	}
	if overwrite {
		link.BaseRevision = item.HeadRevision
	}
	if item.HeadRevision != link.BaseRevision {
		same, err := s.matchesHead(ctx, sess, item, hash)
		if err != nil {
			return PushResult{}, err
		}
		if !same {
			return PushResult{}, errConflict(item.HeadRevision)
		}
		return s.recordPushed(ctx, link, hash, item.HeadRevision, true)
	}
	if upToDate {
		return PushResult{ItemID: link.ItemID, HeadRevision: item.HeadRevision, Unchanged: true}, nil
	}
	title, _, err := s.title(ctx, projectID)
	if err != nil {
		return PushResult{}, err
	}
	out, err := prepare(sess.keys, title, bundle)
	if err != nil {
		return PushResult{}, err
	}
	revision, err := s.send(ctx, sess, link, out)
	if err != nil {
		return PushResult{}, err
	}
	return s.recordPushed(ctx, link, hash, revision, false)
}

func (s *Service) recordPushed(ctx context.Context, link Link, hash string, revision int, unchanged bool) (PushResult, error) {
	link.BaseRevision, link.LastHash, link.LastSyncedAt = revision, hash, s.Now()
	if err := putLink(ctx, s.db(), link); err != nil {
		return PushResult{}, err
	}
	return PushResult{ItemID: link.ItemID, HeadRevision: revision, Unchanged: unchanged}, nil
}

func errConflict(head int) error {
	return reason(rpc.ReasonVaultConflict, &APIError{Status: 409, Code: CodeConflict, HeadRevision: head})
}

// send commits first and uploads what the site says it lacks, so an interrupted
// push resumes with no record of its own: blobs from the earlier attempt are
// already on the server, and only the rest come back as missing.
func (s *Service) send(ctx context.Context, sess session, link Link, out *outgoing) (int, error) {
	total := len(out.blobIDs) + 1
	sent := 0
	for round := 0; round < maxCommitRounds; round++ {
		revision, err := s.Client.Commit(ctx, sess.token, link.ItemID, link.BaseRevision, out.manifestID, out.blobIDs)
		if err == nil {
			s.report(Progress{ProjectID: link.ProjectID, ItemID: link.ItemID, Phase: PhaseUpload, Done: total, Total: total})
			return revision, nil
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Code != CodeBlobsMissing {
			return 0, err
		}
		sent = max(sent, total-len(apiErr.Missing))
		for _, id := range apiErr.Missing {
			sealed := out.sealedManifest
			if id != out.manifestID {
				plain, ok := out.plain[id]
				if !ok {
					return 0, fmt.Errorf("vault: the site asked for blob %s, which is not in this revision", id)
				}
				if _, sealed, err = sess.keys.Seal(plain); err != nil {
					return 0, err
				}
			}
			if err := s.Client.PutBlob(ctx, sess.token, id, sealed); err != nil {
				return 0, err
			}
			sent++
			s.report(Progress{ProjectID: link.ProjectID, ItemID: link.ItemID, Phase: PhaseUpload, Done: min(sent, total), Total: total})
		}
	}
	return 0, errors.New("vault: the site kept asking for blobs it was sent")
}

// PullInput names what to pull: a work this device has, or an item it has never
// held. Revision 0 is the head.
type PullInput struct {
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id"`
	Revision  int    `json:"revision"`
}

// PullResult is what a pull did. CopyProjectID is the work this device's version
// was kept as, when a conflict was answered that way; SnapshotPath is the
// library as it was before.
type PullResult struct {
	ProjectID     string `json:"project_id"`
	ItemID        string `json:"item_id"`
	Revision      int    `json:"revision"`
	HeadRevision  int    `json:"head_revision"`
	SnapshotPath  string `json:"snapshot_path,omitempty"`
	CopyProjectID string `json:"copy_project_id,omitempty"`
	// FilesRestored is false when the database took the work and its skill or
	// memory files did not land. The writer's text is complete either way.
	FilesRestored bool `json:"files_restored"`
}

// Pull replaces a work here with a revision from the vault, keeping every id.
func (s *Service) Pull(ctx context.Context, in PullInput) (PullResult, error) {
	end, err := s.begin()
	if err != nil {
		return PullResult{}, err
	}
	defer end()
	sess, err := s.open()
	if err != nil {
		return PullResult{}, err
	}
	out, err := s.pull(ctx, sess, in, pullOptions{})
	return out, classify(err)
}

type pullOptions struct {
	// force is the writer having already chosen the server's version.
	force bool
	// keepCopy clones this device's version as a new work first, under this
	// title suffix.
	keepCopy string
}

func (s *Service) target(ctx context.Context, sess session, in PullInput) (Link, bool, error) {
	if in.ProjectID != "" {
		link, _, found, err := s.resolve(ctx, sess, in.ProjectID)
		if err != nil {
			return Link{}, false, err
		}
		if !found {
			return Link{}, false, reason(rpc.ReasonVaultItemNotFound, fmt.Errorf("vault: work %s is not in the vault", in.ProjectID))
		}
		_, linked, err := getLink(ctx, s.db(), in.ProjectID)
		return link, linked, err
	}
	links, err := listLinks(ctx, s.db())
	if err != nil {
		return Link{}, false, err
	}
	for _, link := range links {
		if link.ItemID == in.ItemID {
			return link, true, nil
		}
	}
	return Link{ItemID: in.ItemID}, false, nil
}

// pull runs in the order that keeps a failure harmless: everything that can go
// wrong with the download happens before the library is touched, and the
// library changes in one transaction after a snapshot of it exists.
func (s *Service) pull(ctx context.Context, sess session, in PullInput, opts pullOptions) (PullResult, error) {
	link, linked, err := s.target(ctx, sess, in)
	if err != nil {
		return PullResult{}, err
	}
	revisions, err := s.Client.Revisions(ctx, sess.token, link.ItemID)
	if err != nil {
		return PullResult{}, err
	}
	if len(revisions) == 0 {
		return PullResult{}, reason(rpc.ReasonVaultItemNotFound, fmt.Errorf("vault: item %s has no revisions", link.ItemID))
	}
	head := revisions[0]
	chosen := head
	if in.Revision != 0 && in.Revision != head.Revision {
		found := false
		for _, r := range revisions {
			if r.Revision == in.Revision {
				chosen, found = r, true
			}
		}
		if !found {
			return PullResult{}, reason(rpc.ReasonVaultRevisionGone, fmt.Errorf("vault: revision %d is no longer kept", in.Revision))
		}
	}
	// The server went backwards, which restoring the site from a backup can do.
	// Taking its word for the head would lose revisions this device has seen.
	if linked && head.Revision < link.BaseRevision && !opts.force {
		return PullResult{}, errConflict(head.Revision)
	}

	cache, err := s.cacheDir(link.ItemID)
	if err != nil {
		return PullResult{}, err
	}
	bundle, err := s.fetch(ctx, sess, chosen, link, cache)
	if err != nil {
		return PullResult{}, err
	}
	projectID := bundle.Manifest.ProjectID
	if link.ProjectID != "" && link.ProjectID != projectID {
		return PullResult{}, fmt.Errorf("%w: item %s holds a different work", ErrCorrupt, link.ItemID)
	}
	link.ProjectID = projectID
	// A work on this device under an item this library did not know about.
	if !linked {
		existing, ok, err := getLink(ctx, s.db(), projectID)
		if err != nil {
			return PullResult{}, err
		}
		if ok {
			link, linked = existing, true
			link.ItemID = in.ItemID
		}
	}
	schema, err := vaultbundle.SchemaVersion(ctx, s.db())
	if err != nil {
		return PullResult{}, err
	}
	if err := vaultbundle.Validate(bundle, schema); err != nil {
		return PullResult{}, err
	}

	_, exists, err := s.title(ctx, projectID)
	if err != nil {
		return PullResult{}, err
	}
	if exists && !opts.force {
		if err := s.refuseIfChanged(ctx, projectID, link, linked, bundle); err != nil {
			return PullResult{}, err
		}
	}

	out := PullResult{ProjectID: projectID, ItemID: link.ItemID, Revision: chosen.Revision, HeadRevision: head.Revision}
	if exists {
		if out.SnapshotPath, err = s.snapshot(ctx, projectID); err != nil {
			return PullResult{}, err
		}
	}
	if exists && opts.keepCopy != "" {
		if out.CopyProjectID, err = s.keepCopy(ctx, out.SnapshotPath, projectID, opts.keepCopy); err != nil {
			return PullResult{}, err
		}
	}

	s.report(Progress{ProjectID: projectID, ItemID: link.ItemID, Phase: PhaseApply, Done: 0, Total: 1})
	// The base is the head even when an older revision was asked for, and the
	// hash is the head's: the restored text then shows as a local change the
	// writer can push, and the head stays a restore point.
	link.BaseRevision, link.LastSyncedAt = head.Revision, s.Now()
	link.LastHash = bundle.Manifest.ContentHash()
	if chosen.Revision != head.Revision {
		digest, err := s.head(ctx, sess, RemoteItem{ItemID: link.ItemID, HeadRevision: head.Revision})
		if err != nil {
			return PullResult{}, err
		}
		link.LastHash = digest.hash
	}
	if err := s.applyBundle(ctx, bundle, link); err != nil {
		return PullResult{}, err
	}
	s.finishPull(ctx, bundle, &link, chosen.Revision == head.Revision, &out)
	_ = os.RemoveAll(cache)
	s.report(Progress{ProjectID: projectID, ItemID: link.ItemID, Phase: PhaseApply, Done: 1, Total: 1})
	return out, nil
}

func (s *Service) refuseIfChanged(ctx context.Context, projectID string, link Link, linked bool, bundle *vaultbundle.Bundle) error {
	current, err := s.export(ctx, projectID)
	if err != nil {
		return err
	}
	hash := current.Manifest.ContentHash()
	if linked && hash == link.LastHash {
		return nil
	}
	if hash == bundle.Manifest.ContentHash() {
		return nil
	}
	return reason(rpc.ReasonVaultLocalChanges, fmt.Errorf("vault: work %s has changes that are not in the vault", projectID))
}

func (s *Service) cacheDir(itemID string) (string, error) {
	if !itemIDPattern.MatchString(itemID) {
		return "", fmt.Errorf("%w: bad item id", ErrCorrupt)
	}
	dir := filepath.Join(s.Home, incomingDir, itemID)
	return dir, os.MkdirAll(dir, 0o700)
}

// sealedBlob returns a blob from the cache or the server, and keeps what it
// downloads so an interrupted pull does not fetch it twice.
func (s *Service) sealedBlob(ctx context.Context, sess session, cache, id string) ([]byte, error) {
	if !blobIDPattern.MatchString(id) {
		return nil, fmt.Errorf("%w: bad blob id", ErrCorrupt)
	}
	path := filepath.Join(cache, id)
	if sealed, err := os.ReadFile(path); err == nil {
		return sealed, nil
	}
	sealed, err := s.Client.GetBlob(ctx, sess.token, id)
	if err != nil {
		return nil, err
	}
	if err := atomicfile.Write(path, sealed, 0o600); err != nil {
		return nil, err
	}
	return sealed, nil
}

// fetch downloads and opens a revision. Open proves each blob is the one its ID
// names, and a blob that fails is dropped from the cache, so damage on the
// server that gets repaired does not stay stuck here.
func (s *Service) fetch(ctx context.Context, sess session, revision Revision, link Link, cache string) (*vaultbundle.Bundle, error) {
	sealed, err := s.sealedBlob(ctx, sess, cache, revision.ManifestBlob)
	if err != nil {
		return nil, err
	}
	m, err := openManifest(sess.keys, revision.ManifestBlob, sealed)
	if err != nil {
		_ = os.Remove(filepath.Join(cache, revision.ManifestBlob))
		return nil, err
	}
	total := 0
	for _, part := range m.Bundle.Parts {
		total += len(m.Blobs[part.Name])
	}
	bundle := &vaultbundle.Bundle{Manifest: m.Bundle, Parts: map[string][]byte{}}
	done := 0
	for _, part := range m.Bundle.Parts {
		data := make([]byte, 0, part.Size)
		for _, id := range m.Blobs[part.Name] {
			sealed, err := s.sealedBlob(ctx, sess, cache, id)
			if err != nil {
				return nil, err
			}
			plain, err := sess.keys.Open(id, sealed)
			if err != nil {
				_ = os.Remove(filepath.Join(cache, id))
				return nil, err
			}
			data = append(data, plain...)
			done++
			s.report(Progress{ProjectID: link.ProjectID, ItemID: link.ItemID, Phase: PhaseDownload, Done: done, Total: total})
		}
		bundle.Parts[part.Name] = data
	}
	return bundle, nil
}

// snapshot keeps the library, and the work's files that VACUUM INTO cannot see,
// because a pull is about to replace them.
func (s *Service) snapshot(ctx context.Context, projectID string) (string, error) {
	path, err := backup.RunBeforeVaultPull(ctx, s.db(), s.Home, s.now())
	if err != nil {
		return "", err
	}
	files := strings.TrimSuffix(path, ".db") + "-files"
	for name, dir := range map[string]string{
		"skills": vaultbundle.SkillDir(s.Home, projectID),
		"memory": vaultbundle.MemoryDir(s.Home, projectID),
	} {
		if err := copyDir(filepath.Join(files, name), dir); err != nil {
			return "", err
		}
	}
	return path, nil
}

func copyDir(dst, src string) error {
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return os.CopyFS(dst, os.DirFS(src))
}

// keepCopy clones this device's version from the snapshot just taken, through
// the same path the backup screen restores a work with. It makes new ids, which
// is why a pull cannot use it, and why it suits a copy that must stay apart from
// the one the site knows.
func (s *Service) keepCopy(ctx context.Context, snapshotPath, projectID, suffix string) (string, error) {
	merged, err := restore.MergeProject(ctx, s.Store, snapshotPath, "", projectID, suffix, s.now())
	if err != nil {
		return "", err
	}
	if err := s.Index.Rebuild(ctx, merged.ProjectID); err != nil {
		log.Printf("vault: index the kept copy: %v", err)
	}
	if err := copyDir(vaultbundle.SkillDir(s.Home, merged.ProjectID), vaultbundle.SkillDir(s.Home, projectID)); err != nil {
		return "", err
	}
	return merged.ProjectID, copyDir(vaultbundle.MemoryDir(s.Home, merged.ProjectID), vaultbundle.MemoryDir(s.Home, projectID))
}

func (s *Service) applyBundle(ctx context.Context, bundle *vaultbundle.Bundle, link Link) error {
	tx, err := s.db().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := vaultbundle.Apply(ctx, tx, bundle); err != nil {
		return err
	}
	if err := putLink(ctx, tx, link); err != nil {
		return err
	}
	return tx.Commit()
}

// finishPull is what follows the commit. None of it can undo the pull, so a
// failure is logged and reported, not returned: the writer's text is in.
func (s *Service) finishPull(ctx context.Context, bundle *vaultbundle.Bundle, link *Link, isHead bool, out *PullResult) {
	projectID := bundle.Manifest.ProjectID
	if err := s.Index.Rebuild(ctx, projectID); err != nil {
		log.Printf("vault: rebuild search index: %v", err)
	}
	out.FilesRestored = true
	if err := vaultbundle.ApplyFiles(s.Home, bundle); err != nil {
		log.Printf("vault: restore work files: %v", err)
		out.FilesRestored = false
	}
	// A bundle from an older schema re-exports with this one's columns, and that
	// must not read as a change the writer made.
	if isHead && out.FilesRestored {
		if current, err := s.export(ctx, projectID); err == nil {
			link.LastHash = current.Manifest.ContentHash()
			if err := putLink(ctx, s.db(), *link); err != nil {
				log.Printf("vault: record pulled hash: %v", err)
			}
		}
	}
	if s.AfterPull != nil {
		if err := s.AfterPull(ctx, projectID); err != nil {
			log.Printf("vault: reconcile with the site: %v", err)
		}
	}
}

// ResolveInput answers a conflict. TitleSuffix names the copy take_remote keeps.
type ResolveInput struct {
	ProjectID   string `json:"project_id"`
	Choice      string `json:"choice"`
	TitleSuffix string `json:"title_suffix"`
}

// ResolveResult is the outcome of whichever side was chosen.
type ResolveResult struct {
	Push *PushResult `json:"push,omitempty"`
	Pull *PullResult `json:"pull,omitempty"`
}

// ResolveConflict settles a work both sides changed. Neither answer loses a
// version: keep_local leaves the server's head as a restore point, and
// take_remote keeps this device's version as a new work.
func (s *Service) ResolveConflict(ctx context.Context, in ResolveInput) (ResolveResult, error) {
	end, err := s.begin()
	if err != nil {
		return ResolveResult{}, err
	}
	defer end()
	sess, err := s.open()
	if err != nil {
		return ResolveResult{}, err
	}
	if in.Choice == ChoiceKeepLocal {
		out, err := s.push(ctx, sess, in.ProjectID, true)
		if err != nil {
			return ResolveResult{}, classify(err)
		}
		return ResolveResult{Push: &out}, nil
	}
	out, err := s.pull(ctx, sess, PullInput{ProjectID: in.ProjectID}, pullOptions{force: true, keepCopy: in.TitleSuffix})
	if err != nil {
		return ResolveResult{}, classify(err)
	}
	return ResolveResult{Pull: &out}, nil
}

// SetupInput makes or enters the recovery key. RecoveryKey empty means make one.
// Replace swaps a different key already on this device for the one entered.
type SetupInput struct {
	Consented   bool   `json:"consented"`
	RecoveryKey string `json:"recovery_key"`
	Replace     bool   `json:"replace"`
}

// SetupResult carries the key text in exactly one case: the call that created
// it. It is the only response in the engine that ever does. That key is not
// stored yet: the text cannot be shown a second time, so it seals nothing until
// the writer has saved it and hands it back as SetupInput.RecoveryKey. Leaving
// before then costs a new key, not a key nobody kept.
type SetupResult struct {
	KeyStored   bool   `json:"key_stored"`
	RecoveryKey string `json:"recovery_key,omitempty"`
}

// Setup prepares this device. The writer agreeing that sealed copies of a work
// leave the device is required here, and a device with no key cannot push.
func (s *Service) Setup(ctx context.Context, in SetupInput) (SetupResult, error) {
	end, err := s.begin()
	if err != nil {
		return SetupResult{}, err
	}
	defer end()
	out, err := s.setup(ctx, in)
	return out, classify(err)
}

func (s *Service) setup(ctx context.Context, in SetupInput) (SetupResult, error) {
	if !in.Consented {
		return SetupResult{}, reason(rpc.ReasonVaultConsentRequired, errors.New("vault: the writer has not agreed to send sealed copies to the site"))
	}
	token, err := s.token()
	if err != nil {
		return SetupResult{}, err
	}
	items, err := s.Client.Items(ctx, token)
	if err != nil {
		return SetupResult{}, err
	}
	if in.RecoveryKey == "" {
		return s.createKey(items)
	}
	key, err := ParseRecoveryKey(in.RecoveryKey)
	if err != nil {
		return SetupResult{}, err
	}
	keys, err := key.Derive()
	if err != nil {
		return SetupResult{}, err
	}
	// Checked against the server, so a wrong key never replaces a right one.
	if len(items) > 0 {
		if _, err := s.readHead(ctx, session{token: token, keys: keys}, items[0].ItemID); err != nil {
			return SetupResult{}, err
		}
	}
	if err := s.storeKey(key.Text(), in.Replace && len(items) > 0); err != nil {
		return SetupResult{}, err
	}
	s.headsMu.Lock()
	s.heads = map[string]headDigest{}
	s.headsMu.Unlock()
	return SetupResult{KeyStored: true}, nil
}

func (s *Service) createKey(items []RemoteItem) (SetupResult, error) {
	exists, err := s.Settings.VaultKeyExists()
	if err != nil {
		return SetupResult{}, reason(rpc.ReasonVaultKeyUnavailable, err)
	}
	if exists {
		return SetupResult{KeyStored: true}, nil
	}
	// A new key could not open what the account already holds.
	if len(items) > 0 {
		return SetupResult{}, reason(rpc.ReasonVaultKeyRequired, errors.New("vault: the account has items sealed under an existing key"))
	}
	return SetupResult{RecoveryKey: NewRecoveryKey().Text()}, nil
}

func (s *Service) storeKey(text string, replace bool) error {
	err := s.Settings.SetVaultKey(text)
	if errors.Is(err, settings.ErrVaultKeyExists) {
		if !replace {
			return err
		}
		if err := s.Settings.DeleteVaultKey(); err != nil {
			return reason(rpc.ReasonVaultKeyUnavailable, err)
		}
		err = s.Settings.SetVaultKey(text)
	}
	if err != nil {
		return reason(rpc.ReasonVaultKeyUnavailable, err)
	}
	return nil
}

// Overview is the account and what it holds, as the settings pane shows it.
type Overview struct {
	Connected bool `json:"connected"`
	KeyStored bool `json:"key_stored"`
	// KeyMismatch is an item this device's key cannot open: the writer has to
	// enter the key they made elsewhere.
	KeyMismatch bool       `json:"key_mismatch"`
	Account     *Account   `json:"account"`
	Items       []ItemView `json:"items"`
}

// ItemView is one item. Title and ProjectID are empty until the device has a key
// that opens it.
type ItemView struct {
	ItemID       string `json:"item_id"`
	ProjectID    string `json:"project_id"`
	Title        string `json:"title"`
	HeadRevision int    `json:"head_revision"`
	BaseRevision int    `json:"base_revision"`
	Updated      string `json:"updated"`
	Bytes        int64  `json:"bytes"`
	// OnThisDevice says the work is in this library. An item that is not can
	// only be pulled.
	OnThisDevice bool `json:"on_this_device"`
}

// Status reads the account. It says nothing to the site before this device is
// connected.
func (s *Service) Status(ctx context.Context) (Overview, error) {
	out, err := s.status(ctx)
	return out, classify(err)
}

func (s *Service) status(ctx context.Context) (Overview, error) {
	out := Overview{Items: []ItemView{}}
	token, err := s.token()
	if err != nil {
		return out, nil
	}
	out.Connected = true
	if out.KeyStored, err = s.Settings.VaultKeyExists(); err != nil {
		return out, reason(rpc.ReasonVaultKeyUnavailable, err)
	}
	account, err := s.Client.Account(ctx, token)
	if err != nil {
		return out, err
	}
	out.Account = &account
	if account.Mode == ModeNone {
		return out, nil
	}
	items, err := s.Client.Items(ctx, token)
	if err != nil {
		return out, err
	}
	links, err := listLinks(ctx, s.db())
	if err != nil {
		return out, err
	}
	linked := map[string]Link{}
	for _, link := range links {
		linked[link.ItemID] = link
	}
	var sess session
	if out.KeyStored {
		if sess, err = s.open(); err != nil {
			return out, err
		}
	}
	for _, item := range items {
		view := ItemView{ItemID: item.ItemID, HeadRevision: item.HeadRevision, Updated: item.Updated, Bytes: item.Bytes}
		if link, ok := linked[item.ItemID]; ok {
			view.ProjectID, view.BaseRevision = link.ProjectID, link.BaseRevision
		}
		if sess.keys != nil {
			digest, err := s.head(ctx, sess, item)
			switch {
			case errors.Is(err, ErrWrongKey):
				out.KeyMismatch = true
			case errors.Is(err, ErrCorrupt), errors.Is(err, ErrNewerFormat):
				log.Printf("vault: read item: %v", err)
			case err != nil:
				return out, err
			default:
				view.ProjectID, view.Title = digest.projectID, digest.title
			}
		}
		if view.ProjectID != "" {
			if _, view.OnThisDevice, err = s.title(ctx, view.ProjectID); err != nil {
				return out, err
			}
		}
		out.Items = append(out.Items, view)
	}
	return out, nil
}
