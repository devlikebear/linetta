//go:build !mobile

package mcphost

import (
	"context"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/illustration"
	"github.com/devlikebear/linetta/engine/internal/visual"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- linetta_list_illustrations / linetta_request_illustration / linetta_set_representative_illustration ----------
//
// Illustration Phase 4. An agent cannot draw: drawing spends the writer's
// image provider quota, so linetta_request_illustration only leaves a request
// the writer approves (and may edit) on a card in the app. The agent learns
// what became of it from linetta_list_illustrations. Choosing a picture that
// is already in the gallery to stand for a scene or an episode costs nothing
// and is undoable, so that is a plain write.

type listIllustrationsInput struct {
	ProjectID string `json:"project_id" jsonschema:"id of the work"`
}

func (in listIllustrationsInput) scope() (string, string) { return in.ProjectID, "" }

// galleryPicture is deliberately lean: every field is schema the agent is
// sent on every turn (budget_test.go).
type galleryPicture struct {
	ID               string   `json:"id"`
	Caption          string   `json:"caption,omitempty"`
	SourceNodeID     string   `json:"source_node_id,omitempty"`
	UsedIn           []string `json:"used_in,omitempty"`
	RepresentativeOf []string `json:"representative_of,omitempty"`
}

type requestState struct {
	RequestID      string `json:"request_id"`
	NodeID         string `json:"node_id,omitempty"`
	Purpose        string `json:"purpose"`
	Status         string `json:"status"`
	IllustrationID string `json:"illustration_id,omitempty"`
	Error          string `json:"error,omitempty"`
}

type listIllustrationsOutput struct {
	Pictures []galleryPicture `json:"pictures"`
	// Requests are the newest first, at most listedRequests of them.
	Requests []requestState `json:"requests"`
}

// listedRequests bounds the requests one listing returns: the ones still
// waiting plus the latest decided, which is what an agent needs to know.
const listedRequests = 20

type requestIllustrationInput struct {
	ProjectID string   `json:"project_id" jsonschema:"id of the work"`
	NodeID    string   `json:"node_id,omitempty" jsonschema:"the scene, or for representative also an episode or part"`
	Purpose   string   `json:"purpose" jsonschema:"insert (end of the scene), representative (stands for the node) or gallery"`
	Scene     string   `json:"scene,omitempty" jsonschema:"what to draw; composed with the visual sheets and art style"`
	Prompt    string   `json:"prompt,omitempty" jsonschema:"a finished prompt instead of scene"`
	EntityIDs []string `json:"entity_ids,omitempty" jsonschema:"characters in the picture"`
	Names     []string `json:"names,omitempty" jsonschema:"or their names/aliases"`
	Aspect    string   `json:"aspect,omitempty" jsonschema:"square (default), portrait or landscape"`
	Caption   string   `json:"caption,omitempty"`
	Note      string   `json:"note,omitempty" jsonschema:"one line to the writer on the card"`
}

func (in requestIllustrationInput) scope() (string, string) { return in.ProjectID, in.NodeID }

type requestIllustrationOutput struct {
	RequestID string `json:"request_id"`
	// Status is waiting_for_writer: nothing is drawn until they approve.
	Status string `json:"status"`
	Prompt string `json:"prompt"`
}

type setRepresentativeInput struct {
	NodeID         string `json:"node_id" jsonschema:"a scene, an episode or a part"`
	IllustrationID string `json:"illustration_id,omitempty" jsonschema:"a gallery picture of the same work; empty clears"`
}

func (in setRepresentativeInput) scope() (string, string) { return "", in.NodeID }

type setRepresentativeOutput struct {
	NodeID         string `json:"node_id"`
	IllustrationID string `json:"illustration_id"`
	Previous       string `json:"previous"`
	UndoBatchID    string `json:"undo_batch_id"`
}

const requestWaiting = "waiting_for_writer"

func (d ToolDeps) registerIllustrationReadTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "linetta_list_illustrations", Description: "List the work's gallery pictures (scenes using them, nodes they represent) and recent illustration requests with their status."}, record(d, "linetta_list_illustrations", d.listIllustrations))
}

func (d ToolDeps) registerIllustrationWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "linetta_request_illustration", Description: "Ask the writer to draw a picture: leaves a request they approve or edit on a card in the app. Draws nothing itself; tell them to approve it."}, record(d, "linetta_request_illustration", d.requestIllustration))
	mcp.AddTool(s, &mcp.Tool{Name: "linetta_set_representative_illustration", Description: "Make a gallery picture stand for a scene, episode or part, or clear it. Returns undo_batch_id."}, record(d, "linetta_set_representative_illustration", d.setRepresentativeIllustration))
}

func (d ToolDeps) listIllustrations(ctx context.Context, _ *mcp.CallToolRequest, in listIllustrationsInput) (*mcp.CallToolResult, listIllustrationsOutput, error) {
	out := listIllustrationsOutput{Pictures: []galleryPicture{}, Requests: []requestState{}}
	if d.Gallery == nil || d.IllustrationRequests == nil {
		return toolErr("illustrations are unavailable in this build"), out, nil
	}
	p, refusal := d.requireProject(ctx, in.ProjectID)
	if refusal != nil {
		return refusal, out, nil
	}
	metas, err := d.Gallery.ListByProject(ctx, p.ID)
	if err != nil {
		return toolErr("could not read the gallery: %v", err), out, nil
	}
	usage, err := d.Gallery.UsageByProject(ctx, p.ID)
	if err != nil {
		return toolErr("could not read where pictures are used: %v", err), out, nil
	}
	reps, err := d.Gallery.Representatives(ctx, p.ID)
	if err != nil {
		return toolErr("could not read representative pictures: %v", err), out, nil
	}
	representing := map[string][]string{}
	for nodeID, id := range reps {
		representing[id] = append(representing[id], nodeID)
	}
	for _, m := range metas {
		row := galleryPicture{ID: m.ID, Caption: m.Caption, RepresentativeOf: representing[m.ID]}
		if m.SourceNodeID != nil {
			row.SourceNodeID = *m.SourceNodeID
		}
		for _, ref := range usage[m.ID] {
			row.UsedIn = append(row.UsedIn, ref.NodeID)
		}
		out.Pictures = append(out.Pictures, row)
	}
	requests, err := d.IllustrationRequests.List(ctx, p.ID, nil)
	if err != nil {
		return toolErr("could not read illustration requests: %v", err), out, nil
	}
	for _, r := range requests {
		if len(out.Requests) == listedRequests {
			break
		}
		out.Requests = append(out.Requests, requestState{
			RequestID: r.ID, NodeID: r.TargetNodeID, Purpose: r.Purpose, Status: r.Status,
			IllustrationID: r.IllustrationID, Error: r.Error,
		})
	}
	return nil, out, nil
}

func (d ToolDeps) requestIllustration(ctx context.Context, _ *mcp.CallToolRequest, in requestIllustrationInput) (*mcp.CallToolResult, requestIllustrationOutput, error) {
	out := requestIllustrationOutput{}
	if d.IllustrationRequests == nil || d.Visuals == nil {
		return toolErr("illustration requests are unavailable in this build"), out, nil
	}
	p, refusal := d.requireProject(ctx, in.ProjectID)
	if refusal != nil {
		return refusal, out, nil
	}
	if in.NodeID != "" {
		if _, refusal := d.requireNodeInProject(ctx, in.NodeID, p.ID); refusal != nil {
			return refusal, out, nil
		}
	}
	scene, prompt := strings.TrimSpace(in.Scene), strings.TrimSpace(in.Prompt)
	if (scene == "") == (prompt == "") {
		return toolErr("give either scene (to compose from the visual sheets) or prompt, not both"), out, nil
	}
	all, err := d.Entities.ListByProject(ctx, p.ID)
	if err != nil {
		return toolErr("could not read characters: %v", err), out, nil
	}
	var characterIDs []string
	if scene != "" {
		req := visual.ComposeInput{EntityIDs: in.EntityIDs, Names: in.Names, Scene: scene, Format: visual.FormatSingle}
		if err := req.Validate(); err != nil {
			return toolErr("%s", err.Error()), out, nil
		}
		c, err := d.Visuals.Compose(ctx, p.ID, all, req)
		if err != nil {
			return toolErr("%s", err.Error()), out, nil
		}
		prompt = c.Prompt
		for _, ch := range c.Characters {
			characterIDs = append(characterIDs, ch.EntityID)
		}
	} else if len(in.EntityIDs) > 0 || len(in.Names) > 0 {
		chars, err := visual.ResolveCharacters(all, in.EntityIDs, in.Names)
		if err != nil {
			return toolErr("%s", err.Error()), out, nil
		}
		for _, ch := range chars {
			characterIDs = append(characterIDs, ch.ID)
		}
	}
	r, err := d.IllustrationRequests.Submit(ctx, illustration.SubmitInput{
		ProjectID: p.ID, TargetNodeID: in.NodeID, Purpose: in.Purpose, Prompt: prompt,
		CharacterIDs: characterIDs, IncludeReferences: len(characterIDs) > 0,
		Aspect: in.Aspect, Caption: in.Caption, Note: in.Note, Source: d.sourceOrExternal(),
	})
	if err != nil {
		return illustrationToolErr(err), out, nil
	}
	return nil, requestIllustrationOutput{RequestID: r.ID, Status: requestWaiting, Prompt: r.Prompt}, nil
}

func (d ToolDeps) setRepresentativeIllustration(ctx context.Context, _ *mcp.CallToolRequest, in setRepresentativeInput) (*mcp.CallToolResult, setRepresentativeOutput, error) {
	out := setRepresentativeOutput{}
	if d.Gallery == nil || d.Story == nil {
		return toolErr("representative pictures are unavailable in this build"), out, nil
	}
	n, refusal := d.requireNode(ctx, in.NodeID)
	if refusal != nil {
		return refusal, out, nil
	}
	if _, refusal := d.requireProject(ctx, n.ProjectID); refusal != nil {
		return refusal, out, nil
	}
	id := strings.TrimSpace(in.IllustrationID)
	previous, err := d.Gallery.SetRepresentative(ctx, d.now(), n.ID, id)
	if err != nil {
		return illustrationToolErr(err), out, nil
	}
	batch := d.Story.RememberChange(n.ProjectID, func(ctx context.Context) error {
		_, err := d.Gallery.SetRepresentative(ctx, d.now(), n.ID, previous)
		return err
	})
	d.notifyChanged(n.ProjectID, "linetta_set_representative_illustration", []string{n.ID}, batch)
	return nil, setRepresentativeOutput{NodeID: n.ID, IllustrationID: id, Previous: previous, UndoBatchID: batch}, nil
}

func illustrationToolErr(err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, illustration.ErrTooManyPending):
		return toolErr("%v; ask the writer to approve or reject some first", err)
	case errors.Is(err, illustration.ErrNotFound):
		return toolErr("no such picture in this work's gallery; call linetta_list_illustrations")
	case errors.Is(err, illustration.ErrNodeNotFound):
		return toolErr("node not found in this work; call linetta_get_outline")
	}
	return toolErr("%v", err)
}
