package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/illustration"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/visual"
)

// illustrations.* serve the illustration dialog and the work's gallery;
// image_providers.* serve the image settings pane. Pictures travel as base64
// like visuals.* images do.

// ImageProviderService is the image side of the provider layer, with plain
// types for the same reason as ProviderService.
type ImageProviderService interface {
	List(ctx context.Context) (json.RawMessage, error)
	ListModels(ctx context.Context, id string) ([]string, error)
}

// IllustrationGenerator draws and stores one illustration.
type IllustrationGenerator interface {
	Generate(ctx context.Context, in illustration.GenerateInput) (illustration.Meta, error)
}

func illustrationError(err error) error {
	if err == nil {
		return nil
	}
	reasons := []struct {
		err    error
		reason string
	}{
		{illustration.ErrNotFound, rpc.ReasonIllustrationNotFound},
		{illustration.ErrProjectNotFound, rpc.ReasonProjectNotFound},
		{illustration.ErrNodeNotFound, rpc.ReasonNodeNotFound},
		{illustration.ErrTooLarge, rpc.ReasonIllustrationTooLarge},
		{illustration.ErrTooMany, rpc.ReasonTooManyIllustrations},
		{illustration.ErrBadContent, rpc.ReasonIllustrationBadContent},
		{illustration.ErrInUse, rpc.ReasonIllustrationInUse},
		{illustration.ErrIsCover, rpc.ReasonIllustrationIsCover},
		{illustration.ErrIsRepresentative, rpc.ReasonIllustrationIsRepresentative},
		{illustration.ErrRequestNotFound, rpc.ReasonIllustrationRequestNotFound},
		{illustration.ErrNotPending, rpc.ReasonIllustrationRequestDecided},
		{illustration.ErrTooManyPending, rpc.ReasonTooManyIllustrationRequests},
	}
	for _, r := range reasons {
		if errors.Is(err, r.err) {
			return rpc.NotFound(r.reason, err.Error())
		}
	}
	var inputErr *visual.InputError
	if errors.Is(err, illustration.ErrInvalidInput) || errors.As(err, &inputErr) {
		return &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
	}
	// The provider's own reasons (no credential, no consent, refused, ...).
	return rpc.MethodErrorFrom(err)
}

func ImageProvidersList(svc ImageProviderService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := svc.List(ctx)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return out, nil
	}
}

func ImageProvidersListModels(svc ImageProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		p := decodeProviderParams(params)
		models, err := svc.ListModels(ctx, p.Provider)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		if models == nil {
			models = []string{}
		}
		return json.Marshal(struct {
			Models []string `json:"models"`
		}{models})
	}
}

// IllustrationsCompose builds the prompt the dialog shows for the writer to
// read and edit before anything is drawn. It sends nothing anywhere.
func IllustrationsCompose(visuals *visual.Repo, entities *entity.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string   `json:"project_id"`
			EntityIDs []string `json:"entity_ids"`
			Names     []string `json:"names"`
			Scene     string   `json:"scene"`
			Format    string   `json:"format"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		all, err := entities.ListByProject(ctx, in.ProjectID)
		if err != nil {
			return nil, illustrationError(err)
		}
		out, err := visuals.Compose(ctx, in.ProjectID, all, visual.ComposeInput{EntityIDs: in.EntityIDs, Names: in.Names, Scene: in.Scene, Format: in.Format})
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(out)
	}
}

// IllustrationsGenerate draws one picture and stores it in the gallery. It
// runs for as long as the provider takes, up to the service's timeout.
func IllustrationsGenerate(gen IllustrationGenerator) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in illustration.GenerateInput
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		out, err := gen.Generate(ctx, in)
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(out)
	}
}

// IllustrationRequestService keeps the pictures agents asked for until the
// writer approves (draws) or rejects them.
type IllustrationRequestService interface {
	List(ctx context.Context, projectID string, statuses []string) ([]illustration.Request, error)
	Get(ctx context.Context, id string) (illustration.Request, error)
	Approve(ctx context.Context, id string, edits illustration.ApproveEdits) (illustration.Request, error)
	Reject(ctx context.Context, id string) (illustration.Request, error)
}

// IllustrationRequestsList serves illustration_requests.list: a work's
// requests newest first, optionally only some statuses.
func IllustrationRequestsList(svc IllustrationRequestService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			ProjectID string   `json:"project_id"`
			Statuses  []string `json:"statuses"`
		}
		if err := json.Unmarshal(params, &p); err != nil || strings.TrimSpace(p.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		out, err := svc.List(ctx, p.ProjectID, p.Statuses)
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(out)
	}
}

type illustrationRequestParams struct {
	ID string `json:"id"`
	illustration.ApproveEdits
}

func decodeRequestParams(params json.RawMessage) (illustrationRequestParams, error) {
	var p illustrationRequestParams
	if err := json.Unmarshal(params, &p); err != nil || strings.TrimSpace(p.ID) == "" {
		return p, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
	}
	return p, nil
}

// IllustrationRequestsGet serves illustration_requests.get.
func IllustrationRequestsGet(svc IllustrationRequestService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		p, err := decodeRequestParams(params)
		if err != nil {
			return nil, err
		}
		out, err := svc.Get(ctx, p.ID)
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(out)
	}
}

// IllustrationRequestsApprove serves illustration_requests.approve: the
// writer's click that draws the picture, with what they changed on the card
// (prompt, aspect, caption, include_references). It returns when the drawing
// is done or failed, like illustrations.generate.
func IllustrationRequestsApprove(svc IllustrationRequestService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		p, err := decodeRequestParams(params)
		if err != nil {
			return nil, err
		}
		out, err := svc.Approve(ctx, p.ID, p.ApproveEdits)
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(out)
	}
}

// IllustrationRequestsReject serves illustration_requests.reject.
func IllustrationRequestsReject(svc IllustrationRequestService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		p, err := decodeRequestParams(params)
		if err != nil {
			return nil, err
		}
		out, err := svc.Reject(ctx, p.ID)
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(out)
	}
}

// IllustrationsRepresentatives serves illustrations.representatives: for
// each node of the work that has one, the picture that stands for it.
func IllustrationsRepresentatives(repo *illustration.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &p); err != nil || strings.TrimSpace(p.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		out, err := repo.Representatives(ctx, p.ProjectID)
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(out)
	}
}

// IllustrationsSetRepresentative serves illustrations.set_representative:
// the writer choosing (or, with an empty illustration_id, clearing) the
// picture that stands for a scene, an episode or a part.
func IllustrationsSetRepresentative(repo *illustration.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			NodeID         string `json:"node_id"`
			IllustrationID string `json:"illustration_id"`
		}
		if err := json.Unmarshal(params, &p); err != nil || strings.TrimSpace(p.NodeID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		previous, err := repo.SetRepresentative(ctx, now(), p.NodeID, strings.TrimSpace(p.IllustrationID))
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(map[string]string{"previous": previous})
	}
}

func IllustrationsList(repo *illustration.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		items, err := repo.ListByProject(ctx, in.ProjectID)
		if err != nil {
			return nil, illustrationError(err)
		}
		usage, err := repo.UsageByProject(ctx, in.ProjectID)
		if err != nil {
			return nil, illustrationError(err)
		}
		for i := range items {
			items[i].UsedIn = usage[items[i].ID]
		}
		count, size, err := repo.Usage(ctx, in.ProjectID)
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(struct {
			Items []illustration.Meta `json:"items"`
			Count int                 `json:"count"`
			Bytes int64               `json:"bytes"`
		}{items, count, size})
	}
}

func IllustrationsGet(repo *illustration.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in idParam
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		img, err := repo.Get(ctx, in.ID)
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(struct {
			illustration.Meta
			DataBase64 string `json:"data_base64"`
		}{img.Meta, base64.StdEncoding.EncodeToString(img.Data)})
	}
}

func IllustrationsUpdate(repo *illustration.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ID      string `json:"id"`
			Caption string `json:"caption"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.UpdateCaption(ctx, in.ID, in.Caption)
		if err != nil {
			return nil, illustrationError(err)
		}
		return json.Marshal(out)
	}
}

func IllustrationsDelete(repo *illustration.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in idParam
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if err := repo.Delete(ctx, in.ID); err != nil {
			return nil, illustrationError(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
