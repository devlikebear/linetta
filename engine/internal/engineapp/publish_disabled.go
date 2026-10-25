//go:build mobile

package engineapp

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// Publishing is out of scope on iOS for now: the screens hide it on
// publish_available = false, and the methods refuse rather than vanish, the
// way git sync does on the builds without it.
const publishAvailable = false

type publishDeps struct {
	server        *rpc.Server
	settingsStore *settings.Store
	projects      *project.Repo
	nodes         *node.Repo
	db            *sql.DB
	language      func(context.Context) string
	clock         func() int64
	// images is the gallery: published episodes carry their pictures.
	images publish.Illustrations
	// sheets are the writer's sheets; unused where publishing is off.
	sheets publish.SheetSource
}

func setupPublish(deps publishDeps) (*publish.Service, func() error) {
	unavailable := func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, &rpc.MethodError{
			Code:    rpc.CodeMethodNotFound,
			Message: "publishing is not available in this build",
		}
	}
	for _, method := range []string{
		"publish.connect_start", "publish.connect_status", "publish.disconnect",
		"publish.work_status", "publish.work_upsert",
		"publish.episode_publish", "publish.episode_set_visibility",
		"publish.sheet_get", "publish.sheet_set", "publish.cover_set", "publish.sheets_sync",
	} {
		deps.server.Handle(method, unavailable)
	}
	return nil, func() error { return nil }
}
