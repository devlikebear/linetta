package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// Sheets and the cover (works Phase 8). The writer turns publishing on for
// each sheet and picks a gallery picture as the cover; SyncSheets then makes
// the site hold exactly that. Relationship notes and visual notes never leave
// the device, and a sheet the writer did not turn on is never read for
// sending.

// Limits the site enforces on sheets (docs/api.md). Longer text is trimmed
// here, as genres are, instead of bouncing the whole set as invalid_request.
const (
	MaxSheets         = 200
	maxSheetRelations = 500
	maxSheetNameRunes = 100
	maxAliases        = 20
	maxAliasRunes     = 50
	maxRoleRunes      = 50
	maxSummaryRunes   = 5000
	maxAttributes     = 30
	maxAttrKeyRunes   = 50
	maxAttrValueRunes = 1000
	maxRelationRunes  = 50
	maxPictureBytes   = 5 * 1024 * 1024
)

// LocalSheet is a sheet as the writer has it.
type LocalSheet struct {
	ID         string
	Kind       string
	Name       string
	Aliases    []string
	Role       string
	Summary    string
	Attributes map[string]string
}

// LocalRelation is a relationship between two sheets. Its notes are not here
// on purpose: they are the writer's.
type LocalRelation struct {
	From  string
	To    string
	Label string
}

// SheetSource is where publishing reads a work's sheets from.
type SheetSource interface {
	// ProjectSheets returns every sheet of the project in a stable order and
	// the relationships among them. Publishing keeps the chosen ones.
	ProjectSheets(ctx context.Context, projectID string) ([]LocalSheet, []LocalRelation, error)
	// Portrait is the sheet's first reference picture; ok is false if it has none.
	Portrait(ctx context.Context, entityID string) (mime string, data []byte, ok bool, err error)
	// SheetProject is the project a sheet belongs to.
	SheetProject(ctx context.Context, entityID string) (string, error)
}

// ErrSheetNotFound is returned for a sheet id the source does not know.
var ErrSheetNotFound = errors.New("publish: sheet not found")

// SheetView is one sheet as the site has it.
type SheetView struct {
	EntityID string `json:"entity_id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	State    string `json:"state"`
	Reason   string `json:"moderation_reason,omitempty"`
}

// CoverView is the cover as the site has it.
type CoverView struct {
	IllustrationID string `json:"illustration_id"`
	State          string `json:"state"`
	Reason         string `json:"moderation_reason,omitempty"`
}

// SheetsStatus is what the publish dialog shows about sheets and the cover:
// the writer's choices here and, when the site answered, what it holds.
type SheetsStatus struct {
	Chosen  int    `json:"chosen"`
	CoverID string `json:"cover_id,omitempty"`
	// Remote is false when the site's half below is unknown (not
	// registered, offline).
	Remote bool        `json:"remote"`
	Sheets []SheetView `json:"sheets"`
	Cover  *CoverView  `json:"cover,omitempty"`
}

// WithSheets lets the work carry the writer's sheets.
func (s *Service) WithSheets(src SheetSource) *Service {
	s.sheets = src
	return s
}

// moderationState maps the site's moderation to the states the app shows.
func moderationState(moderation string) string {
	switch moderation {
	case "passed":
		return EpisodePublic
	case "rejected":
		return EpisodeRejected
	case "needs_review":
		return EpisodeNeedsReview
	}
	return EpisodePending
}

func sheetViews(remote []RemoteSheet) []SheetView {
	out := make([]SheetView, 0, len(remote))
	for _, r := range remote {
		out = append(out, SheetView{EntityID: r.EntityID, Kind: r.Kind, Name: r.Name, State: moderationState(r.Moderation), Reason: r.ModerationReason})
	}
	return out
}

func coverView(c *RemoteCover) *CoverView {
	if c == nil {
		return nil
	}
	return &CoverView{IllustrationID: c.IllustrationID, State: moderationState(c.Moderation), Reason: c.ModerationReason}
}

// localSheetsStatus is the writer's half of SheetsStatus.
func (s *Service) localSheetsStatus(ctx context.Context, projectID string) (SheetsStatus, error) {
	out := SheetsStatus{Sheets: []SheetView{}}
	chosen, err := s.state.PublishedSheets(ctx, projectID)
	if err != nil {
		return out, err
	}
	out.Chosen = len(chosen)
	if out.CoverID, _, err = s.state.Cover(ctx, projectID); err != nil {
		return out, err
	}
	return out, nil
}

// SheetPublished reports whether the writer turned on publishing for a sheet.
func (s *Service) SheetPublished(ctx context.Context, entityID string) (bool, error) {
	return s.state.SheetPublished(ctx, entityID)
}

// SetSheetPublished turns publishing for a sheet on or off here. Nothing
// reaches the site until SyncSheets.
func (s *Service) SetSheetPublished(ctx context.Context, entityID string, on bool) error {
	if s.sheets == nil {
		return errors.New("publish: no sheet source is wired")
	}
	projectID, err := s.sheets.SheetProject(ctx, entityID)
	if errors.Is(err, ErrSheetNotFound) {
		return rpc.NotFound(rpc.ReasonEntityNotFound, "entity not found")
	}
	if err != nil {
		return err
	}
	return s.state.SetSheetPublished(ctx, projectID, entityID, on, s.clock())
}

// SetCover picks a gallery picture of the project as the cover; an empty id
// clears it. Nothing reaches the site until SyncSheets.
func (s *Service) SetCover(ctx context.Context, projectID, illustrationID string) error {
	if _, err := s.project(ctx, projectID); err != nil {
		return err
	}
	if illustrationID != "" {
		if s.illustrations == nil {
			return errors.New("publish: no gallery is wired")
		}
		known, err := s.illustrations.IDsInProject(ctx, projectID)
		if err != nil {
			return err
		}
		if !known[illustrationID] {
			return rpc.NotFound(rpc.ReasonIllustrationNotFound, "illustration not found")
		}
	}
	return s.state.SetCover(ctx, projectID, illustrationID, s.clock())
}

// remoteSheetsStatus fills the site's half for a registered work.
func (s *Service) remoteSheetsStatus(ctx context.Context, tok, projectID string, work Work, out *SheetsStatus) error {
	remote, _, err := s.client.GetSheets(ctx, tok, projectID)
	if err != nil {
		return err
	}
	out.Remote = true
	out.Sheets = sheetViews(remote)
	out.Cover = coverView(work.Cover)
	return nil
}

// SyncSheets makes the site hold the sheets the writer turned on, with their
// relationships and portraits, and the chosen cover: portraits first (only the
// ones the site does not have), then the whole sheet set, then the cover. A
// sheet turned off disappears from the site; no cover chosen takes the
// site's cover down. The work must be registered.
func (s *Service) SyncSheets(ctx context.Context, projectID string) (SheetsStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, err := s.token()
	if err != nil {
		return SheetsStatus{}, err
	}
	if _, err := s.project(ctx, projectID); err != nil {
		return SheetsStatus{}, err
	}
	if s.sheets == nil {
		return SheetsStatus{}, errors.New("publish: no sheet source is wired")
	}
	work, _, err := s.client.GetWork(ctx, tok, projectID)
	if isNotFound(err) {
		return SheetsStatus{}, reason(rpc.ReasonPublishWorkNotRegistered, err)
	}
	if err != nil {
		return SheetsStatus{}, s.siteError(err)
	}
	out, err := s.localSheetsStatus(ctx, projectID)
	if err != nil {
		return SheetsStatus{}, err
	}

	inputs, relations, err := s.sheetInputs(ctx, projectID)
	if err != nil {
		return SheetsStatus{}, err
	}
	held, _, err := s.client.GetSheets(ctx, tok, projectID)
	if err != nil {
		return SheetsStatus{}, s.siteError(err)
	}
	have := map[string]string{}
	for _, r := range held {
		have[r.EntityID] = r.PortraitSHA256
	}
	for i := range inputs {
		mime, data, ok, err := s.sheets.Portrait(ctx, inputs[i].EntityID)
		if err != nil {
			return SheetsStatus{}, fmt.Errorf("publish: read portrait of %s: %w", inputs[i].EntityID, err)
		}
		if !ok || !sitePicture(mime, data) {
			continue
		}
		sum := sha256.Sum256(data)
		sha := hex.EncodeToString(sum[:])
		if have[inputs[i].EntityID] != sha {
			if err := s.client.PutSheetPortrait(ctx, tok, projectID, inputs[i].EntityID, mime, data); err != nil {
				return SheetsStatus{}, s.imageError(err)
			}
		}
		inputs[i].PortraitSHA256 = sha
	}
	remote, err := s.client.PutSheets(ctx, tok, projectID, inputs, relations)
	if err != nil {
		return SheetsStatus{}, s.siteError(err)
	}
	out.Remote = true
	out.Sheets = sheetViews(remote)

	cover, err := s.syncCover(ctx, tok, projectID, out.CoverID, work.Cover)
	if err != nil {
		return SheetsStatus{}, err
	}
	out.Cover = coverView(cover)
	return out, nil
}

// sheetInputs builds the request for the sheets the writer turned on.
func (s *Service) sheetInputs(ctx context.Context, projectID string) ([]SheetInput, []SheetRelation, error) {
	chosen, err := s.state.PublishedSheets(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	all, rels, err := s.sheets.ProjectSheets(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	inputs := []SheetInput{}
	for _, sh := range all {
		if !chosen[sh.ID] {
			continue
		}
		inputs = append(inputs, sheetInput(sh))
	}
	if len(inputs) > MaxSheets {
		return nil, nil, reason(rpc.ReasonPublishTooManySheets,
			fmt.Errorf("publish: %d sheets are turned on; the site takes %d", len(inputs), MaxSheets))
	}
	relations := []SheetRelation{}
	seen := map[SheetRelation]bool{}
	for _, r := range rels {
		label := trimRunes(strings.TrimSpace(r.Label), maxRelationRunes)
		rel := SheetRelation{From: r.From, To: r.To, Label: label}
		if !chosen[r.From] || !chosen[r.To] || r.From == r.To || label == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		relations = append(relations, rel)
		if len(relations) == maxSheetRelations {
			break
		}
	}
	return inputs, relations, nil
}

func sheetInput(sh LocalSheet) SheetInput {
	in := SheetInput{
		EntityID:   sh.ID,
		Kind:       sh.Kind,
		Name:       trimRunes(strings.TrimSpace(sh.Name), maxSheetNameRunes),
		Aliases:    []string{},
		Role:       trimRunes(strings.TrimSpace(sh.Role), maxRoleRunes),
		Summary:    trimRunes(strings.TrimSpace(sh.Summary), maxSummaryRunes),
		Attributes: []SheetAttribute{},
	}
	for _, a := range sh.Aliases {
		if a = trimRunes(strings.TrimSpace(a), maxAliasRunes); a != "" && len(in.Aliases) < maxAliases {
			in.Aliases = append(in.Aliases, a)
		}
	}
	// Attributes are a map here; the site keeps an order, so send them by key.
	keys := make([]string, 0, len(sh.Attributes))
	for k := range sh.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		key := trimRunes(strings.TrimSpace(k), maxAttrKeyRunes)
		value := trimRunes(strings.TrimSpace(sh.Attributes[k]), maxAttrValueRunes)
		if key == "" || value == "" || len(in.Attributes) == maxAttributes {
			continue
		}
		in.Attributes = append(in.Attributes, SheetAttribute{Key: key, Value: value})
	}
	return in
}

// syncCover makes the site's cover the chosen picture, or takes it down when
// none is chosen. A chosen picture the gallery no longer has counts as none.
func (s *Service) syncCover(ctx context.Context, tok, projectID, chosen string, held *RemoteCover) (*RemoteCover, error) {
	if chosen != "" && s.illustrations != nil {
		mime, data, err := s.illustrations.IllustrationImage(ctx, chosen)
		if err == nil && sitePicture(mime, data) {
			sum := sha256.Sum256(data)
			if held != nil && held.SHA256 == hex.EncodeToString(sum[:]) && held.IllustrationID == chosen {
				return held, nil
			}
			cover, err := s.client.PutCover(ctx, tok, projectID, chosen, mime, data)
			if err != nil {
				return nil, s.imageError(err)
			}
			return &cover, nil
		}
	}
	if held == nil {
		return nil, nil
	}
	if err := s.client.DeleteCover(ctx, tok, projectID); err != nil {
		return nil, s.siteError(err)
	}
	return nil, nil
}

// sitePicture reports whether the site takes the picture as it is.
func sitePicture(mime string, data []byte) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/webp":
		return len(data) > 0 && len(data) <= maxPictureBytes
	}
	return false
}

func trimRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
