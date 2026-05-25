package llm

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Provider is the minimal interface every LLM backend implements so the
// agents/suggesters can stay backend-agnostic. The Label string is shown in
// /api/version for user-facing diagnostics.
type Provider interface {
	ChatText(ctx context.Context, messages []Message, temperature float64) (string, error)
	ChatJSON(ctx context.Context, messages []Message, temperature float64, out any) error
	Label() string
}

// ErrNoProvider replaces ErrNoAPIKey for the multi-backend factory path. The
// older ErrNoAPIKey is still returned by direct OpenAI calls when an API key
// is absent.
var ErrNoProvider = errors.New("llm: no provider configured (set OPENAI_API_KEY, or configure codex in tessera.yaml)")

// tesseraConfig is the subset of the user's tessera.yaml we read for LLM
// routing. Field names match the schema defined by tessera's pkg/config.
type tesseraConfig struct {
	LLM struct {
		DefaultProvider string                    `yaml:"default_provider"`
		Providers       map[string]tesseraProvCfg `yaml:"providers"`
	} `yaml:"llm"`
}

type tesseraProvCfg struct {
	Provider        string `yaml:"provider"` // "openai-codex" | "openai" | ...
	Model           string `yaml:"model"`
	BaseURL         string `yaml:"base_url"`
	AuthMode        string `yaml:"auth_mode"` // "oauth" | "api_key"
	APIKeyEnv       string `yaml:"api_key_env"`
	WorkDir         string `yaml:"work_dir"`
	ReasoningEffort string `yaml:"reasoning_effort"`
}

// NewProvider resolves a Provider from the user's environment by inspecting,
// in order:
//
//  1. LINETTA_TESSERA_CONFIG env override path → llm.default_provider
//  2. ~/.linetta/tessera.yaml → llm.default_provider
//  3. OPENAI_API_KEY env var → direct OpenAI Chat Completions
//
// Returns ErrNoProvider when nothing usable is configured, OR when
// LINETTA_DISABLE_LLM=1 (set by go test runs to keep unit tests offline).
// Callers that don't want to fail (the agents) should fall back to their
// deterministic stubs.
func NewProvider() (Provider, error) {
	if os.Getenv("LINETTA_DISABLE_LLM") == "1" {
		return nil, ErrNoProvider
	}
	if cfg, name, ok := readTesseraLLM(tesseraConfigPath()); ok {
		switch cfg.Provider {
		case "openai-codex":
			if path, err := exec.LookPath("codex"); err == nil {
				return &CodexClient{
					BinPath:         path,
					Model:           cfg.Model,
					ReasoningEffort: cfg.ReasoningEffort,
					WorkDir:         expandWorkDir(cfg.WorkDir),
					provName:        name,
				}, nil
			}
			// codex configured but binary missing → fall through to OPENAI_API_KEY
		case "openai":
			keyVar := cfg.APIKeyEnv
			if keyVar == "" {
				keyVar = "OPENAI_API_KEY"
			}
			if key := os.Getenv(keyVar); key != "" {
				return &OpenAIChatClient{
					APIKey:  key,
					BaseURL: orDefault(cfg.BaseURL, "https://api.openai.com/v1"),
					Model:   orDefault(cfg.Model, "gpt-4o-mini"),
				}, nil
			}
		}
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return &OpenAIChatClient{
			APIKey:  key,
			BaseURL: orDefault(os.Getenv("LINETTA_LLM_BASE_URL"), "https://api.openai.com/v1"),
			Model:   orDefault(os.Getenv("LINETTA_LLM_MODEL"), "gpt-4o-mini"),
		}, nil
	}
	return nil, ErrNoProvider
}

// ProviderInfo describes what NewProvider would resolve to, for /api/version.
type ProviderInfo struct {
	Enabled  bool
	Provider string // "openai-codex" | "openai" | "fallback"
	Model    string
	Reason   string // human-readable explanation when Enabled is false
}

// DescribeProvider returns metadata about the currently-resolvable provider
// without actually constructing it. Used by the /api/version handler.
func DescribeProvider() ProviderInfo {
	if os.Getenv("LINETTA_DISABLE_LLM") == "1" {
		return ProviderInfo{Enabled: false, Provider: "fallback", Reason: "LINETTA_DISABLE_LLM=1"}
	}
	if cfg, name, ok := readTesseraLLM(tesseraConfigPath()); ok {
		switch cfg.Provider {
		case "openai-codex":
			if _, err := exec.LookPath("codex"); err == nil {
				return ProviderInfo{Enabled: true, Provider: "openai-codex", Model: cfg.Model, Reason: "tessera.yaml(" + name + ")"}
			}
			return ProviderInfo{Enabled: false, Provider: "openai-codex", Model: cfg.Model, Reason: "codex CLI not found in PATH"}
		case "openai":
			keyVar := cfg.APIKeyEnv
			if keyVar == "" {
				keyVar = "OPENAI_API_KEY"
			}
			if os.Getenv(keyVar) != "" {
				return ProviderInfo{Enabled: true, Provider: "openai", Model: orDefault(cfg.Model, "gpt-4o-mini"), Reason: "tessera.yaml(" + name + ")"}
			}
			return ProviderInfo{Enabled: false, Provider: "openai", Model: cfg.Model, Reason: keyVar + " is empty"}
		}
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return ProviderInfo{Enabled: true, Provider: "openai", Model: orDefault(os.Getenv("LINETTA_LLM_MODEL"), "gpt-4o-mini"), Reason: "OPENAI_API_KEY env"}
	}
	return ProviderInfo{Enabled: false, Provider: "fallback", Model: "", Reason: "no tessera.yaml provider and OPENAI_API_KEY empty"}
}

func readTesseraLLM(path string) (tesseraProvCfg, string, bool) {
	if path == "" {
		return tesseraProvCfg{}, "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return tesseraProvCfg{}, "", false
	}
	var tc tesseraConfig
	if err := yaml.Unmarshal(data, &tc); err != nil {
		return tesseraProvCfg{}, "", false
	}
	name := tc.LLM.DefaultProvider
	if name == "" {
		// Pick any provider if no default — first one alphabetically wins.
		for k := range tc.LLM.Providers {
			if name == "" || k < name {
				name = k
			}
		}
	}
	if name == "" {
		return tesseraProvCfg{}, "", false
	}
	cfg, ok := tc.LLM.Providers[name]
	if !ok || cfg.Provider == "" {
		return tesseraProvCfg{}, "", false
	}
	return cfg, name, true
}

func tesseraConfigPath() string {
	if p := os.Getenv("LINETTA_TESSERA_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".linetta", "tessera.yaml")
}

func expandWorkDir(s string) string {
	if s == "" || s == "." {
		return ""
	}
	if s[0] == '~' {
		home, _ := os.UserHomeDir()
		if home != "" {
			return filepath.Join(home, s[1:])
		}
	}
	return s
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

