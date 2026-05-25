package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// CodexClient invokes OpenAI's `codex exec` CLI as the LLM backend. This is the
// path activated when the user's tessera.yaml specifies
// `provider: openai-codex` with `auth_mode: oauth` — the codex CLI handles
// authentication (ChatGPT OAuth) and model selection, so the engine never
// needs an OPENAI_API_KEY env var.
//
// Output handling:
//   - codex exec writes ONLY the final model response to stdout.
//   - The session banner, transcript, and "tokens used" footer go to stderr.
//     We discard stderr (we don't surface it in /api/version).
//   - For ChatJSON, we instruct the model to return raw JSON and parse stdout
//     directly.
type CodexClient struct {
	BinPath         string // path to the codex binary (filled by exec.LookPath)
	Model           string // optional model override (e.g., gpt-5.5)
	ReasoningEffort string // optional: low | medium | high | xhigh
	WorkDir         string // optional cwd for codex (default: temp-safe location)

	provName string // tessera.yaml provider key used to populate Label
}

func (c *CodexClient) Label() string {
	if c == nil {
		return "openai-codex"
	}
	parts := []string{"openai-codex"}
	if c.Model != "" {
		parts = append(parts, c.Model)
	}
	return strings.Join(parts, " · ")
}

// ChatText flattens system+user messages into a single prompt, pipes via stdin,
// and returns the stdout (the model's response).
func (c *CodexClient) ChatText(ctx context.Context, messages []Message, temperature float64) (string, error) {
	prompt := flattenMessages(messages, false)
	out, err := c.exec(ctx, prompt)
	if err != nil {
		return "", err
	}
	return out, nil
}

// ChatJSON appends a JSON-only instruction to the prompt and decodes stdout
// (possibly stripped of code fences) into out.
func (c *CodexClient) ChatJSON(ctx context.Context, messages []Message, temperature float64, out any) error {
	prompt := flattenMessages(messages, true)
	body, err := c.exec(ctx, prompt)
	if err != nil {
		return err
	}
	cleaned := stripJSONFence(body)
	if err := json.Unmarshal([]byte(cleaned), out); err != nil {
		return fmt.Errorf("llm.CodexClient.ChatJSON: %w (body=%q)", err, cleaned)
	}
	return nil
}

// exec runs `codex exec - [--model X] [--config reasoning_effort=Y]`, piping
// prompt via stdin. stderr is discarded. Context cancellation propagates.
func (c *CodexClient) exec(ctx context.Context, prompt string) (string, error) {
	if c.BinPath == "" {
		return "", fmt.Errorf("llm.CodexClient: BinPath empty")
	}
	args := []string{"exec", "-"}
	if c.Model != "" {
		args = append(args, "-m", c.Model)
	}
	if c.ReasoningEffort != "" {
		args = append(args, "-c", fmt.Sprintf("reasoning_effort=%q", c.ReasoningEffort))
	}
	cmd := exec.CommandContext(ctx, c.BinPath, args...)
	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Include a snippet of stderr for diagnosability.
		errSnippet := truncateForError(stderr.String(), 240)
		return "", fmt.Errorf("llm.CodexClient: %s exec failed: %w (stderr=%s)", c.BinPath, err, errSnippet)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func flattenMessages(messages []Message, jsonOnly bool) string {
	var b strings.Builder
	for _, m := range messages {
		switch m.Role {
		case "system":
			b.WriteString("[System Instructions]\n")
			b.WriteString(m.Content)
			b.WriteString("\n\n")
		case "user":
			b.WriteString("[Writer's request]\n")
			b.WriteString(m.Content)
			b.WriteString("\n\n")
		case "assistant":
			b.WriteString("[Previous assistant reply]\n")
			b.WriteString(m.Content)
			b.WriteString("\n\n")
		default:
			b.WriteString(m.Content)
			b.WriteString("\n\n")
		}
	}
	if jsonOnly {
		b.WriteString("\n[Output format]\nReturn ONLY a single valid JSON object. No prose, no code fences, no commentary. Just the JSON body.\n")
	}
	return strings.TrimSpace(b.String())
}

// stripJSONFence handles cases where the model wraps the JSON in ```json fences
// despite being instructed not to.
func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Drop the opening fence (with or without "json" tag).
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func truncateForError(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Unused but kept for symmetry: io.Discard for ignoring stderr without buffer.
var _ = io.Discard
