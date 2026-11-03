package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type listMentionsParams struct {
	NodeID string `json:"node_id"`
}

func ListMentionsForNode(repo *mention.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listMentionsParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		list, err := repo.ListEntitiesForNode(ctx, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []entity.Entity{}
		}
		return json.Marshal(list)
	}
}

// RelinkWork returns a handler for mentions.relink_work: every scene's text run
// through the same linking a write does. A scene written before its names were
// registered has no links, and revising it does not add them, so the first scenes
// of a work are missing from everything that follows links. The words do not
// change, and a summary that was current stays current.
func RelinkWork(nodes *node.Repo, entities *entity.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		ents, err := entities.ListByProject(ctx, in.ProjectID)
		if err != nil {
			return nil, err
		}
		all, err := nodes.ListByProject(ctx, in.ProjectID)
		if err != nil {
			return nil, err
		}
		candidates := mention.BuildCandidates(ents)
		out := struct {
			Scenes int `json:"scenes"`
			Links  int `json:"links"`
		}{}
		for _, n := range all {
			if n.Kind != node.KindLeaf || n.ContentDoc == nil {
				continue
			}
			doc, linked, err := mention.AutoLinkDoc([]byte(*n.ContentDoc), candidates)
			if err != nil || len(linked) == 0 {
				continue
			}
			if err := nodes.UpdateContent(ctx, n.ID, string(doc), now()); err != nil {
				return nil, err
			}
			if n.SummaryFresh() {
				got, err := nodes.Get(ctx, n.ID)
				if err != nil {
					return nil, err
				}
				if err := nodes.SetWrittenSummary(ctx, n.ID, n.Summary, got.ContentVersion); err != nil {
					return nil, err
				}
			}
			out.Scenes++
			out.Links += len(linked)
		}
		return json.Marshal(out)
	}
}
