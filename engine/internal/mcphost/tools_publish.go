package mcphost

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/episode"
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

// PublishApprover puts a publish write in front of the writer and waits for
// their answer (#238): nil means approved. Publishing is the one thing an
// agent does that readers see and the app cannot undo, so a tool description
// saying "only when the writer asks" is not the guard; this is.
type PublishApprover interface {
	Ask(ctx context.Context, req publish.ApprovalRequest) error
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

// approvalNote follows "Only when the writer asks" in each publish write's
// description: the call waits on the writer (#238), and an agent that knows
// this tells them to look at the app. Short, because these descriptions ride
// along with every request (budget_test.go).
const approvalNote = "; they approve it in the app."

func (d ToolDeps) registerPublishWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_register_work",
		Description: "Register the work on Linetta Works with its blurb, or change its blurb or serial status " +
			"there. Only when the writer asks, with a blurb they wrote or approved" + approvalNote + " Title and " +
			"genres come from the work.",
	}, record(d, "linetta_register_work", d.registerWork))
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_publish_episode",
		Description: "Publish one episode to Linetta Works for anyone to read, or update its text there. Only " +
			"when the writer asks" + approvalNote + " Register the work first.",
	}, record(d, "linetta_publish_episode", d.publishEpisode))
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_publish_cover",
		Description: "Make a gallery picture the work's cover on Linetta Works and upload it, replacing any cover " +
			"there. Only when the writer asks" + approvalNote + " The work must be registered first. Sheets are not sent.",
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

func (d ToolDeps) registerWork(ctx context.Context, req *mcp.CallToolRequest, in registerWorkInput) (*mcp.CallToolResult, registerWorkOutput, error) {
	if d.Publish == nil {
		return toolErr("publishing is not available in this build"), registerWorkOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, registerWorkOutput{}, nil
	}
	ws, errResult := d.publishReady(ctx, p.ID, false)
	if errResult != nil {
		return errResult, registerWorkOutput{}, nil
	}
	blurb, serial := strings.TrimSpace(in.Blurb), strings.TrimSpace(in.SerialStatus)
	// The site wants the blurb on every update; a writer who only says "mark
	// it completed" should not need the agent to retype it.
	if ws.Work != nil {
		if blurb == "" {
			blurb = ws.Work.Blurb
		}
		if serial == "" {
			serial = ws.Work.SerialStatus
		}
	}
	if n := utf8.RuneCountInString(blurb); n > publish.MaxBlurbRunes {
		return toolErr("the blurb is %d characters; the site takes at most %d", n, publish.MaxBlurbRunes), registerWorkOutput{}, nil
	}
	if errResult := d.askToPublish(ctx, req, publish.ApprovalRequest{
		Kind: publish.ApprovalRegisterWork, ProjectID: p.ID, WorkTitle: p.Title,
		Blurb: blurb, SerialStatus: serial, Republish: ws.Registered,
	}); errResult != nil {
		return errResult, registerWorkOutput{}, nil
	}
	w, err := d.Publish.UpsertWork(ctx, publish.UpsertInput{ProjectID: p.ID, Blurb: blurb, SerialStatus: serial})
	if err != nil {
		return toolErr("%s", publishFailure(err)), registerWorkOutput{}, nil
	}
	// An open publish dialog or outline badge reads the work's state.
	d.notifyChanged(p.ID, "linetta_register_work", nil, "")
	return nil, registerWorkOutput{WorkURL: w.URL, SerialStatus: w.SerialStatus}, nil
}

func (d ToolDeps) publishEpisode(ctx context.Context, req *mcp.CallToolRequest, in publishEpisodeInput) (*mcp.CallToolResult, publishEpisodeOutput, error) {
	if d.Publish == nil {
		return toolErr("publishing is not available in this build"), publishEpisodeOutput{}, nil
	}
	n, errResult := d.requireNodeInProject(ctx, in.NodeID, in.ProjectID)
	if errResult != nil {
		return errResult, publishEpisodeOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, n.ProjectID)
	if errResult != nil {
		return errResult, publishEpisodeOutput{}, nil
	}
	// The writer is asked about the episode, not the scene the agent passed.
	ep, err := episode.Resolve(ctx, d.Nodes, p.OutlinePreset, n.ID)
	if errors.Is(err, episode.ErrNotFound) {
		return toolErr("%s", publishFailure(&rpc.ReasonError{Reason: rpc.ReasonPublishNotEpisode, Err: err})), publishEpisodeOutput{}, nil
	}
	if err != nil {
		return toolErr("could not find the episode: %v", err), publishEpisodeOutput{}, nil
	}
	ws, errResult := d.publishReady(ctx, p.ID, true)
	if errResult != nil {
		return errResult, publishEpisodeOutput{}, nil
	}
	republish := false
	for _, e := range ws.Episodes {
		if e.NodeID == ep.NodeID && e.State != "" && e.State != "unpublished" {
			republish = true
		}
	}
	if errResult := d.askToPublish(ctx, req, publish.ApprovalRequest{
		Kind: publish.ApprovalPublishEpisode, ProjectID: p.ID, WorkTitle: p.Title,
		NodeID: ep.NodeID, EpisodeTitle: strings.TrimSpace(ep.Label + " " + ep.Title), Republish: republish,
	}); errResult != nil {
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

func (d ToolDeps) publishCover(ctx context.Context, req *mcp.CallToolRequest, in publishCoverInput) (*mcp.CallToolResult, publishedCover, error) {
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
	ws, errResult := d.publishReady(ctx, p.ID, true)
	if errResult != nil {
		return errResult, publishedCover{}, nil
	}
	if errResult := d.askToPublish(ctx, req, publish.ApprovalRequest{
		Kind: publish.ApprovalPublishCover, ProjectID: p.ID, WorkTitle: p.Title,
		IllustrationID: id, Republish: ws.Sheets.Cover != nil,
	}); errResult != nil {
		return errResult, publishedCover{}, nil
	}
	cover, err := d.Publish.PublishCover(ctx, p.ID, id)
	if err != nil {
		return toolErr("%s", publishFailure(err)), publishedCover{}, nil
	}
	d.notifyChanged(p.ID, "linetta_publish_cover", nil, "")
	return nil, toPublishedCover(cover), nil
}

// publishReady reads the work's state on the site and refuses what is bound
// to fail, so the writer is never asked to approve a publish that cannot
// happen. A site it could not read is left to the publish call itself.
func (d ToolDeps) publishReady(ctx context.Context, projectID string, needRegistered bool) (publish.WorkStatus, *mcp.CallToolResult) {
	ws, err := d.Publish.WorkStatus(ctx, projectID)
	if err != nil {
		return publish.WorkStatus{}, toolErr("%s", publishFailure(err))
	}
	if !ws.Connected {
		return ws, toolErr("%s", publishFailure(&rpc.ReasonError{Reason: rpc.ReasonPublishNotConnected}))
	}
	if needRegistered && !ws.Registered && !ws.Offline {
		return ws, toolErr("%s", publishFailure(&rpc.ReasonError{Reason: rpc.ReasonPublishWorkNotRegistered}))
	}
	return ws, nil
}

// askToPublish waits for the writer's answer in the app. It returns nil only
// when they approved; anything else is a refusal that says nothing was sent.
func (d ToolDeps) askToPublish(ctx context.Context, req *mcp.CallToolRequest, ar publish.ApprovalRequest) *mcp.CallToolResult {
	if d.PublishApproval == nil {
		return toolErr("publishing needs the writer's approval in the app, which this server cannot ask for; nothing was published")
	}
	ar.Source = d.Source
	if ar.Source == "" {
		ar.Source = SourceExternal
	}
	if ar.Source == SourceExternal {
		ar.Client = clientName(req)
	}
	err := d.PublishApproval.Ask(ctx, ar)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, publish.ErrApprovalDenied):
		return toolErr("the writer declined this in the app; nothing was published. Do not ask again unless they bring it up")
	case errors.Is(err, publish.ErrApprovalTimeout):
		return toolErr("the writer did not approve this in the app in time; nothing was published. Tell them it needs " +
			"their approval in Linetta and try again only when they ask")
	default:
		return toolErr("nothing was published: %v", err)
	}
}

// clientName is the name an external client gave itself when it connected,
// shown to the writer as a label. The client chose it, so it is cut short
// and never used to decide anything.
func clientName(req *mcp.CallToolRequest) string {
	if req == nil || req.Session == nil {
		return ""
	}
	ip := req.Session.InitializeParams()
	if ip == nil || ip.ClientInfo == nil {
		return ""
	}
	name := strings.TrimSpace(ip.ClientInfo.Name)
	if utf8.RuneCountInString(name) > 40 {
		name = string([]rune(name)[:40])
	}
	return name
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
