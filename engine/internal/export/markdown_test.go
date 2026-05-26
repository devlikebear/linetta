package export

import "testing"

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
