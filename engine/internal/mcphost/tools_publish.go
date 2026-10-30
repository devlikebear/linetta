//go:build !mobile

package mcphost

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// Publisher is the slice of internal/publish the publish tools use. An agent
// can look at what is on Linetta Works, register a work, publish its episodes
// and put up its cover. Connecting the account is the writer's alone — it is approved in
// their browser — so there is no tool for it.
type Publisher interface {
	WorkStatus(ctx context.Context, projectID string) (publish.WorkStatus, error)
	UpsertWork(ctx context.Context, in publish.UpsertInput) (publish.WorkView, error)
	PublishEpisode(ctx context.Context, nodeID string) (publish.PublishResult, error)
	PublishCover(ctx context.Context, projectID, illustrationID string) (publish.CoverView, error)
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

// publishedCover is the cover the site holds. State is pending, public,
// rejected or needs_review.
type publishedCover struct {
	IllustrationID string `json:"illustration_id"`
	State          string `json:"state"`
	Reason         string `json:"moderation_reason,omitempty"`
}

type publishStatusOutput struct {
	// Connected false: the writer has not connected an account. Registered
	// false: the work is not on the site yet. Offline: the site could not be
	// reached and the states are the last ones seen.
	Connected  bool   `json:"connected"`
	Registered bool   `json:"registered"`
	Offline    bool   `json:"offline,omitempty"`
	WorkURL    string `json:"work_url,omitempty"`
	// Cover is absent while the site has none.
	Cover *publishedCover `json:"cover,omitempty"`
	// Episodes are the work's episodes in outline order.
	Episodes []publishedEpisode `json:"episodes"`
	// SiteOnly are episodes still on the site whose node was deleted here.
	SiteOnly []publishedEpisode `json:"site_only,omitempty"`
}

// ---------- linetta_register_work ----------

type registerWorkInput struct {
	ProjectID string `json:"project_id,omitempty"`
	Blurb     string `json:"blurb,omitempty" jsonschema:"the introduction readers see; required the first time, empty keeps the current one"`
	// SerialStatus is ongoing or completed; empty keeps it (ongoing for a new work).
	SerialStatus string `json:"serial_status,omitempty"`
}

func (in registerWorkInput) scope() (string, string) { return in.ProjectID, "" }

type registerWorkOutput struct {
	WorkURL      string `json:"work_url"`
	SerialStatus string `json:"serial_status"`
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

// ---------- linetta_publish_cover ----------

type publishCoverInput struct {
	ProjectID      string `json:"project_id,omitempty"`
	IllustrationID string `json:"illustration_id" jsonschema:"a picture in the gallery, from linetta_list_illustrations"`
}

func (in publishCoverInput) scope() (string, string) { return in.ProjectID, "" }

func (d ToolDeps) registerPublishReadTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_publish_status",
		Description: "Each episode's state on Linetta Works, the writer's reading site, and its address. " +
			"modified means edited since it was published.",
	}, record(d, "linetta_publish_status", d.publishStatus))
}

func (d ToolDeps) registerPublishWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_register_work",
		Description: "Register the work on Linetta Works with its blurb, or change its blurb or serial status " +
			"there. Only when the writer asks, with a blurb they wrote or approved. Title and genres come " +
			"from the work.",
	}, record(d, "linetta_register_work", d.registerWork))
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_publish_episode",
		Description: "Publish one episode to Linetta Works for anyone to read, or update its text there. Only " +
			"when the writer asks. The work must be registered first (linetta_register_work).",
	}, record(d, "linetta_publish_episode", d.publishEpisode))
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_publish_cover",
		Description: "Make a gallery picture the work's cover on Linetta Works and upload it, replacing any cover " +
			"there. Only when the writer asks. The work must be registered first. Sheets are not sent.",
	}, record(d, "linetta_publish_cover", d.publishCover))
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
	if c := ws.Sheets.Cover; c != nil {
		cover := toPublishedCover(*c)
		out.Cover = &cover
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

func toPublishedCover(c publish.CoverView) publishedCover {
	return publishedCover{IllustrationID: c.IllustrationID, State: c.State, Reason: c.Reason}
}

func (d ToolDeps) registerWork(ctx context.Context, _ *mcp.CallToolRequest, in registerWorkInput) (*mcp.CallToolResult, registerWorkOutput, error) {
	if d.Publish == nil {
		return toolErr("publishing is not available in this build"), registerWorkOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, registerWorkOutput{}, nil
	}
	blurb, serial := strings.TrimSpace(in.Blurb), strings.TrimSpace(in.SerialStatus)
	// The site wants the blurb on every update; a writer who only says "mark
	// it completed" should not need the agent to retype it.
	if blurb == "" || serial == "" {
		ws, err := d.Publish.WorkStatus(ctx, p.ID)
		if err != nil {
			return toolErr("%s", publishFailure(err)), registerWorkOutput{}, nil
		}
		if ws.Work != nil {
			if blurb == "" {
				blurb = ws.Work.Blurb
			}
			if serial == "" {
				serial = ws.Work.SerialStatus
			}
		}
	}
	if n := utf8.RuneCountInString(blurb); n > publish.MaxBlurbRunes {
		return toolErr("the blurb is %d characters; the site takes at most %d", n, publish.MaxBlurbRunes), registerWorkOutput{}, nil
	}
	w, err := d.Publish.UpsertWork(ctx, publish.UpsertInput{ProjectID: p.ID, Blurb: blurb, SerialStatus: serial})
	if err != nil {
		return toolErr("%s", publishFailure(err)), registerWorkOutput{}, nil
	}
	// An open publish dialog or outline badge reads the work's state.
	d.notifyChanged(p.ID, "linetta_register_work", nil, "")
	return nil, registerWorkOutput{WorkURL: w.URL, SerialStatus: w.SerialStatus}, nil
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

func (d ToolDeps) publishCover(ctx context.Context, _ *mcp.CallToolRequest, in publishCoverInput) (*mcp.CallToolResult, publishedCover, error) {
	if d.Publish == nil {
		return toolErr("publishing is not available in this build"), publishedCover{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, publishedCover{}, nil
	}
	id := strings.TrimSpace(in.IllustrationID)
	if id == "" {
		return toolErr("illustration_id is required; choose a picture from linetta_list_illustrations"), publishedCover{}, nil
	}
	cover, err := d.Publish.PublishCover(ctx, p.ID, id)
	if err != nil {
		return toolErr("%s", publishFailure(err)), publishedCover{}, nil
	}
	d.notifyChanged(p.ID, "linetta_publish_cover", nil, "")
	return nil, toPublishedCover(cover), nil
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
		return "this work is not on Linetta Works yet. Register it with linetta_register_work first; it " +
			"needs a blurb the writer wrote or approved"
	case rpc.ReasonPublishBlurbRequired:
		return "a blurb is required to register the work; ask the writer for one or offer a draft for them to approve"
	case rpc.ReasonPublishSlugTaken, rpc.ReasonPublishInvalidSlug:
		return "the site could not give the work an address from its title; the writer can choose one in the app's publish dialog"
	case rpc.ReasonPublishNotEnabled:
		return "publishing is not open for the writer's account yet"
	case rpc.ReasonPublishEmptyEpisode:
		return "the episode has no text to publish"
	case rpc.ReasonPublishNotEpisode:
		return "that node is not inside one episode; pass an episode or a scene within one"
	case rpc.ReasonPublishTooLarge:
		return "the episode is longer than the site accepts; the manuscript is unchanged"
	case rpc.ReasonPublishImageTooLarge:
		return "the picture is larger than the site accepts; choose another one"
	case rpc.ReasonPublishQuotaExceeded:
		return "the writer's account has reached its limit of works or this work's limit of episodes"
	case rpc.ReasonPublishRateLimited:
		return "the site is rate limiting; try again in a minute"
	case rpc.ReasonPublishUnreachable, rpc.ReasonPublishTimeout, rpc.ReasonPublishUnavailable:
		return "could not reach Linetta Works; nothing was published and the manuscript is unchanged — try again later"
	}
	return "publishing failed (" + strings.ReplaceAll(re.Reason, "_", " ") + ")"
}
