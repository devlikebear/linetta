package export

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/export/docx"
	"github.com/devlikebear/linetta/engine/internal/export/manuscriptir"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

// Document formats export.document writes (#383). Markdown keeps its own
// path (export.project, export.node), which carries frontmatter for import.
const (
	FormatDOCX = "docx"
	FormatTXT  = "txt"
)

// DocumentOptions is the export dialog's layout choices. A field left out
// takes DefaultOptions' value, so an older dialog still gets a sound file.
type DocumentOptions struct {
	Font                string  `json:"font,omitempty"`
	SizePt              float64 `json:"size_pt,omitempty"`
	LineSpacingPct      int     `json:"line_spacing_pct,omitempty"`
	FirstLineIndent     *bool   `json:"first_line_indent,omitempty"`
	PageBreakPerEpisode *bool   `json:"page_break_per_episode,omitempty"`
	Headings            *bool   `json:"headings,omitempty"`
	SceneBreak          *string `json:"scene_break,omitempty"`
	Illustrations       bool    `json:"illustrations,omitempty"`
}

// DocumentRequest names what to export: the whole work, or NodeID's subtree.
type DocumentRequest struct {
	ProjectID string          `json:"project_id"`
	NodeID    string          `json:"node_id,omitempty"`
	Format    string          `json:"format"`
	Options   DocumentOptions `json:"options"`
	// Preview counts what would be exported without writing it, for the
	// dialog's summary line.
	Preview bool `json:"preview,omitempty"`
}

// DocumentPayload is the file, base64 for the JSON-RPC wire, with what the
// save dialog needs. Scenes and Chars describe what it holds; Chars follows
// the work's own character count basis.
type DocumentPayload struct {
	BytesBase64       string `json:"bytes_base64,omitempty"`
	SuggestedFilename string `json:"suggested_filename,omitempty"`
	MIME              string `json:"mime,omitempty"`
	Scenes            int    `json:"scenes"`
	Chars             int    `json:"chars"`
}

// ErrBadDocumentRequest wraps a request the dialog should not have sent.
type ErrBadDocumentRequest struct{ Reason string }

func (e *ErrBadDocumentRequest) Error() string { return "export.document: " + e.Reason }

func bad(format string, args ...any) error {
	return &ErrBadDocumentRequest{Reason: fmt.Sprintf(format, args...)}
}

// ExportDocument writes the work, or one part of it, as a DOCX or TXT file.
// Nothing is written to disk here: the app saves the bytes where the writer
// chooses, the same way the Markdown export does.
func ExportDocument(ctx context.Context, projects *project.Repo, nr *node.Repo, req DocumentRequest, language string) (DocumentPayload, error) {
	if req.Format != FormatDOCX && req.Format != FormatTXT {
		return DocumentPayload{}, bad("format %q is not docx or txt", req.Format)
	}
	opts, err := docxOptions(req.Options, language)
	if err != nil {
		return DocumentPayload{}, err
	}
	p, err := projects.Get(ctx, req.ProjectID)
	if err != nil {
		return DocumentPayload{}, err
	}
	nodes, err := nr.ListByProject(ctx, p.ID)
	if err != nil {
		return DocumentPayload{}, err
	}
	root := strings.TrimSpace(req.NodeID)
	scenes, chars := countScope(nodes, root)
	if root != "" && scenes < 0 {
		return DocumentPayload{}, bad("node %s is not in this work", root)
	}
	out := DocumentPayload{Scenes: max(scenes, 0), Chars: chars}
	if req.Preview {
		return out, nil
	}
	doc, err := manuscriptir.Build(p.Title, nodes, root, manuscriptir.Options{
		Language: language, Headings: req.Options.Headings == nil || *req.Options.Headings,
	})
	if err != nil {
		return DocumentPayload{}, err
	}
	var data []byte
	switch req.Format {
	case FormatDOCX:
		data, err = docx.Write(doc, opts)
		if err != nil {
			return DocumentPayload{}, err
		}
		out.MIME = docx.MIME
	case FormatTXT:
		data = []byte(manuscriptir.Text(doc, manuscriptir.TextOptions{
			SceneBreak: opts.SceneBreak, Illustrations: opts.Illustrations, IllustrationLabel: opts.IllustrationLabel,
		}))
		out.MIME = "text/plain; charset=utf-8"
	}
	out.BytesBase64 = base64.StdEncoding.EncodeToString(data)
	out.SuggestedFilename = documentFilename(p.Title, nodes, root, language) + "." + req.Format
	return out, nil
}

func docxOptions(in DocumentOptions, language string) (docx.Options, error) {
	o := docx.DefaultOptions()
	if f := strings.TrimSpace(in.Font); f != "" {
		if utf8.RuneCountInString(f) > 64 || strings.ContainsAny(f, "<>&\"") {
			return o, bad("font %q is not a font name", f)
		}
		o.Font = f
	}
	if in.SizePt != 0 {
		if in.SizePt < 6 || in.SizePt > 72 {
			return o, bad("size_pt %.1f is outside 6-72", in.SizePt)
		}
		o.SizePt = in.SizePt
	}
	if in.LineSpacingPct != 0 {
		if in.LineSpacingPct < 80 || in.LineSpacingPct > 400 {
			return o, bad("line_spacing_pct %d is outside 80-400", in.LineSpacingPct)
		}
		o.LineSpacingPct = in.LineSpacingPct
	}
	if in.FirstLineIndent != nil {
		o.FirstLineIndent = *in.FirstLineIndent
	}
	if in.PageBreakPerEpisode != nil {
		o.PageBreakPerSection = *in.PageBreakPerEpisode
	}
	if in.SceneBreak != nil {
		if utf8.RuneCountInString(*in.SceneBreak) > 20 {
			return o, bad("scene_break is longer than 20 characters")
		}
		o.SceneBreak = *in.SceneBreak
	}
	o.Illustrations = in.Illustrations
	o.IllustrationLabel = illustrationLabel(language)
	return o, nil
}

func illustrationLabel(language string) string {
	switch {
	case strings.HasPrefix(language, "en"):
		return "Illustration"
	case strings.HasPrefix(language, "ja"):
		return "挿絵"
	}
	return "삽화"
}

// countScope counts the scenes under root (the whole work when "") and their
// characters. scenes is -1 when root is not one of nodes.
func countScope(nodes []node.Node, root string) (scenes, chars int) {
	if root == "" {
		for _, n := range nodes {
			if n.Kind == node.KindLeaf {
				scenes++
				chars += n.WordCount
			}
		}
		return scenes, chars
	}
	parent := map[string]string{}
	found := false
	for _, n := range nodes {
		if n.ParentID != nil {
			parent[n.ID] = *n.ParentID
		}
		if n.ID == root {
			found = true
		}
	}
	if !found {
		return -1, 0
	}
	under := func(id string) bool {
		for cur := id; cur != ""; cur = parent[cur] {
			if cur == root {
				return true
			}
		}
		return false
	}
	for _, n := range nodes {
		if n.Kind == node.KindLeaf && under(n.ID) {
			scenes++
			chars += n.WordCount
		}
	}
	return scenes, chars
}

// documentFilename is the work's title, then the exported part's label when
// it is not the whole work: "은빛 열쇠-12화".
func documentFilename(title string, nodes []node.Node, root, language string) string {
	name := slugify(title)
	if name == "" {
		name = "manuscript"
	}
	for _, n := range nodes {
		if n.ID == root {
			label := slugify(node.DisplayLabel(n.Label, language))
			if label == "" {
				label = slugify(n.Title)
			}
			if label != "" {
				name += "-" + label
			}
		}
	}
	return name
}
