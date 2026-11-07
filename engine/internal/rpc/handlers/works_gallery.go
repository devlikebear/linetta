package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// works.* show what readers see on Linetta Works inside the app. They need no
// account and send nothing of the writer's.

// WorksGallerySource is the site's public search.
type WorksGallerySource interface {
	Gallery(ctx context.Context, in publish.GalleryQuery) (publish.GalleryPage, error)
	GalleryCover(ctx context.Context, workID string) ([]byte, string, error)
}

func galleryError(err error) error {
	if errors.Is(err, publish.ErrGalleryQuery) {
		return invalidParams(err.Error())
	}
	return publishError(err)
}

// WorksGallery returns a handler for works.gallery.
func WorksGallery(src WorksGallerySource) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in publish.GalleryQuery
		if len(params) > 0 && string(params) != "null" {
			if err := json.Unmarshal(params, &in); err != nil {
				return nil, invalidParams("works.gallery: bad params")
			}
		}
		page, err := src.Gallery(ctx, in)
		if err != nil {
			return nil, galleryError(err)
		}
		return json.Marshal(page)
	}
}

// WorksGalleryCover returns a handler for works.gallery_cover. The cover comes
// back as a data URL: the page loads pictures only from the app itself.
func WorksGalleryCover(src WorksGallerySource) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			WorkID string `json:"work_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || !publish.ValidGalleryID(in.WorkID) {
			return nil, invalidParams("works.gallery_cover: work_id is required")
		}
		data, mime, err := src.GalleryCover(ctx, in.WorkID)
		if err != nil {
			return nil, galleryError(err)
		}
		return json.Marshal(map[string]string{
			"data_url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data),
		})
	}
}
