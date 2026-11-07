package engineapp

import (
	"database/sql"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/continuity"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/forecast"
	"github.com/devlikebear/linetta/engine/internal/illustration"
	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/proposals"
	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/style"
	"github.com/devlikebear/linetta/engine/internal/thread"
	"github.com/devlikebear/linetta/engine/internal/visual"
)

// mcpDeps is what setupMCP needs from register().
type mcpDeps struct {
	settingsStore *settings.Store
	home          string
	repos         mcpToolRepos
}

// mcpToolRepos collects what the tool layer reads from. The context builder is
// a second instance wired with fact/memory/reference sources — the builder the
// AI runner uses stays untouched so its prompts do not change.
type mcpToolRepos struct {
	visuals       *visual.Repo
	illustrations *illustration.Repo
	// requests are the pictures agents asked for (illustration Phase 4).
	requests *illustration.RequestService
	style    *style.Repo
	projects *project.Repo
	nodes    *node.Repo
	entities *entity.Repo
	mentions *mention.Repo
	facts    *fact.Repo
	// beats and threads back linetta_report_continuity's beat refs: a beat
	// carries only a thread_id, so checking one belongs to this work means
	// resolving its thread (story-assist Phase 2).
	beats         *beat.Repo
	relationships *relationship.Repo
	threads       *thread.Repo
	continuity    *continuity.Repo
	ledger        *ledger.Repo
	// proposals holds story-structure change candidates waiting for the
	// writer to accept them one by one (story-assist Phase 3). Shared with
	// the proposals.* RPC handlers — one Service, one undo-free apply path.
	proposals *proposals.Service
	// forecasts holds possible continuations an agent drafted for the
	// writer to compare (story-assist Phase 4). Shared with the
	// forecast.* RPC handlers and with proposals (purpose=forecast's own
	// ForecastID check) — one Service, one fingerprint.
	forecasts *forecast.Service
	// plans answers the plan gate's question for both servers.
	plans      mcphost.PlanLookup
	plot       *plot.Builder
	manuscript *manuscript.Searcher
	context    *storycontext.ContextBuilder
	snapshots  *snapshot.Repo
	story      *storyops.Service
	msEdit     *manuscriptedit.Service
	// memory is the two curated documents linetta_edit_memory writes. The
	// same *agentmemory.Repo the companion reads them back through and the
	// settings pane edits: one row per document, one writer per row at a
	// time, so an agent's edit and a writer's textarea cannot fork.
	memory *agentmemory.Repo
	// history backs the agent-only conversation tools.
	history *companion.HistoryRepo
	// skills are the SKILL.md documents linetta_edit_skill writes and
	// linetta_read_skill opens: a filesystem store under the Linetta home,
	// plus the version history every write lands a row in so the writer can
	// revert what an agent wrote. Both are shared with the built-in agent's
	// server, the same way memory is — one home directory, one table.
	skills       *agentskills.Store
	skillHistory *agentskills.History
	// publish is nil on a build that cannot publish.
	publish *publish.Service
	enqueue func(nodeID string)
	notify  func(method string, params any)
	clock   func() int64
	db      *sql.DB
}

// agentToolDeps is what setupMCP hands the built-in agent to clone for its
// own in-memory server (#93). Every build has it: the agent reaches the tools
// over an in-memory pipe, so a build that hosts no MCP server (mobile) still
// gets the full tool set.
type agentToolDeps = mcphost.ToolDeps

// buildToolDeps assembles the tool layer from the repos. The external host and
// the agent's in-memory server are both wired from this one list.
func buildToolDeps(deps mcpDeps, activity *mcphost.ActivityRepo) mcphost.ToolDeps {
	tools := mcphost.ToolDeps{
		Visuals:              deps.repos.visuals,
		Illustrations:        deps.repos.illustrations,
		Gallery:              deps.repos.illustrations,
		IllustrationRequests: deps.repos.requests,
		Style:                deps.repos.style,
		Projects:             deps.repos.projects,
		Nodes:                deps.repos.nodes,
		Entities:             deps.repos.entities,
		Mentions:             deps.repos.mentions,
		Facts:                deps.repos.facts,
		Beats:                deps.repos.beats,
		Threads:              deps.repos.threads,
		Relationships:        deps.repos.relationships,
		Continuity:           deps.repos.continuity,
		Ledger:               deps.repos.ledger,
		Proposals:            deps.repos.proposals,
		Forecasts:            deps.repos.forecasts,
		Plans:                deps.repos.plans,
		Plot:                 deps.repos.plot,
		Manuscript:           deps.repos.manuscript,
		Context:              deps.repos.context,
		Settings:             deps.settingsStore,
		Activity:             activity,
		Memory:               deps.repos.memory,
		History:              deps.repos.history,

		Skills:       deps.repos.skills,
		SkillHistory: deps.repos.skillHistory,

		Snapshots:      deps.repos.snapshots,
		Story:          deps.repos.story,
		ManuscriptEdit: deps.repos.msEdit,
		Limiter:        mcphost.NewLimiter(),
		EnqueueSummary: deps.repos.enqueue,
		Notify:         deps.repos.notify,
		Clock:          deps.repos.clock,
	}
	// A nil *publish.Service in the interface field would be a non-nil
	// Publisher that panics; leave the field nil instead, which the tools
	// refuse cleanly.
	if deps.repos.publish != nil {
		tools.Publish = deps.repos.publish
	}
	// A nil *publish.Service in the interface field would be a non-nil
	// Publisher that panics; leave the field nil instead, which the tools
	// refuse cleanly.
	if deps.repos.publish != nil {
		tools.Publish = deps.repos.publish
	}
	// What the tool set costs, for the Settings pane's tool-budget group
	// (#99). Wired here because internal/settings may not import mcphost.
	deps.settingsStore.WithToolBudget(mcphost.MeasureToolBudget)
	return tools
}
