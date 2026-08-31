package handlers

import (
	"context"
	"encoding/json"
	"fmt"
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
func ImportMarkdown(pr *project.Repo, nr *node.Repo, er *entity.Repo, rr *relationship.Repo, extras importmd.Extras, now Clock) rpc.Handler {
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
			return nil, importFailure(ctx, pr, built.Project.ID, err)
		}
		built.Warnings = append(doc.Warnings, built.Warnings...)
		if err := restoreProjectOutlinePreset(ctx, pr, ts, doc.Metadata.OutlinePreset, &built); err != nil {
			return nil, importFailure(ctx, pr, built.Project.ID, err)
		}
		if err := importmd.RestoreProjectDetails(ctx, pr, ts, doc.Metadata, &built); err != nil {
			return nil, importFailure(ctx, pr, built.Project.ID, err)
		}
		if err := importmd.RestoreMetadata(ctx, er, rr, ts, built.Project.ID, doc.Metadata, &built); err != nil {
			return nil, importFailure(ctx, pr, built.Project.ID, err)
		}
		nodeIDMap, err := importmd.AlignNodes(ctx, nr, ts, built.Project.ID, doc.Metadata.Nodes, &built)
		if err != nil {
			return nil, importFailure(ctx, pr, built.Project.ID, err)
		}
		if err := importmd.RestoreExtras(ctx, extras, ts, built.Project.ID, doc.Metadata, nodeIDMap, &built); err != nil {
			return nil, importFailure(ctx, pr, built.Project.ID, err)
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

func importFailure(ctx context.Context, pr *project.Repo, projectID string, cause error) *rpc.MethodError {
	message := cause.Error()
	if projectID != "" {
		if err := pr.Delete(context.WithoutCancel(ctx), projectID); err != nil {
			message = fmt.Sprintf("%s; rollback partial import: %v", message, err)
		}
	}
	return &rpc.MethodError{Code: rpc.CodeInternalError, Message: message}
}

func restoreProjectOutlinePreset(ctx context.Context, pr *project.Repo, now int64, preset string, built *importmd.BuildResult) error {
	preset = strings.TrimSpace(preset)
	if preset == "" {
		return nil
	}
	if !project.ValidOutlinePreset(preset) {
		built.Warnings = append(built.Warnings, importmd.WarnUnknownOutlinePreset+":"+preset)
		return nil
	}
	updated, err := pr.Update(ctx, now, project.UpdateInput{ID: built.Project.ID, OutlinePreset: &preset})
	if err != nil {
		return err
	}
	built.Project = updated
	return nil
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
