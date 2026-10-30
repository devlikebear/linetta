//go:build !mobile

package engineapp

import (
	"context"
	"database/sql"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// Publishing to Linetta Works is on in every desktop build, the Mac App Store
// one included: both entitlement files already carry network.client, and
// the feature only talks to the site once the writer connects.
const publishAvailable = true

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
	// sheets are the writer's sheets the work can show on the site.
	sheets publish.SheetSource
}

// setupPublish registers the publish.* handlers and returns the service the
// MCP tools publish through, plus its closer.
func setupPublish(deps publishDeps) (*publish.Service, func() error) {
	client := publish.NewClient(publish.BaseURLFromEnv(), DefaultVersion)
	creds := publish.NewCredentials(deps.settingsStore)
	state := publish.NewStateRepo(deps.db)
	connector := publish.NewConnector(client, creds, state.Clear)
	svc := publish.NewService(client, creds, connector, state, deps.projects, deps.nodes, deps.language, deps.clock).
		WithIllustrations(deps.images).
		WithSheets(deps.sheets)

	s := deps.server
	s.Handle("publish.connect_start", handlers.PublishConnectStart(connector))
	s.Handle("publish.connect_status", handlers.PublishConnectStatus(connector))
	s.Handle("publish.disconnect", handlers.PublishDisconnect(connector))
	s.Handle("publish.work_status", handlers.PublishWorkStatus(svc))
	s.Handle("publish.work_upsert", handlers.PublishWorkUpsert(svc))
	s.Handle("publish.episode_publish", handlers.PublishEpisode(svc))
	s.Handle("publish.episode_set_visibility", handlers.PublishEpisodeSetVisibility(svc))
	s.Handle("publish.sheet_get", handlers.PublishSheetGet(svc))
	s.Handle("publish.sheet_set", handlers.PublishSheetSet(svc))
	s.Handle("publish.cover_set", handlers.PublishCoverSet(svc))
	s.Handle("publish.sheets_sync", handlers.PublishSheetsSync(svc))
	return svc, connector.Close
}
