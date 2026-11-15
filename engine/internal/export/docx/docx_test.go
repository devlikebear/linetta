package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/export/manuscriptir"
)

func sample(t *testing.T) manuscriptir.Doc {
	t.Helper()
	blocks, err := manuscriptir.FromDoc(`{"type":"doc","content":[
	 {"type":"paragraph","content":[
	   {"type":"mention","attrs":{"id":"entity-7f3a","label":"수아"}},
	   {"type":"text","text":"는 "},
	   {"type":"text","marks":[{"type":"bold"}],"text":"은빛 <열쇠> & "},
	   {"type":"noteMarker","attrs":{"noteId":"note-secret-9"}},
	   {"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com/?a=1&b=\"2\""}}],"text":"링크"},
	   {"type":"hardBreak"},
	   {"type":"text","text":"둘째 줄\u0001"}
	 ]},
	 {"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"인용"}]}]},
	 {"type":"illustration","attrs":{"id":"ill-1","caption":"비 오는 음악실"}}
	]}`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manuscriptir.FromDoc(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"다음 회차"}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	return manuscriptir.Doc{Title: "작품 & 제목", Sections: []manuscriptir.Section{
		{Heading: "1부", Level: 1},
		{Heading: "1화", Level: 2, HoldsScenes: true, Blocks: append(blocks, manuscriptir.Block{Kind: manuscriptir.SceneBreak})},
		{Heading: "2화", Level: 2, HoldsScenes: true, Blocks: second},
	}}
}

func unzip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}

// paragraphs reads document.xml back the way a reader would: each w:p's
// w:t text, w:br as a newline, plus the paragraph's style and page break.
func paragraphs(t *testing.T, doc string) []struct {
	style, text string
	pageBreak   bool
} {
	t.Helper()
	var out []struct {
		style, text string
		pageBreak   bool
	}
	dec := xml.NewDecoder(strings.NewReader(doc))
	var cur *struct {
		style, text string
		pageBreak   bool
	}
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("document.xml is not well-formed: %v", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "p":
				out = append(out, struct {
					style, text string
					pageBreak   bool
				}{})
				cur = &out[len(out)-1]
			case "pStyle":
				cur.style = el.Attr[0].Value
			case "pageBreakBefore":
				cur.pageBreak = true
			case "br":
				cur.text += "\n"
			case "t":
				inText = true
			}
		case xml.EndElement:
			if el.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				cur.text += string(el)
			}
		}
	}
	return out
}

func TestWriteMakesAWellFormedPackage(t *testing.T) {
	data, err := Write(sample(t), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, data)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml",
		"word/settings.xml", "word/_rels/document.xml.rels", "docProps/core.xml", "docProps/app.xml"} {
		body, ok := files[name]
		if !ok {
			t.Fatalf("package lacks %s", name)
		}
		dec := xml.NewDecoder(strings.NewReader(body))
		for {
			if _, err := dec.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s is not well-formed: %v", name, err)
			}
		}
		for _, leaked := range []string{"entity-7f3a", "note-secret-9", "ill-1"} {
			if strings.Contains(body, leaked) {
				t.Errorf("%s carries %q", name, leaked)
			}
		}
	}
	if !strings.Contains(files["word/styles.xml"], `w:eastAsia="함초롬바탕"`) ||
		!strings.Contains(files["word/styles.xml"], `<w:sz w:val="20"/>`) ||
		!strings.Contains(files["word/styles.xml"], `w:line="384"`) {
		t.Errorf("styles do not carry the font, 10pt and 160%%:\n%s", files["word/styles.xml"])
	}
	if !strings.Contains(files["word/_rels/document.xml.rels"], `Target="https://example.com/?a=1&amp;b=&#34;2&#34;" TargetMode="External"`) {
		t.Errorf("hyperlink relationship:\n%s", files["word/_rels/document.xml.rels"])
	}
	if !strings.Contains(files["docProps/core.xml"], "<dc:title>작품 &amp; 제목</dc:title>") {
		t.Errorf("core title:\n%s", files["docProps/core.xml"])
	}
}

func TestWriteKeepsTheTextStylesAndPages(t *testing.T) {
	opts := DefaultOptions()
	opts.Created = time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	data, err := Write(sample(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	got := paragraphs(t, unzip(t, data)["word/document.xml"])
	want := []struct {
		style, text string
		pageBreak   bool
	}{
		{"LinettaHeading1", "1부", false},
		{"LinettaHeading2", "1화", false},
		{"LinettaBody", "수아는 은빛 <열쇠> & 링크\n둘째 줄", false},
		{"LinettaQuote", "인용", false},
		{"LinettaSceneBreak", "* * *", false},
		{"LinettaHeading2", "2화", true},
		{"LinettaBody", "다음 회차", false},
	}
	if len(got) != len(want) {
		t.Fatalf("paragraphs:\n got %+v\nwant %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("paragraph %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestWriteOptionsChangeTheLayout(t *testing.T) {
	opts := DefaultOptions()
	opts.PageBreakPerSection = false
	opts.Illustrations = true
	opts.FirstLineIndent = false
	opts.SceneBreak = "◇"
	data, err := Write(sample(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, data)
	got := paragraphs(t, files["word/document.xml"])
	var texts []string
	for _, p := range got {
		if p.pageBreak {
			t.Errorf("page break with page breaks off: %+v", p)
		}
		texts = append(texts, p.text)
	}
	joined := strings.Join(texts, "|")
	if !strings.Contains(joined, "[삽화: 비 오는 음악실]") || !strings.Contains(joined, "◇") {
		t.Errorf("paragraphs: %s", joined)
	}
	if strings.Contains(files["word/styles.xml"], "firstLineChars=\"100\"") {
		t.Error("first-line indent written while off")
	}
}
