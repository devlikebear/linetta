package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(60 + 150*x/w), uint8(90 + 100*y/h), 170, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// unzip returns every part of a package, checking each XML part parses.
func unzip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		parts[f.Name] = b
		if strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels") || strings.HasSuffix(f.Name, ".hpf") {
			d := xml.NewDecoder(bytes.NewReader(b))
			for {
				if _, err := d.Token(); err != nil {
					if !errors.Is(err, io.EOF) {
						t.Fatalf("%s is not well-formed: %v", f.Name, err)
					}
					break
				}
			}
		}
	}
	return parts
}

// A scene showing one picture twice, a WebP the document cannot hold, and a
// picture the gallery no longer has.
func picturesFixture(t *testing.T, format string) (DocumentPayload, map[string][]byte) {
	t.Helper()
	_, pr, nr, _, _ := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, newProjectInput())
	if err != nil {
		t.Fatal(err)
	}
	episode, err := nr.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1화", "비", 10)
	if err != nil {
		t.Fatal(err)
	}
	scene, err := nr.CreateChild(ctx, episode.ID, "leaf", "씬", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	doc := pictureDoc("앞 문단", "pic:a|비 오는 골목", "pic:w|웹피", "pic:gone|사라진 그림", "pic:a|비 오는 골목", "뒤 문단")
	if err := nr.UpdateContent(ctx, scene.ID, doc, 30); err != nil {
		t.Fatal(err)
	}
	gallery := fakeImages{"a": pngBytes(t, 1600, 800), "w": []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")}
	out, err := ExportDocument(ctx, pr, nr, gallery, DocumentRequest{
		ProjectID: p.ID, Format: format,
		Options: DocumentOptions{IllustrationImages: true, Cover: true, Contents: true},
	}, "ko")
	if err != nil {
		t.Fatal(err)
	}
	return out, unzip(t, decode(t, out))
}

func TestDOCXEmbedsPicturesWithCoverAndContents(t *testing.T) {
	out, parts := picturesFixture(t, FormatDOCX)
	if out.PicturesLeftOut != 2 {
		t.Fatalf("pictures left out = %d, want 2 (the WebP and the missing one)", out.PicturesLeftOut)
	}
	if !bytes.HasPrefix(parts["word/media/image1.png"], []byte("\x89PNG")) || parts["word/media/image2.png"] != nil {
		t.Fatalf("media = %v, want one PNG stored once", keys(parts, "word/media/"))
	}
	doc := string(parts["word/document.xml"])
	if n := strings.Count(doc, `r:embed="rIdImage1"`); n != 2 {
		t.Fatalf("picture drawn %d times, want 2", n)
	}
	// 1600px at 96dpi is wider than the page: scaled to 150mm by 75mm.
	if !strings.Contains(doc, `<wp:extent cx="5400000" cy="2700000"/>`) {
		t.Fatalf("picture size not fitted to the page:\n%s", doc)
	}
	for _, want := range []string{"비 오는 골목", "[삽화: 웹피]", "[삽화: 사라진 그림]", `LinettaTitle`, "차례", `LinettaContents`} {
		if !strings.Contains(doc, want) {
			t.Errorf("document lacks %q", want)
		}
	}
	if !strings.Contains(string(parts["word/_rels/document.xml.rels"]), `Target="media/image1.png"`) ||
		!strings.Contains(string(parts["[Content_Types].xml"]), `<Default Extension="png" ContentType="image/png"/>`) {
		t.Fatal("the picture is not registered in the package")
	}
	// The contents start a page after the cover, and the manuscript one
	// after the contents.
	if n := strings.Count(doc, "<w:pageBreakBefore/>"); n != 2 {
		t.Fatalf("page breaks = %d, want 2", n)
	}
}

func TestHWPXEmbedsPicturesWithCoverAndContents(t *testing.T) {
	out, parts := picturesFixture(t, FormatHWPX)
	if out.PicturesLeftOut != 2 {
		t.Fatalf("pictures left out = %d, want 2", out.PicturesLeftOut)
	}
	if !bytes.HasPrefix(parts["BinData/image1.png"], []byte("\x89PNG")) || parts["BinData/image2.png"] != nil {
		t.Fatalf("BinData = %v, want one PNG stored once", keys(parts, "BinData/"))
	}
	if !strings.Contains(string(parts["Contents/content.hpf"]), `<opf:item id="image1" href="BinData/image1.png" media-type="image/png" isEmbeded="1"/>`) {
		t.Fatal("content.hpf does not list the picture")
	}
	sec := string(parts["Contents/section0.xml"])
	if n := strings.Count(sec, `binaryItemIDRef="image1"`); n != 2 {
		t.Fatalf("picture drawn %d times, want 2", n)
	}
	// 150mm by 75mm in HWPUNIT.
	if !strings.Contains(sec, `<hp:sz width="42520" height="21260"`) {
		t.Fatalf("picture size not fitted to the page")
	}
	for _, want := range []string{"비 오는 골목", "[삽화: 웹피]", "차례"} {
		if !strings.Contains(sec, want) {
			t.Errorf("section lacks %q", want)
		}
	}
	if n := strings.Count(sec, `pageBreak="1"`); n != 2 {
		t.Fatalf("page breaks = %d, want 2", n)
	}
}

func TestTextExportIgnoresPictureEmbedding(t *testing.T) {
	out, err := func() (DocumentPayload, error) {
		_, req, run, _ := documentFixture(t)
		req.Options = DocumentOptions{IllustrationImages: true, Cover: true, Contents: true}
		return run(req)
	}()
	if err != nil || out.PicturesLeftOut != 0 {
		t.Fatalf("txt export = %+v, %v", out, err)
	}
}

func keys(parts map[string][]byte, prefix string) []string {
	var out []string
	for k := range parts {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out
}
