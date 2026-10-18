package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/node"
)

// SceneBreak joins the scenes of one episode. Linetta Works' importer splits
// a whole-project export the same way, so an episode imported from a file and
// the same episode published from the app hash alike.
const SceneBreak = "\n\n* * *\n\n"

// EpisodePayload is one episode as Linetta Works receives it.
type EpisodePayload struct {
	Markdown string `json:"markdown"`
	Title    string `json:"title"`
	// CharCount counts characters other than whitespace, the way the site
	// counts them against its length limit.
	CharCount int `json:"char_count"`
	// ContentHash is the lowercase hex SHA-256 of Markdown's UTF-8 bytes,
	// which the site recomputes and compares (docs/api.md in linetta-artworks).
	ContentHash string `json:"content_hash"`
}

// ExportEpisodeMarkdown renders an episode's scenes for publishing: each
// scene's markdown, trimmed, joined by SceneBreak.
//
// A published episode is read by strangers, so it carries none of the
// editing marks the export for the writer's own files keeps: a mention is
// its plain name, not "@name", and margin-note anchors are dropped.
// workTitle titles an episode that is the whole work.
func ExportEpisodeMarkdown(ctx context.Context, nr *node.Repo, ep episode.Episode, workTitle string) (EpisodePayload, error) {
	var parts []string
	for _, id := range ep.LeafIDs {
		n, err := nr.Get(ctx, id)
		if err != nil {
			return EpisodePayload{}, err
		}
		if n.ContentDoc == nil {
			continue
		}
		md, err := PublishDocToMarkdown([]byte(*n.ContentDoc))
		if err != nil {
			return EpisodePayload{}, fmt.Errorf("export episode %s: %w", ep.NodeID, err)
		}
		if md = strings.TrimSpace(collapseBlankLines(md)); md != "" {
			parts = append(parts, md)
		}
	}
	md := strings.Join(parts, SceneBreak)
	return EpisodePayload{
		Markdown:    md,
		Title:       EpisodeTitle(ep, workTitle),
		CharCount:   CharCount(md),
		ContentHash: ContentHash(md),
	}, nil
}

// EpisodeTitle is the title a reader sees next to the episode number. The
// node's title when it has one; otherwise its label, unless the label is only
// the number ("3화"), which the site already shows.
func EpisodeTitle(ep episode.Episode, workTitle string) string {
	if ep.Whole {
		return strings.TrimSpace(workTitle)
	}
	if title := strings.TrimSpace(ep.Title); title != "" {
		return title
	}
	label := strings.TrimSpace(ep.Label)
	if episode.IsNumberedLabel(label) {
		return ""
	}
	return label
}

// ContentHash is the lowercase hex SHA-256 of s's UTF-8 bytes.
func ContentHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// CharCount counts the runes in s that are not whitespace.
func CharCount(s string) int {
	count := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			count++
		}
	}
	return count
}

// PublishDocToMarkdown is DocToMarkdown for a reader rather than for the
// writer's own files: mentions become their plain label and note anchors
// disappear. It rewrites the document and hands it to DocToMarkdown, so the
// rest of the rendering — and the export the writer already relies on — is
// exactly the same.
func PublishDocToMarkdown(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("export: parse doc: %w", err)
	}
	clean, err := json.Marshal(stripEditingMarks(doc))
	if err != nil {
		return "", err
	}
	return DocToMarkdown(clean)
}

func stripEditingMarks(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	content, ok := m["content"].([]any)
	if !ok {
		return m
	}
	out := make([]any, 0, len(content))
	for _, child := range content {
		cm, ok := child.(map[string]any)
		if !ok {
			out = append(out, child)
			continue
		}
		switch cm["type"] {
		case "noteMarker":
			continue
		case "mention":
			label := stringAttr(cm, "label")
			if label == "" {
				continue
			}
			text := map[string]any{"type": "text", "text": label}
			if marks, ok := cm["marks"]; ok {
				text["marks"] = marks
			}
			out = append(out, text)
		default:
			out = append(out, stripEditingMarks(cm))
		}
	}
	copied := make(map[string]any, len(m))
	for k, val := range m {
		copied[k] = val
	}
	copied["content"] = out
	return copied
}
