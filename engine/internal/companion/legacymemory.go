package companion

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LegacyMemoryDirName is the root pre-1.0 builds kept per-project memory
// workspaces under. NewService was handed filepath.Join(home, "companion")
// until c8d2454 (the 1.0 MCP pivot, #60) handed it home itself; memRoot stayed
// filepath.Join(memBase, projectID), so the files did not move with it. They
// were never deleted — they simply sit where nothing reads them, and a writer
// who told the companion to remember something before 1.0 gets an empty
// `## Memories` section with no way to know why (#114).
const LegacyMemoryDirName = "companion"

// memoryWorkspaceMarker is the file that makes a directory a memory workspace.
// Checked before a move so the migration relocates memory, not whatever else
// happens to sit under the old root.
var memoryWorkspaceMarker = filepath.Join("memory", "experiences.jsonl")

// reservedHomeNames are names directly under <home> that belong to something
// other than a project's memory workspace. Project ids are uuids, so a real
// collision is not reachable; the list exists so a hand-made or corrupted
// directory under the old root cannot be renamed *onto* one of them.
//
// Note this migration runs before store.Open creates library.db, so "the
// destination already exists" would not catch every one of these on a first
// run. Names, not existence, are the guard.
var reservedHomeNames = map[string]bool{
	LegacyMemoryDirName:   true, // the old root itself
	"backups":             true, // backup.Start
	"codex":               true, // codexauth home
	"skills":              true, // agentskills.Store (#98)
	"folder-sync-staging": true, // foldersync staging area
	"library.db":          true, // the store
	"settings.json":       true, // settings
	"mcp.json":            true, // mcphost discovery file
}

// LegacyMemoryReport is what one migration pass did. Every field is
// informational: the migration has no failure that should stop a caller.
type LegacyMemoryReport struct {
	// Moved holds the directory names relocated to <home>/<name>.
	Moved []string
	// Skipped holds "<name>: <why>" for entries deliberately left alone.
	Skipped []string
	// Failed holds "<name>: <error>" for entries an error left alone.
	Failed []string
	// RootRemoved is true when the emptied old root was removed. A root that
	// still holds something is left in place, and that is not a failure.
	RootRemoved bool
}

// Empty reports whether the pass had nothing to say.
func (r LegacyMemoryReport) Empty() bool {
	return len(r.Moved) == 0 && len(r.Skipped) == 0 && len(r.Failed) == 0 && !r.RootRemoved
}

// Lines renders the report for a log, one line per thing that happened.
func (r LegacyMemoryReport) Lines() []string {
	out := make([]string, 0, len(r.Moved)+len(r.Skipped)+len(r.Failed)+1)
	for _, name := range r.Moved {
		out = append(out, fmt.Sprintf("moved %s/%s to %s", LegacyMemoryDirName, name, name))
	}
	for _, s := range r.Skipped {
		out = append(out, "left alone: "+s)
	}
	for _, f := range r.Failed {
		out = append(out, "could not move: "+f)
	}
	if r.RootRemoved {
		out = append(out, "removed the emptied "+LegacyMemoryDirName+" directory")
	}
	return out
}

// MigrateLegacyMemory moves pre-1.0 memory workspaces from <home>/companion/
// up into <home>/, where memRoot looks for them today (#114).
//
// It is a recovery of what an old build left behind, not a change to where
// memories live: nothing here knows about memRoot beyond the fact that its
// base is now <home>.
//
// Deliberately total: every problem is recorded and the pass continues. A
// missing old root is a no-op with an empty report. Rules, each pinned by a
// test in legacymemory_test.go:
//
//   - A destination that already exists means both directories are left alone.
//     Merging two experiences.jsonl files is the alternative and it is the
//     riskier one — it must not happen silently.
//   - A symlinked entry is skipped. Renaming a link moves the link, not its
//     target, so <home>/<name> would become a link whose contents live outside
//     the home and every later memory write would follow it there.
//   - A directory without memory/experiences.jsonl is not a memory workspace
//     and is not moved.
//   - A name that is not a plain single path segment is refused. The names come
//     off the filesystem rather than from a model, so this is a belt on a pair
//     of braces, but ".." must not be able to name a destination outside home.
//   - A rename that fails — across filesystems, most plausibly — is recorded
//     and skipped, never retried as a copy. A copy would have to decide what to
//     do with a half-copied tree and whether to delete a source it may not have
//     fully read; the source here is intact and the writer loses nothing by the
//     app trying again next launch or moving it by hand.
func MigrateLegacyMemory(home string) LegacyMemoryReport {
	var rep LegacyMemoryReport
	root := filepath.Join(home, LegacyMemoryDirName)

	entries, err := os.ReadDir(root)
	if err != nil {
		// No old root is the ordinary case for everyone who never ran a
		// pre-1.0 build, and for everyone already migrated.
		if !errors.Is(err, fs.ErrNotExist) {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", LegacyMemoryDirName, err))
		}
		return rep
	}

	for _, entry := range entries {
		name := entry.Name()
		switch {
		case !plainSegment(name):
			rep.Skipped = append(rep.Skipped, name+": not a plain directory name")
			continue
		case entry.Type()&fs.ModeSymlink != 0:
			rep.Skipped = append(rep.Skipped, name+": a symlink, not a directory")
			continue
		case !entry.IsDir():
			rep.Skipped = append(rep.Skipped, name+": not a directory")
			continue
		case reservedHomeNames[name]:
			rep.Skipped = append(rep.Skipped, name+": the name is reserved directly under the app data directory")
			continue
		}

		src := filepath.Join(root, name)
		if !isMemoryWorkspace(src) {
			rep.Skipped = append(rep.Skipped, name+": no "+memoryWorkspaceMarker+" inside")
			continue
		}

		dst := filepath.Join(home, name)
		// Lstat, not Stat: a symlink already sitting at the destination is an
		// occupied name too, and following it would be the same mistake as
		// following a symlinked source.
		if _, err := os.Lstat(dst); err == nil {
			rep.Skipped = append(rep.Skipped, name+": something already exists at the destination; both left as they are")
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", name, err))
			continue
		}

		if err := os.Rename(src, dst); err != nil {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		rep.Moved = append(rep.Moved, name)
	}

	// Remove the old root only if it is now empty: os.Remove refuses a
	// directory that still holds something, which is exactly the rule wanted.
	// A refusal is the ordinary outcome whenever anything was skipped, so it
	// is not reported as a failure.
	if err := os.Remove(root); err == nil {
		rep.RootRemoved = true
	}
	return rep
}

// isMemoryWorkspace reports whether dir holds the marker file as a regular
// file. Lstat so a symlinked marker does not vouch for a directory.
func isMemoryWorkspace(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, memoryWorkspaceMarker))
	return err == nil && info.Mode().IsRegular()
}

// plainSegment reports whether name is a single path segment that can only
// name a child of the directory it was read from.
func plainSegment(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	// Base is the clause that carries Windows drive-relative names such as
	// "C:foo", which the separator scan below does not see. On a unix
	// filesystem it is implied by that scan, and kept as the catch-all.
	if filepath.IsAbs(name) || filepath.Base(name) != name {
		return false
	}
	// Both separators, not just this platform's: a name carrying the other
	// one is not something Linetta wrote either way.
	return !strings.ContainsAny(name, `/\`)
}
