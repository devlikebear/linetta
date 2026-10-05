//go:build !mobile

package mcphost

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/visual"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type visualTarget struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"work id; required when using name"`
	EntityID  string `json:"entity_id,omitempty" jsonschema:"character id; use this or name"`
	Name      string `json:"name,omitempty" jsonschema:"character name or alias"`
}

func (in visualTarget) scope() (string, string) { return in.ProjectID, in.EntityID }

type setCharacterVisualsInput struct {
	visualTarget
	AgeRange  *string `json:"age_range,omitempty"`
	Build     *string `json:"build,omitempty"`
	Hair      *string `json:"hair,omitempty"`
	Outfit    *string `json:"outfit,omitempty"`
	Signature *string `json:"signature,omitempty"`
	Palette   *string `json:"palette,omitempty"`
	Notes     *string `json:"notes,omitempty"`
}
type setArtStyleInput struct {
	ProjectID      string  `json:"project_id"`
	Style          *string `json:"style,omitempty"`
	NegativePrompt *string `json:"negative_prompt,omitempty"`
}

func (in setArtStyleInput) scope() (string, string) { return in.ProjectID, "" }

type addReferenceImageInput struct {
	visualTarget
	DataBase64 string `json:"data_base64"`
	MIME       string `json:"mime"`
	Caption    string `json:"caption,omitempty"`
}
type deleteReferenceImageInput struct {
	ImageID string `json:"image_id"`
}

func (in deleteReferenceImageInput) scope() (string, string) { return "", in.ImageID }

type sheetWriteOutput struct {
	visual.Sheet
	UndoBatchID string `json:"undo_batch_id"`
}
type styleWriteOutput struct {
	visual.ArtStyle
	UndoBatchID string `json:"undo_batch_id"`
}
type imageWriteOutput struct {
	visual.ImageMeta
	UndoBatchID string `json:"undo_batch_id"`
}

func (d ToolDeps) registerVisualWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "linetta_set_character_visuals", Description: "Patch a character's visual sheet. Omitted fields stay unchanged; empty strings clear them. Returns the sheet and undo_batch_id."}, record(d, "linetta_set_character_visuals", d.setCharacterVisuals))
	mcp.AddTool(s, &mcp.Tool{Name: "linetta_set_art_style", Description: "Patch the work's art style and negative prompt. Omitted fields stay unchanged; empty strings clear them. Returns the style and undo_batch_id."}, record(d, "linetta_set_art_style", d.setArtStyle))
	mcp.AddTool(s, &mcp.Tool{Name: "linetta_add_reference_image", Description: "Attach a base64 PNG, JPEG, WebP or GIF reference to a character (5 MiB, 8 images maximum). Returns metadata and undo_batch_id."}, record(d, "linetta_add_reference_image", d.addReferenceImage))
	mcp.AddTool(s, &mcp.Tool{Name: "linetta_delete_reference_image", Description: "Delete a reference image by image_id. Returns deleted metadata and undo_batch_id; undo restores the original bytes."}, record(d, "linetta_delete_reference_image", d.deleteReferenceImage))
}
func (d ToolDeps) requireVisualWrite() *mcp.CallToolResult {
	if d.Visuals == nil || d.Story == nil {
		return toolErr("visual writes are unavailable in this build")
	}
	return nil
}
func (d ToolDeps) visualTarget(ctx context.Context, in visualTarget) (entity.Entity, *mcp.CallToolResult) {
	in.Name = strings.TrimSpace(in.Name)
	in.EntityID = strings.TrimSpace(in.EntityID)
	if (in.Name == "") == (in.EntityID == "") {
		return entity.Entity{}, toolErr("pass exactly one of name or entity_id")
	}
	if in.EntityID != "" {
		e, err := d.Entities.Get(ctx, in.EntityID)
		if err != nil {
			return e, toolErr("could not read character: %v", err)
		}
		if in.ProjectID != "" && strings.TrimSpace(in.ProjectID) != e.ProjectID {
			return e, toolErr("character belongs to another work")
		}
		if _, ref := d.requireProject(ctx, e.ProjectID); ref != nil {
			return e, ref
		}
		if e.Kind != entity.KindCharacter {
			return e, toolErr("entity must be a character")
		}
		return e, nil
	}
	p, ref := d.requireProject(ctx, in.ProjectID)
	if ref != nil {
		return entity.Entity{}, ref
	}
	chars, ref := d.resolveVisualCharacters(ctx, p.ID, nil, []string{in.Name})
	if ref != nil {
		return entity.Entity{}, ref
	}
	return chars[0], nil
}
func patchString(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}
func (d ToolDeps) visualChanged(projectID, tool string, undo func(context.Context) error) string {
	id := d.Story.RememberChange(projectID, undo)
	d.notifyChanged(projectID, tool, nil, id)
	return id
}
func (d ToolDeps) setCharacterVisuals(ctx context.Context, _ *mcp.CallToolRequest, in setCharacterVisualsInput) (*mcp.CallToolResult, sheetWriteOutput, error) {
	out := sheetWriteOutput{}
	if ref := d.requireVisualWrite(); ref != nil {
		return ref, out, nil
	}
	e, ref := d.visualTarget(ctx, in.visualTarget)
	if ref != nil {
		return ref, out, nil
	}
	before, err := d.Visuals.GetSheet(ctx, e.ID)
	if err != nil {
		return toolErr("%v", err), out, nil
	}
	sheets, err := d.Visuals.ListSheetsByProject(ctx, e.ProjectID)
	if err != nil {
		return toolErr("%v", err), out, nil
	}
	_, existed := sheets[e.ID]
	next := before
	patchString(&next.AgeRange, in.AgeRange)
	patchString(&next.Build, in.Build)
	patchString(&next.Hair, in.Hair)
	patchString(&next.Outfit, in.Outfit)
	patchString(&next.Signature, in.Signature)
	patchString(&next.Palette, in.Palette)
	patchString(&next.Notes, in.Notes)
	out.Sheet, err = d.Visuals.SetSheet(ctx, d.now(), next)
	if err != nil {
		return toolErr("%v", err), out, nil
	}
	out.UndoBatchID = d.visualChanged(e.ProjectID, "linetta_set_character_visuals", func(ctx context.Context) error { return d.Visuals.RestoreSheet(ctx, before, existed) })
	return nil, out, nil
}
func (d ToolDeps) setArtStyle(ctx context.Context, _ *mcp.CallToolRequest, in setArtStyleInput) (*mcp.CallToolResult, styleWriteOutput, error) {
	out := styleWriteOutput{}
	if ref := d.requireVisualWrite(); ref != nil {
		return ref, out, nil
	}
	p, ref := d.requireProject(ctx, in.ProjectID)
	if ref != nil {
		return ref, out, nil
	}
	before, err := d.Visuals.GetArtStyle(ctx, p.ID)
	if err != nil {
		return toolErr("%v", err), out, nil
	}
	existed, err := d.Visuals.HasArtStyle(ctx, p.ID)
	if err != nil {
		return toolErr("%v", err), out, nil
	}
	next := before
	patchString(&next.Style, in.Style)
	patchString(&next.NegativePrompt, in.NegativePrompt)
	out.ArtStyle, err = d.Visuals.SetArtStyle(ctx, d.now(), next)
	if err != nil {
		return toolErr("%v", err), out, nil
	}
	out.UndoBatchID = d.visualChanged(p.ID, "linetta_set_art_style", func(ctx context.Context) error {
		return d.Visuals.RestoreArtStyle(ctx, before, existed)
	})
	return nil, out, nil
}
func (d ToolDeps) addReferenceImage(ctx context.Context, _ *mcp.CallToolRequest, in addReferenceImageInput) (*mcp.CallToolResult, imageWriteOutput, error) {
	out := imageWriteOutput{}
	if ref := d.requireVisualWrite(); ref != nil {
		return ref, out, nil
	}
	e, ref := d.visualTarget(ctx, in.visualTarget)
	if ref != nil {
		return ref, out, nil
	}
	if len(in.DataBase64) > base64.StdEncoding.EncodedLen(visual.MaxImageBytes) {
		return toolErr("%v", visual.ErrImageTooLarge), out, nil
	}
	data, err := base64.StdEncoding.DecodeString(in.DataBase64)
	if err != nil {
		return toolErr("invalid data_base64: %v", err), out, nil
	}
	out.ImageMeta, err = d.Visuals.AddImage(ctx, d.now(), e.ID, in.MIME, in.Caption, data)
	if err != nil {
		return toolErr("%v", err), out, nil
	}
	id := out.ID
	out.UndoBatchID = d.visualChanged(e.ProjectID, "linetta_add_reference_image", func(ctx context.Context) error { return d.Visuals.DeleteImage(ctx, id) })
	return nil, out, nil
}
func (d ToolDeps) deleteReferenceImage(ctx context.Context, _ *mcp.CallToolRequest, in deleteReferenceImageInput) (*mcp.CallToolResult, imageWriteOutput, error) {
	out := imageWriteOutput{}
	if ref := d.requireVisualWrite(); ref != nil {
		return ref, out, nil
	}
	before, err := d.Visuals.GetImage(ctx, strings.TrimSpace(in.ImageID))
	if err != nil {
		return toolErr("%v", err), out, nil
	}
	e, ref := d.visualTarget(ctx, visualTarget{EntityID: before.EntityID})
	if ref != nil {
		return ref, out, nil
	}
	if err = d.Visuals.DeleteImage(ctx, before.ID); err != nil {
		return toolErr("%v", err), out, nil
	}
	out.ImageMeta = before.ImageMeta
	out.UndoBatchID = d.visualChanged(e.ProjectID, "linetta_delete_reference_image", func(ctx context.Context) error { return d.Visuals.RestoreImage(ctx, before) })
	return nil, out, nil
}
