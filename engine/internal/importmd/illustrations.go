package importmd

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/illustration"
	"github.com/devlikebear/linetta/engine/internal/illustref"
	"github.com/devlikebear/linetta/engine/internal/node"
)

// ImportedImage is a picture a Markdown file carried: one data-URI reference
// definition, keyed by the id in its label.
type ImportedImage struct {
	MIME string
	Data []byte
}

// What export writes for an embedded picture: `![caption][illustration-<id>]`
// on its own line, and `[illustration-<id>]: data:<mime>;base64,<bytes>` once
// at the end of the file.
var (
	illustrationRefRE = regexp.MustCompile(`^\s*!\[((?:\\.|[^\]\\])*)\]\[illustration-([0-9A-Za-z-]{1,64})\]\s*$`)
	illustrationDefRE = regexp.MustCompile(`^\s*\[illustration-([0-9A-Za-z-]{1,64})\]:\s*data:(image/(?:jpeg|png|webp));base64,([A-Za-z0-9+/=]+)\s*$`)
	altEscapeRE       = regexp.MustCompile(`\\(.)`)
)

// extractIllustrationDefs takes the picture definitions out of a document
// before it is parsed, so the long data lines never become prose. A line that
// does not decode stays where it is, as text.
func extractIllustrationDefs(text string) (string, map[string]ImportedImage) {
	if !strings.Contains(text, "[illustration-") {
		return text, nil
	}
	images := map[string]ImportedImage{}
	lines := strings.Split(text, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		m := illustrationDefRE.FindStringSubmatch(strings.TrimRight(ln, "\r"))
		if m == nil {
			kept = append(kept, ln)
			continue
		}
		data, err := base64.StdEncoding.DecodeString(m[3])
		if err != nil || len(data) == 0 {
			kept = append(kept, ln)
			continue
		}
		images[m[1]] = ImportedImage{MIME: m[2], Data: data}
	}
	return strings.Join(kept, "\n"), images
}

// illustrationBlock reads a reference-image line as the illustration node,
// still carrying the id the file used; RestoreIllustrations replaces it.
func illustrationBlock(line string) (TiptapBlock, bool) {
	m := illustrationRefRE.FindStringSubmatch(line)
	if m == nil {
		return TiptapBlock{}, false
	}
	caption := altEscapeRE.ReplaceAllString(m[1], "$1")
	return TiptapBlock{Type: illustref.NodeType, Attrs: map[string]any{"id": m[2], "caption": caption}}, true
}

// RestoreIllustrations puts the pictures the imported scenes show into the new
// work's gallery and points the scenes at them. A picture the file did not
// carry, or that the gallery refuses (too large, not an image, the gallery
// full), is left in the scene as its placeholder line and warned about.
func RestoreIllustrations(ctx context.Context, repo *illustration.Repo, nr *node.Repo, now int64, projectID string, images map[string]ImportedImage, res *BuildResult) error {
	leaves, err := nr.ListByProject(ctx, projectID)
	if err != nil {
		return err
	}
	newIDs := map[string]string{}
	tried := map[string]bool{}
	missing, rejected := false, false
	for _, n := range leaves {
		if n.Kind != node.KindLeaf || n.ContentDoc == nil {
			continue
		}
		ids, err := illustref.IDs(*n.ContentDoc)
		if err != nil || len(ids) == 0 {
			continue
		}
		for _, id := range ids {
			if tried[id] {
				continue
			}
			tried[id] = true
			img, ok := images[id]
			if !ok {
				missing = true
				continue
			}
			meta, err := repo.Add(ctx, now, illustration.NewInput{
				ProjectID: projectID, SourceNodeID: n.ID, Provider: "import", Data: img.Data,
			})
			if err != nil {
				rejected = true
				continue
			}
			newIDs[id] = meta.ID
		}
		doc, _, err := illustref.RewriteIDs(*n.ContentDoc, newIDs)
		if err != nil {
			return err
		}
		known := map[string]bool{}
		for _, id := range newIDs {
			known[id] = true
		}
		if doc, err = illustref.KeepKnown(doc, func(id string) bool { return known[id] }); err != nil {
			return err
		}
		if doc != *n.ContentDoc {
			if err := nr.UpdateContent(ctx, n.ID, doc, now); err != nil {
				return err
			}
		}
	}
	if missing {
		res.Warnings = appendOnce(res.Warnings, WarnIllustrationsMissing)
	}
	if rejected {
		res.Warnings = appendOnce(res.Warnings, WarnIllustrationsRejected)
	}
	return nil
}
