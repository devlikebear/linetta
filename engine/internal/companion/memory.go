package companion

import (
	"path/filepath"

	"github.com/devlikebear/tars/pkg/memory"
)

// recallLimit caps how many remembered facts are injected per turn.
const recallLimit = 5

// memRoot returns the per-project memory workspace root.
func memRoot(memBase, projectID string) string {
	return filepath.Join(memBase, projectID)
}

// Remember persists a fact to the project's keyword memory (experiences.jsonl).
func (s *Service) Remember(projectID, text, category string) error {
	return memory.AppendExperience(memRoot(s.memBase, projectID), memory.Experience{
		Summary:  text,
		Category: category,
	})
}

// MemoryRoot is where this project's remembered facts live on disk. Surfaced
// so an export can point at the raw log: the file outlives the companion,
// since storyops keeps writing to it after the pivot.
func (s *Service) MemoryRoot(projectID string) string {
	return memRoot(s.memBase, projectID)
}

// Recall returns up to limit remembered fact summaries matching query
// (case-insensitive substring). Best-effort: returns nil on error/empty.
func (s *Service) Recall(projectID, query string, limit int) []string {
	hits, err := memory.SearchExperiences(memRoot(s.memBase, projectID), memory.SearchOptions{
		Query: query, Limit: limit,
	})
	if err != nil || len(hits) == 0 {
		return nil
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		if h.Summary != "" {
			out = append(out, h.Summary)
		}
	}
	return out
}
