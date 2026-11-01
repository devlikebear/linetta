package vault

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/vaultbundle"
)

// States only automatic sync reports.
const (
	// StateDeleted is a work another device deleted. It stays here, archived.
	StateDeleted = "deleted"
	// StateRemoved is a work the server no longer holds, because the writer took
	// it off. Only a manual upload puts it back.
	StateRemoved = "removed"
)

// SyncPolicy is what the caller knows and the library does not. A nil function
// answers false, except Settled, where nil means every work has settled.
type SyncPolicy struct {
	// Open says the work is on screen: pulling would replace it underneath the
	// writer, so it waits for them.
	Open func(projectID string) bool
	// Busy says an agent is writing to the work: nothing moves either way.
	Busy func(projectID string) bool
	// Settled says the writer has stopped editing long enough to upload.
	Settled func(projectID string) bool
	// Only limits the run to one work, skipping the library and other works.
	Only string
}

func (p SyncPolicy) open(id string) bool { return p.Open != nil && p.Open(id) }
func (p SyncPolicy) busy(id string) bool { return p.Busy != nil && p.Busy(id) }
func (p SyncPolicy) settled(id string) bool {
	return p.Settled == nil || p.Settled(id)
}

// WorkSync is one work after a run. Problem is a reason code for a work that
// could not be moved while the others could.
type WorkSync struct {
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id,omitempty"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Problem   string `json:"problem,omitempty"`
}

// SyncReport is what one run found and did.
type SyncReport struct {
	Works    []WorkSync `json:"works"`
	Pushed   []string   `json:"pushed"`
	Pulled   []string   `json:"pulled"`
	Archived []string   `json:"archived"`
}

// workProblems are reasons that concern one work. Anything else, a refused
// account or a dead connection, would fail for every work the same way.
var workProblems = map[string]bool{
	rpc.ReasonVaultDamaged:      true,
	rpc.ReasonVaultNewerVersion: true,
	rpc.ReasonVaultLocalChanges: true,
	rpc.ReasonVaultTooLarge:     true,
	rpc.ReasonVaultWorkNotFound: true,
	rpc.ReasonVaultItemNotFound: true,
	rpc.ReasonVaultConflict:     true,
}

func reasonCode(err error) string {
	var re *rpc.ReasonError
	if errors.As(classify(err), &re) {
		return re.Reason
	}
	return ""
}

// SyncAll compares every work with the vault and moves what can be moved without
// asking: works unchanged here are taken, works unchanged there are sent, and
// anything else is reported and left alone. It runs under the same one at a time
// rule as a manual push.
func (s *Service) SyncAll(ctx context.Context, policy SyncPolicy) (SyncReport, error) {
	end, err := s.begin()
	if err != nil {
		return SyncReport{}, err
	}
	defer end()
	sess, err := s.open()
	if err != nil {
		return SyncReport{}, err
	}
	out, err := s.syncAll(ctx, sess, policy)
	return out, classify(err)
}

func (s *Service) syncAll(ctx context.Context, sess session, policy SyncPolicy) (SyncReport, error) {
	out := SyncReport{Works: []WorkSync{}, Pushed: []string{}, Pulled: []string{}, Archived: []string{}}
	account, err := s.Client.Account(ctx, sess.token)
	if err != nil {
		return out, err
	}
	switch account.Mode {
	case ModeFull:
	case ModeTermsOutdated:
		return out, reason(rpc.ReasonVaultTermsOutdated, errors.New("vault: the agreement changed"))
	default:
		return out, reason(rpc.ReasonVaultNotEnabled, fmt.Errorf("vault: account is %s", account.Mode))
	}
	if policy.Only == "" {
		if err := s.syncLibrary(ctx, sess); err != nil {
			if !errors.Is(err, ErrNewerFormat) && !errors.Is(err, vaultbundle.ErrNewerFormat) && !errors.Is(err, ErrCorrupt) && !errors.Is(err, vaultbundle.ErrCorrupt) {
				return out, err
			}
			log.Printf("vault: library: %v", err)
		}
		if err := s.flushDeletions(ctx, sess); err != nil {
			return out, err
		}
	}
	remote, err := s.scanRemote(ctx, sess)
	if err != nil {
		return out, err
	}
	works, err := s.localWorks(ctx, policy.Only)
	if err != nil {
		return out, err
	}
	for _, work := range works {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		line := WorkSync{ProjectID: work.id, Title: work.title}
		err := s.syncWork(ctx, sess, policy, remote, work, &line, &out)
		if err != nil {
			code := reasonCode(err)
			if !workProblems[code] {
				return out, err
			}
			line.Problem = code
		}
		out.Works = append(out.Works, line)
	}
	if policy.Only != "" {
		return out, nil
	}
	for _, entry := range remote.unclaimed() {
		line := WorkSync{ProjectID: entry.digest.projectID, ItemID: entry.item.ItemID, Title: entry.digest.title, State: StateRemoteChanges}
		switch {
		case entry.digest.deleted:
			line.State = StateDeleted
		case entry.problem != nil:
			line.Problem = reasonCode(entry.problem)
		default:
			pulled, err := s.pull(ctx, sess, PullInput{ItemID: entry.item.ItemID}, pullOptions{auto: true})
			if err != nil {
				code := reasonCode(err)
				if !workProblems[code] {
					return out, err
				}
				line.Problem = code
				break
			}
			line.State = StateSynced
			line.ProjectID = pulled.ProjectID
			out.Pulled = append(out.Pulled, pulled.ProjectID)
		}
		out.Works = append(out.Works, line)
	}
	return out, nil
}

type localWork struct {
	id, title string
}

func (s *Service) localWorks(ctx context.Context, only string) ([]localWork, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT id, title FROM projects ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []localWork
	for rows.Next() {
		var work localWork
		if err := rows.Scan(&work.id, &work.title); err != nil {
			return nil, err
		}
		if only == "" || only == work.id {
			out = append(out, work)
		}
	}
	return out, rows.Err()
}

// remoteEntry is one server item as far as this run needs it. digest is only
// read when the head moved or nothing here knows the item: one Items call says
// everything about the rest.
type remoteEntry struct {
	item    RemoteItem
	digest  headDigest
	read    bool
	problem error
	claimed bool
}

type remoteScan struct {
	byItem    map[string]*remoteEntry
	byProject map[string]*remoteEntry
	links     map[string]Link
	removed   map[string]bool
}

func (r *remoteScan) unclaimed() []*remoteEntry {
	var out []*remoteEntry
	for _, entry := range r.byItem {
		if !entry.claimed {
			out = append(out, entry)
		}
	}
	return out
}

func (s *Service) scanRemote(ctx context.Context, sess session) (*remoteScan, error) {
	items, err := s.Client.Items(ctx, sess.token)
	if err != nil {
		return nil, err
	}
	links, err := listLinks(ctx, s.db())
	if err != nil {
		return nil, err
	}
	scan := &remoteScan{byItem: map[string]*remoteEntry{}, byProject: map[string]*remoteEntry{}, links: map[string]Link{}, removed: map[string]bool{}}
	linkedItems := map[string]Link{}
	for _, link := range links {
		scan.links[link.ProjectID] = link
		linkedItems[link.ItemID] = link
	}
	rows, err := s.db().QueryContext(ctx, `SELECT project_id FROM sync_removed`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		scan.removed[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// A marker that has not landed yet must not read as a work to download.
	pending, err := s.pendingDeletions(ctx)
	if err != nil {
		return nil, err
	}
	leaving := map[string]bool{}
	for _, p := range pending {
		leaving[p.itemID] = true
	}
	for _, item := range items {
		if item.ItemID == sess.keys.LibraryItemID() || leaving[item.ItemID] {
			continue
		}
		entry := &remoteEntry{item: item}
		scan.byItem[item.ItemID] = entry
		link, linked := linkedItems[item.ItemID]
		if linked && link.BaseRevision == item.HeadRevision {
			scan.byProject[link.ProjectID] = entry
			continue
		}
		digest, err := s.head(ctx, sess, item)
		switch {
		case errors.Is(err, ErrCorrupt), errors.Is(err, ErrNewerFormat):
			entry.problem = err
		case err != nil:
			return nil, err
		default:
			entry.digest, entry.read = digest, true
			scan.byProject[digest.projectID] = entry
		}
		if linked {
			scan.byProject[link.ProjectID] = entry
		}
	}
	return scan, nil
}

func (s *Service) syncWork(ctx context.Context, sess session, policy SyncPolicy, remote *remoteScan, work localWork, line *WorkSync, out *SyncReport) error {
	link, linked := remote.links[work.id]
	entry, found := remote.byProject[work.id]
	if found {
		entry.claimed = true
		line.ItemID = entry.item.ItemID
	}
	if !found {
		line.State = StateNotUploaded
		if linked || remote.removed[work.id] {
			line.State = StateRemoved
			return nil
		}
		if policy.busy(work.id) || !policy.settled(work.id) {
			return nil
		}
		return s.autoPush(ctx, sess, work.id, line, out)
	}
	if entry.problem != nil {
		line.State = StateConflict
		return entry.problem
	}
	if entry.read && entry.digest.deleted {
		return s.takeDeletion(ctx, policy, work, entry, line, out)
	}
	bundle, err := s.export(ctx, work.id)
	if err != nil {
		return err
	}
	hash := bundle.Manifest.ContentHash()
	if !linked {
		link = Link{ProjectID: work.id, ItemID: entry.item.ItemID}
	}
	state, err := s.compare(ctx, sess, &link, entry.item, hash)
	if err != nil {
		return err
	}
	line.State = state
	if policy.busy(work.id) {
		return nil
	}
	switch state {
	case StateLocalChanges:
		if policy.settled(work.id) {
			return s.autoPush(ctx, sess, work.id, line, out)
		}
	case StateRemoteChanges:
		if !policy.open(work.id) {
			if _, err := s.pull(ctx, sess, PullInput{ProjectID: work.id}, pullOptions{auto: true}); err != nil {
				return err
			}
			line.State = StateSynced
			out.Pulled = append(out.Pulled, work.id)
		}
	}
	return nil
}

func (s *Service) autoPush(ctx context.Context, sess session, id string, line *WorkSync, out *SyncReport) error {
	pushed, err := s.push(ctx, sess, id, false)
	if isConflict(err) {
		line.State = StateConflict
		return nil
	}
	if err != nil {
		return err
	}
	line.State, line.ItemID = StateSynced, pushed.ItemID
	if !pushed.Unchanged {
		out.Pushed = append(out.Pushed, id)
	}
	return nil
}

func isConflict(err error) bool {
	return reasonCode(err) == rpc.ReasonVaultConflict
}

// takeDeletion answers a deletion marker. Deleting travels as far as the archive
// and no further: a bug or a slip on one device must not take the manuscript off
// all of them, so the writer does the real deleting here, by hand. The link
// moves to the marker, so the archived work reads as in step, and writing in it
// again would bring the item back to life.
func (s *Service) takeDeletion(ctx context.Context, policy SyncPolicy, work localWork, entry *remoteEntry, line *WorkSync, out *SyncReport) error {
	line.State = StateDeleted
	if policy.open(work.id) || policy.busy(work.id) {
		return nil
	}
	result, err := s.db().ExecContext(ctx, `UPDATE projects SET archived_at = ? WHERE id = ? AND archived_at IS NULL`, s.Now(), work.id)
	if err != nil {
		return err
	}
	archived, err := result.RowsAffected()
	if err != nil {
		return err
	}
	bundle, err := s.export(ctx, work.id)
	if err != nil {
		return err
	}
	link := Link{ProjectID: work.id, ItemID: entry.item.ItemID, BaseRevision: entry.item.HeadRevision, LastHash: bundle.Manifest.ContentHash(), LastSyncedAt: s.Now()}
	if err := putLink(ctx, s.db(), link); err != nil {
		return err
	}
	if archived > 0 {
		out.Archived = append(out.Archived, work.id)
	}
	return nil
}

// QueueDeletion remembers that a work on the server is about to be deleted here.
// Call it before the project row goes, which takes its link along. A work with
// no link never left this device and has nothing to tell anyone.
func (s *Service) QueueDeletion(ctx context.Context, projectID string) error {
	link, linked, err := getLink(ctx, s.db(), projectID)
	if err != nil || !linked {
		return err
	}
	title, _, err := s.title(ctx, projectID)
	if err != nil {
		return err
	}
	_, err = s.db().ExecContext(ctx, `
INSERT INTO vault_deletions (item_id, project_id, title, queued_at) VALUES (?, ?, ?, ?)
ON CONFLICT(item_id) DO NOTHING`, link.ItemID, projectID, title, s.Now())
	return err
}

type pendingDeletion struct {
	itemID, projectID, title string
}

func (s *Service) pendingDeletions(ctx context.Context) ([]pendingDeletion, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT item_id, project_id, title FROM vault_deletions ORDER BY queued_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pendingDeletion
	for rows.Next() {
		var p pendingDeletion
		if err := rows.Scan(&p.itemID, &p.projectID, &p.title); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// flushDeletions commits a marker for each deleted work, on top of whatever the
// head is by now. The revisions under it stay on the server as restore points.
func (s *Service) flushDeletions(ctx context.Context, sess session) error {
	pending, err := s.pendingDeletions(ctx)
	if err != nil || len(pending) == 0 {
		return err
	}
	items, err := s.Client.Items(ctx, sess.token)
	if err != nil {
		return err
	}
	heads := map[string]int{}
	for _, item := range items {
		heads[item.ItemID] = item.HeadRevision
	}
	for _, p := range pending {
		if head, ok := heads[p.itemID]; ok {
			marker, err := seal(sess.keys, revisionManifest{
				Deleted: true,
				Title:   p.title,
				Bundle:  vaultbundle.Manifest{Format: vaultbundle.FormatVersion, ProjectID: p.projectID, CreatedAt: s.Now()},
			}, nil)
			if err != nil {
				return err
			}
			if _, err := s.send(ctx, sess, Link{ItemID: p.itemID, BaseRevision: head, quiet: true}, marker); err != nil {
				if isCode(err, CodeConflict) {
					continue // another device moved it; the next run goes on top of that
				}
				return err
			}
		}
		if _, err := s.db().ExecContext(ctx, `DELETE FROM vault_deletions WHERE item_id = ?`, p.itemID); err != nil {
			return err
		}
	}
	return nil
}

// snapshotBeforePull keeps the library before a pull. An automatic pull only ever
// replaces a work identical to its server revision, so that state is a restore
// point already, and a full copy once a day is enough to stop backups/ growing
// with every revision another device uploads.
func (s *Service) snapshotBeforePull(ctx context.Context, projectID string, auto bool) (string, error) {
	if auto {
		taken, err := backup.TookVaultPullToday(s.Home, s.now())
		if err != nil {
			return "", err
		}
		if taken {
			return "", nil
		}
	}
	return s.snapshot(ctx, projectID)
}

// RemovedWork records that the writer took a work off the server.
func recordRemoved(ctx context.Context, db execer, projectID string, now int64) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO sync_removed (project_id, removed_at) SELECT id, ? FROM projects WHERE id = ?
ON CONFLICT(project_id) DO UPDATE SET removed_at = excluded.removed_at`, now, projectID)
	return err
}

func clearRemoved(ctx context.Context, db execer, projectID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sync_removed WHERE project_id = ?`, projectID)
	return err
}
