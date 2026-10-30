package export

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/atomicfile"
	"github.com/devlikebear/linetta/engine/internal/illustref"
)

// IllustrationSource supplies the bytes of a work's pictures. A picture the
// gallery no longer has returns ErrIllustrationMissing (or any error the
// caller wraps it in); the export then leaves that picture out.
type IllustrationSource interface {
	IllustrationImage(ctx context.Context, id string) (mime string, data []byte, err error)
}

// ErrIllustrationMissing marks a picture a scene shows that the gallery no
// longer has.
var ErrIllustrationMissing = errors.New("illustration not in the gallery")

// ImageFile is one picture a sync export writes into the work's image folder.
type ImageFile struct {
	Name string // file name inside the folder: <id>.<ext>
	Data []byte
}

// IllustrationRefPrefix starts the reference label of an embedded picture:
// `![caption][illustration-<id>]`, defined once at the end of the file.
const IllustrationRefPrefix = "illustration-"

// SyncImageDir is the folder beside a synced work's file that holds its
// pictures, named after the file so works never share one.
func SyncImageDir(title, projectID string) string {
	return strings.TrimSuffix(SyncFilename(title, projectID), ".md") + ".images"
}

func imageExt(mime string) string {
	switch mime {
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	}
	return "jpg"
}

// escapeAlt makes a caption safe inside `![...]`.
func escapeAlt(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`)
	return r.Replace(s)
}

type loadedImage struct {
	mime string
	data []byte
}

// imageRenderer turns a work's illustration nodes into Markdown. With dir
// empty the pictures are embedded: each node is a reference image and one
// data-URI definition per picture goes at the end of the file, so the prose
// stays readable and a picture used twice is stored once. With dir set, each
// node links to dir/<id>.<ext> and the bytes come back as files.
type imageRenderer struct {
	dir    string
	images map[string]loadedImage
	order  []string
}

// newImageRenderer loads every picture the docs show, in first-use order.
func newImageRenderer(ctx context.Context, src IllustrationSource, dir string, docs []string) (*imageRenderer, error) {
	r := &imageRenderer{dir: dir, images: map[string]loadedImage{}}
	for _, doc := range docs {
		ids, err := illustref.IDs(doc)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, seen := r.images[id]; seen {
				continue
			}
			mime, data, err := src.IllustrationImage(ctx, id)
			if errors.Is(err, ErrIllustrationMissing) {
				r.images[id] = loadedImage{}
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("export: illustration %s: %w", id, err)
			}
			r.images[id] = loadedImage{mime: mime, data: data}
			r.order = append(r.order, id)
		}
	}
	return r, nil
}

func (r *imageRenderer) render(id, caption string) string {
	img := r.images[id]
	if len(img.data) == 0 {
		return ""
	}
	if r.dir == "" {
		return "![" + escapeAlt(caption) + "][" + IllustrationRefPrefix + id + "]"
	}
	return "![" + escapeAlt(caption) + "](" + r.dir + "/" + id + "." + imageExt(img.mime) + ")"
}

func (r *imageRenderer) options() MarkdownOptions {
	return MarkdownOptions{Illustration: r.render}
}

// definitions is the block of embedded pictures for the end of the file.
func (r *imageRenderer) definitions() string {
	if r.dir != "" || len(r.order) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, id := range r.order {
		img := r.images[id]
		sb.WriteString("[" + IllustrationRefPrefix + id + "]: data:" + img.mime + ";base64,")
		sb.WriteString(base64.StdEncoding.EncodeToString(img.data))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (r *imageRenderer) files() []ImageFile {
	if r.dir == "" {
		return nil
	}
	out := make([]ImageFile, 0, len(r.order))
	for _, id := range r.order {
		img := r.images[id]
		out = append(out, ImageFile{Name: id + "." + imageExt(img.mime), Data: img.data})
	}
	return out
}

// WriteImageDir makes dir hold exactly files. A file already there under the
// same name is left alone (a picture's bytes never change under its id, so a
// sync does not touch it again and Git sees no change); files no longer used
// are removed, and the folder itself goes when it would be empty.
func WriteImageDir(dir string, files []ImageFile) error {
	keep := map[string]bool{}
	for _, f := range files {
		keep[f.Name] = true
	}
	if len(files) > 0 {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	for _, f := range files {
		dest := filepath.Join(dir, f.Name)
		if info, err := os.Stat(dest); err == nil && info.Size() == int64(len(f.Data)) {
			continue
		}
		if err := atomicfile.Write(dest, f.Data, 0o644); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	remaining := 0
	for _, e := range entries {
		if e.IsDir() || keep[e.Name()] {
			remaining++
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	if remaining == 0 {
		return os.Remove(dir)
	}
	return nil
}
