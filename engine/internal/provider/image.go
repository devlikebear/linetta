package provider

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// ImageGenerator builds the client that draws illustrations with id, or with
// the writer's image provider when id is empty. Like Client it refuses before
// any network activity: without a credential, without the writer's image
// consent (which is not the agent's), or with no model to draw with. The
// returned Resolved carries the image model in Model.
//
// For Codex the model is the routing model that calls the hosted image tool.
// An unset one falls back to the first model the account's Codex list shows:
// a ChatGPT account refuses any other, and the agent's default is one of
// those it refuses. Gemini has no such list order to trust, so the writer
// picks an image model.
func (s *Source) ImageGenerator(ctx context.Context, id string) (llm.ImageGenerator, Resolved, error) {
	if id == "" {
		id = s.settings.ActiveImageProvider()
	}
	if id == "" {
		return nil, Resolved{}, &rpc.ReasonError{
			Reason: rpc.ReasonImageGenerationOff,
			Err:    errors.New("no image provider is chosen"),
		}
	}
	if !slices.Contains(settings.ValidImageProviders(), id) {
		return nil, Resolved{}, &rpc.ReasonError{
			Reason: rpc.ReasonProviderNotConfigured,
			Err:    fmt.Errorf("%s cannot draw images", id),
		}
	}
	r, err := s.Resolve(id)
	if err != nil {
		return nil, Resolved{}, err
	}
	if !r.Configured() {
		return nil, r, &rpc.ReasonError{
			Reason: rpc.ReasonProviderNotConfigured,
			Err:    fmt.Errorf("%s: no credential", r.ID),
		}
	}
	if !s.settings.HasImageConsent(id) {
		return nil, r, &rpc.ReasonError{
			Reason: rpc.ReasonImageConsentRequired,
			Err:    fmt.Errorf("%s: image consent required", r.ID),
		}
	}
	r.Model = strings.TrimSpace(s.settings.ImageProviderConfigFor(id).Model)
	if r.Model == "" {
		if id != settings.ProviderOpenAICodex {
			return nil, r, &rpc.ReasonError{
				Reason: rpc.ReasonImageModelRequired,
				Err:    fmt.Errorf("%s: no image model chosen", r.ID),
			}
		}
		models, err := s.fetcher.FetchModels(ctx, Options(r))
		if err != nil {
			return nil, r, Classify(r.ID, err)
		}
		if len(models) == 0 {
			return nil, r, &rpc.ReasonError{
				Reason: rpc.ReasonImageModelRequired,
				Err:    fmt.Errorf("%s: the account lists no models", r.ID),
			}
		}
		r.Model = models[0]
	}
	c, err := s.factory(Options(r))
	if err != nil {
		return nil, r, Classify(r.ID, err)
	}
	gen, ok := c.(llm.ImageGenerator)
	if !ok {
		return nil, r, fmt.Errorf("%s client %T cannot generate images", r.ID, c)
	}
	return gen, r, nil
}

// ListImageModels lists what the image settings can offer for id. Codex's are
// its routing models, in the account's own order; Gemini's are the models
// whose name marks image output ("gemini-3.1-flash-image"), since the models
// endpoint has no field that says so.
func (s *Source) ListImageModels(ctx context.Context, id string) ([]string, error) {
	if !slices.Contains(settings.ValidImageProviders(), id) {
		return nil, &rpc.ReasonError{
			Reason: rpc.ReasonProviderNotConfigured,
			Err:    fmt.Errorf("%s cannot draw images", id),
		}
	}
	r, err := s.Resolve(id)
	if err != nil {
		return nil, err
	}
	if !r.Configured() {
		return nil, &rpc.ReasonError{
			Reason: rpc.ReasonProviderNotConfigured,
			Err:    fmt.Errorf("%s: no credential", r.ID),
		}
	}
	models, err := s.fetcher.FetchModels(ctx, Options(r))
	if err != nil {
		return nil, Classify(r.ID, err)
	}
	if id == settings.ProviderOpenAICodex {
		return models, nil
	}
	out := make([]string, 0, len(models))
	for _, m := range models {
		m = strings.TrimPrefix(strings.TrimSpace(m), "models/")
		if strings.Contains(m, "-image") {
			out = append(out, m)
		}
	}
	slices.Sort(out)
	return out, nil
}

// ClassifyImage maps a drawing failure to a reason. Refusals and a Gemini key
// with no image quota are told apart first: Classify alone would call the
// first a rejection and the second (a 429) a rate limit to wait out.
func ClassifyImage(id string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, llm.ErrImageRefused):
		return &rpc.ReasonError{Reason: rpc.ReasonImageRefused, Err: err}
	case errors.Is(err, llm.ErrImageBillingRequired):
		return &rpc.ReasonError{Reason: rpc.ReasonImageBillingRequired, Err: err}
	}
	return Classify(id, err)
}

// ImageStatus is what the image settings pane shows per image provider. Like
// Status it never carries a secret.
type ImageStatus struct {
	ID         string `json:"id"`
	Active     bool   `json:"active"`
	Configured bool   `json:"configured"` // the agent provider's key or login exists
	Consented  bool   `json:"consented"`  // image consent, not the agent's
	Model      string `json:"model,omitempty"`
}

// ImageList describes every image provider, in ValidImageProviders order.
func (s *Source) ImageList() []ImageStatus {
	active := s.settings.ActiveImageProvider()
	out := make([]ImageStatus, 0, len(settings.ValidImageProviders()))
	for _, id := range settings.ValidImageProviders() {
		st := ImageStatus{
			ID:        id,
			Active:    id == active,
			Consented: s.settings.HasImageConsent(id),
			Model:     s.settings.ImageProviderConfigFor(id).Model,
		}
		if r, err := s.Resolve(id); err == nil {
			st.Configured = r.Configured()
		}
		out = append(out, st)
	}
	return out
}

// DrawReference is one reference image for Draw.
type DrawReference struct {
	MIME string
	Data []byte
}

// DrawRequest and DrawResult are Draw's plain-typed request and answer, so a
// caller can draw without linking tars/pkg/llm.
type DrawRequest struct {
	Prompt     string
	References []DrawReference
	Aspect     string // "square", "portrait" or "landscape"
}

type DrawResult struct {
	Data          []byte
	MIME          string
	RevisedPrompt string
	Provider      string
	Model         string
}

// Draw draws one image with the writer's image provider, gated the way
// ImageGenerator is. Failures come back classified (ClassifyImage).
func (s *Source) Draw(ctx context.Context, req DrawRequest) (DrawResult, error) {
	gen, r, err := s.ImageGenerator(ctx, "")
	if err != nil {
		return DrawResult{}, err
	}
	refs := make([]llm.ContentBlock, 0, len(req.References))
	for _, ref := range req.References {
		refs = append(refs, llm.ContentBlock{Type: "image", MediaType: ref.MIME, Data: base64.StdEncoding.EncodeToString(ref.Data)})
	}
	res, err := gen.GenerateImage(ctx, llm.ImageRequest{Prompt: req.Prompt, References: refs, Aspect: llm.ImageAspect(req.Aspect)})
	if err != nil {
		return DrawResult{}, ClassifyImage(r.ID, err)
	}
	return DrawResult{Data: res.Data, MIME: res.MediaType, RevisedPrompt: res.RevisedPrompt, Provider: r.ID, Model: r.Model}, nil
}
