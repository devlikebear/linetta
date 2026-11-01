package companion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/pkg/memory"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
)

// importedMarker sits beside experiences.jsonl once its lines are in the
// archive. Without it, a line the writer deleted from the archive would come
// back on the next launch.
const importedMarker = "experiences.imported"

// importExperienceLimit is far past what any log holds. SearchExperiences needs
// a number, and a cap here would quietly leave lines behind.
const importExperienceLimit = 1 << 20

// memRoot returns the per-project memory workspace root.
func memRoot(memBase, projectID string) string {
	return filepath.Join(memBase, projectID)
}

// Remember keeps a fact where the agent can find it later: the work's archive.
// category came with the old keyword log and nothing reads it any more.
func (s *Service) Remember(projectID, text, _ string) error {
	if s.memories == nil {
		return errors.New("memory is not available")
	}
	return s.memories.Remember(context.Background(), projectID, text, time.Now().UnixMilli())
}

// MemoryRoot is where this project's old keyword log lives on disk, if it has
// one. Surfaced so an export can point at the raw file.
func (s *Service) MemoryRoot(projectID string) string {
	return memRoot(s.memBase, projectID)
}

// Recall returns up to limit archived lines matching query, or the newest ones
// for an empty query. Best-effort: returns nil on error/empty.
func (s *Service) Recall(projectID, query string, limit int) []string {
	if s.memories == nil {
		return nil
	}
	var (
		hits []agentmemory.Archived
		err  error
	)
	if strings.TrimSpace(query) == "" {
		hits, err = s.memories.ListArchived(context.Background(), projectID, limit)
	} else {
		hits, err = s.memories.SearchArchived(context.Background(), projectID, query, limit)
	}
	if err != nil || len(hits) == 0 {
		return nil
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Body)
	}
	return out
}

// ImportExperiences moves every work's experiences.jsonl into its archive, once.
// The file itself stays: an export names it, and nothing writes to it any more.
//
// Like MigrateLegacyMemory it is total and reports rather than stops. A log
// whose work is gone is skipped and stays unmarked; a line the screen refuses is
// reported and the rest of the log still goes in.
func (s *Service) ImportExperiences(ctx context.Context) []string {
	if s.memories == nil {
		return nil
	}
	entries, err := os.ReadDir(s.memBase)
	if err != nil {
		return []string{fmt.Sprintf("read %s: %v", s.memBase, err)}
	}
	var report []string
	for _, entry := range entries {
		projectID := entry.Name()
		root := memRoot(s.memBase, projectID)
		if !entry.IsDir() || !isMemoryWorkspace(root) {
			continue
		}
		marker := filepath.Join(filepath.Dir(filepath.Join(root, memoryWorkspaceMarker)), importedMarker)
		if _, err := os.Stat(marker); err == nil {
			continue
		}
		report = append(report, s.importExperiences(ctx, projectID, root, marker)...)
	}
	return report
}

func (s *Service) importExperiences(ctx context.Context, projectID, root, marker string) []string {
	hits, err := memory.SearchExperiences(root, memory.SearchOptions{Limit: importExperienceLimit})
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", projectID, err)}
	}
	var report []string
	for _, h := range hits {
		if strings.TrimSpace(h.Summary) == "" {
			continue
		}
		err := s.memories.ImportArchived(ctx, projectID, h.Summary, time.Now().UnixMilli())
		if errors.Is(err, agentmemory.ErrUnknownProject) {
			return append(report, fmt.Sprintf("%s: no such work, left alone", projectID))
		}
		if err != nil {
			report = append(report, fmt.Sprintf("%s: left out a line: %v", projectID, err))
		}
	}
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		report = append(report, fmt.Sprintf("%s: %v", projectID, err))
	}
	return report
}
