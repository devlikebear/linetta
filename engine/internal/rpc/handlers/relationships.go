package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// CreateOneRelationship handles relationships.create_one (singleton, no inverse).
func CreateOneRelationship(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in relationship.NewInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ProjectID == "" || in.FromID == "" || in.ToID == "" || in.Label == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams,
				Message: "project_id, from_id, to_id, label required"}
		}
		rel, err := repo.CreateOne(ctx, in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(rel)
	}
}

// CreatePairRelationship handles relationships.create_pair (two rows, shared pair_id).
func CreatePairRelationship(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in relationship.NewPairInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ProjectID == "" || in.FromID == "" || in.ToID == "" ||
			in.Label == "" || in.InverseLabel == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams,
				Message: "project_id, from_id, to_id, label, inverse_label required"}
		}
		rows, err := repo.CreatePair(ctx, in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(rows)
	}
}

type listByEntityParams struct {
	EntityID string `json:"entity_id"`
}

// ListRelationshipsByEntity handles relationships.list_by_entity.
func ListRelationshipsByEntity(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listByEntityParams
		if err := json.Unmarshal(params, &p); err != nil || p.EntityID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "entity_id required"}
		}
		list, err := repo.ListByEntity(ctx, p.EntityID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []relationship.Relationship{}
		}
		return json.Marshal(list)
	}
}

type listRelationshipsParams struct {
	ProjectID string `json:"project_id"`
}

// ListRelationships handles relationships.list — every relationship in a work.
//
// The per-entity list answers "who is this one tied to"; a browser showing the
// whole cast at once needs the other question, "how connected is each of them",
// and asking it one entity at a time is a round trip per row.
func ListRelationships(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listRelationshipsParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		list, err := repo.ListByProject(ctx, p.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []relationship.Relationship{}
		}
		return json.Marshal(list)
	}
}

// UpdateRelationship handles relationships.update (single row only).
func UpdateRelationship(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in relationship.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Update(ctx, in); err != nil {
			if errors.Is(err, relationship.ErrNotFound) {
				return nil, rpc.NotFound(rpc.ReasonRelationshipNotFound, "relationship not found")
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		got, err := repo.Get(ctx, in.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

// DeleteRelationship handles relationships.delete (atomic pair delete if pair_id non-NULL).
func DeleteRelationship(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Delete(ctx, p.ID); err != nil {
			if errors.Is(err, relationship.ErrNotFound) {
				return nil, rpc.NotFound(rpc.ReasonRelationshipNotFound, "relationship not found")
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]bool{"ok": true})
	}
}
