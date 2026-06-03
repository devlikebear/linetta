package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func CreateEntity(repo *entity.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in entity.NewInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ProjectID == "" || in.Name == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and name required"}
		}
		e, err := repo.Create(ctx, now(), in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(e)
	}
}

type searchEntitiesParams struct {
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
	Limit     int    `json:"limit"`
}

func SearchEntities(repo *entity.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p searchEntitiesParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		list, err := repo.Search(ctx, p.ProjectID, p.Query, p.Limit)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []entity.Entity{}
		}
		return json.Marshal(list)
	}
}

func ListEntities(repo *entity.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p searchEntitiesParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		list, err := repo.ListByProject(ctx, p.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []entity.Entity{}
		}
		return json.Marshal(list)
	}
}

func GetEntity(repo *entity.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		e, err := repo.Get(ctx, p.ID)
		if errors.Is(err, entity.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "entity not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(e)
	}
}

func UpdateEntity(repo *entity.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in entity.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Update(ctx, now(), in); err != nil {
			if errors.Is(err, entity.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "entity not found"}
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

type entityScenesParams struct {
	EntityID string `json:"entity_id"`
}

// SceneMention is one scene where an entity appears (RPC result shape).
type SceneMention struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
}

// EntityScenes returns the distinct scenes (leaf nodes) where the entity is
// mentioned, in document order (tree DFS), each with a breadcrumb label.
func EntityScenes(mentions *mention.Repo, nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p entityScenesParams
		if err := json.Unmarshal(params, &p); err != nil || p.EntityID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "entity_id required"}
		}
		ids, projectID, err := mentions.MentionedNodeIDs(ctx, p.EntityID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if len(ids) == 0 {
			return json.Marshal([]SceneMention{})
		}
		mentioned := make(map[string]bool, len(ids))
		for _, id := range ids {
			mentioned[id] = true
		}
		all, err := nodes.ListByProject(ctx, projectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		byID := make(map[string]node.Node, len(all))
		children := map[string][]node.Node{}
		for _, n := range all {
			byID[n.ID] = n
			key := ""
			if n.ParentID != nil {
				key = *n.ParentID
			}
			children[key] = append(children[key], n)
		}
		out := []SceneMention{}
		var walk func(parent string)
		walk = func(parent string) {
			for _, c := range children[parent] {
				if c.Kind == "leaf" && mentioned[c.ID] {
					out = append(out, SceneMention{NodeID: c.ID, Label: breadcrumbLabel(byID, c)})
				}
				walk(c.ID)
			}
		}
		walk("")
		return json.Marshal(out)
	}
}

// breadcrumbLabel builds "부 / 장 / 씬" by walking parent_id up to the root.
func breadcrumbLabel(byID map[string]node.Node, n node.Node) string {
	parts := []string{n.Label}
	cur := n
	for cur.ParentID != nil {
		par, ok := byID[*cur.ParentID]
		if !ok {
			break
		}
		parts = append([]string{par.Label}, parts...)
		cur = par
	}
	return strings.Join(parts, " / ")
}
