package manuscriptir

import (
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
)

// Every node and mark the editor has, once (testdata/editor_schema.json).
const everyNode = `{"type":"doc","content":[
 {"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"장 안 제목"}]},
 {"type":"paragraph","content":[
   {"type":"mention","attrs":{"id":"entity-7f3a","label":"수아"}},
   {"type":"text","text":"는 "},
   {"type":"text","marks":[{"type":"bold"}],"text":"은빛"},
   {"type":"text","text":" "},
   {"type":"text","marks":[{"type":"italic"},{"type":"underline"}],"text":"열쇠"},
   {"type":"noteMarker","attrs":{"noteId":"note-secret-9"}},
   {"type":"text","marks":[{"type":"strike"}],"text":"를"},
   {"type":"hardBreak"},
   {"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com"}}],"text":"링크"},
   {"type":"text","marks":[{"type":"code"}],"text":"x=1"}
 ]},
 {"type":"paragraph"},
 {"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"인용"}]}]},
 {"type":"horizontalRule"},
 {"type":"bulletList","content":[
   {"type":"listItem","content":[
     {"type":"paragraph","content":[{"type":"text","text":"하나"}]},
     {"type":"orderedList","attrs":{"start":3},"content":[
       {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"안쪽"}]}]}
     ]}
   ]}
 ]},
 {"type":"codeBlock","content":[{"type":"text","text":"a\nb"}]},
 {"type":"illustration","attrs":{"id":"ill-1","caption":"비 오는 음악실"}}
]}`

func TestFromDocHandlesEveryEditorNode(t *testing.T) {
	blocks, err := FromDoc(everyNode)
	if err != nil {
		t.Fatal(err)
	}
	type want struct {
		kind   BlockKind
		level  int
		marker string
		text   string
	}
	wants := []want{
		{Heading, 2, "", "장 안 제목"},
		{Paragraph, 0, "", "수아는 은빛 열쇠를\n링크x=1"},
		{Quote, 0, "", "인용"},
		{SceneBreak, 0, "", ""},
		{ListItem, 1, "•", "하나"},
		{ListItem, 2, "3.", "안쪽"},
		{CodeLine, 0, "", "a"},
		{CodeLine, 0, "", "b"},
		{IllustrationPlaceholder, 0, "", "비 오는 음악실"},
	}
	if len(blocks) != len(wants) {
		t.Fatalf("got %d blocks: %+v", len(blocks), blocks)
	}
	for i, w := range wants {
		b := blocks[i]
		if b.Kind != w.kind || b.Level != w.level || b.Marker != w.marker || b.Text() != w.text {
			t.Errorf("block %d = %+v (%q), want %+v", i, b, b.Text(), w)
		}
	}
	runs := blocks[1].Runs
	find := func(text string) Run {
		for _, r := range runs {
			if r.Text == text {
				return r
			}
		}
		t.Fatalf("no run %q in %+v", text, runs)
		return Run{}
	}
	if r := find("은빛"); !r.Bold || r.Italic {
		t.Errorf("은빛 = %+v", r)
	}
	if r := find("열쇠"); !r.Italic || !r.Underline {
		t.Errorf("열쇠 = %+v", r)
	}
	if r := find("를"); !r.Strike {
		t.Errorf("를 = %+v", r)
	}
	if r := find("링크"); r.Href != "https://example.com" {
		t.Errorf("링크 = %+v", r)
	}
	if r := find("x=1"); !r.Code {
		t.Errorf("x=1 = %+v", r)
	}
	// A mention is its label as plain text, one run with the particle after it.
	if r := find("수아는 "); r.Bold || r.Href != "" {
		t.Errorf("수아는 = %+v", r)
	}
}

func scene(id, parent string, ordinal int, text string) node.Node {
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
	n := node.Node{ID: id, Kind: node.KindLeaf, Label: "씬", Ordinal: ordinal, ContentDoc: &doc}
	if parent != "" {
		n.ParentID = &parent
	}
	return n
}

func container(id, parent string, ordinal int, label, title string) node.Node {
	n := node.Node{ID: id, Kind: node.KindContainer, Label: label, Title: title, Ordinal: ordinal}
	if parent != "" {
		n.ParentID = &parent
	}
	return n
}

// 1부 holds 1화 (two scenes) and 2화 (one scene); a scene sits at the root
// after the part. Given out of order: Build reads the outline's order.
func outline() []node.Node {
	return []node.Node{
		scene("s3", "e2", 0, "셋째"),
		scene("s2", "e1", 1, "둘째"),
		container("e2", "p1", 1, "2화", ""),
		scene("s1", "e1", 0, "첫째"),
		container("p1", "", 0, "1부", "시작"),
		container("e1", "p1", 0, "1화", "비"),
		scene("s4", "", 1, "끝"),
	}
}

func sectionsOf(d Doc) []string {
	var out []string
	for _, s := range d.Sections {
		var texts []string
		for _, b := range s.Blocks {
			if b.Kind == SceneBreak {
				texts = append(texts, "*")
				continue
			}
			texts = append(texts, b.Text())
		}
		out = append(out, s.Heading+"|"+strings.Join(texts, ","))
	}
	return out
}

func TestBuildFollowsTheOutline(t *testing.T) {
	doc, err := Build("작품", outline(), "", Options{Language: "en", Headings: true})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(sectionsOf(doc), " / ")
	want := "Part 1 시작| / Episode 1 비|첫째,*,둘째 / Episode 2|셋째 / |끝"
	if got != want {
		t.Fatalf("sections:\n got %s\nwant %s", got, want)
	}
	if doc.Sections[1].Level != 2 || !doc.Sections[1].HoldsScenes || doc.Sections[0].HoldsScenes {
		t.Fatalf("levels and scene holding: %+v", doc.Sections)
	}
}

func TestBuildWithoutHeadingsKeepsOnlyTheProse(t *testing.T) {
	doc, err := Build("작품", outline(), "", Options{Language: "ko"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(sectionsOf(doc), " / ")
	if want := "|첫째,*,둘째 / |셋째 / |끝"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestBuildExportsOneContainerOrOneScene(t *testing.T) {
	doc, err := Build("작품", outline(), "e1", Options{Language: "ko", Headings: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(sectionsOf(doc), " / "); got != "1화 비|첫째,*,둘째" {
		t.Fatalf("episode: %s", got)
	}
	doc, err = Build("작품", outline(), "s3", Options{Language: "ko", Headings: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(sectionsOf(doc), " / "); got != "|셋째" {
		t.Fatalf("scene: %s", got)
	}
	if _, err := Build("작품", outline(), "elsewhere", Options{}); err == nil {
		t.Fatal("a node from another work was exported")
	}
}

func TestTextWritesPlainParagraphs(t *testing.T) {
	blocks, err := FromDoc(everyNode)
	if err != nil {
		t.Fatal(err)
	}
	doc := Doc{Sections: []Section{{Heading: "1화", Blocks: blocks}}}
	got := Text(doc, TextOptions{SceneBreak: "* * *", Illustrations: true, IllustrationLabel: "삽화"})
	for _, want := range []string{"1화\n\n장 안 제목\n\n수아는 은빛 열쇠를\n링크x=1\n\n인용\n\n* * *", "• 하나\n\n  3. 안쪽", "[삽화: 비 오는 음악실]"} {
		if !strings.Contains(got, want) {
			t.Errorf("text lacks %q:\n%s", want, got)
		}
	}
	for _, leaked := range []string{"entity-7f3a", "note-secret-9", "ill-1", "**", "@"} {
		if strings.Contains(got, leaked) {
			t.Errorf("text carries %q", leaked)
		}
	}
	if without := Text(doc, TextOptions{SceneBreak: "* * *"}); strings.Contains(without, "삽화") {
		t.Error("an illustration was written without being asked for")
	}
}
