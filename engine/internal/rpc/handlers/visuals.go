package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/visual"
)

// visuals.* serve the character visual sheet editor (#154). Images travel as
// base64 in JSON because that is what the JSON-RPC bridge carries; the 5 MiB
// cap in visual.AddImage keeps one message a reasonable size.

const errIDRequired = "id required"

func visualError(err error) error {
	if err == nil {
		return nil
	}
	reasons := []struct {
		err    error
		reason string
	}{
		{visual.ErrEntityNotFound, rpc.ReasonEntityNotFound}, {visual.ErrProjectNotFound, rpc.ReasonProjectNotFound},
		{visual.ErrImageNotFound, rpc.ReasonImageNotFound}, {visual.ErrImageTooLarge, rpc.ReasonImageTooLarge},
		{visual.ErrUnsupportedImageType, rpc.ReasonUnsupportedImageType}, {visual.ErrTooManyImages, rpc.ReasonTooManyImages},
	}
	for _, r := range reasons {
		if errors.Is(err, r.err) {
			return rpc.NotFound(r.reason, err.Error())
		}
	}
	return &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
}

type visualImageResult struct {
	visual.ImageMeta
	DataBase64 string `json:"data_base64"`
}

func GetVisualSheet(repo *visual.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in visual.Sheet
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.EntityID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.GetSheet(ctx, in.EntityID)
		if err != nil {
			return nil, visualError(err)
		}
		return json.Marshal(out)
	}
}
func SetVisualSheet(repo *visual.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in visual.Sheet
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.EntityID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.SetSheet(ctx, now(), in)
		if err != nil {
			return nil, visualError(err)
		}
		return json.Marshal(out)
	}
}
func ListVisualImages(repo *visual.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in visual.Sheet
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.EntityID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.ListImages(ctx, in.EntityID)
		if err != nil {
			return nil, visualError(err)
		}
		return json.Marshal(out)
	}
}
func GetVisualImage(repo *visual.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in idParam
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		img, err := repo.GetImage(ctx, in.ID)
		out := visualImageResult{img.ImageMeta, base64.StdEncoding.EncodeToString(img.Data)}
		if err != nil {
			return nil, visualError(err)
		}
		return json.Marshal(out)
	}
}
func AddVisualImage(repo *visual.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			EntityID   string `json:"entity_id"`
			MIME       string `json:"mime"`
			Caption    string `json:"caption"`
			DataBase64 string `json:"data_base64"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.EntityID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if len(in.DataBase64) > base64.StdEncoding.EncodedLen(visual.MaxImageBytes) {
			return nil, visualError(visual.ErrImageTooLarge)
		}
		data, err := base64.StdEncoding.DecodeString(in.DataBase64)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "invalid data_base64"}
		}
		out, err := repo.AddImage(ctx, now(), in.EntityID, in.MIME, in.Caption, data)
		if err != nil {
			return nil, visualError(err)
		}
		return json.Marshal(out)
	}
}
func UpdateVisualImage(repo *visual.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ID      string `json:"id"`
			Caption string `json:"caption"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.UpdateImageCaption(ctx, in.ID, in.Caption)
		if err != nil {
			return nil, visualError(err)
		}
		return json.Marshal(out)
	}
}
func DeleteVisualImage(repo *visual.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in idParam
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		err := repo.DeleteImage(ctx, in.ID)
		out := map[string]bool{"ok": true}
		if err != nil {
			return nil, visualError(err)
		}
		return json.Marshal(out)
	}
}
func GetArtStyle(repo *visual.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in visual.ArtStyle
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.GetArtStyle(ctx, in.ProjectID)
		if err != nil {
			return nil, visualError(err)
		}
		return json.Marshal(out)
	}
}
func SetArtStyle(repo *visual.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in visual.ArtStyle
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.SetArtStyle(ctx, now(), in)
		if err != nil {
			return nil, visualError(err)
		}
		return json.Marshal(out)
	}
}
