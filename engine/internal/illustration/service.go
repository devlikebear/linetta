package illustration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/visual"
)

// Aspects a picture can be asked for. The provider treats them as a request;
// the stored width and height come from the bytes it returns.
const (
	AspectSquare    = "square"
	AspectPortrait  = "portrait"
	AspectLandscape = "landscape"
)

const (
	// MaxReferences bounds the reference images sent with one drawing: the
	// first images of the characters in the picture, in order.
	MaxReferences = 4
	// maxReferenceBytes bounds what those images add to the request.
	maxReferenceBytes = 12 * 1024 * 1024
	// drawTimeout bounds one drawing. Codex took 25-35 s and Gemini 4-17 s in
	// 2026-10; a refusal took Codex up to a minute before its retries were
	// stopped.
	drawTimeout = 180 * time.Second
)

// ErrInvalidInput marks a request the writer can fix before anything is sent.
var ErrInvalidInput = errors.New("invalid illustration request")

// Reference is one image sent along for the provider to draw from.
type Reference struct {
	MIME string
	Data []byte
}

// PaintRequest is one drawing. Prompt goes to the provider as given.
type PaintRequest struct {
	Prompt     string
	References []Reference
	Aspect     string
}

// PaintResult is what the provider drew, and with what.
type PaintResult struct {
	Data          []byte
	MIME          string
	RevisedPrompt string
	Provider      string
	Model         string
}

// Painter draws with the writer's image provider. It is declared here with
// plain types because this package must not link tars/pkg/llm
// (scripts/validate-story-core-deps.sh); engineapp adapts the provider to it.
type Painter interface {
	Paint(ctx context.Context, req PaintRequest) (PaintResult, error)
}

// GenerateInput is a drawing the writer asked for.
type GenerateInput struct {
	ProjectID    string `json:"project_id"`
	SourceNodeID string `json:"source_node_id,omitempty"`
	// Prompt is the final text the writer saw and approved. Nothing is added
	// to it: what they read is what is sent.
	Prompt string `json:"prompt"`
	// CharacterIDs are the characters in the picture, whose reference images
	// go along when IncludeReferences is set.
	CharacterIDs      []string `json:"character_ids,omitempty"`
	IncludeReferences bool     `json:"include_references,omitempty"`
	Aspect            string   `json:"aspect,omitempty"` // AspectSquare when empty
	Caption           string   `json:"caption,omitempty"`
}

// Service draws illustrations and keeps them in the gallery.
type Service struct {
	repo    *Repo
	visuals *visual.Repo
	painter Painter
	now     func() int64
}

func NewService(repo *Repo, visuals *visual.Repo, painter Painter, now func() int64) *Service {
	return &Service{repo: repo, visuals: visuals, painter: painter, now: now}
}

// Generate draws one picture and stores it. The request is checked and the
// references gathered before the provider is called, so a mistake costs no
// drawing; a failed or refused drawing stores nothing.
func (s *Service) Generate(ctx context.Context, in GenerateInput) (Meta, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return Meta{}, fmt.Errorf("%w: prompt is empty", ErrInvalidInput)
	}
	switch in.Aspect {
	case "":
		in.Aspect = AspectSquare
	case AspectSquare, AspectPortrait, AspectLandscape:
	default:
		return Meta{}, fmt.Errorf("%w: aspect must be square, portrait or landscape", ErrInvalidInput)
	}
	if err := s.repo.checkPlace(ctx, in.ProjectID, in.SourceNodeID, in.CharacterIDs); err != nil {
		return Meta{}, err
	}
	refs, err := s.references(ctx, in)
	if err != nil {
		return Meta{}, err
	}

	drawCtx, cancel := context.WithTimeout(ctx, drawTimeout)
	defer cancel()
	res, err := s.painter.Paint(drawCtx, PaintRequest{Prompt: in.Prompt, References: refs, Aspect: in.Aspect})
	if err != nil {
		return Meta{}, err
	}
	// The stored prompt is the one sent, not the provider's rewrite: it is
	// what the writer can send again.
	return s.repo.Add(ctx, s.now(), NewInput{
		ProjectID:    in.ProjectID,
		SourceNodeID: in.SourceNodeID,
		Caption:      in.Caption,
		Prompt:       in.Prompt,
		Provider:     res.Provider,
		Model:        res.Model,
		Data:         res.Data,
	})
}

func (s *Service) references(ctx context.Context, in GenerateInput) ([]Reference, error) {
	refs := []Reference{}
	if !in.IncludeReferences {
		return refs, nil
	}
	total := 0
	for _, id := range in.CharacterIDs {
		images, err := s.visuals.ListImages(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("read reference images: %w", err)
		}
		for _, meta := range images {
			if len(refs) == MaxReferences {
				return refs, nil
			}
			if total+meta.ByteSize > maxReferenceBytes {
				continue
			}
			img, err := s.visuals.GetImage(ctx, meta.ID)
			if err != nil {
				return nil, fmt.Errorf("read reference image: %w", err)
			}
			refs = append(refs, Reference{MIME: img.MIME, Data: img.Data})
			total += len(img.Data)
		}
	}
	return refs, nil
}

// checkPlace confirms the work exists and that the scene and the characters
// belong to it, before anything is drawn.
func (r *Repo) checkPlace(ctx context.Context, projectID, nodeID string, characterIDs []string) error {
	var exists bool
	if err := r.s.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM projects WHERE id = ?)", projectID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrProjectNotFound
	}
	if nodeID != "" {
		if err := r.s.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM nodes WHERE id = ? AND project_id = ?)", nodeID, projectID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNodeNotFound
		}
	}
	for _, id := range characterIDs {
		if err := r.s.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM entities WHERE id = ? AND project_id = ?)", id, projectID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: character %q is not in this work", ErrInvalidInput, id)
		}
	}
	return nil
}
