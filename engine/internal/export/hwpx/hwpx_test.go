package hwpx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/export/manuscriptir"
)

func sample(t *testing.T) manuscriptir.Doc {
	t.Helper()
	blocks, err := manuscriptir.FromDoc(`{"type":"doc","content":[
	 {"type":"paragraph","content":[
	   {"type":"mention","attrs":{"id":"entity-7f3a","label":"수아"}},
	   {"type":"text","text":"는 "},
	   {"type":"text","marks":[{"type":"bold"}],"text":"은빛"},
	   {"type":"text","text":" "},
	   {"type":"text","marks":[{"type":"italic"}],"text":"열쇠"},
	   {"type":"noteMarker","attrs":{"noteId":"note-secret-9"}},
	   {"type":"hardBreak"},
	   {"type":"text","text":"둘째\t줄 & <꺾쇠>"}
	 ]},
	 {"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"인용"}]}]},
	 {"type":"illustration","attrs":{"id":"ill-1","caption":"비 오는 음악실"}}
	]}`)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := manuscriptir.FromDoc(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"다음 회차"}]}]}`)
	return manuscriptir.Doc{Title: "작품 & 제목", Sections: []manuscriptir.Section{
		{Heading: "1부", Level: 1},
		{Heading: "1화", Level: 2, HoldsScenes: true, Blocks: append(blocks, manuscriptir.Block{Kind: manuscriptir.SceneBreak})},
		{Heading: "2화", Level: 2, HoldsScenes: true, Blocks: next},
	}}
}

func open(t *testing.T, data []byte) (*zip.Reader, map[string]string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[f.Name] = string(b)
	}
	return zr, files
}

// Hangul refuses a package that is not shaped the way it writes one: the
// mimetype first and stored, then the parts it expects, each well-formed.
func TestWriteKeepsHangulsPackageShape(t *testing.T) {
	data, err := Write(sample(t), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	zr, files := open(t, data)
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	want := []string{"mimetype", "version.xml", "Contents/header.xml", "Contents/section0.xml", "Preview/PrvText.txt",
		"settings.xml", "META-INF/container.rdf", "Contents/content.hpf", "META-INF/container.xml", "META-INF/manifest.xml"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("entries = %v", names)
	}
	if zr.File[0].Method != zip.Store || files["mimetype"] != MIME {
		t.Fatalf("mimetype entry: method %d, %q", zr.File[0].Method, files["mimetype"])
	}
	for _, name := range []string{"version.xml", "settings.xml", "META-INF/container.rdf", "META-INF/container.xml", "META-INF/manifest.xml"} {
		if files[name] != part(name[strings.LastIndex(name, "/")+1:]) {
			t.Errorf("%s differs from the template Hangul saved", name)
		}
	}
	for name, body := range files {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".hpf") && !strings.HasSuffix(name, ".rdf") {
			continue
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
	if !strings.Contains(files["Contents/content.hpf"], "<opf:title>작품 &amp; 제목</opf:title>") {
		t.Error("the work's title is not in content.hpf")
	}
}

// Every shape the body points at is defined, and each list's count says how
// many it holds; a dangling id is one more thing Hangul will not open.
func TestWriteDefinesEveryShapeTheBodyUses(t *testing.T) {
	data, err := Write(sample(t), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	_, files := open(t, data)
	header, section := files["Contents/header.xml"], files["Contents/section0.xml"]
	for _, list := range []struct{ name, item string }{{"charProperties", "charPr"}, {"paraProperties", "paraPr"}} {
		cnt := regexp.MustCompile(`<hh:` + list.name + ` itemCnt="(\d+)">`).FindStringSubmatch(header)
		n := len(regexp.MustCompile(`<hh:`+list.item+` id="`).FindAllString(header, -1))
		if cnt == nil || cnt[1] != strconv.Itoa(n) {
			t.Errorf("%s itemCnt %v, %d defined", list.name, cnt, n)
		}
	}
	defined := func(item, id string) bool {
		return strings.Contains(header, `<hh:`+item+` id="`+id+`"`)
	}
	for _, m := range regexp.MustCompile(`charPrIDRef="(\d+)"`).FindAllStringSubmatch(section, -1) {
		if !defined("charPr", m[1]) {
			t.Errorf("charPr %s is used but not defined", m[1])
		}
	}
	for _, m := range regexp.MustCompile(`paraPrIDRef="(\d+)"`).FindAllStringSubmatch(section, -1) {
		if !defined("paraPr", m[1]) {
			t.Errorf("paraPr %s is used but not defined", m[1])
		}
	}
	if strings.Count(section, "<hp:secPr ") != 1 || !strings.Contains(section[:strings.Index(section, "</hp:p>")], "<hp:secPr ") {
		t.Error("the section's page settings are not on the first paragraph, once")
	}
}

type hp struct {
	text      string
	pageBreak bool
	shapes    []string
}

// paragraphsOf reads section0.xml back: each paragraph's text (a line break
// as \n, a tab as \t), its page break, and the charPr of each run.
func paragraphsOf(t *testing.T, section string) []hp {
	t.Helper()
	var out []hp
	dec := xml.NewDecoder(strings.NewReader(section))
	inT := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			attr := func(n string) string {
				for _, a := range el.Attr {
					if a.Name.Local == n {
						return a.Value
					}
				}
				return ""
			}
			switch el.Name.Local {
			case "p":
				out = append(out, hp{pageBreak: attr("pageBreak") == "1"})
			case "run":
				out[len(out)-1].shapes = append(out[len(out)-1].shapes, attr("charPrIDRef"))
			case "t":
				inT = true
			case "lineBreak":
				out[len(out)-1].text += "\n"
			case "tab":
				out[len(out)-1].text += "\t"
			}
		case xml.EndElement:
			if el.Name.Local == "t" {
				inT = false
			}
		case xml.CharData:
			if inT {
				out[len(out)-1].text += string(el)
			}
		}
	}
	return out
}

func TestWriteKeepsTheTextMarksAndPages(t *testing.T) {
	opts := DefaultOptions()
	opts.Illustrations = true
	data, err := Write(sample(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	_, files := open(t, data)
	paras := paragraphsOf(t, files["Contents/section0.xml"])
	var texts []string
	for _, p := range paras {
		texts = append(texts, p.text)
	}
	want := []string{"1부", "1화", "수아는 은빛 열쇠\n둘째\t줄 & <꺾쇠>", "인용", "[삽화: 비 오는 음악실]", "* * *", "2화", "다음 회차"}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Fatalf("paragraphs:\n got %q\nwant %q", texts, want)
	}
	for i, p := range paras {
		if p.pageBreak != (i == 6) {
			t.Errorf("paragraph %d %q page break = %v; only 2화 starts a page (1부 shares one with 1화)", i, p.text, p.pageBreak)
		}
	}
	header := files["Contents/header.xml"]
	shape := func(id string) string {
		return regexp.MustCompile(`<hh:charPr id="` + id + `".*?</hh:charPr>`).FindString(header)
	}
	body := paras[2].shapes // 수아는 · 은빛 · " " · 열쇠 · break · 둘째…
	if !strings.Contains(shape(body[1]), "<hh:bold/>") || strings.Contains(shape(body[0]), "<hh:bold/>") {
		t.Errorf("은빛 is not the bold run: %v", body)
	}
	if !strings.Contains(shape(body[3]), "<hh:italic/>") {
		t.Errorf("열쇠 is not the italic run: %v", body)
	}
	if !strings.Contains(files["Preview/PrvText.txt"], "수아는 은빛 열쇠") {
		t.Errorf("preview = %q", files["Preview/PrvText.txt"])
	}
}

func TestWriteOptionsChangeTheLook(t *testing.T) {
	opts := DefaultOptions()
	opts.Font, opts.SizePt, opts.LineSpacingPct = "나눔명조", 11, 180
	opts.FirstLineIndent, opts.PageBreakPerSection = false, false
	data, err := Write(sample(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	_, files := open(t, data)
	header := files["Contents/header.xml"]
	if strings.Contains(header, `<hh:font id="1" face="함초롬바탕"`) || strings.Count(header, `<hh:font id="1" face="나눔명조"`) != 7 {
		t.Error("the body font was not swapped in every language")
	}
	if !strings.Contains(header, `height="1100"`) || !strings.Contains(header, `<hh:lineSpacing type="PERCENT" value="180"`) {
		t.Error("size or line spacing not applied")
	}
	if strings.Contains(files["Contents/section0.xml"], `pageBreak="1"`) {
		t.Error("a page break with page breaks off")
	}
	if strings.Contains(files["Contents/section0.xml"], "삽화") {
		t.Error("an illustration was written without being asked for")
	}
}
