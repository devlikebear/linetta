// Package docx writes a manuscriptir.Doc as a Word document (#383): a minimal
// OOXML package Word, Pages, LibreOffice and Hangul open as is. It uses only
// archive/zip and the standard library, so the engine stays pure Go.
//
// Every paragraph carries a named style (LinettaBody, LinettaHeading1-3,
// LinettaQuote, LinettaSceneBreak, LinettaCode), so whoever receives the file
// changes the look of the whole manuscript by changing a style once.
package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/export/manuscriptir"
)

// MIME is the type a .docx is saved with.
const MIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// Options is how the document looks. The zero value is not usable; start from
// DefaultOptions.
type Options struct {
	// Font is the typeface for body and headings, East Asian text included.
	Font string
	// SizePt is the body text size in points.
	SizePt float64
	// LineSpacingPct is the body's line spacing, 100 for single.
	LineSpacingPct int
	// FirstLineIndent indents each body paragraph's first line by one
	// character, as Korean manuscripts do.
	FirstLineIndent bool
	// PageBreakPerSection starts each section that holds scenes (an episode,
	// usually) on a new page.
	PageBreakPerSection bool
	// SceneBreak is the line written between scenes; "" leaves a blank line.
	SceneBreak string
	// Illustrations writes "[삽화: caption]" where a picture is; otherwise a
	// picture is left out.
	Illustrations bool
	// IllustrationLabel is the word before an illustration's caption.
	IllustrationLabel string
	// Pictures are the gallery pictures to embed, by id. An illustration
	// whose picture is here is drawn, its caption under it; one whose
	// picture is not falls back to Illustrations' placeholder.
	Pictures map[string]manuscriptir.Picture
	// Cover starts the document with a title page.
	Cover bool
	// Contents adds a table of contents, the outline's headings down to
	// the second level, on its own page; ContentsLabel is its heading.
	Contents      bool
	ContentsLabel string
	// Created stamps the document's creation time; zero means now.
	Created time.Time
}

// DefaultOptions follows Korean manuscript practice: 함초롬바탕 10pt, 160%
// line spacing, first-line indent, each episode on a new page.
func DefaultOptions() Options {
	return Options{
		Font: "함초롬바탕", SizePt: 10, LineSpacingPct: 160, FirstLineIndent: true,
		PageBreakPerSection: true, SceneBreak: "* * *", IllustrationLabel: "삽화", ContentsLabel: "차례",
	}
}

// Write renders doc as a .docx.
func Write(doc manuscriptir.Doc, opts Options) ([]byte, error) {
	if opts.Created.IsZero() {
		opts.Created = time.Now()
	}
	w := &writer{opts: opts}
	body := w.body(doc)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := []struct{ name, data string }{
		{"[Content_Types].xml", w.contentTypes()},
		{"_rels/.rels", rootRels},
		{"word/document.xml", documentXML(body)},
		{"word/styles.xml", w.styles()},
		{"word/settings.xml", settingsXML},
		{"word/_rels/document.xml.rels", w.documentRels()},
		{"docProps/core.xml", coreXML(doc.Title, opts.Created)},
		{"docProps/app.xml", appXML},
	}
	for _, f := range files {
		fw, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write([]byte(f.data)); err != nil {
			return nil, err
		}
	}
	for _, m := range w.media {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: "word/media/" + m.name, Method: zip.Store})
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write(m.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type writer struct {
	opts  Options
	links []string
	// media is each embedded picture once, in first-use order; mediaRel its
	// relationship id by gallery id.
	media    []mediaFile
	mediaRel map[string]string
	drawings int
}

type mediaFile struct {
	name, rel string
	data      []byte
}

func (w *writer) body(doc manuscriptir.Doc) string {
	var b strings.Builder
	started, headingOnly := false, false
	// The cover and the contents each fill a page; the manuscript starts
	// on the next one.
	pendingBreak := false
	if w.opts.Cover {
		w.paragraph(&b, "LinettaTitle", false, []manuscriptir.Run{{Text: doc.Title}})
		pendingBreak = true
	}
	if entries := doc.Contents(2); w.opts.Contents && len(entries) > 0 {
		w.paragraph(&b, "LinettaHeading1", pendingBreak, []manuscriptir.Run{{Text: w.opts.ContentsLabel}})
		for _, e := range entries {
			w.paragraphIndented(&b, "LinettaContents", false, []manuscriptir.Run{{Text: e.Heading}}, e.Level-1)
		}
		pendingBreak = true
	}
	for _, sec := range doc.Sections {
		breakBefore := false
		if w.opts.PageBreakPerSection && (sec.HoldsScenes || sec.Heading != "") {
			// Not before the first, and not right after a heading with nothing
			// under it: a part's title shares the page with its first episode.
			breakBefore = started && !headingOnly
			started = true
		}
		headingOnly = sec.Heading != "" && len(sec.Blocks) == 0
		if pendingBreak {
			breakBefore, pendingBreak = true, false
		}
		if sec.Heading != "" {
			style := fmt.Sprintf("LinettaHeading%d", clamp(sec.Level, 1, 3))
			w.paragraph(&b, style, breakBefore, []manuscriptir.Run{{Text: sec.Heading}})
			breakBefore = false
		}
		for _, blk := range sec.Blocks {
			if w.block(&b, blk, breakBefore) {
				breakBefore = false
			}
		}
	}
	return b.String()
}

// block writes one block and reports whether it wrote a paragraph.
func (w *writer) block(b *strings.Builder, blk manuscriptir.Block, breakBefore bool) bool {
	switch blk.Kind {
	case manuscriptir.Heading:
		w.paragraph(b, fmt.Sprintf("LinettaHeading%d", clamp(blk.Level, 1, 3)), breakBefore, blk.Runs)
	case manuscriptir.Quote:
		w.paragraph(b, "LinettaQuote", breakBefore, blk.Runs)
	case manuscriptir.SceneBreak:
		w.paragraph(b, "LinettaSceneBreak", breakBefore, []manuscriptir.Run{{Text: w.opts.SceneBreak}})
	case manuscriptir.ListItem:
		runs := blk.Runs
		if blk.Marker != "" {
			runs = append([]manuscriptir.Run{{Text: blk.Marker + " "}}, runs...)
		}
		w.paragraphIndented(b, "LinettaList", breakBefore, runs, blk.Level)
	case manuscriptir.CodeLine:
		w.paragraph(b, "LinettaCode", breakBefore, blk.Runs)
	case manuscriptir.IllustrationPlaceholder:
		if pic, ok := w.opts.Pictures[blk.ImageID]; ok && len(pic.Data) > 0 {
			w.picture(b, blk.ImageID, pic, blk.Text(), breakBefore)
			if caption := strings.TrimSpace(blk.Text()); caption != "" {
				w.paragraph(b, "LinettaCaption", false, []manuscriptir.Run{{Text: caption}})
			}
			return true
		}
		if !w.opts.Illustrations {
			return false
		}
		text := fmt.Sprintf("[%s: %s]", w.opts.IllustrationLabel, blk.Text())
		if strings.TrimSpace(blk.Text()) == "" {
			text = fmt.Sprintf("[%s]", w.opts.IllustrationLabel)
		}
		w.paragraph(b, "LinettaSceneBreak", breakBefore, []manuscriptir.Run{{Text: text}})
	default:
		w.paragraph(b, "LinettaBody", breakBefore, blk.Runs)
	}
	return true
}

// The text block of an A4 page with the margins documentXML sets, less a
// little, in millimetres: the most a picture may take.
const (
	pictureMaxWMM = 150
	pictureMaxHMM = 200
	emuPerMM      = 36000
)

// picture writes a centred paragraph holding the picture inline, adding its
// bytes to the package the first time it is used.
func (w *writer) picture(b *strings.Builder, id string, pic manuscriptir.Picture, caption string, breakBefore bool) {
	rel, ok := w.mediaRel[id]
	if !ok {
		if w.mediaRel == nil {
			w.mediaRel = map[string]string{}
		}
		rel = fmt.Sprintf("rIdImage%d", len(w.media)+1)
		w.mediaRel[id] = rel
		w.media = append(w.media, mediaFile{name: fmt.Sprintf("image%d.%s", len(w.media)+1, pic.Ext), rel: rel, data: pic.Data})
	}
	w.drawings++
	n := w.drawings
	wmm, hmm := pic.SizeMM(pictureMaxWMM, pictureMaxHMM)
	cx, cy := int(wmm*emuPerMM), int(hmm*emuPerMM)
	b.WriteString(`<w:p><w:pPr><w:pStyle w:val="LinettaPicture"/>`)
	if breakBefore {
		b.WriteString(`<w:pageBreakBefore/>`)
	}
	b.WriteString(`</w:pPr><w:r><w:drawing>`)
	fmt.Fprintf(b, `<wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/>`, cx, cy)
	fmt.Fprintf(b, `<wp:docPr id="%d" name="Picture %d" descr="%s"/>`, n, n, escapeAttr(caption))
	b.WriteString(`<wp:cNvGraphicFramePr><a:graphicFrameLocks xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" noChangeAspect="1"/></wp:cNvGraphicFramePr>`)
	b.WriteString(`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`)
	b.WriteString(`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">`)
	fmt.Fprintf(b, `<pic:nvPicPr><pic:cNvPr id="%d" name="Picture %d"/><pic:cNvPicPr/></pic:nvPicPr>`, n, n)
	fmt.Fprintf(b, `<pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`, rel)
	fmt.Fprintf(b, `<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`, cx, cy)
	b.WriteString(`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`)
}

func (w *writer) paragraph(b *strings.Builder, style string, breakBefore bool, runs []manuscriptir.Run) {
	w.paragraphIndented(b, style, breakBefore, runs, 0)
}

func (w *writer) paragraphIndented(b *strings.Builder, style string, breakBefore bool, runs []manuscriptir.Run, level int) {
	b.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + style + `"/>`)
	if breakBefore {
		b.WriteString(`<w:pageBreakBefore/>`)
	}
	if level > 0 {
		fmt.Fprintf(b, `<w:ind w:left="%d" w:hanging="360"/>`, 360*level)
	}
	b.WriteString(`</w:pPr>`)
	for _, r := range runs {
		w.run(b, r)
	}
	b.WriteString(`</w:p>`)
}

func (w *writer) run(b *strings.Builder, r manuscriptir.Run) {
	if r.Break {
		b.WriteString(`<w:r><w:br/></w:r>`)
		return
	}
	if r.Text == "" {
		return
	}
	if r.Href != "" {
		w.links = append(w.links, r.Href)
		fmt.Fprintf(b, `<w:hyperlink r:id="rIdLink%d">`, len(w.links))
	}
	b.WriteString(`<w:r>`)
	var props strings.Builder
	if r.Href != "" {
		props.WriteString(`<w:rStyle w:val="Hyperlink"/>`)
	}
	if r.Code {
		props.WriteString(`<w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/>`)
	}
	if r.Bold {
		props.WriteString(`<w:b/><w:bCs/>`)
	}
	if r.Italic {
		props.WriteString(`<w:i/><w:iCs/>`)
	}
	if r.Strike {
		props.WriteString(`<w:strike/>`)
	}
	if r.Underline || r.Href != "" {
		props.WriteString(`<w:u w:val="single"/>`)
	}
	if props.Len() > 0 {
		b.WriteString(`<w:rPr>` + props.String() + `</w:rPr>`)
	}
	b.WriteString(`<w:t xml:space="preserve">`)
	b.WriteString(escape(r.Text))
	b.WriteString(`</w:t></w:r>`)
	if r.Href != "" {
		b.WriteString(`</w:hyperlink>`)
	}
}

func escape(s string) string {
	var b bytes.Buffer
	// A control character XML 1.0 cannot hold would make the whole document
	// unreadable; a stray line break inside a run reads as a space, anything
	// else is dropped.
	_ = xml.EscapeText(&b, []byte(strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r':
			return ' '
		case r < 0x20 && r != '\t':
			return -1
		}
		return r
	}, s)))
	return b.String()
}

func clamp(v, lo, hi int) int {
	return max(lo, min(hi, v))
}

func (w *writer) styles() string {
	font := escapeAttr(w.opts.Font)
	half := int(w.opts.SizePt*2 + 0.5)
	line := w.opts.LineSpacingPct * 240 / 100
	indent := ""
	if w.opts.FirstLineIndent {
		// One character of the body size: firstLineChars is in hundredths of
		// a character, and firstLine (twentieths of a point) is for readers
		// that ignore it.
		indent = fmt.Sprintf(`<w:ind w:firstLineChars="100" w:firstLine="%d"/>`, int(w.opts.SizePt*20+0.5))
	}
	fonts := fmt.Sprintf(`<w:rFonts w:ascii="%[1]s" w:hAnsi="%[1]s" w:eastAsia="%[1]s" w:cs="%[1]s"/>`, font)
	heading := func(id string, n int, size int) string {
		return fmt.Sprintf(`<w:style w:type="paragraph" w:styleId="%[1]s"><w:name w:val="Linetta Heading %[2]d"/><w:basedOn w:val="LinettaBody"/><w:next w:val="LinettaBody"/><w:qFormat/>`+
			`<w:pPr><w:keepNext/><w:spacing w:before="480" w:after="240"/><w:ind w:firstLine="0" w:firstLineChars="0"/><w:outlineLvl w:val="%[3]d"/></w:pPr>`+
			`<w:rPr><w:b/><w:bCs/><w:sz w:val="%[4]d"/><w:szCs w:val="%[4]d"/></w:rPr></w:style>`, id, n, n-1, size)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:docDefaults><w:rPrDefault><w:rPr>` + fonts +
		fmt.Sprintf(`<w:sz w:val="%[1]d"/><w:szCs w:val="%[1]d"/><w:lang w:val="ko-KR" w:eastAsia="ko-KR"/>`, half) +
		`</w:rPr></w:rPrDefault><w:pPrDefault><w:pPr>` +
		fmt.Sprintf(`<w:spacing w:after="0" w:line="%d" w:lineRule="auto"/>`, line) +
		`</w:pPr></w:pPrDefault></w:docDefaults>` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="LinettaBody"><w:name w:val="Linetta Body"/><w:qFormat/><w:pPr>` +
		`<w:jc w:val="both"/>` + indent + `</w:pPr></w:style>` +
		heading("LinettaHeading1", 1, half+12) +
		heading("LinettaHeading2", 2, half+6) +
		heading("LinettaHeading3", 3, half+2) +
		`<w:style w:type="paragraph" w:styleId="LinettaQuote"><w:name w:val="Linetta Quote"/><w:basedOn w:val="LinettaBody"/>` +
		`<w:pPr><w:ind w:left="720" w:right="720" w:firstLine="0" w:firstLineChars="0"/></w:pPr><w:rPr><w:i/><w:iCs/></w:rPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="LinettaSceneBreak"><w:name w:val="Linetta Scene Break"/><w:basedOn w:val="LinettaBody"/>` +
		`<w:pPr><w:spacing w:before="240" w:after="240"/><w:ind w:firstLine="0" w:firstLineChars="0"/><w:jc w:val="center"/></w:pPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="LinettaTitle"><w:name w:val="Linetta Title"/><w:basedOn w:val="LinettaBody"/>` +
		`<w:pPr><w:spacing w:before="4800" w:after="480"/><w:ind w:firstLine="0" w:firstLineChars="0"/><w:jc w:val="center"/></w:pPr>` +
		fmt.Sprintf(`<w:rPr><w:b/><w:bCs/><w:sz w:val="%[1]d"/><w:szCs w:val="%[1]d"/></w:rPr></w:style>`, half*2+8) +
		`<w:style w:type="paragraph" w:styleId="LinettaContents"><w:name w:val="Linetta Contents"/><w:basedOn w:val="LinettaBody"/>` +
		`<w:pPr><w:ind w:firstLine="0" w:firstLineChars="0"/><w:jc w:val="left"/></w:pPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="LinettaPicture"><w:name w:val="Linetta Picture"/><w:basedOn w:val="LinettaBody"/>` +
		`<w:pPr><w:keepNext/><w:spacing w:before="240" w:line="240" w:lineRule="auto"/><w:ind w:firstLine="0" w:firstLineChars="0"/><w:jc w:val="center"/></w:pPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="LinettaCaption"><w:name w:val="Linetta Caption"/><w:basedOn w:val="LinettaBody"/>` +
		fmt.Sprintf(`<w:pPr><w:spacing w:after="240"/><w:ind w:firstLine="0" w:firstLineChars="0"/><w:jc w:val="center"/></w:pPr><w:rPr><w:sz w:val="%[1]d"/><w:szCs w:val="%[1]d"/></w:rPr></w:style>`, max(half-2, 12)) +
		`<w:style w:type="paragraph" w:styleId="LinettaList"><w:name w:val="Linetta List"/><w:basedOn w:val="LinettaBody"/>` +
		`<w:pPr><w:ind w:firstLine="0" w:firstLineChars="0"/></w:pPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="LinettaCode"><w:name w:val="Linetta Code"/><w:basedOn w:val="LinettaBody"/>` +
		`<w:pPr><w:jc w:val="left"/><w:ind w:firstLine="0" w:firstLineChars="0"/></w:pPr>` +
		`<w:rPr><w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/></w:rPr></w:style>` +
		`<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:rPr><w:color w:val="0563C1"/></w:rPr></w:style>` +
		`</w:styles>`
}

func (w *writer) documentRels() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	b.WriteString(`<Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`)
	b.WriteString(`<Relationship Id="rIdSettings" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>`)
	for _, m := range w.media {
		fmt.Fprintf(&b, `<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/%s"/>`, m.rel, m.name)
	}
	for i, href := range w.links {
		fmt.Fprintf(&b, `<Relationship Id="rIdLink%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="%s" TargetMode="External"/>`,
			i+1, escapeAttr(href))
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

// escapeAttr is escape: xml.EscapeText already writes quotes as &#34; and
// &#39;, so its output is safe inside an attribute too.
func escapeAttr(s string) string { return escape(s) }

func documentXML(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
		`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"><w:body>` +
		body +
		// A4 with 25mm sides and 30mm top and bottom, the usual Korean page.
		`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/>` +
		`<w:pgMar w:top="1701" w:right="1417" w:bottom="1701" w:left="1417" w:header="851" w:footer="992" w:gutter="0"/></w:sectPr>` +
		`</w:body></w:document>`
}

func coreXML(title string, created time.Time) string {
	stamp := created.UTC().Format("2006-01-02T15:04:05Z")
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" ` +
		`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + escape(title) + `</dc:title>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + stamp + `</dcterms:created>` +
		`<dcterms:modified xsi:type="dcterms:W3CDTF">` + stamp + `</dcterms:modified>` +
		`</cp:coreProperties>`
}

// contentTypes declares each part's type, and one Default for each picture
// format the package holds.
func (w *writer) contentTypes() string {
	var defaults strings.Builder
	seen := map[string]bool{}
	for _, m := range w.media {
		ext := m.name[strings.LastIndex(m.name, ".")+1:]
		if !seen[ext] {
			seen[ext] = true
			fmt.Fprintf(&defaults, `<Default Extension="%s" ContentType="image/%s"/>`, ext, ext)
		}
	}
	return strings.Replace(contentTypesXML, `<Default Extension="xml"`, defaults.String()+`<Default Extension="xml"`, 1)
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>` +
	`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
	`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>` +
	`</Types>`

const rootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
	`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
	`</Relationships>`

const settingsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
	`<w:defaultTabStop w:val="800"/><w:characterSpacingControl w:val="doNotCompress"/><w:compat/>` +
	`</w:settings>`

const appXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">` +
	`<Application>Linetta</Application></Properties>`
