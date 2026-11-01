package vault

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/devlikebear/linetta/engine/internal/vaultbundle"
)

// A revision's manifest says what kind of item it belongs to. "" is a work.
const kindLibrary = "library"

// The places a library can be kept. These are stored in vault_items and in
// settings, so they are permanent.
const (
	TargetWorks  = "works"
	TargetGit    = "git"
	TargetFolder = "folder"
)

// maxLibraryRounds bounds syncing against another device doing the same thing.
const maxLibraryRounds = 3

// ErrBadCategory is a selection that names a category that cannot be turned off.
var ErrBadCategory = errors.New("vault: not a category that can be turned off")

type libraryState struct {
	BaseRevision int
	LastHash     string
	BaseParts    map[string]string
	LastSyncedAt int64
	Mark         string
}

func getLibraryState(ctx context.Context, q queryer, target string) (libraryState, error) {
	state := libraryState{BaseParts: map[string]string{}}
	var parts string
	err := q.QueryRowContext(ctx, `
SELECT base_revision, last_hash, base_parts, last_synced_at, head_mark
FROM vault_library WHERE target = ?`, target).
		Scan(&state.BaseRevision, &state.LastHash, &parts, &state.LastSyncedAt, &state.Mark)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	return state, json.Unmarshal([]byte(parts), &state.BaseParts)
}

func putLibraryState(ctx context.Context, db execer, target string, state libraryState) error {
	parts, err := json.Marshal(state.BaseParts)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO vault_library (target, base_revision, last_hash, base_parts, last_synced_at, head_mark)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(target) DO UPDATE SET
  base_revision = excluded.base_revision, last_hash = excluded.last_hash,
  base_parts = excluded.base_parts, last_synced_at = excluded.last_synced_at,
  head_mark = excluded.head_mark`,
		target, state.BaseRevision, state.LastHash, string(parts), state.LastSyncedAt, state.Mark)
	return err
}

func getSelection(ctx context.Context, q queryer) (vaultbundle.LibrarySettings, error) {
	var selection vaultbundle.LibrarySettings
	var omit string
	err := q.QueryRowContext(ctx, `SELECT omit, updated_at FROM sync_selection WHERE id = 1`).Scan(&omit, &selection.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return selection, nil
	}
	if err != nil {
		return selection, err
	}
	return selection, json.Unmarshal([]byte(omit), &selection.Omit)
}

func putSelection(ctx context.Context, db execer, selection vaultbundle.LibrarySettings) error {
	omit := selection.Omit
	if omit == nil {
		omit = []string{}
	}
	raw, err := json.Marshal(omit)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO sync_selection (id, omit, updated_at) VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET omit = excluded.omit, updated_at = excluded.updated_at`,
		string(raw), selection.UpdatedAt)
	return err
}

// libraryLink is the library item in the shape send and fetch take. Quiet: this
// is housekeeping around a work, and the progress bar is the work's.
func libraryLink(sess session, base int) Link {
	return Link{ItemID: sess.keys.LibraryItemID(), BaseRevision: base, quiet: true}
}

// sealLibrary packs a library bundle like a work's, under its own kind.
func sealLibrary(keys *Keys, bundle *vaultbundle.LibraryBundle) (*outgoing, error) {
	return seal(keys, revisionManifest{Kind: kindLibrary, Bundle: bundle.Manifest, Library: &bundle.Meta}, bundle.Parts)
}

func (s *Service) fetchLibrary(ctx context.Context, sess session, revision Revision, link Link) (*vaultbundle.LibraryBundle, error) {
	cache, err := s.cacheDir(link.ItemID)
	if err != nil {
		return nil, err
	}
	sealed, err := s.sealedBlob(ctx, sess, cache, revision.ManifestBlob)
	if err != nil {
		return nil, err
	}
	m, err := openManifest(sess.keys, revision.ManifestBlob, sealed)
	if err != nil {
		_ = os.Remove(filepath.Join(cache, revision.ManifestBlob))
		return nil, err
	}
	if m.Kind != kindLibrary || m.Library == nil {
		return nil, fmt.Errorf("%w: the library item holds something else", ErrCorrupt)
	}
	plain, err := s.fetch(ctx, sess, revision, link, cache)
	if err != nil {
		return nil, err
	}
	bundle := &vaultbundle.LibraryBundle{Bundle: *plain, Meta: *m.Library}
	if err := vaultbundle.ValidateLibrary(bundle); err != nil {
		return nil, err
	}
	_ = os.RemoveAll(cache)
	return bundle, nil
}

// takeLibrary merges the item's head into this library, then adopts its
// selection if it is the newer of the two.
func (s *Service) takeLibrary(ctx context.Context, sess session, item RemoteItem, state *libraryState) error {
	revisions, err := sess.store.Revisions(ctx, item.ItemID)
	if err != nil {
		return err
	}
	if len(revisions) == 0 {
		return fmt.Errorf("%w: the library item has no revisions", ErrCorrupt)
	}
	remote, err := s.fetchLibrary(ctx, sess, revisions[0], libraryLink(sess, item.HeadRevision))
	if err != nil {
		return err
	}
	selection, err := getSelection(ctx, s.db())
	if err != nil {
		return err
	}
	applied, err := vaultbundle.ApplyLibrary(ctx, s.db(), s.Home, remote, state.BaseParts, selection, s.now())
	if err != nil {
		return err
	}
	s.setLibraryWarning(applied.ProfileRefused)
	if remote.Meta.Settings.UpdatedAt > selection.UpdatedAt {
		if err := s.adoptSelection(ctx, selection, remote.Meta.Settings); err != nil {
			return err
		}
	}
	state.BaseRevision = revisions[0].Revision
	state.LastHash = remote.ContentHash()
	state.BaseParts = remote.PartHashes()
	state.LastSyncedAt = s.Now()
	state.Mark = revisions[0].ManifestBlob
	return putLibraryState(ctx, s.db(), s.targetID(), *state)
}

// adoptSelection takes another device's selection. Works it left unchanged here
// are re-hashed under the new choice: that device is uploading them, and the
// bare change of category must not read as an edit made here and turn into a
// conflict with its upload.
func (s *Service) adoptSelection(ctx context.Context, old, next vaultbundle.LibrarySettings) error {
	if !vaultbundle.SameWorkCategories(old.Omit, next.Omit) {
		links, err := listLinks(ctx, s.db(), s.targetID())
		if err != nil {
			return err
		}
		for _, link := range links {
			before, err := vaultbundle.Export(ctx, s.db(), s.Home, link.ProjectID, s.now(), old.Omit)
			if err != nil {
				return err
			}
			if before.Manifest.ContentHash() != link.LastHash {
				continue
			}
			after, err := vaultbundle.Export(ctx, s.db(), s.Home, link.ProjectID, s.now(), next.Omit)
			if err != nil {
				return err
			}
			link.LastHash = after.Manifest.ContentHash()
			if err := putLink(ctx, s.db(), s.targetID(), link); err != nil {
				return err
			}
		}
	}
	return putSelection(ctx, s.db(), next)
}

func (s *Service) libraryItem(ctx context.Context, sess session) (RemoteItem, bool, error) {
	items, err := sess.store.Items(ctx)
	if err != nil {
		return RemoteItem{}, false, err
	}
	id := sess.keys.LibraryItemID()
	for _, item := range items {
		if item.ItemID == id {
			return item, true, nil
		}
	}
	return RemoteItem{}, false, nil
}

func libraryMoved(item RemoteItem, state libraryState) bool {
	return item.HeadRevision != state.BaseRevision || (item.Mark != "" && state.Mark != "" && item.Mark != state.Mark)
}

// pullLibrary brings this device up to the library item, if it has moved.
func (s *Service) pullLibrary(ctx context.Context, sess session) error {
	item, found, err := s.libraryItem(ctx, sess)
	if err != nil || !found {
		return err
	}
	state, err := getLibraryState(ctx, s.db(), s.targetID())
	if err != nil {
		return err
	}
	if !libraryMoved(item, state) {
		return nil
	}
	return s.takeLibrary(ctx, sess, item, &state)
}

// syncLibrary takes the item's changes and sends this device's. Another device
// committing in between is not a conflict to ask about: the merge just runs
// again on top of it. A library with nothing in it does not make an item.
func (s *Service) syncLibrary(ctx context.Context, sess session) error {
	for range maxLibraryRounds {
		item, found, err := s.libraryItem(ctx, sess)
		if err != nil {
			return err
		}
		state, err := getLibraryState(ctx, s.db(), s.targetID())
		if err != nil {
			return err
		}
		if found && libraryMoved(item, state) {
			if err := s.takeLibrary(ctx, sess, item, &state); err != nil {
				return err
			}
		}
		if !found {
			state.BaseRevision, state.LastHash = 0, ""
		}
		selection, err := getSelection(ctx, s.db())
		if err != nil {
			return err
		}
		local, err := vaultbundle.ExportLibrary(ctx, s.db(), s.Home, selection, s.now())
		if err != nil {
			return err
		}
		hash := local.ContentHash()
		if hash == state.LastHash || (!found && local.Empty()) {
			return nil
		}
		out, err := sealLibrary(sess.keys, local)
		if err != nil {
			return err
		}
		revision, err := s.send(ctx, sess, libraryLink(sess, state.BaseRevision), out)
		if isCode(err, CodeConflict) {
			continue
		}
		if err != nil {
			return err
		}
		state.BaseRevision, state.LastHash, state.LastSyncedAt, state.Mark = revision, hash, s.Now(), out.manifestID
		state.BaseParts = local.PartHashes()
		return putLibraryState(ctx, s.db(), s.targetID(), state)
	}
	return errors.New("vault: the library item kept moving")
}

// bestEffortLibrary runs library housekeeping around a work. The work is what
// the writer asked for, and it reports its own problems; a library that could
// not be reached waits for the next time.
func (s *Service) bestEffortLibrary(ctx context.Context, run func() error) {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("vault: library: %v", err)
	}
}

func (s *Service) setLibraryWarning(refused bool) {
	warning := ""
	if refused {
		warning = LibraryWarningProfileRefused
	}
	s.warningMu.Lock()
	s.libraryWarning = warning
	s.warningMu.Unlock()
}

func (s *Service) warning() string {
	s.warningMu.Lock()
	defer s.warningMu.Unlock()
	return s.libraryWarning
}

// LibraryWarningProfileRefused says the writer profile from another device could
// not be stored here, and this device's stands.
const LibraryWarningProfileRefused = "profile_refused"

// SyncLibrary takes and sends the library item.
func (s *Service) SyncLibrary(ctx context.Context) error {
	end, err := s.begin()
	if err != nil {
		return err
	}
	defer end()
	sess, err := s.open()
	if err != nil {
		return err
	}
	return classify(s.syncLibrary(ctx, sess))
}

// ItemCategory is one line of the settings pane. Required is the line that
// cannot be turned off, whose ID is empty.
type ItemCategory struct {
	ID       string `json:"id"`
	Bytes    int64  `json:"bytes"`
	Enabled  bool   `json:"enabled"`
	Required bool   `json:"required"`
}

// SyncPreview is what turning automatic sync on would send, as far as this device
// can tell: the screen asks the writer about it before anything leaves.
type SyncPreview struct {
	Works int   `json:"works"`
	Bytes int64 `json:"bytes"`
}

// Preview adds up the selected categories. It sends nothing.
func (s *Service) Preview(ctx context.Context) (SyncPreview, error) {
	view, err := s.Items(ctx)
	if err != nil {
		return SyncPreview{}, err
	}
	var out SyncPreview
	for _, category := range view.Categories {
		if category.Enabled {
			out.Bytes += category.Bytes
		}
	}
	err = s.db().QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&out.Works)
	return out, err
}

// ItemsView is what can be carried and what is.
type ItemsView struct {
	Categories []ItemCategory `json:"categories"`
}

// Items reads the selection and what each category holds on this device.
func (s *Service) Items(ctx context.Context) (ItemsView, error) {
	selection, err := getSelection(ctx, s.db())
	if err != nil {
		return ItemsView{}, err
	}
	sizes, err := vaultbundle.Sizes(ctx, s.db(), s.Home, s.now())
	if err != nil {
		return ItemsView{}, err
	}
	out := ItemsView{Categories: []ItemCategory{{Bytes: sizes[""], Enabled: true, Required: true}}}
	for _, id := range vaultbundle.Categories() {
		out.Categories = append(out.Categories, ItemCategory{ID: id, Bytes: sizes[id], Enabled: !selection.Omits(id)})
	}
	return out, nil
}

// SetItems records a new selection. It goes out with the library item right away
// when this device can, and with the next upload when it cannot: every device
// has to follow one choice, but a device with no key yet has nothing to send.
func (s *Service) SetItems(ctx context.Context, omit []string) (ItemsView, error) {
	for _, id := range omit {
		if !vaultbundle.ValidCategory(id) {
			return ItemsView{}, fmt.Errorf("%w: %q", ErrBadCategory, id)
		}
	}
	end, err := s.begin()
	if err != nil {
		return ItemsView{}, err
	}
	defer end()
	sorted := slices.Clone(omit)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)
	if err := putSelection(ctx, s.db(), vaultbundle.LibrarySettings{Omit: sorted, UpdatedAt: s.Now()}); err != nil {
		return ItemsView{}, err
	}
	if sess, err := s.open(); err == nil {
		s.bestEffortLibrary(ctx, func() error { return s.syncLibrary(ctx, sess) })
	}
	return s.Items(ctx)
}
