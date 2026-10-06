package export

import (
	"encoding/json"
	"testing"
)

func TestDocToMarkdown_paragraphAndBoldItalic(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"paragraph","content":[
	    {"type":"text","text":"안녕 "},
	    {"type":"text","marks":[{"type":"bold"}],"text":"세계"},
	    {"type":"text","text":" "},
	    {"type":"text","marks":[{"type":"italic"}],"text":"여기"}
	  ]}
	]}`
	got, err := DocToMarkdown([]byte(doc))
	if err != nil {
		t.Fatalf("DocToMarkdown: %v", err)
	}
	want := "안녕 **세계** _여기_\n\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestDocToMarkdown_headings(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"3장"}]},
	  {"type":"paragraph","content":[{"type":"text","text":"본문"}]}
	]}`
	got, _ := DocToMarkdown([]byte(doc))
	want := "## 3장\n\n본문\n\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDocToMarkdown_blockquoteAndHardBreak(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"blockquote","content":[
	    {"type":"paragraph","content":[
	      {"type":"text","text":"첫 줄"},
	      {"type":"hardBreak"},
	      {"type":"text","text":"둘째 줄"}
	    ]}
	  ]}
	]}`
	got, _ := DocToMarkdown([]byte(doc))
	want := "> 첫 줄  \n> 둘째 줄\n\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDocToMarkdown_mentionRendersAsPlainText(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"paragraph","content":[
	    {"type":"text","text":"오늘은 "},
	    {"type":"mention","attrs":{"id":"e1","label":"해진"}},
	    {"type":"text","text":"이 도시에 도착했다."}
	  ]}
	]}`
	got, _ := DocToMarkdown([]byte(doc))
	want := "오늘은 @해진이 도시에 도착했다.\n\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDocToMarkdown_boldItalicCombined(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"paragraph","content":[
	    {"type":"text","marks":[{"type":"bold"},{"type":"italic"}],"text":"강조"}
	  ]}
	]}`
	got, _ := DocToMarkdown([]byte(doc))
	want := "**_강조_**\n\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDocToMarkdown_emptyDoc(t *testing.T) {
	got, err := DocToMarkdown([]byte(`{"type":"doc","content":[]}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestDocToMarkdown_corruptInput(t *testing.T) {
	_, err := DocToMarkdown([]byte("not-json"))
	if err == nil {
		t.Error("expected parse error")
	}
}

func mustMarkdown(t *testing.T, doc string) string {
	t.Helper()
	got, err := DocToMarkdown([]byte(doc))
	if err != nil {
		t.Fatalf("DocToMarkdown: %v", err)
	}
	return got
}

func TestDocToMarkdown_lists(t *testing.T) {
	cases := []struct{ name, doc, want string }{
		{
			"bullet",
			`{"type":"doc","content":[{"type":"bulletList","content":[
			  {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"하나"}]}]},
			  {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"둘"}]}]}
			]}]}`,
			"- 하나\n- 둘\n\n",
		},
		{
			"ordered keeps its start",
			`{"type":"doc","content":[{"type":"orderedList","attrs":{"start":3,"type":null},"content":[
			  {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"셋"}]}]},
			  {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"넷"}]}]}
			]}]}`,
			"3. 셋\n4. 넷\n\n",
		},
		{
			"nested",
			`{"type":"doc","content":[{"type":"bulletList","content":[
			  {"type":"listItem","content":[
			    {"type":"paragraph","content":[{"type":"text","text":"바깥"}]},
			    {"type":"orderedList","attrs":{"start":1},"content":[
			      {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"안"}]}]}
			    ]}
			  ]}
			]}]}`,
			"- 바깥\n  1. 안\n\n",
		},
		{
			"an item with two paragraphs is loose",
			`{"type":"doc","content":[{"type":"bulletList","content":[
			  {"type":"listItem","content":[
			    {"type":"paragraph","content":[{"type":"text","text":"첫 문단"}]},
			    {"type":"paragraph","content":[{"type":"text","text":"둘째 문단"}]}
			  ]},
			  {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"다음"}]}]}
			]}]}`,
			"- 첫 문단\n\n  둘째 문단\n\n- 다음\n\n",
		},
	}
	for _, tc := range cases {
		if got := mustMarkdown(t, tc.doc); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
}

func TestDocToMarkdown_codeBlock(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"a := 1\n\n\n# not a heading"}]},
	  {"type":"paragraph","content":[{"type":"text","text":"뒤"}]}
	]}`
	want := "```go\na := 1\n\n\n# not a heading\n```\n\n뒤\n\n"
	if got := mustMarkdown(t, doc); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	// The whole-project export squeezes blank lines; a code block's are content.
	if got := collapseBlankLines(want); got != want {
		t.Errorf("collapse changed a code block:\n got %q\nwant %q", got, want)
	}
	if got, want := collapseBlankLines("a\n\n\n\nb\n\n```\nx\n```\n\n\n\nc\n\n"), "a\n\nb\n\n```\nx\n```\n\nc\n\n"; got != want {
		t.Errorf("collapse outside a fence:\n got %q\nwant %q", got, want)
	}
}

func TestDocToMarkdown_codeBlockFenceOutgrowsItsContent(t *testing.T) {
	doc := "{\"type\":\"doc\",\"content\":[{\"type\":\"codeBlock\",\"attrs\":{\"language\":null},\"content\":[{\"type\":\"text\",\"text\":\"```\\ninner\\n```\"}]}]}"
	want := "````\n```\ninner\n```\n````\n\n"
	if got := mustMarkdown(t, doc); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestDocToMarkdown_inlineMarks(t *testing.T) {
	cases := []struct{ name, content, want string }{
		{"code", `{"type":"text","marks":[{"type":"code"}],"text":"go test"}`, "`go test`"},
		{"code holding a backtick", `{"type":"text","marks":[{"type":"code"}],"text":"a` + "`" + `b"}`, "``a`b``"},
		{"strike", `{"type":"text","marks":[{"type":"strike"}],"text":"지움"}`, "~~지움~~"},
		{"underline", `{"type":"text","marks":[{"type":"underline"}],"text":"밑줄"}`, "<u>밑줄</u>"},
		{
			"link with the attrs the editor adds",
			`{"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com/a","target":"_blank","rel":"noopener noreferrer nofollow","class":null,"title":null}}],"text":"문서"}`,
			"[문서](https://example.com/a)",
		},
		{
			"link whose address needs brackets",
			`{"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com/a b(c)"}}],"text":"문서"}`,
			"[문서](<https://example.com/a b(c)>)",
		},
		{
			"one bold span across three text nodes",
			`{"type":"text","marks":[{"type":"bold"}],"text":"a "},{"type":"text","marks":[{"type":"bold"},{"type":"italic"}],"text":"b"},{"type":"text","marks":[{"type":"bold"}],"text":" c"}`,
			"**a _b_ c**",
		},
		{
			"bold inside a link",
			`{"type":"text","marks":[{"type":"link","attrs":{"href":"https://e.com"}}],"text":"가 "},{"type":"text","marks":[{"type":"link","attrs":{"href":"https://e.com"}},{"type":"bold"}],"text":"나"}`,
			"[가 **나**](https://e.com)",
		},
	}
	for _, tc := range cases {
		doc := `{"type":"doc","content":[{"type":"paragraph","content":[` + tc.content + `]}]}`
		if got, want := mustMarkdown(t, doc), tc.want+"\n\n"; got != want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, want)
		}
	}
}

// Prose that only looks like syntax has to come back as prose. Before lists
// were exported nothing read "- " back as a list, so nothing escaped it.
func TestDocToMarkdown_escapesProseThatLooksLikeSyntax(t *testing.T) {
	cases := []struct{ text, want string }{
		{"- 그는 웃었다", `\- 그는 웃었다`},
		{"1. 첫째", `1\. 첫째`},
		{"# 해시", `\# 해시`},
		{"> 꺾쇠", `\> 꺾쇠`},
		{"---", `\---`},
		{"***", `\*\*\*`},
		{"a*b_c", `a\*b\_c`},
		{"그래~~ 좋아", `그래\~\~ 좋아`},
		{"한 번~ 더", "한 번~ 더"},
		{"[시스템] 접속", "[시스템] 접속"},
		{"[가](나)", `[가\](나)`},
		{"a<u>b", `a\<u>b`},
		{`back\slash`, `back\\slash`},
		{"-붙은 줄표", "-붙은 줄표"},
	}
	for _, tc := range cases {
		doc, _ := jsonParagraph(tc.text)
		if got, want := mustMarkdown(t, doc), tc.want+"\n\n"; got != want {
			t.Errorf("%q:\n got %q\nwant %q", tc.text, got, want)
		}
	}
}

func TestDocToMarkdown_blockquoteHoldsBlocks(t *testing.T) {
	doc := `{"type":"doc","content":[{"type":"blockquote","content":[
	  {"type":"paragraph","content":[{"type":"text","text":"하나"}]},
	  {"type":"bulletList","content":[
	    {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"둘"}]}]}
	  ]}
	]}]}`
	want := "> 하나\n>\n> - 둘\n\n"
	if got := mustMarkdown(t, doc); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestDocToPlainText_keepsCodeBlockText(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"paragraph","content":[{"type":"text","text":"앞"}]},
	  {"type":"codeBlock","content":[{"type":"text","text":"x = 1"}]}
	]}`
	got, err := DocToPlainText([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if want := "앞\n\nx = 1"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func jsonParagraph(text string) (string, error) {
	raw, err := json.Marshal(map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}},
	}})
	return string(raw), err
}
