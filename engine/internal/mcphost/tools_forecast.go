//go:build !mobile

package mcphost

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/forecast"
)

// ---------- linetta_save_forecast ----------
//
// story-assist Phase 4. An agent drafts 2-5 possible continuations from the
// latest written episode and saves them here for the writer to compare in
// the forecast sheet. Nothing about this call touches canon: the outline,
// the manuscript and Story World are unchanged, and the writer's own
// forecast.select RPC — not an MCP tool, not an LLM turn — is what puts a
// chosen branch into the brief as a plan. Reflecting a chosen branch into
// the outline afterwards goes through linetta_propose_story_ops
// purpose=forecast, which this tool does not do itself.

// saveForecastInput is linetta_save_forecast's input. No branch carries an
// id: the engine assigns b1..bN in submission order (forecast.Service.Save).
type saveForecastInput struct {
	ProjectID  string                 `json:"project_id"`
	Divergence string                 `json:"divergence" jsonschema:"the question the writer asked, up to 300 characters, e.g. '다음 회차 갈래 3개'"`
	Horizon    int                    `json:"horizon" jsonschema:"1..10: how many episodes ahead these branches look"`
	Branches   []forecast.BranchInput `json:"branches" jsonschema:"2..5 branches to compare"`
}

func (in saveForecastInput) scope() (string, string) { return in.ProjectID, "" }

// saveForecastStatus mirrors proposeStoryOpsStatus/
// continuity.ReportStatusWaitingForWriter's convention: nothing an agent
// does here moves a forecast further than waiting for the writer to pick.
const saveForecastStatus = "waiting_for_writer"

type saveForecastOutput struct {
	ForecastID string   `json:"forecast_id"`
	BranchIDs  []string `json:"branch_ids"`
	// BaseEpisodeLabel is the episode these branches were drawn from
	// (episode.LastWritten at save time), so the agent can tell the writer
	// which chapter they diverge from without a separate round trip.
	BaseEpisodeLabel string `json:"base_episode_label,omitempty"`
	Status           string `json:"status"`
}

func (d ToolDeps) registerForecastWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_save_forecast",
		Description: "Save 2–5 possible continuations from the latest written episode for the writer to " +
			"compare. Not canon: the outline, manuscript and Story World do not change, and the writer " +
			"picks a branch (or none) in the app. Draw every branch from the brief; put conflicts with " +
			"the work's own record under risks, citing the entity or fact.",
	}, record(d, "linetta_save_forecast", d.saveForecast))
}

func (d ToolDeps) saveForecast(ctx context.Context, _ *mcp.CallToolRequest, in saveForecastInput) (*mcp.CallToolResult, saveForecastOutput, error) {
	if d.Forecasts == nil {
		return toolErr("forecasts are not available in this build"), saveForecastOutput{}, nil
	}
	f, problems, err := d.Forecasts.Save(ctx, d.now(), forecast.SaveInput{
		ProjectID: in.ProjectID, Divergence: in.Divergence, Horizon: in.Horizon, Branches: in.Branches,
	}, d.sourceOrExternal())
	if err != nil {
		return nil, saveForecastOutput{}, err
	}
	if len(problems) > 0 {
		return toolErr("%s", forecastProblemsText(problems)), saveForecastOutput{}, nil
	}

	branchIDs := make([]string, len(f.Branches))
	for i, b := range f.Branches {
		branchIDs[i] = b.ID
	}
	label := ""
	if f.BaseNodeID != "" && d.Projects != nil && d.Nodes != nil {
		if proj, perr := d.Projects.Get(ctx, in.ProjectID); perr == nil {
			if ep, eerr := episode.Resolve(ctx, d.Nodes, proj.OutlinePreset, f.BaseNodeID); eerr == nil {
				label = ep.Label
			}
		}
	}

	d.notifyChanged(in.ProjectID, "linetta_save_forecast", nil, "")
	return nil, saveForecastOutput{
		ForecastID: f.ID, BranchIDs: branchIDs, BaseEpisodeLabel: label, Status: saveForecastStatus,
	}, nil
}

// forecastProblemsText joins every forecast.Problem into one refusal,
// numbered so a model fixing several at once can tell them apart —
// proposalProblemsText's convention, for forecast.Problem's identical shape
// in a different package.
func forecastProblemsText(problems []forecast.Problem) string {
	if len(problems) == 1 {
		return problems[0].Message
	}
	lines := make([]string, len(problems))
	for i, p := range problems {
		lines[i] = p.Message
	}
	return strings.Join(lines, "\n")
}
