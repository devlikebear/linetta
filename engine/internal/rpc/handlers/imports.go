package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/importmd"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type importMarkdownParams struct {
	FileName string `json:"file_name"`
	Content  string `json:"content"`
}

type importMarkdownResult struct {
	ProjectID         string   `json:"project_id"`
	ContainerCount    int      `json:"container_count"`
	LeafCount         int      `json:"leaf_count"`
	EntityCount       int      `json:"entity_count"`
	RelationshipCount int      `json:"relationship_count"`
	Warnings          []string `json:"warnings"`
}

// ImportMarkdown returns a handler for imports.markdown.
// The renderer passes the file content as a string; the engine never reads
// the disk. The file_name is used only to derive a fallback project title
// when the markdown has no H1.
func ImportMarkdown(pr *project.Repo, nr *node.Repo, er *entity.Repo, rr *relationship.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p importMarkdownParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		fallback := fallbackTitleFromFileName(p.FileName)
		doc := importmd.ParseDocument(p.Content)
		ts := now()
		built, err := importmd.BuildProject(ctx, pr, nr, ts, doc.Outline, fallback)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		built.Warnings = append(doc.Warnings, built.Warnings...)
		if err := importmd.RestoreMetadata(ctx, er, rr, ts, built.Project.ID, doc.Metadata, &built); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		out := importMarkdownResult{
			ProjectID:         built.Project.ID,
			ContainerCount:    built.ContainerCount,
			LeafCount:         built.LeafCount,
			EntityCount:       built.EntityCount,
			RelationshipCount: built.RelationshipCount,
			Warnings:          built.Warnings,
		}
		if out.Warnings == nil {
			out.Warnings = []string{}
		}
		return json.Marshal(out)
	}
}

// ImportPreview returns a handler for imports.preview. It parses the markdown
// and returns the would-be tree (label, kind, children) plus counts and
// warnings — without creating any project or node rows.
func ImportPreview() rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p importMarkdownParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		doc := importmd.ParseDocument(p.Content)
		pv := importmd.Preview(doc.Outline, p.FileName)
		pv.Warnings = append(doc.Warnings, pv.Warnings...)
		if pv.Warnings == nil {
			pv.Warnings = []string{}
		}
		if pv.Roots == nil {
			pv.Roots = []*importmd.PreviewNode{}
		}
		return json.Marshal(pv)
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
