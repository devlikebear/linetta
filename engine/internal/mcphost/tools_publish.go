//go:build !mobile

package mcphost

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// Publisher is the slice of internal/publish the publish tools use. An agent
// can look at what is on Linetta Works and publish an episode, and that is
// all: connecting the account and registering the work are the writer's to
// do in the app, so there is no tool for either.
type Publisher interface {
	WorkStatus(ctx context.Context, projectID string) (publish.WorkStatus, error)
	PublishEpisode(ctx context.Context, nodeID string) (publish.PublishResult, error)
}

// ---------- linetta_publish_status ----------

type publishStatusInput struct {
	ProjectID string `json:"project_id" jsonschema:"id of the work, from linetta_list_works"`
}

func (in publishStatusInput) scope() (string, string) { return in.ProjectID, "" }

// publishedEpisode is deliberately lean: every field here is schema the
// agent is sent on every turn (see budget_test.go).
type publishedEpisode struct {
	NodeID string `json:"node_id"`
	Title  string `json:"title,omitempty"`
	// State is unpublished, public, modified (public, but edited since),
	// pending, rejected, needs_review, private, or site_only.
	State  string `json:"state"`
	Number int    `json:"number,omitempty"`
	URL    string `json:"url,omitempty"`
	Reason string `json:"moderation_reason,omitempty"`
}

type publishStatusOutput struct {
	// Connected false: the writer has not connected an account. Registered
	// false: the work is not on the site yet. Offline: the site could not be
	// reached and the states are the last ones seen.
	Connected  bool   `json:"connected"`
	Registered bool   `json:"registered"`
	Offline    bool   `json:"offline,omitempty"`
	WorkURL    string `json:"work_url,omitempty"`
	// Episodes are the work's episodes in outline order.
	Episodes []publishedEpisode `json:"episodes"`
	// SiteOnly are episodes still on the site whose node was deleted here.
	SiteOnly []publishedEpisode `json:"site_only,omitempty"`
}

// ---------- linetta_publish_episode ----------

type publishEpisodeInput struct {
	NodeID    string `json:"node_id" jsonschema:"the episode or a scene inside it"`
	ProjectID string `json:"project_id,omitempty"`
}

func (in publishEpisodeInput) scope() (string, string) { return in.ProjectID, in.NodeID }

// publishEpisodeOutput.Created is false when the episode's text on the site
// was replaced rather than published for the first time.
type publishEpisodeOutput struct {
	NodeID  string `json:"node_id"`
	Created bool   `json:"created"`
	Number  int    `json:"number"`
	URL     string `json:"url"`
	State   string `json:"state"`
}

func (d ToolDeps) registerPublishReadTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_publish_status",
		Description: "Each episode's state on Linetta Works, the writer's reading site, and its address. " +
			"modified means edited since it was published.",
	}, record(d, "linetta_publish_status", d.publishStatus))
}

func (d ToolDeps) registerPublishWriteTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_publish_episode",
		Description: "Publish one episode to Linetta Works for anyone to read, or update its text there. Only " +
			"when the writer asks. Needs the account connected and the work registered in the app first; " +
			"you cannot do either.",
	}, record(d, "linetta_publish_episode", d.publishEpisode))
}

func (d ToolDeps) publishStatus(ctx context.Context, _ *mcp.CallToolRequest, in publishStatusInput) (*mcp.CallToolResult, publishStatusOutput, error) {
	if d.Publish == nil {
		return toolErr("publishing is not available in this build"), publishStatusOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, publishStatusOutput{}, nil
	}
	ws, err := d.Publish.WorkStatus(ctx, p.ID)
	if err != nil {
		return toolErr("%s", publishFailure(err)), publishStatusOutput{}, nil
	}
	out := publishStatusOutput{
		Connected: ws.Connected, Registered: ws.Registered, Offline: ws.Offline,
		Episodes: make([]publishedEpisode, 0, len(ws.Episodes)),
	}
	if ws.Work != nil {
		out.WorkURL = ws.Work.URL
	}
	for _, ep := range ws.Episodes {
		out.Episodes = append(out.Episodes, toPublishedEpisode(ep))
	}
	for _, ep := range ws.SiteOnly {
		out.SiteOnly = append(out.SiteOnly, toPublishedEpisode(ep))
	}
	return nil, out, nil
}

func toPublishedEpisode(ep publish.EpisodeView) publishedEpisode {
	return publishedEpisode{
		NodeID: ep.NodeID, Title: ep.Title, State: ep.State, Number: ep.Number,
		URL: ep.URL, Reason: ep.ModerationReason,
	}
}

func (d ToolDeps) publishEpisode(ctx context.Context, _ *mcp.CallToolRequest, in publishEpisodeInput) (*mcp.CallToolResult, publishEpisodeOutput, error) {
	if d.Publish == nil {
		return toolErr("publishing is not available in this build"), publishEpisodeOutput{}, nil
	}
	n, errResult := d.requireNodeInProject(ctx, in.NodeID, in.ProjectID)
	if errResult != nil {
		return errResult, publishEpisodeOutput{}, nil
	}
	res, err := d.Publish.PublishEpisode(ctx, n.ID)
	if err != nil {
		return toolErr("%s", publishFailure(err)), publishEpisodeOutput{}, nil
	}
	// The episode's nodes are now marked published; the open outline should
	// show it without the writer reloading.
	d.notifyChanged(n.ProjectID, "linetta_publish_episode", append([]string{res.Episode.NodeID}, res.Episode.LeafIDs...), "")
	return nil, publishEpisodeOutput{
		NodeID: res.Episode.NodeID, Created: res.Created, Number: res.Episode.Number,
		URL: res.Episode.URL, State: res.Episode.State,
	}, nil
}

// publishFailure says what went wrong in words an agent can relay. The
// refusals an agent must not try to work around say so.
func publishFailure(err error) string {
	var re *rpc.ReasonError
	if !errors.As(err, &re) {
		return "publishing failed: " + err.Error()
	}
	switch re.Reason {
	case rpc.ReasonPublishNotConnected:
		return "this device is not connected to Linetta Works. The writer connects their account in " +
			"Settings → Publishing; you cannot do it for them"
	case rpc.ReasonPublishUnauthorized:
		return "the writer disconnected this device on the Linetta Works site. They reconnect in " +
			"Settings → Publishing; you cannot do it for them"
	case rpc.ReasonPublishWorkNotRegistered:
		return "this work is not on Linetta Works yet. The writer registers it the first time they publish " +
			"from the app's publish dialog (it needs a blurb); you cannot do it for them"
	case rpc.ReasonPublishNotEnabled:
		return "publishing is not open for the writer's account yet"
	case rpc.ReasonPublishEmptyEpisode:
		return "the episode has no text to publish"
	case rpc.ReasonPublishNotEpisode:
		return "that node is not inside one episode; pass an episode or a scene within one"
	case rpc.ReasonPublishTooLarge:
		return "the episode is longer than the site accepts; the manuscript is unchanged"
	case rpc.ReasonPublishQuotaExceeded:
		return "the writer's account has reached its limit of works or this work's limit of episodes"
	case rpc.ReasonPublishRateLimited:
		return "the site is rate limiting; try again in a minute"
	case rpc.ReasonPublishUnreachable, rpc.ReasonPublishTimeout, rpc.ReasonPublishUnavailable:
		return "could not reach Linetta Works; nothing was published and the manuscript is unchanged — try again later"
	}
	return "publishing failed (" + strings.ReplaceAll(re.Reason, "_", " ") + ")"
}
