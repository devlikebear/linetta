// Package hwpx writes a manuscriptir.Doc as a Hangul document (#383 Phase 2):
// an OWPML (KS X 6101) package Hancom Office Hangul opens. Hangul is strict
// about a package's shape, so the writer does not compose one from the spec:
// it starts from a document Hangul itself saved (template/, see NOTICE.md),
// keeps every part as it is, appends the character and paragraph shapes the
// manuscript needs to that document's header, and replaces only its body.
//
// Only archive/zip and the standard library: the engine stays pure Go.
package hwpx

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/export/manuscriptir"
)

// MIME is the type a .hwpx is saved with, and the content of its mimetype entry.
const MIME = "application/hwp+zip"

//go:embed template/*.xml template/*.rdf
var templateFS embed.FS

func part(name string) string {
	b, err := templateFS.ReadFile("template/" + name)
	if err != nil {
		panic(fmt.Sprintf("hwpx: template part %s: %v", name, err))
	}
	return string(b)
}

// Options is how the document looks; the zero value is not usable, start from
// DefaultOptions.
type Options struct {
	Font                string
	SizePt              float64
	LineSpacingPct      int
	FirstLineIndent     bool
	PageBreakPerSection bool
	SceneBreak          string
	Illustrations       bool
	IllustrationLabel   string
	Created             time.Time
}

// DefaultOptions are Hangul's own defaults for a Korean manuscript: 함초롬바탕
// 10pt, 160% line spacing, first lines indented, each episode on a new page.
func DefaultOptions() Options {
	return Options{
		Font: templateFont, SizePt: 10, LineSpacingPct: 160, FirstLineIndent: true,
		PageBreakPerSection: true, SceneBreak: "* * *", IllustrationLabel: "삽화",
	}
}

// templateFont is the template's body font (font id 1 in every language).
const templateFont = "함초롬바탕"

// Write renders doc as a .hwpx.
func Write(doc manuscriptir.Doc, opts Options) ([]byte, error) {
	if opts.Created.IsZero() {
		opts.Created = time.Now()
	}
	w := &writer{opts: opts, base: int(opts.SizePt*100 + 0.5), chars: map[charShape]int{}, paras: map[paraShape]int{}}
	w.charBase = w.countIn(headerTemplate, "charProperties")
	w.paraBase = w.countIn(headerTemplate, "paraProperties")
	body, preview := w.body(doc)
	header, err := w.header()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// The mimetype entry comes first and is stored, not deflated: a reader
	// identifies the package by reading it at a fixed offset.
	mt, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, err
	}
	if _, err := mt.Write([]byte(MIME)); err != nil {
		return nil, err
	}
	// The rest in the order Hangul writes them.
	for _, f := range []struct{ name, data string }{
		{"version.xml", part("version.xml")},
		{"Contents/header.xml", header},
		{"Contents/section0.xml", body},
		{"Preview/PrvText.txt", preview},
		{"settings.xml", part("settings.xml")},
		{"META-INF/container.rdf", part("container.rdf")},
		{"Contents/content.hpf", contentHPF(doc.Title, opts.Created)},
		{"META-INF/container.xml", part("container.xml")},
		{"META-INF/manifest.xml", part("manifest.xml")},
	} {
		fw, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write([]byte(f.data)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var headerTemplate = part("header.xml")

type charShape struct {
	height                          int
	bold, italic, underline, strike bool
}

type paraShape struct {
	align                                string // JUSTIFY, CENTER, LEFT
	indent, left, right, prev, next, pct int
	keepWithNext                         bool
}

type writer struct {
	opts               Options
	base               int // body text height, in hundredths of a point
	charBase, paraBase int // how many shapes the template header already defines
	chars              map[charShape]int
	paras              map[paraShape]int
}

func (w *writer) countIn(header, list string) int {
	m := regexp.MustCompile(`<hh:` + list + ` itemCnt="(\d+)">`).FindStringSubmatch(header)
	if m == nil {
		panic("hwpx: template header has no " + list)
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func (w *writer) charID(s charShape) int {
	if id, ok := w.chars[s]; ok {
		return id
	}
	id := w.charBase + len(w.chars)
	w.chars[s] = id
	return id
}

func (w *writer) paraID(s paraShape) int {
	if id, ok := w.paras[s]; ok {
		return id
	}
	id := w.paraBase + len(w.paras)
	w.paras[s] = id
	return id
}

// The paragraph kinds the manuscript uses, as Hangul shapes. Lengths are
// HWPUNIT, a hundredth of a point.
func (w *writer) bodyPara() paraShape {
	s := paraShape{align: "JUSTIFY", pct: w.opts.LineSpacingPct}
	if w.opts.FirstLineIndent {
		s.indent = w.base // one character of the body size
	}
	return s
}

func (w *writer) headingPara() paraShape {
	return paraShape{align: "LEFT", prev: 1200, next: 600, pct: w.opts.LineSpacingPct, keepWithNext: true}
}

func (w *writer) centerPara() paraShape {
	return paraShape{align: "CENTER", prev: 600, next: 600, pct: w.opts.LineSpacingPct}
}

func (w *writer) quotePara() paraShape {
	return paraShape{align: "JUSTIFY", left: 2000, right: 2000, pct: w.opts.LineSpacingPct}
}

func (w *writer) listPara(level int) paraShape {
	return paraShape{align: "LEFT", left: 1500 * max(level, 1), pct: w.opts.LineSpacingPct}
}

type paragraph struct {
	para      int
	pageBreak bool
	runs      []manuscriptir.Run
	charOf    func(manuscriptir.Run) charShape
}

func (w *writer) body(doc manuscriptir.Doc) (section, preview string) {
	plain := func(r manuscriptir.Run) charShape {
		return charShape{height: w.base, bold: r.Bold, italic: r.Italic, underline: r.Underline || r.Href != "", strike: r.Strike}
	}
	heading := func(level int) func(manuscriptir.Run) charShape {
		bump := map[int]int{1: 600, 2: 300, 3: 100}[min(max(level, 1), 3)]
		return func(r manuscriptir.Run) charShape {
			s := plain(r)
			s.height, s.bold = w.base+bump, true
			return s
		}
	}
	quote := func(r manuscriptir.Run) charShape {
		s := plain(r)
		s.italic = true
		return s
	}

	var paras []paragraph
	var lines []string
	started, headingOnly := false, false
	for _, sec := range doc.Sections {
		breakBefore := false
		if w.opts.PageBreakPerSection && (sec.HoldsScenes || sec.Heading != "") {
			// Not before the first, nor right after a heading with nothing
			// under it: a part's title shares the page with its first episode.
			breakBefore = started && !headingOnly
			started = true
		}
		headingOnly = sec.Heading != "" && len(sec.Blocks) == 0
		add := func(p paragraph) {
			if breakBefore {
				p.pageBreak, breakBefore = true, false
			}
			paras = append(paras, p)
			var sb strings.Builder
			for _, r := range p.runs {
				if r.Break {
					sb.WriteString(" ")
				} else {
					sb.WriteString(r.Text)
				}
			}
			lines = append(lines, sb.String())
		}
		if sec.Heading != "" {
			add(paragraph{para: w.paraID(w.headingPara()), runs: []manuscriptir.Run{{Text: sec.Heading}}, charOf: heading(sec.Level)})
		}
		for _, b := range sec.Blocks {
			switch b.Kind {
			case manuscriptir.Heading:
				add(paragraph{para: w.paraID(w.headingPara()), runs: b.Runs, charOf: heading(b.Level)})
			case manuscriptir.Quote:
				add(paragraph{para: w.paraID(w.quotePara()), runs: b.Runs, charOf: quote})
			case manuscriptir.SceneBreak:
				add(paragraph{para: w.paraID(w.centerPara()), runs: []manuscriptir.Run{{Text: w.opts.SceneBreak}}, charOf: plain})
			case manuscriptir.ListItem:
				runs := b.Runs
				if b.Marker != "" {
					runs = append([]manuscriptir.Run{{Text: b.Marker + " "}}, runs...)
				}
				add(paragraph{para: w.paraID(w.listPara(b.Level)), runs: runs, charOf: plain})
			case manuscriptir.CodeLine:
				add(paragraph{para: w.paraID(paraShape{align: "LEFT", pct: w.opts.LineSpacingPct}), runs: b.Runs, charOf: plain})
			case manuscriptir.IllustrationPlaceholder:
				if !w.opts.Illustrations {
					continue
				}
				text := fmt.Sprintf("[%s]", w.opts.IllustrationLabel)
				if caption := strings.TrimSpace(b.Text()); caption != "" {
					text = fmt.Sprintf("[%s: %s]", w.opts.IllustrationLabel, caption)
				}
				add(paragraph{para: w.paraID(w.centerPara()), runs: []manuscriptir.Run{{Text: text}}, charOf: plain})
			default:
				add(paragraph{para: w.paraID(w.bodyPara()), runs: b.Runs, charOf: plain})
			}
		}
	}
	if len(paras) == 0 {
		paras = append(paras, paragraph{para: w.paraID(w.bodyPara()), charOf: plain})
	}

	tmpl := part("section0.xml")
	open := tmpl[:strings.Index(tmpl, "<hs:sec ")]
	open += tmpl[len(open) : strings.Index(tmpl[len(open):], ">")+len(open)+1]
	secStart := strings.Index(tmpl, `<hp:run charPrIDRef="0"><hp:secPr`)
	secEnd := strings.Index(tmpl, `</hp:ctrl></hp:run>`) + len(`</hp:ctrl></hp:run>`)
	if secStart < 0 || secEnd < secStart {
		panic("hwpx: template section has no section properties")
	}
	secPr := tmpl[secStart:secEnd]

	var b strings.Builder
	b.WriteString(open)
	for i, p := range paras {
		pb := "0"
		if p.pageBreak {
			pb = "1"
		}
		fmt.Fprintf(&b, `<hp:p id="%d" paraPrIDRef="%d" styleIDRef="0" pageBreak="%s" columnBreak="0" merged="0">`, i, p.para, pb)
		if i == 0 {
			// The section's page settings ride on the first paragraph, as
			// Hangul writes them.
			b.WriteString(secPr)
		}
		wrote := false
		for _, r := range p.runs {
			if r.Text == "" && !r.Break {
				continue
			}
			fmt.Fprintf(&b, `<hp:run charPrIDRef="%d"><hp:t>`, w.charID(p.charOf(r)))
			if r.Break {
				b.WriteString(`<hp:lineBreak/>`)
			} else {
				b.WriteString(textXML(r.Text))
			}
			b.WriteString(`</hp:t></hp:run>`)
			wrote = true
		}
		if !wrote {
			fmt.Fprintf(&b, `<hp:run charPrIDRef="%d"><hp:t/></hp:run>`, w.charID(charShape{height: w.base}))
		}
		b.WriteString(`</hp:p>`)
	}
	b.WriteString(`</hs:sec>`)

	// Preview/PrvText.txt: the opening text, as the file browser shows it.
	pv := strings.Join(lines, "\r\n")
	if r := []rune(pv); len(r) > 1000 {
		pv = string(r[:1000])
	}
	return b.String(), pv + "\r\n"
}

// textXML escapes text for an hp:t element. A tab becomes Hangul's tab
// element; a control character XML cannot hold is dropped, and a stray line
// break inside a run reads as a space.
func textXML(s string) string {
	var out strings.Builder
	parts := strings.Split(s, "\t")
	for i, piece := range parts {
		if i > 0 {
			out.WriteString(`<hp:tab/>`)
		}
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(strings.Map(func(r rune) rune {
			switch {
			case r == '\n' || r == '\r':
				return ' '
			case r < 0x20:
				return -1
			}
			return r
		}, piece)))
		out.Write(b.Bytes())
	}
	return out.String()
}

var (
	charPrTemplate = regexp.MustCompile(`(?s)<hh:charPr id="0".*?</hh:charPr>`).FindString(headerTemplate)
	paraPrTemplate = regexp.MustCompile(`(?s)<hh:paraPr id="0".*?</hh:paraPr>`).FindString(headerTemplate)
)

// header is the template's header with the writer's shapes appended and the
// body font swapped in.
func (w *writer) header() (string, error) {
	if charPrTemplate == "" || paraPrTemplate == "" {
		return "", fmt.Errorf("hwpx: template header has no shape 0 to copy")
	}
	h := headerTemplate
	if f := strings.TrimSpace(w.opts.Font); f != "" && f != templateFont {
		h = strings.ReplaceAll(h, `<hh:font id="1" face="`+templateFont+`"`, `<hh:font id="1" face="`+attr(f)+`"`)
	}

	type entry struct {
		id  int
		xml string
	}
	var chars []entry
	for s, id := range w.chars {
		chars = append(chars, entry{id, charPr(id, s)})
	}
	var paras []entry
	for s, id := range w.paras {
		paras = append(paras, entry{id, paraPr(id, s)})
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i].id < chars[j].id })
	sort.Slice(paras, func(i, j int) bool { return paras[i].id < paras[j].id })

	appendTo := func(h, list string, base int, items []entry) string {
		var b strings.Builder
		for _, e := range items {
			b.WriteString(e.xml)
		}
		h = strings.Replace(h, `</hh:`+list+`>`, b.String()+`</hh:`+list+`>`, 1)
		return strings.Replace(h, fmt.Sprintf(`<hh:%s itemCnt="%d">`, list, base),
			fmt.Sprintf(`<hh:%s itemCnt="%d">`, list, base+len(items)), 1)
	}
	h = appendTo(h, "charProperties", w.charBase, chars)
	h = appendTo(h, "paraProperties", w.paraBase, paras)
	return h, nil
}

func charPr(id int, s charShape) string {
	x := strings.Replace(charPrTemplate, `<hh:charPr id="0" height="1000"`, fmt.Sprintf(`<hh:charPr id="%d" height="%d"`, id, s.height), 1)
	var marks string
	if s.italic {
		marks += `<hh:italic/>`
	}
	if s.bold {
		marks += `<hh:bold/>`
	}
	if marks != "" {
		// In the schema's order: after offset, before underline.
		x = strings.Replace(x, `<hh:underline `, marks+`<hh:underline `, 1)
	}
	if s.underline {
		x = strings.Replace(x, `<hh:underline type="NONE"`, `<hh:underline type="BOTTOM"`, 1)
	}
	if s.strike {
		x = strings.Replace(x, `<hh:strikeout shape="NONE"`, `<hh:strikeout shape="SOLID"`, 1)
	}
	return x
}

func paraPr(id int, s paraShape) string {
	x := strings.Replace(paraPrTemplate, `<hh:paraPr id="0"`, fmt.Sprintf(`<hh:paraPr id="%d"`, id), 1)
	x = strings.Replace(x, `horizontal="JUSTIFY"`, `horizontal="`+s.align+`"`, 1)
	if s.keepWithNext {
		x = strings.Replace(x, `keepWithNext="0"`, `keepWithNext="1"`, 1)
	}
	// The margins and line spacing appear twice, in the switch's case and
	// its default; both say the same.
	for _, m := range []struct {
		name  string
		value int
	}{{"intent", s.indent}, {"left", s.left}, {"right", s.right}, {"prev", s.prev}, {"next", s.next}} {
		x = strings.ReplaceAll(x, `<hc:`+m.name+` value="0" unit="HWPUNIT"/>`,
			fmt.Sprintf(`<hc:%s value="%d" unit="HWPUNIT"/>`, m.name, m.value))
	}
	return strings.ReplaceAll(x, `<hh:lineSpacing type="PERCENT" value="160"`,
		fmt.Sprintf(`<hh:lineSpacing type="PERCENT" value="%d"`, s.pct))
}

func attr(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// contentHPF is the package manifest, with the work's title and today's date.
func contentHPF(title string, created time.Time) string {
	stamp := created.UTC().Format("2006-01-02T15:04:05Z")
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>` +
		`<opf:package xmlns:ha="http://www.hancom.co.kr/hwpml/2011/app" xmlns:hp="http://www.hancom.co.kr/hwpml/2011/paragraph" ` +
		`xmlns:hp10="http://www.hancom.co.kr/hwpml/2016/paragraph" xmlns:hs="http://www.hancom.co.kr/hwpml/2011/section" ` +
		`xmlns:hc="http://www.hancom.co.kr/hwpml/2011/core" xmlns:hh="http://www.hancom.co.kr/hwpml/2011/head" ` +
		`xmlns:hhs="http://www.hancom.co.kr/hwpml/2011/history" xmlns:hm="http://www.hancom.co.kr/hwpml/2011/master-page" ` +
		`xmlns:hpf="http://www.hancom.co.kr/schema/2011/hpf" xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:opf="http://www.idpf.org/2007/opf/" xmlns:ooxmlchart="http://www.hancom.co.kr/hwpml/2016/ooxmlchart" ` +
		`xmlns:hwpunitchar="http://www.hancom.co.kr/hwpml/2016/HwpUnitChar" xmlns:epub="http://www.idpf.org/2007/ops" ` +
		`xmlns:config="urn:oasis:names:tc:opendocument:xmlns:config:1.0" version="" unique-identifier="" id="">` +
		`<opf:metadata><opf:title>` + attr(title) + `</opf:title><opf:language>ko</opf:language>` +
		`<opf:meta name="creator" content="text"/><opf:meta name="subject" content="text"/>` +
		`<opf:meta name="description" content="text"/><opf:meta name="lastsaveby" content="text">Linetta</opf:meta>` +
		`<opf:meta name="CreatedDate" content="text">` + stamp + `</opf:meta>` +
		`<opf:meta name="ModifiedDate" content="text">` + stamp + `</opf:meta>` +
		`<opf:meta name="date" content="text">` + stamp + `</opf:meta><opf:meta name="keyword" content="text"/></opf:metadata>` +
		`<opf:manifest><opf:item id="header" href="Contents/header.xml" media-type="application/xml"/>` +
		`<opf:item id="section0" href="Contents/section0.xml" media-type="application/xml"/>` +
		`<opf:item id="settings" href="settings.xml" media-type="application/xml"/></opf:manifest>` +
		`<opf:spine><opf:itemref idref="header" linear="yes"/><opf:itemref idref="section0" linear="yes"/></opf:spine></opf:package>`
}
