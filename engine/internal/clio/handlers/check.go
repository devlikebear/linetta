package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/clio/check"
	"github.com/devlikebear/linetta/engine/internal/clio/post"
	"github.com/devlikebear/linetta/engine/internal/clio/voice"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// VoiceCache keeps the stored posts and their profile between check.run
// calls, which arrive on every pause in typing. Invalidate it whenever posts
// change.
type VoiceCache struct {
	repo  *post.Repo
	mu    sync.Mutex
	ok    bool
	posts []post.Post
	prof  voice.Profile
}

// NewVoiceCache returns an empty cache over repo.
func NewVoiceCache(repo *post.Repo) *VoiceCache { return &VoiceCache{repo: repo} }

// Invalidate drops the cached posts so the next call reloads them.
func (c *VoiceCache) Invalidate() {
	c.mu.Lock()
	c.ok = false
	c.posts, c.prof = nil, voice.Profile{}
	c.mu.Unlock()
}

func (c *VoiceCache) get(ctx context.Context) ([]post.Post, voice.Profile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ok {
		posts, err := c.repo.List(ctx, post.ListParams{})
		if err != nil {
			return nil, voice.Profile{}, err
		}
		c.posts, c.prof, c.ok = posts, voice.Build(posts), true
	}
	return c.posts, c.prof, nil
}

// InvalidateAfter wraps a handler that changes posts so a successful call
// drops the cache.
func (c *VoiceCache) InvalidateAfter(h rpc.Handler) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		out, err := h(ctx, params)
		if err == nil {
			c.Invalidate()
		}
		return out, err
	}
}

// RunCheck returns a handler for check.run: {body, platform, kind} →
// {findings, context_used}. context_used is the profile context the findings
// were measured against, which differs from platform/kind on fallback.
func RunCheck(cache *VoiceCache) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			Body     string `json:"body"`
			Platform string `json:"platform"`
			Kind     string `json:"kind"`
		}
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, clioInvalid("invalid params")
		}
		if !clioPlatform(in.Platform) || !clioKind(in.Kind) {
			return nil, clioInvalid("unknown platform or kind")
		}
		out := struct {
			Findings    []check.Finding `json:"findings"`
			ContextUsed string          `json:"context_used"`
		}{Findings: []check.Finding{}}
		if strings.TrimSpace(in.Body) == "" {
			out.ContextUsed = in.Platform + "/" + in.Kind
			return json.Marshal(out)
		}
		posts, prof, err := cache.get(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		cp := prof.For(in.Platform, in.Kind)
		out.Findings = check.Run(in.Body, cp, check.BuildCorpus(posts, cp.Fallback))
		out.ContextUsed = cp.Fallback
		return json.Marshal(out)
	}
}
