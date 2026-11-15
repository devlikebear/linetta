package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type manuscriptSearchParams struct {
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
	Limit     int    `json:"limit"`
	// Mode is auto (default), words or meaning (#384).
	Mode string `json:"mode,omitempty"`
}

func SearchManuscript(searcher *manuscript.Searcher) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p manuscriptSearchParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if strings.TrimSpace(p.ProjectID) == "" || strings.TrimSpace(p.Query) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and query required"}
		}
		mode := p.Mode
		if mode != manuscript.ModeWords && mode != manuscript.ModeMeaning {
			mode = manuscript.ModeAuto
		}
		hits, _, err := searcher.QueryMode(ctx, p.ProjectID, p.Query, p.Limit, mode)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if hits == nil {
			hits = []manuscript.Hit{}
		}
		return json.Marshal(hits)
	}
}
