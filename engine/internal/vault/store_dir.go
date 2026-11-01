package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/atomicfile"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// A folder keeps revisions like the site does, as files. It is laid out so that
// whatever carries the folder between devices, Dropbox or Git, never sees two
// devices change one file:
//
//	.linetta/format.json
//	.linetta/blobs/ab/<blob id>                 sealed, named by content
//	.linetta/items/<item id>/
//	  00000012-<device>-<manifest>.json         one revision
//
// With no server to compare and swap, the file name does that job. Two devices
// can write revision 12 and both files survive. The head is the largest
// (revision, file name), which every device works out the same way, and the one
// that lost finds the head is not what it sent. The blobs go first and the
// revision file last, so a run cut short leaves unused blobs and nothing else.
const (
	dirRoot        = ".linetta"
	dirFormatFile  = "format.json"
	dirBlobs       = "blobs"
	dirItems       = "items"
	dirFormat      = 1
	dirNameMark    = 8
	dirDailyPoints = 7
	dirDay         = 24 * time.Hour
)

// dirRestorePoints matches the site's rule for what stays.
const dirRestorePoints = 5

// dirBlobGrace is how long a blob no revision uses is left alone. A device may be
// about to commit it, and a synced folder may hold the revision back for a while.
const dirBlobGrace = dirDay

var dirRevisionName = regexp.MustCompile(`^(\d{8})-([0-9a-z]+)-([0-9a-f]{8})\.json$`)

// dirStore is a target in a folder. Git is the same store with a round around it.
type dirStore struct {
	folder string
	device string
	now    func() time.Time
	// before runs ahead of a run, for Git to take what the remote has. It may be
	// nil.
	before func(ctx context.Context) error
	// after runs at the end of a run, for the Markdown export that shares the
	// folder and for Git to push. It may be nil.
	after func(ctx context.Context) error
}

// DirConfig describes a folder target.
type DirConfig struct {
	// Folder holds the Markdown export, with the sealed copy under .linetta.
	Folder string
	// Device tells this device's revision files from others'.
	Device string
	Now    func() time.Time
	Before func(ctx context.Context) error
	After  func(ctx context.Context) error
}

// NewDirStore opens a folder target.
func NewDirStore(cfg DirConfig) Store {
	return dirStore{folder: cfg.Folder, device: cfg.Device, now: cfg.Now, before: cfg.Before, after: cfg.After}
}

// dirRevision is one revision file.
type dirRevision struct {
	Revision     int      `json:"revision"`
	ManifestBlob string   `json:"manifest_blob"`
	BlobIDs      []string `json:"blob_ids"`
	Created      string   `json:"created"`
	Bytes        int64    `json:"bytes"`
}

func (d dirStore) root() string { return filepath.Join(d.folder, dirRoot) }

// prepare makes the layout and refuses a folder written by a newer app.
func (d dirStore) prepare() error {
	if strings.TrimSpace(d.folder) == "" {
		return reason(rpc.ReasonSyncTargetMissing, errors.New("vault: no folder is set"))
	}
	if info, err := os.Stat(d.folder); err != nil || !info.IsDir() {
		return reason(rpc.ReasonSyncTargetMissing, fmt.Errorf("vault: folder %s is not there", d.folder))
	}
	file := filepath.Join(d.root(), dirFormatFile)
	raw, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(d.root(), 0o700); err != nil {
			return err
		}
		return atomicfile.Write(file, []byte(`{"format":1}`), 0o600)
	}
	if err != nil {
		return err
	}
	var format struct {
		Format int `json:"format"`
	}
	if err := json.Unmarshal(raw, &format); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrCorrupt, dirFormatFile, err)
	}
	if format.Format > dirFormat {
		return ErrNewerFormat
	}
	return nil
}

func (d dirStore) blobPath(id string) (string, error) {
	if !blobIDPattern.MatchString(id) {
		return "", fmt.Errorf("%w: bad blob id", ErrCorrupt)
	}
	return filepath.Join(d.root(), dirBlobs, id[:2], id), nil
}

func (d dirStore) itemDir(itemID string) (string, error) {
	if !itemIDPattern.MatchString(itemID) {
		return "", fmt.Errorf("%w: bad item id", ErrCorrupt)
	}
	return filepath.Join(d.root(), dirItems, itemID), nil
}

// dirEntry is a revision file before it is read. Files another app left, such as
// a "conflicted copy", do not match the name and are not revisions.
type dirEntry struct {
	name     string
	revision int
}

func listRevisionFiles(dir string) ([]dirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []dirEntry
	for _, entry := range entries {
		match := dirRevisionName.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		revision, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		out = append(out, dirEntry{name: entry.Name(), revision: revision})
	}
	// Newest first: by number, then by name, the same answer on every device.
	slices.SortFunc(out, func(a, b dirEntry) int {
		if a.revision != b.revision {
			return b.revision - a.revision
		}
		return strings.Compare(b.name, a.name)
	})
	return out, nil
}

func readRevisionFile(file string) (dirRevision, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return dirRevision{}, err
	}
	var revision dirRevision
	if err := json.Unmarshal(raw, &revision); err != nil {
		return dirRevision{}, fmt.Errorf("%w: revision %s: %v", ErrCorrupt, filepath.Base(file), err)
	}
	return revision, nil
}

func (d dirStore) readRevisions(itemID string) ([]dirRevision, error) {
	dir, err := d.itemDir(itemID)
	if err != nil {
		return nil, err
	}
	entries, err := listRevisionFiles(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]dirRevision, 0, len(entries))
	for _, entry := range entries {
		revision, err := readRevisionFile(filepath.Join(dir, entry.name))
		if err != nil {
			return nil, err
		}
		out = append(out, revision)
	}
	return out, nil
}

// Account reports a folder as always open. Its only limit is the disk, and the
// used bytes are what the sync screen shows for it.
func (d dirStore) Account(context.Context) (Account, error) {
	if err := d.prepare(); err != nil {
		return Account{}, err
	}
	var size int64
	err := filepath.WalkDir(d.root(), func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	return Account{Mode: ModeFull, Status: "approved", TermsAccepted: true, Usage: Usage{Bytes: size}}, err
}

// Items reads every item's head. Its Mark is what tells two revisions of one
// number apart.
func (d dirStore) Items(context.Context) ([]RemoteItem, error) {
	if err := d.prepare(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(d.root(), dirItems))
	if errors.Is(err, fs.ErrNotExist) {
		return []RemoteItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := []RemoteItem{}
	for _, entry := range entries {
		if !entry.IsDir() || !itemIDPattern.MatchString(entry.Name()) {
			continue
		}
		revisions, err := d.readRevisions(entry.Name())
		if err != nil {
			return nil, err
		}
		if len(revisions) == 0 {
			continue
		}
		head := revisions[0]
		items = append(items, RemoteItem{ItemID: entry.Name(), HeadRevision: head.Revision, Updated: head.Created, Bytes: head.Bytes, Mark: head.ManifestBlob})
	}
	return items, nil
}

// Revisions lists restore points, newest first.
func (d dirStore) Revisions(_ context.Context, itemID string) ([]Revision, error) {
	if err := d.prepare(); err != nil {
		return nil, err
	}
	stored, err := d.readRevisions(itemID)
	if err != nil {
		return nil, err
	}
	if len(stored) == 0 {
		return nil, &APIError{Status: http.StatusNotFound, Code: CodeNotFound}
	}
	out := make([]Revision, 0, len(stored))
	for _, r := range stored {
		out = append(out, Revision{Revision: r.Revision, ManifestBlob: r.ManifestBlob, BlobIDs: r.BlobIDs, Bytes: r.Bytes, Created: r.Created})
	}
	return out, nil
}

// PutBlob writes a blob, once. Its name is its content.
func (d dirStore) PutBlob(_ context.Context, id string, sealed []byte) error {
	file, err := d.blobPath(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(file); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	return atomicfile.Write(file, sealed, 0o600)
}

// GetBlob reads a blob. One that is missing has most likely not arrived yet, so
// it is CodeBlobPending rather than damage: the run waits and looks again.
func (d dirStore) GetBlob(_ context.Context, id string) ([]byte, error) {
	file, err := d.blobPath(id)
	if err != nil {
		return nil, err
	}
	sealed, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &APIError{Status: http.StatusNotFound, Code: CodeBlobPending}
	}
	return sealed, err
}

// Commit writes the revision file, after the same checks the site makes, in the
// same order. The check is against what this device can see: another device may
// be committing the same number now, which is what the file name is for.
func (d dirStore) Commit(_ context.Context, itemID string, base int, manifest string, blobs []string) (int, error) {
	dir, err := d.itemDir(itemID)
	if err != nil {
		return 0, err
	}
	ids := append(slices.Clone(blobs), manifest)
	var missing []string
	var size int64
	for _, id := range ids {
		file, err := d.blobPath(id)
		if err != nil {
			return 0, err
		}
		info, err := os.Stat(file)
		if errors.Is(err, fs.ErrNotExist) {
			missing = append(missing, id)
			continue
		}
		if err != nil {
			return 0, err
		}
		size += info.Size()
	}
	if len(missing) > 0 {
		return 0, &APIError{Status: http.StatusConflict, Code: CodeBlobsMissing, Missing: missing}
	}
	entries, err := listRevisionFiles(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	head := 0
	if len(entries) > 0 {
		head = entries[0].revision
	}
	if head != base {
		return 0, &APIError{Status: http.StatusConflict, Code: CodeConflict, HeadRevision: head}
	}
	next := head + 1
	revision := dirRevision{Revision: next, ManifestBlob: manifest, BlobIDs: blobs, Created: d.now().UTC().Format(time.RFC3339), Bytes: size}
	raw, err := json.Marshal(revision)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	name := fmt.Sprintf("%08d-%s-%s.json", next, d.device, manifest[:dirNameMark])
	if err := atomicfile.Write(filepath.Join(dir, name), raw, 0o600); err != nil {
		return 0, err
	}
	return next, d.pruneRevisions(dir)
}

// pruneRevisions keeps the site's rule: the five newest, and the last of each of
// the past seven days.
func (d dirStore) pruneRevisions(dir string) error {
	entries, err := listRevisionFiles(dir)
	if err != nil {
		return err
	}
	if len(entries) <= dirRestorePoints {
		return nil
	}
	now := d.now()
	days := map[string]bool{}
	for i, entry := range entries {
		file := filepath.Join(dir, entry.name)
		revision, err := readRevisionFile(file)
		if err != nil {
			return err
		}
		created, err := time.Parse(time.RFC3339, revision.Created)
		if err != nil {
			continue // not ours to judge; a later run or the writer can see it
		}
		day := created.UTC().Format("2006-01-02")
		keep := i < dirRestorePoints || (now.Sub(created) <= dirDailyPoints*dirDay && !days[day])
		days[day] = true
		if !keep {
			if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

// DeleteItem removes the item's revisions. Its blobs go in a later sweep, since
// another item may share them.
func (d dirStore) DeleteItem(_ context.Context, itemID string) error {
	dir, err := d.itemDir(itemID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return &APIError{Status: http.StatusNotFound, Code: CodeNotFound}
	}
	return os.RemoveAll(dir)
}

// Begin brings the folder up to date. A plain folder is carried by something
// that runs on its own, so it only has to be there.
func (d dirStore) Begin(ctx context.Context) error {
	if d.before != nil {
		if err := d.before(ctx); err != nil {
			return err
		}
	}
	return d.prepare()
}

// Finish clears what no revision uses and hands over to the Markdown export.
func (d dirStore) Finish(ctx context.Context) error {
	if err := d.sweepBlobs(); err != nil {
		return err
	}
	if d.after != nil {
		return d.after(ctx)
	}
	return nil
}

// sweepBlobs deletes blobs no revision points at, once they are old enough that
// no commit can still be on its way to them.
func (d dirStore) sweepBlobs() error {
	used := map[string]bool{}
	items, err := os.ReadDir(filepath.Join(d.root(), dirItems))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		revisions, err := d.readRevisions(item.Name())
		if err != nil {
			return err
		}
		for _, revision := range revisions {
			used[revision.ManifestBlob] = true
			for _, id := range revision.BlobIDs {
				used[id] = true
			}
		}
	}
	cutoff := d.now().Add(-dirBlobGrace)
	err = filepath.WalkDir(filepath.Join(d.root(), dirBlobs), func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() || !blobIDPattern.MatchString(entry.Name()) || used[entry.Name()] {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(cutoff) {
			return os.Remove(file)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
