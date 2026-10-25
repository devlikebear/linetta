package engineapp

import (
	"context"
	"errors"
	"sort"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/visual"
)

// publishSheets hands publishing the writer's sheets (works Phase 8): the
// sheet text, the relationship labels without their notes, and each sheet's
// first reference picture as its portrait. Visual notes stay here.
type publishSheets struct {
	entities      *entity.Repo
	relationships *relationship.Repo
	visuals       *visual.Repo
}

// ProjectSheets lists the project's sheets oldest first, so the site keeps
// the order the writer made them in.
func (p publishSheets) ProjectSheets(ctx context.Context, projectID string) ([]publish.LocalSheet, []publish.LocalRelation, error) {
	list, err := p.entities.ListByProject(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].CreatedAt != list[j].CreatedAt {
			return list[i].CreatedAt < list[j].CreatedAt
		}
		return list[i].ID < list[j].ID
	})
	sheets := make([]publish.LocalSheet, 0, len(list))
	for _, e := range list {
		sheets = append(sheets, publish.LocalSheet{
			ID: e.ID, Kind: e.Kind, Name: e.Name, Aliases: e.Aliases, Role: e.Role, Summary: e.Summary, Attributes: e.Attributes,
		})
	}
	rels, err := p.relationships.ListByProject(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	relations := make([]publish.LocalRelation, 0, len(rels))
	for _, r := range rels {
		relations = append(relations, publish.LocalRelation{From: r.FromID, To: r.ToID, Label: r.Label})
	}
	return sheets, relations, nil
}

// Portrait is the sheet's first reference picture.
func (p publishSheets) Portrait(ctx context.Context, entityID string) (string, []byte, bool, error) {
	images, err := p.visuals.ListImages(ctx, entityID)
	if err != nil || len(images) == 0 {
		return "", nil, false, err
	}
	img, err := p.visuals.GetImage(ctx, images[0].ID)
	if errors.Is(err, visual.ErrImageNotFound) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	return img.MIME, img.Data, true, nil
}

func (p publishSheets) SheetProject(ctx context.Context, entityID string) (string, error) {
	e, err := p.entities.Get(ctx, entityID)
	if errors.Is(err, entity.ErrNotFound) {
		return "", publish.ErrSheetNotFound
	}
	return e.ProjectID, err
}
