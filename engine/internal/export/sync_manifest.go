package export

import (
	"encoding/json"
	"time"

	"github.com/devlikebear/linetta/engine/internal/project"
)

const SyncManifestFilename = ".linetta-sync.json"

type syncManifest struct {
	Format        string                `json:"format"`
	FormatVersion int                   `json:"format_version"`
	GeneratedAt   int64                 `json:"generated_at"`
	Projects      []syncManifestProject `json:"projects"`
}

type syncManifestProject struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Filename string `json:"filename"`
}

// BuildSyncManifest records the exact project-to-file mapping for one complete
// sync pass. Consumers can detect missing files without guessing from titles.
func BuildSyncManifest(projects []project.Project, generatedAt time.Time) ([]byte, error) {
	manifest := syncManifest{
		Format:        "linetta-markdown-sync",
		FormatVersion: 1,
		GeneratedAt:   generatedAt.UnixMilli(),
		Projects:      make([]syncManifestProject, 0, len(projects)),
	}
	for _, item := range projects {
		manifest.Projects = append(manifest.Projects, syncManifestProject{
			ID:       item.ID,
			Title:    item.Title,
			Filename: SyncFilename(item.Title, item.ID),
		})
	}
	return json.MarshalIndent(manifest, "", "  ")
}
