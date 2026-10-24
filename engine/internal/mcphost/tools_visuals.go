//go:build !mobile

package mcphost

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/visual"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- linetta_get_character_visuals / linetta_build_illustration_prompt ----------
//
// The writer's character designs (#154), so an agent asked for a chapter's
// illustration or 4컷만화 draws the same 한도윤 every time. Both tools only
// read: they compose a prompt for whatever image model the writer uses, and
// generate nothing themselves.

type characterVisualsInput struct {
	ProjectID     string   `json:"project_id" jsonschema:"id of the work"`
	EntityIDs     []string `json:"entity_ids,omitempty" jsonschema:"omit (with names) for every character"`
	Names         []string `json:"names,omitempty" jsonschema:"names or aliases instead of entity_ids"`
	IncludeImages bool     `json:"include_images,omitempty" jsonschema:"also return reference images as image content"`
}

func (in characterVisualsInput) scope() (string, string) { return in.ProjectID, "" }

type illustrationPromptInput struct {
	ProjectID string   `json:"project_id" jsonschema:"id of the work"`
	EntityIDs []string `json:"entity_ids,omitempty" jsonschema:"characters in the picture"`
	Names     []string `json:"names,omitempty" jsonschema:"or names/aliases of the characters in the picture"`
	Scene     string   `json:"scene" jsonschema:"what happens; for four_panel, beats like 1컷: ... 2컷: ..."`
	NodeID    string   `json:"node_id,omitempty" jsonschema:"optional scene being illustrated"`
	Format    string   `json:"format,omitempty" jsonschema:"single (default) or four_panel"`
}

func (in illustrationPromptInput) scope() (string, string) { return in.ProjectID, in.NodeID }

type visualReference struct {
	ImageID  string `json:"image_id"`
	MIME     string `json:"mime"`
	Caption  string `json:"caption"`
	ByteSize int    `json:"byte_size"`
}
type visualCharacter struct {
	visual.Sheet
	Name            string            `json:"name"`
	Role            string            `json:"role"`
	HasSheet        bool              `json:"has_sheet"`
	ReferenceImages []visualReference `json:"reference_images"`
}
type visualStyle struct {
	Style          string `json:"style"`
	NegativePrompt string `json:"negative_prompt"`
}
type characterVisualsOutput struct {
	ProjectID       string            `json:"project_id"`
	ArtStyle        visualStyle       `json:"art_style"`
	Characters      []visualCharacter `json:"characters"`
	SkippedImageIDs []string          `json:"skipped_image_ids"`
}
type promptCharacter struct {
	EntityID string `json:"entity_id"`
	Name     string `json:"name"`
}
type illustrationPromptOutput struct {
	Prompt            string            `json:"prompt"`
	NegativePrompt    string            `json:"negative_prompt"`
	Format            string            `json:"format"`
	SceneLabel        string            `json:"scene_label,omitempty"`
	Characters        []promptCharacter `json:"characters"`
	ReferenceImageIDs []string          `json:"reference_image_ids"`
	MissingSheets     []string          `json:"missing_sheets"`
}

func registerVisualTools(s *mcp.Server, d ToolDeps) {
	mcp.AddTool(s, &mcp.Tool{Name: "linetta_get_character_visuals", Description: "Read the work's character visual sheets (appearance, palette, reference images) and illustration art style."}, record(d, "linetta_get_character_visuals", d.getCharacterVisuals))
	mcp.AddTool(s, &mcp.Tool{Name: "linetta_build_illustration_prompt", Description: "Compose an image prompt (single illustration or four-panel comic) from the characters' visual sheets, the art style, and a scene. Generates no image."}, record(d, "linetta_build_illustration_prompt", d.buildIllustrationPrompt))
}
func (d ToolDeps) resolveVisualCharacters(ctx context.Context, projectID string, ids, names []string) ([]entity.Entity, *mcp.CallToolResult) {
	all, err := d.Entities.ListByProject(ctx, projectID)
	if err != nil {
		return nil, toolErr("could not read characters: %v", err)
	}
	chars, err := visual.ResolveCharacters(all, ids, names)
	if err != nil {
		return nil, toolErr("%s", err.Error())
	}
	return chars, nil
}
func (d ToolDeps) getCharacterVisuals(ctx context.Context, _ *mcp.CallToolRequest, in characterVisualsInput) (*mcp.CallToolResult, characterVisualsOutput, error) {
	out := characterVisualsOutput{Characters: []visualCharacter{}, SkippedImageIDs: []string{}}
	if d.Visuals == nil {
		return toolErr("character visuals are unavailable in this build"), out, nil
	}
	p, refusal := d.requireProject(ctx, in.ProjectID)
	if refusal != nil {
		return refusal, out, nil
	}
	chars, refusal := d.resolveVisualCharacters(ctx, p.ID, in.EntityIDs, in.Names)
	if refusal != nil {
		return refusal, out, nil
	}
	style, err := d.Visuals.GetArtStyle(ctx, p.ID)
	if err != nil {
		return toolErr("could not read art style: %v", err), out, nil
	}
	sheets, err := d.Visuals.ListSheetsByProject(ctx, p.ID)
	if err != nil {
		return toolErr("could not read designs: %v", err), out, nil
	}
	out.ProjectID = p.ID
	out.ArtStyle = visualStyle{style.Style, style.NegativePrompt}
	content := []mcp.Content{}
	used := 0
	for _, e := range chars {
		sheet, has := sheets[e.ID]
		sheet.EntityID = e.ID
		row := visualCharacter{Sheet: sheet, Name: e.Name, Role: e.Role, HasSheet: has, ReferenceImages: []visualReference{}}
		images, err := d.Visuals.ListImages(ctx, e.ID)
		if err != nil {
			return toolErr("could not read references: %v", err), out, nil
		}
		for _, meta := range images {
			row.ReferenceImages = append(row.ReferenceImages, visualReference{meta.ID, meta.MIME, meta.Caption, meta.ByteSize})
			if !in.IncludeImages {
				continue
			}
			// Bound binary output independently of the number of selected characters.
			if used+meta.ByteSize > 12*1024*1024 {
				out.SkippedImageIDs = append(out.SkippedImageIDs, meta.ID)
				continue
			}
			img, err := d.Visuals.GetImage(ctx, meta.ID)
			if err != nil {
				return toolErr("could not read image: %v", err), out, nil
			}
			content = append(content, &mcp.ImageContent{Data: img.Data, MIMEType: meta.MIME})
			used += len(img.Data)
		}
		out.Characters = append(out.Characters, row)
	}
	if !in.IncludeImages {
		return nil, out, nil
	}
	// The SDK fills Content with the JSON of out only when the handler returns
	// no result of its own; returning images means writing that text ourselves,
	// first, so a text-only client still gets the structured answer.
	// SDK v1.7 does not add JSON text when object output already has image content.
	body, err := json.Marshal(out)
	if err != nil {
		return nil, out, err
	}
	return &mcp.CallToolResult{Content: append([]mcp.Content{&mcp.TextContent{Text: string(body)}}, content...)}, out, nil
}
func (d ToolDeps) buildIllustrationPrompt(ctx context.Context, _ *mcp.CallToolRequest, in illustrationPromptInput) (*mcp.CallToolResult, illustrationPromptOutput, error) {
	out := illustrationPromptOutput{Characters: []promptCharacter{}, ReferenceImageIDs: []string{}, MissingSheets: []string{}}
	if d.Visuals == nil {
		return toolErr("character visuals are unavailable in this build"), out, nil
	}
	req := visual.ComposeInput{EntityIDs: in.EntityIDs, Names: in.Names, Scene: in.Scene, Format: in.Format}
	if err := req.Validate(); err != nil {
		return toolErr("%s", err.Error()), out, nil
	}
	p, refusal := d.requireProject(ctx, in.ProjectID)
	if refusal != nil {
		return refusal, out, nil
	}
	if in.NodeID != "" {
		n, refusal := d.requireNode(ctx, in.NodeID)
		if refusal != nil {
			return refusal, out, nil
		}
		if n.ProjectID != p.ID {
			return toolErr("scene belongs to another work"), out, nil
		}
		out.SceneLabel = n.Label
	}
	all, err := d.Entities.ListByProject(ctx, p.ID)
	if err != nil {
		return toolErr("could not read characters: %v", err), out, nil
	}
	c, err := d.Visuals.Compose(ctx, p.ID, all, req)
	if err != nil {
		return toolErr("%s", err.Error()), out, nil
	}
	for _, ch := range c.Characters {
		out.Characters = append(out.Characters, promptCharacter{ch.EntityID, ch.Name})
	}
	out.Prompt, out.NegativePrompt, out.Format = c.Prompt, c.NegativePrompt, c.Format
	out.ReferenceImageIDs, out.MissingSheets = c.ReferenceImageIDs, c.MissingSheets
	return nil, out, nil
}
