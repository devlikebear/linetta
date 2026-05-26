// Package settings persists user-controlled preferences in $LINETTA_HOME/settings.json.
// The struct is intentionally tiny in MVP: provider choice + typewriter default,
// plus a read-only backup_dir field surfaced for the UI.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/paths"
)

// Allowed provider IDs (must match tars/pkg/llm provider names + UI labels).
const (
	ProviderClaudeCodeCLI = "claude-code-cli"
	ProviderOpenAICodex   = "openai-codex"
)

func validProviders() []string { return []string{ProviderClaudeCodeCLI, ProviderOpenAICodex} }

// Config is the on-disk JSON. backup_dir is computed at Load time and not
// persisted (the field is omitted from JSON marshalling on write).
type Config struct {
	Provider          string `json:"provider"`
	TypewriterDefault bool   `json:"typewriter_default"`
	BackupDir         string `json:"backup_dir,omitempty"`
}

// Patch holds optional updates. Nil pointers mean "leave the field alone".
type Patch struct {
	Provider          *string `json:"provider,omitempty"`
	TypewriterDefault *bool   `json:"typewriter_default,omitempty"`
}

// Store reads and writes the settings file with internal locking.
type Store struct {
	mu  sync.RWMutex
	cfg Config
	dir string
}

// New constructs a Store, ensuring $LINETTA_HOME exists and loading the file.
// Missing or corrupt files yield defaults (and a quiet rewrite on next Set).
func New() (*Store, error) {
	if err := paths.EnsureHome(); err != nil {
		return nil, err
	}
	home, err := paths.Home()
	if err != nil {
		return nil, err
	}
	s := &Store{dir: home, cfg: defaults(home)}
	_ = s.load() // benign: defaults already set
	return s, nil
}

func defaults(home string) Config {
	return Config{
		Provider:          ProviderClaudeCodeCLI,
		TypewriterDefault: false,
		BackupDir:         filepath.Join(home, "backups"),
	}
}

func (s *Store) load() error {
	path := filepath.Join(s.dir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // keep defaults
		}
		return err
	}
	var disk Config
	if err := json.Unmarshal(data, &disk); err != nil {
		return nil // ignore corrupt file; defaults stand
	}
	s.mu.Lock()
	if disk.Provider != "" {
		s.cfg.Provider = disk.Provider
	}
	s.cfg.TypewriterDefault = disk.TypewriterDefault
	s.mu.Unlock()
	return nil
}

// Get returns a copy of the current Config (with backup_dir filled in).
func (s *Store) Get(ctx context.Context) (Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.cfg
	c.BackupDir = filepath.Join(s.dir, "backups")
	return c, nil
}

// Provider returns the active provider id (cheap, lock-protected).
// ai.Runner calls this on every Start so provider changes take effect at once.
func (s *Store) Provider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Provider
}

// Set applies a partial patch, validates, persists atomically, returns the new Config.
func (s *Store) Set(ctx context.Context, p Patch) (Config, error) {
	s.mu.Lock()
	next := s.cfg
	if p.Provider != nil {
		if !contains(validProviders(), *p.Provider) {
			s.mu.Unlock()
			return Config{}, fmt.Errorf("settings: unknown provider %q", *p.Provider)
		}
		next.Provider = *p.Provider
	}
	if p.TypewriterDefault != nil {
		next.TypewriterDefault = *p.TypewriterDefault
	}
	s.cfg = next
	s.mu.Unlock()

	// Persist (no backup_dir on disk).
	persistable := Config{Provider: next.Provider, TypewriterDefault: next.TypewriterDefault}
	body, err := json.MarshalIndent(persistable, "", "  ")
	if err != nil {
		return Config{}, err
	}
	tmp := filepath.Join(s.dir, "settings.json.tmp")
	target := filepath.Join(s.dir, "settings.json")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return Config{}, err
	}
	if err := os.Rename(tmp, target); err != nil {
		return Config{}, err
	}
	return s.Get(ctx)
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
