package provider

import (
	"context"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

// codexModelsTTL is how long one Codex model list answers for. The list
// changes when the backend ships or retires a model, not between turns, and
// asking on every turn would put a round trip in front of each one.
const codexModelsTTL = 30 * time.Minute

// codexModelsTimeout bounds the list call a turn waits on. Past it the turn
// goes ahead on tars' own default rather than not at all.
const codexModelsTimeout = 10 * time.Second

// codexModelCache remembers the last list one Codex login answered with, in
// the backend's priority order (not the sorted order ListModels returns).
type codexModelCache struct {
	mu      sync.Mutex
	home    string
	models  []string
	fetched time.Time
}

func (c *codexModelCache) get(home string, now time.Time) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.home != home || len(c.models) == 0 || now.Sub(c.fetched) > codexModelsTTL {
		return nil
	}
	return c.models
}

func (c *codexModelCache) put(home string, models []string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.home, c.models, c.fetched = home, append([]string(nil), models...), now
}

// codexModel is the model a Codex call goes out with. A model the writer
// chose is theirs and is sent as written. When they have not chosen one — the
// field is empty, or still holds the default 1.x stored for everyone — the
// account's own list decides: the stored legacy default if this account
// still lists it (another plan may), otherwise the first model listed. If
// the list cannot be had, the model is left empty and tars applies its own
// default, which is still better than a slug known to be refused.
func (s *Source) codexModel(ctx context.Context, r Resolved) string {
	if !settings.UsesOpenAICodexDefaultModel(r.Model) {
		return r.Model
	}
	stored := strings.TrimSpace(r.Model)
	models := s.codexCache.get(r.CodexHome, time.Now())
	if models == nil {
		ctx, cancel := context.WithTimeout(ctx, codexModelsTimeout)
		defer cancel()
		fetched, err := s.fetcher.FetchModels(ctx, Options(Resolved{ID: r.ID, BaseURL: r.BaseURL, CodexHome: r.CodexHome}))
		if err != nil || len(fetched) == 0 {
			log.Printf("provider: listing Codex models for the default failed; leaving the model to tars: %v", Classify(r.ID, err))
			return ""
		}
		s.codexCache.put(r.CodexHome, fetched, time.Now())
		models = fetched
	}
	if stored != "" && slices.Contains(models, stored) {
		return stored
	}
	return models[0]
}
