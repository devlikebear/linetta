package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/relgraph"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// RelationshipGraphDeps is what relationships.graph reads.
type RelationshipGraphDeps struct {
	Projects      *project.Repo
	Nodes         *node.Repo
	Entities      *entity.Repo
	Relationships *relationship.Repo
	Mentions      *mention.Repo
}

// RelationshipGraph handles relationships.graph (#386 B2): the work's
// entities and relationships as they stand at one episode. episode is the
// 1-based ordinal; without it the graph shows the episode node_id is in, or
// the latest written one.
func RelationshipGraph(d RelationshipGraphDeps) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
			Episode   int    `json:"episode,omitempty"`
			NodeID    string `json:"node_id,omitempty"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		p, err := d.Projects.Get(ctx, in.ProjectID)
		if err != nil {
			return nil, rpc.NotFound(rpc.ReasonProjectNotFound, err.Error())
		}
		nodes, err := d.Nodes.ListByProject(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		ents, err := d.Entities.ListByProject(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		rels, err := d.Relationships.ListByProject(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		seen, err := d.Mentions.AppearancesByProject(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(relgraph.Build(relgraph.Input{
			Nodes: nodes, Preset: p.OutlinePreset, Entities: ents, Relationships: rels,
			Appearances: seen, AsOf: in.Episode, CurrentNodeID: in.NodeID,
		}))
	}
}
