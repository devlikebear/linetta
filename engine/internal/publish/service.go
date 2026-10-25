package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// Episode states the outline and the publish dialog show. They combine the
// site's status with whether the local text still matches what was sent.
const (
	EpisodeUnpublished = "unpublished"
	EpisodePublic      = "public"
	EpisodeModified    = "modified" // public, but the text has changed since
	EpisodePending     = "pending"
	EpisodeRejected    = "rejected"
	EpisodeNeedsReview = "needs_review"
	EpisodePrivate     = "private"
	EpisodeSiteOnly    = "site_only" // on the site, but its node is gone here
)

// Limits the site enforces on a work (docs/api.md). Checked here so a
// writer's long blurb or a project's eleventh genre is refused or trimmed
// before anything is sent, instead of bouncing as invalid_request.
const (
	// MaxBlurbRunes is exported so the agent's tool can say why it refused.
	MaxBlurbRunes = 2000
	maxGenres     = 10
	maxGenreRunes = 20
)

// EpisodeView is one episode as the app shows it.
type EpisodeView struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	// Title is what the site shows next to the number: the node title, or a
	// label that is more than a number.
	Title   string `json:"title"`
	Ordinal int    `json:"ordinal,omitempty"`
	// LeafIDs are the scenes the episode is made of, so the app can find
	// the episode a selected scene belongs to without its own copy of the
	// rules.
	LeafIDs   []string `json:"leaf_ids,omitempty"`
	CharCount int      `json:"char_count"`
	Empty     bool     `json:"empty,omitempty"`
	// Illustrations is how many pictures go up with the episode.
	Illustrations int    `json:"illustrations,omitempty"`
	State         string `json:"state"`
	// Modified is true when the local text differs from the site's,
	// whatever the state. "modified" is only the state of a public episode;
	// a private one with new text is still private.
	Modified         bool   `json:"modified,omitempty"`
	Number           int    `json:"number,omitempty"`
	URL              string `json:"url,omitempty"`
	RemoteStatus     string `json:"remote_status,omitempty"`
	ModerationReason string `json:"moderation_reason,omitempty"`
	PublishedAt      string `json:"published_at,omitempty"`
}

// WorkView is a work as the site has it.
type WorkView struct {
	URL          string `json:"url"`
	Slug         string `json:"slug"`
	Blurb        string `json:"blurb"`
	SerialStatus string `json:"serial_status"`
	Visibility   string `json:"visibility,omitempty"`
}

// WorkStatus is a work's publishing picture: its episodes here, each with
// what the site has, and the site's episodes that have no node here any more.
type WorkStatus struct {
	ProjectID  string `json:"project_id"`
	Connected  bool   `json:"connected"`
	Registered bool   `json:"registered"`
	// Offline means the site could not be reached and the remote half comes
	// from the copy saved at LastSyncedAt.
	Offline      bool          `json:"offline,omitempty"`
	LastSyncedAt int64         `json:"last_synced_at,omitempty"`
	Work         *WorkView     `json:"work,omitempty"`
	Episodes     []EpisodeView `json:"episodes"`
	SiteOnly     []EpisodeView `json:"site_only"`
	// Sheets is the sheets and cover: the writer's choices always, the
	// site's copy when it answered.
	Sheets SheetsStatus `json:"sheets"`
}

// PublishResult is the outcome of publishing one episode.
type PublishResult struct {
	Created bool        `json:"created"`
	Episode EpisodeView `json:"episode"`
}

// UpsertInput is what the writer sets about a work on the site.
type UpsertInput struct {
	ProjectID    string `json:"project_id"`
	Blurb        string `json:"blurb"`
	SerialStatus string `json:"serial_status"`
	// Slug is the work's address on the site. Used only when the work is
	// created; empty lets the site derive one from the title.
	Slug string `json:"slug,omitempty"`
}

// Illustrations is the work's gallery as publishing needs it: which pictures
// a project has (published text shows only those) and their bytes (uploaded
// before the episode that shows them).
type Illustrations interface {
	IDsInProject(ctx context.Context, projectID string) (map[string]bool, error)
	IllustrationImage(ctx context.Context, id string) (mime string, data []byte, err error)
}

// Service publishes a library's works. Every call that reaches the site
// first checks for a stored connection, so a writer who never connected sends
// nothing anywhere.
type Service struct {
	client    *Client
	creds     Credentials
	connector *Connector
	state     *StateRepo
	projects  *project.Repo
	nodes     *node.Repo
	language  func(context.Context) string
	clock     func() int64
	// illustrations is nil until WithIllustrations: published text then
	// shows no pictures.
	illustrations Illustrations
	// sheets is nil until WithSheets: the work then publishes no sheets.
	sheets SheetSource

	// mu serialises calls that change the site. Two publishes of one work
	// racing would each compute an episode order from a list the other is
	// about to change.
	mu sync.Mutex
}

// NewService wires a Service. language is the app's UI language, which the
// site records as the work's language.
func NewService(client *Client, creds Credentials, connector *Connector, state *StateRepo,
	projects *project.Repo, nodes *node.Repo, language func(context.Context) string, clock func() int64) *Service {
	return &Service{
		client: client, creds: creds, connector: connector, state: state,
		projects: projects, nodes: nodes, language: language, clock: clock,
	}
}

// WithIllustrations lets published episodes carry the pictures in their
// scenes.
func (s *Service) WithIllustrations(src Illustrations) *Service {
	s.illustrations = src
	return s
}

// token returns the stored token, or the reason there is none.
func (s *Service) token() (string, error) {
	if _, ok := s.creds.Account(); !ok {
		return "", reason(rpc.ReasonPublishNotConnected, errors.New("publish: this device is not connected"))
	}
	tok := s.creds.Token()
	if tok == "" {
		return "", reason(rpc.ReasonPublishNotConnected, errors.New("publish: no token is stored"))
	}
	return tok, nil
}

// siteError classifies err, forgetting the token when the site refused it.
func (s *Service) siteError(err error) error {
	if isUnauthorized(err) {
		s.connector.Revoked()
	}
	return classify(err)
}

// localEpisode is an episode with its text rendered for the site.
type localEpisode struct {
	episode.Episode
	payload export.EpisodePayload
}

func (s *Service) localEpisodes(ctx context.Context, p project.Project) ([]localEpisode, error) {
	list, err := episode.List(ctx, s.nodes, p.ID, p.OutlinePreset)
	if err != nil {
		return nil, err
	}
	var known map[string]bool
	if s.illustrations != nil {
		if known, err = s.illustrations.IDsInProject(ctx, p.ID); err != nil {
			return nil, err
		}
	}
	out := make([]localEpisode, 0, len(list))
	for _, ep := range list {
		payload, err := export.ExportEpisodeMarkdown(ctx, s.nodes, ep, p.Title, known)
		if err != nil {
			return nil, err
		}
		out = append(out, localEpisode{Episode: ep, payload: payload})
	}
	return out, nil
}

func (s *Service) project(ctx context.Context, projectID string) (project.Project, error) {
	p, err := s.projects.Get(ctx, projectID)
	if errors.Is(err, project.ErrNotFound) {
		return project.Project{}, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
	}
	return p, err
}

// WorkStatus merges the work's episodes with what the site has. It asks the
// site once; when the site cannot be reached it answers from the saved copy
// and says so. Not connected, it asks nothing.
func (s *Service) WorkStatus(ctx context.Context, projectID string) (WorkStatus, error) {
	p, err := s.project(ctx, projectID)
	if err != nil {
		return WorkStatus{}, err
	}
	local, err := s.localEpisodes(ctx, p)
	if err != nil {
		return WorkStatus{}, err
	}
	out := WorkStatus{ProjectID: projectID, Episodes: []EpisodeView{}, SiteOnly: []EpisodeView{}}
	if out.Sheets, err = s.localSheetsStatus(ctx, projectID); err != nil {
		return WorkStatus{}, err
	}
	tok, err := s.token()
	if err != nil {
		for _, ep := range local {
			out.Episodes = append(out.Episodes, merge(ep, nil))
		}
		return out, nil
	}
	out.Connected = true

	work, remote, err := s.client.GetWork(ctx, tok, projectID)
	switch {
	case err == nil:
		w := s.workState(projectID, work)
		if err := s.state.Sync(ctx, w, episodeStates(projectID, remote)); err != nil {
			return WorkStatus{}, err
		}
		out.Registered, out.LastSyncedAt = true, w.LastSyncedAt
		out.Work = workView(w, work.Visibility)
		// The sheets are a second request; without them the episodes are
		// still worth showing.
		if err := s.remoteSheetsStatus(ctx, tok, projectID, work, &out.Sheets); err != nil {
			log.Printf("publish: read sheets: %v", err)
		}
	case isNotFound(err):
		// Not on the site (or deleted there): nothing saved about it is true.
		if err := s.state.Forget(ctx, projectID); err != nil {
			return WorkStatus{}, err
		}
		remote = nil
	case isOffline(err):
		saved, ok, serr := s.state.Work(ctx, projectID)
		if serr != nil {
			return WorkStatus{}, serr
		}
		out.Offline = true
		if ok && saved.LastSyncedAt > 0 {
			out.Registered, out.LastSyncedAt = true, saved.LastSyncedAt
			out.Work = workView(saved, "")
		}
		states, serr := s.state.Episodes(ctx, projectID)
		if serr != nil {
			return WorkStatus{}, serr
		}
		remote = remoteFromStates(states)
	default:
		return WorkStatus{}, s.siteError(err)
	}

	byNode := map[string]RemoteEpisode{}
	for _, r := range remote {
		byNode[r.NodeID] = r
	}
	seen := map[string]bool{}
	for _, ep := range local {
		var r *RemoteEpisode
		if found, ok := byNode[ep.NodeID]; ok {
			r = &found
		}
		seen[ep.NodeID] = true
		out.Episodes = append(out.Episodes, merge(ep, r))
	}
	for _, r := range remote {
		if seen[r.NodeID] {
			continue
		}
		out.SiteOnly = append(out.SiteOnly, siteOnly(r))
	}
	return out, nil
}

func (s *Service) workState(projectID string, w Work) WorkState {
	return WorkState{
		ProjectID: projectID, RemoteURL: w.URL, Slug: w.Slug, Blurb: w.Blurb,
		SerialStatus: w.SerialStatus, LastSyncedAt: s.clock(),
	}
}

func workView(w WorkState, visibility string) *WorkView {
	return &WorkView{URL: w.RemoteURL, Slug: w.Slug, Blurb: w.Blurb, SerialStatus: w.SerialStatus, Visibility: visibility}
}

func episodeStates(projectID string, remote []RemoteEpisode) []EpisodeState {
	out := make([]EpisodeState, 0, len(remote))
	for _, r := range remote {
		out = append(out, EpisodeState{
			NodeID: r.NodeID, ProjectID: projectID, RemoteURL: r.URL, Number: r.Number, Title: r.Title,
			ContentHash: r.ContentHash, RemoteStatus: r.Status, ModerationReason: r.ModerationReason,
			PublishedAt: r.PublishedAt,
		})
	}
	return out
}

func remoteFromStates(states []EpisodeState) []RemoteEpisode {
	out := make([]RemoteEpisode, 0, len(states))
	for _, e := range states {
		out = append(out, RemoteEpisode{
			NodeID: e.NodeID, Number: e.Number, Title: e.Title, ContentHash: e.ContentHash, URL: e.RemoteURL,
			Status: e.RemoteStatus, ModerationReason: e.ModerationReason, PublishedAt: e.PublishedAt,
		})
	}
	return out
}

func merge(ep localEpisode, r *RemoteEpisode) EpisodeView {
	v := EpisodeView{
		NodeID: ep.NodeID, Label: ep.Label, Title: ep.payload.Title, Ordinal: ep.Ordinal, LeafIDs: ep.LeafIDs,
		CharCount: ep.payload.CharCount, Empty: ep.payload.Markdown == "", State: EpisodeUnpublished,
		Illustrations: len(ep.payload.Illustrations),
	}
	if r == nil {
		return v
	}
	v.Number, v.URL, v.RemoteStatus = r.Number, r.URL, r.Status
	v.ModerationReason, v.PublishedAt = r.ModerationReason, r.PublishedAt
	v.Modified = r.ContentHash != ep.payload.ContentHash
	v.State = remoteState(r.Status, v.Modified)
	return v
}

func remoteState(status string, modified bool) string {
	switch status {
	case RemotePending:
		return EpisodePending
	case RemoteRejected:
		return EpisodeRejected
	case RemoteNeedsReview:
		return EpisodeNeedsReview
	case RemotePrivate:
		return EpisodePrivate
	}
	if modified {
		return EpisodeModified
	}
	return EpisodePublic
}

func siteOnly(r RemoteEpisode) EpisodeView {
	return EpisodeView{
		NodeID: r.NodeID, Title: r.Title, CharCount: r.CharCount, State: EpisodeSiteOnly,
		Number: r.Number, URL: r.URL, RemoteStatus: r.Status, ModerationReason: r.ModerationReason,
		PublishedAt: r.PublishedAt,
	}
}

// UpsertWork registers the work on the site or updates what the site says
// about it. Title and genres come from the project, the language from the
// app's language; the writer supplies the blurb, the serial status and, the
// first time only, the address.
func (s *Service) UpsertWork(ctx context.Context, in UpsertInput) (WorkView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, err := s.token()
	if err != nil {
		return WorkView{}, err
	}
	p, err := s.project(ctx, in.ProjectID)
	if err != nil {
		return WorkView{}, err
	}
	blurb := strings.TrimSpace(in.Blurb)
	if blurb == "" {
		return WorkView{}, reason(rpc.ReasonPublishBlurbRequired, errors.New("publish: a blurb is required"))
	}
	if utf8.RuneCountInString(blurb) > MaxBlurbRunes {
		return WorkView{}, reason(rpc.ReasonPublishRejected, fmt.Errorf("publish: blurb is over %d characters", MaxBlurbRunes))
	}
	serial := in.SerialStatus
	if serial == "" {
		serial = SerialOngoing
	}
	if serial != SerialOngoing && serial != SerialCompleted {
		return WorkView{}, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "serial_status must be ongoing or completed"}
	}
	body := WorkInput{
		Title:        strings.TrimSpace(p.Title),
		Slug:         strings.TrimSpace(in.Slug),
		Blurb:        &blurb,
		Genres:       siteGenres(p.Genres),
		SerialStatus: serial,
		Language:     s.siteLanguage(ctx),
	}
	_, work, err := s.client.PutWork(ctx, tok, p.ID, body)
	if err != nil {
		return WorkView{}, s.siteError(err)
	}
	w := s.workState(p.ID, work)
	if err := s.state.PutWork(ctx, w); err != nil {
		return WorkView{}, err
	}
	return *workView(w, work.Visibility), nil
}

func (s *Service) siteLanguage(ctx context.Context) string {
	if s.language != nil {
		if lang := s.language(ctx); lang != "" {
			return lang
		}
	}
	return "ko"
}

func siteGenres(genres []string) []string {
	out := []string{}
	for _, g := range genres {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if r := []rune(g); len(r) > maxGenreRunes {
			g = string(r[:maxGenreRunes])
		}
		out = append(out, g)
		if len(out) == maxGenres {
			break
		}
	}
	return out
}

// PublishEpisode puts the episode holding nodeID on the site, or replaces its
// text there. The work must already be registered (UpsertWork): publishing
// does not quietly create a work with no blurb.
//
// Episode numbers follow the outline. When the outline order and the site's
// numbering disagree — an episode moved, one inserted before published ones —
// the site's episodes are renumbered first (episodes:reorder), so the number
// sent is free. Episodes that are only on the site keep their place relative
// to the episode before them.
//
// Nothing local changes unless the site accepted the episode: on any failure
// the manuscript and the saved state are as they were. On success the
// episode's nodes are marked published.
func (s *Service) PublishEpisode(ctx context.Context, nodeID string) (PublishResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, err := s.token()
	if err != nil {
		return PublishResult{}, err
	}
	n, err := s.nodes.Get(ctx, nodeID)
	if errors.Is(err, node.ErrNotFound) {
		return PublishResult{}, rpc.NotFound(rpc.ReasonNodeNotFound, "node not found")
	}
	if err != nil {
		return PublishResult{}, err
	}
	p, err := s.project(ctx, n.ProjectID)
	if err != nil {
		return PublishResult{}, err
	}
	local, err := s.localEpisodes(ctx, p)
	if err != nil {
		return PublishResult{}, err
	}
	target, err := episode.Resolve(ctx, s.nodes, p.OutlinePreset, nodeID)
	if errors.Is(err, episode.ErrNotFound) {
		return PublishResult{}, reason(rpc.ReasonPublishNotEpisode, fmt.Errorf("publish: node %s is not inside one episode", nodeID))
	}
	if err != nil {
		return PublishResult{}, err
	}
	var mine *localEpisode
	for i := range local {
		if local[i].NodeID == target.NodeID {
			mine = &local[i]
		}
	}
	if mine == nil {
		return PublishResult{}, reason(rpc.ReasonPublishNotEpisode, fmt.Errorf("publish: episode %s is not in the outline", target.NodeID))
	}
	if mine.payload.Markdown == "" {
		return PublishResult{}, reason(rpc.ReasonPublishEmptyEpisode, fmt.Errorf("publish: episode %s has no text", target.NodeID))
	}
	if n := len(mine.payload.Illustrations); n > MaxImagesPerEpisode {
		return PublishResult{}, reason(rpc.ReasonPublishTooManyImages,
			fmt.Errorf("publish: episode %s shows %d pictures; the site takes %d", target.NodeID, n, MaxImagesPerEpisode))
	}

	created, published, err := s.sendEpisode(ctx, tok, p.ID, local, *mine)
	if errors.Is(err, errNumberTaken) {
		// Someone else — another device on the same account — changed the
		// numbering between our read and our write. Read again, once.
		created, published, err = s.sendEpisode(ctx, tok, p.ID, local, *mine)
	}
	if err != nil {
		return PublishResult{}, err
	}

	// The site took it. Refresh the saved copy; if that read fails the
	// episode's own row is still worth keeping.
	if work, remote, gerr := s.client.GetWork(ctx, tok, p.ID); gerr == nil {
		if err := s.state.Sync(ctx, s.workState(p.ID, work), episodeStates(p.ID, remote)); err != nil {
			log.Printf("publish: save state: %v", err)
		}
	} else if err := s.state.PutEpisode(ctx, episodeStates(p.ID, []RemoteEpisode{published})[0]); err != nil {
		log.Printf("publish: save state: %v", err)
	}

	now := s.clock()
	for _, id := range append([]string{mine.NodeID}, mine.LeafIDs...) {
		if err := s.nodes.SetStatus(ctx, id, node.StatusPublished, now); err != nil && !errors.Is(err, node.ErrNotFound) {
			log.Printf("publish: mark %s published: %v", id, err)
		}
	}
	return PublishResult{Created: created, Episode: merge(*mine, &published)}, nil
}

var errNumberTaken = errors.New("publish: the episode number was taken")

// sendEpisode does one publish attempt against the site's current list.
func (s *Service) sendEpisode(ctx context.Context, tok, projectID string, local []localEpisode, mine localEpisode) (bool, RemoteEpisode, error) {
	_, remote, err := s.client.GetWork(ctx, tok, projectID)
	if isNotFound(err) {
		return false, RemoteEpisode{}, reason(rpc.ReasonPublishWorkNotRegistered, err)
	}
	if err != nil {
		return false, RemoteEpisode{}, s.siteError(err)
	}
	current := make([]string, 0, len(remote))
	exists := false
	for _, r := range remote {
		current = append(current, r.NodeID)
		exists = exists || r.NodeID == mine.NodeID
	}
	localOrder := make([]string, 0, len(local))
	for _, ep := range local {
		localOrder = append(localOrder, ep.NodeID)
	}
	desired := desiredOrder(current, localOrder, mine.NodeID)
	var held []RemoteImage
	if r, ok := find(remote, mine.NodeID); ok {
		held = r.Images
	}
	if err := s.uploadImages(ctx, tok, projectID, mine, held); err != nil {
		return false, RemoteEpisode{}, err
	}
	in := EpisodeInput{Title: mine.payload.Title, BodyMD: mine.payload.Markdown, ContentHash: mine.payload.ContentHash}

	if !exists {
		// A new episode goes at the end first — the only number certain to
		// be free — and moves into place with one reorder if it belongs
		// earlier.
		in.Number = len(current) + 1
		created, ep, err := s.client.PutEpisode(ctx, tok, projectID, mine.NodeID, in)
		if err != nil {
			return false, RemoteEpisode{}, s.episodeError(err)
		}
		if !slices.Equal(append(current, mine.NodeID), desired) {
			reordered, err := s.client.ReorderEpisodes(ctx, tok, projectID, desired)
			if err != nil {
				return false, RemoteEpisode{}, s.siteError(err)
			}
			if r, ok := find(reordered, mine.NodeID); ok {
				ep = r
			}
		}
		return created, ep, nil
	}

	if !slices.Equal(current, desired) {
		if _, err := s.client.ReorderEpisodes(ctx, tok, projectID, desired); err != nil {
			return false, RemoteEpisode{}, s.siteError(err)
		}
	}
	in.Number = slices.Index(desired, mine.NodeID) + 1
	created, ep, err := s.client.PutEpisode(ctx, tok, projectID, mine.NodeID, in)
	if err != nil {
		return false, RemoteEpisode{}, s.episodeError(err)
	}
	return created, ep, nil
}

// uploadImages sends the pictures the episode's text shows that the site
// does not hold yet. The site takes the episode only once every one is
// there, so a failure here stops the publish with nothing changed.
func (s *Service) uploadImages(ctx context.Context, tok, projectID string, mine localEpisode, held []RemoteImage) error {
	if len(mine.payload.Illustrations) == 0 {
		return nil
	}
	if s.illustrations == nil {
		return fmt.Errorf("publish: episode %s shows pictures but no gallery is wired", mine.NodeID)
	}
	have := map[string]string{}
	for _, img := range held {
		have[img.IllustrationID] = img.SHA256
	}
	for _, id := range mine.payload.Illustrations {
		mime, data, err := s.illustrations.IllustrationImage(ctx, id)
		if err != nil {
			return fmt.Errorf("publish: read picture %s: %w", id, err)
		}
		sum := sha256.Sum256(data)
		if have[id] == hex.EncodeToString(sum[:]) {
			continue
		}
		if _, _, err := s.client.PutEpisodeImage(ctx, tok, projectID, mine.NodeID, id, mime, data); err != nil {
			return s.imageError(err)
		}
	}
	return nil
}

// imageError classifies a failed picture upload. Too large and over quota
// are about the pictures, not the text, and say so.
func (s *Service) imageError(err error) error {
	switch {
	case IsCode(err, CodeTooLarge):
		return reason(rpc.ReasonPublishImageTooLarge, err)
	case IsCode(err, CodeQuotaExceeded):
		return reason(rpc.ReasonPublishTooManyImages, err)
	}
	return s.siteError(err)
}

func (s *Service) episodeError(err error) error {
	switch {
	case IsCode(err, CodeNumberTaken):
		return fmt.Errorf("%w: %v", errNumberTaken, classify(err))
	case isNotFound(err):
		return reason(rpc.ReasonPublishWorkNotRegistered, err)
	}
	return s.siteError(err)
}

func find(list []RemoteEpisode, nodeID string) (RemoteEpisode, bool) {
	for _, r := range list {
		if r.NodeID == nodeID {
			return r, true
		}
	}
	return RemoteEpisode{}, false
}

// desiredOrder is the order the site's episodes should be in once target is
// published: the episodes this library has, in outline order, with each
// site-only episode kept right after the episode it followed on the site
// (or first, if it led).
func desiredOrder(current, localOrder []string, target string) []string {
	inLocal := map[string]bool{}
	for _, id := range localOrder {
		inLocal[id] = true
	}
	onSite := map[string]bool{target: true}
	for _, id := range current {
		onSite[id] = true
	}
	after := map[string][]string{}
	anchor := ""
	for _, id := range current {
		if inLocal[id] {
			anchor = id
			continue
		}
		after[anchor] = append(after[anchor], id)
	}
	out := append([]string{}, after[""]...)
	for _, id := range localOrder {
		if !onSite[id] {
			continue
		}
		out = append(out, id)
		out = append(out, after[id]...)
	}
	return out
}

// SetEpisodeVisibility makes an episode public or private on the site. It
// takes the project, not just the node, because a site-only episode has no
// node here to find the project by.
func (s *Service) SetEpisodeVisibility(ctx context.Context, projectID, nodeID, visibility string) (EpisodeView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if visibility != VisibilityPublic && visibility != VisibilityPrivate {
		return EpisodeView{}, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "visibility must be public or private"}
	}
	tok, err := s.token()
	if err != nil {
		return EpisodeView{}, err
	}
	r, err := s.client.SetEpisodeVisibility(ctx, tok, projectID, nodeID, visibility)
	if isNotFound(err) {
		return EpisodeView{}, reason(rpc.ReasonPublishEpisodeNotFound, err)
	}
	if err != nil {
		return EpisodeView{}, s.siteError(err)
	}
	if err := s.state.PutEpisode(ctx, episodeStates(projectID, []RemoteEpisode{r})[0]); err != nil {
		log.Printf("publish: save state: %v", err)
	}
	return EpisodeView{
		NodeID: r.NodeID, Title: r.Title, CharCount: r.CharCount, State: remoteState(r.Status, false),
		Number: r.Number, URL: r.URL, RemoteStatus: r.Status, ModerationReason: r.ModerationReason,
		PublishedAt: r.PublishedAt,
	}, nil
}
