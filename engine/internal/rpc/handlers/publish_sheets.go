package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// PublishSheetsService is the part of publish.Service the sheet and cover
// handlers use (works Phase 8).
type PublishSheetsService interface {
	SheetPublished(ctx context.Context, entityID string) (bool, error)
	SetSheetPublished(ctx context.Context, entityID string, on bool) error
	SetCover(ctx context.Context, projectID, illustrationID string) error
	PublishCover(ctx context.Context, projectID, illustrationID string) (publish.CoverView, error)
	SyncSheets(ctx context.Context, projectID string) (publish.SheetsStatus, error)
}

type sheetPublishParams struct {
	EntityID  string `json:"entity_id"`
	Published *bool  `json:"published"`
}

// PublishSheetGet returns a handler for publish.sheet_get: whether the writer
// turned on publishing for a sheet.
func PublishSheetGet(svc PublishSheetsService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in sheetPublishParams
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.EntityID) == "" {
			return nil, invalidParams("entity_id required")
		}
		on, err := svc.SheetPublished(ctx, in.EntityID)
		if err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(map[string]bool{"published": on})
	}
}

// PublishSheetSet returns a handler for publish.sheet_set. It changes only the
// writer's choice; publish.sheets_sync sends it.
func PublishSheetSet(svc PublishSheetsService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in sheetPublishParams
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.EntityID) == "" || in.Published == nil {
			return nil, invalidParams("entity_id and published required")
		}
		if err := svc.SetSheetPublished(ctx, in.EntityID, *in.Published); err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(map[string]bool{"published": *in.Published})
	}
}

// PublishCoverSet returns a handler for publish.cover_set. An empty
// illustration_id clears the cover.
func PublishCoverSet(svc PublishSheetsService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID      string `json:"project_id"`
			IllustrationID string `json:"illustration_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, invalidParams("project_id required")
		}
		if err := svc.SetCover(ctx, in.ProjectID, strings.TrimSpace(in.IllustrationID)); err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(map[string]string{"cover_id": strings.TrimSpace(in.IllustrationID)})
	}
}

// PublishCoverPublish returns a handler for publish.cover_publish: make a
// gallery picture the cover and upload it now, without the sheets.
func PublishCoverPublish(svc PublishSheetsService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID      string `json:"project_id"`
			IllustrationID string `json:"illustration_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.IllustrationID) == "" {
			return nil, invalidParams("project_id and illustration_id required")
		}
		out, err := svc.PublishCover(ctx, in.ProjectID, strings.TrimSpace(in.IllustrationID))
		if err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(out)
	}
}

// PublishSheetsSync returns a handler for publish.sheets_sync: send the chosen
// sheets and cover to the site.
func PublishSheetsSync(svc PublishSheetsService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, invalidParams("project_id required")
		}
		out, err := svc.SyncSheets(ctx, in.ProjectID)
		if err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(out)
	}
}
