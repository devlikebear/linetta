package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/importmd"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type importMarkdownParams struct {
	FileName string `json:"file_name"`
	Content  string `json:"content"`
}

type importMarkdownResult struct {
	ProjectID string `json:"project_id"`
}

// ImportMarkdown returns a handler for imports.markdown.
// The renderer passes the file content as a string; the engine never reads
// the disk. The file_name is used only to derive a fallback project title
// when the markdown has no H1.
func ImportMarkdown(pr *project.Repo, nr *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p importMarkdownParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		fallback := fallbackTitleFromFileName(p.FileName)
		outline := importmd.ParseOutline(p.Content)
		built, err := importmd.BuildProject(ctx, pr, nr, now(), outline, fallback)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(importMarkdownResult{ProjectID: built.Project.ID})
	}
}

// fallbackTitleFromFileName strips the .md or .markdown suffix.
// Empty result is fine — BuildProject defaults to "가져온 작품".
func fallbackTitleFromFileName(name string) string {
	if name == "" {
		return ""
	}
	for _, suf := range []string{".markdown", ".md"} {
		if strings.HasSuffix(strings.ToLower(name), suf) {
			return name[:len(name)-len(suf)]
		}
	}
	return name
}
