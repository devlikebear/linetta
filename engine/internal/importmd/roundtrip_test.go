package importmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/export"
)

func text(s string, marks ...any) map[string]any {
	n := map[string]any{"type": "text", "text": s}
	if len(marks) > 0 {
		n["marks"] = marks
	}
	return n
}

func mark(kind string) map[string]any { return map[string]any{"type": kind} }

func link(href string) map[string]any {
	return map[string]any{"type": "link", "attrs": map[string]any{"href": href}}
}

func para(inlines ...any) map[string]any {
	n := map[string]any{"type": "paragraph"}
	if len(inlines) > 0 {
		n["content"] = inlines
	}
	return n
}

func block(kind string, children ...any) map[string]any {
	return map[string]any{"type": kind, "content": children}
}

func withAttrs(n map[string]any, attrs map[string]any) map[string]any {
	n["attrs"] = attrs
	return n
}

var hardBreak = map[string]any{"type": "hardBreak"}

// Everything the editor can put in a scene body, exported and read back, has
// to come out as the same document (#160). Headings, mentions and note
// markers are not here: a heading in the body is outline structure, and the
// other two travel in the frontmatter.
func TestExportImportRoundTrip(t *testing.T) {
	cases := map[string][]any{
		"marks": {
			para(
				text("평문 "), text("굵게", mark("bold")), text(" "), text("기울임", mark("italic")),
				text(" "), text("지움", mark("strike")), text(" "), text("밑줄", mark("underline")),
				text(" "), text("go test ./...", mark("code")),
			),
		},
		"nested marks": {
			para(text("a ", mark("bold")), text("b", mark("bold"), mark("italic")), text(" c", mark("bold"))),
			para(text("x", mark("italic")), text("y", mark("italic"), mark("strike"))),
		},
		"links": {
			para(text("앞 "), text("문서", link("https://example.com/a?b=1")), text(" 뒤")),
			para(text("굵은 ", link("https://e.com")), text("링크", link("https://e.com"), mark("bold"))),
			para(text("괄호", link("https://example.com/a b(c)"))),
		},
		"code spans": {
			para(text("a`b", mark("code")), text(" "), text("`", mark("code")), text(" "), text(" 앞뒤 공백 ", mark("code"))),
		},
		"hard breaks": {
			para(text("첫 줄"), hardBreak, text("둘째 줄")),
			para(text("굵은 첫 줄", mark("bold")), hardBreak, text("굵은 둘째 줄", mark("bold"))),
		},
		"bullet list": {
			block("bulletList",
				block("listItem", para(text("하나"))),
				block("listItem", para(text("둘", mark("bold")))),
			),
		},
		"ordered list": {
			withAttrs(block("orderedList",
				block("listItem", para(text("셋"))),
				block("listItem", para(text("넷"))),
			), map[string]any{"start": float64(3)}),
		},
		"nested lists": {
			block("bulletList",
				block("listItem",
					para(text("바깥")),
					withAttrs(block("orderedList",
						block("listItem", para(text("안 하나"))),
						block("listItem", para(text("안 둘")), block("bulletList", block("listItem", para(text("가장 안"))))),
					), map[string]any{"start": float64(1)}),
				),
				block("listItem", para(text("다음"))),
			),
		},
		"loose list": {
			block("bulletList",
				block("listItem", para(text("첫 문단")), para(text("둘째 문단"))),
				block("listItem", para(text("다음"))),
			),
			para(text("목록 뒤 문단")),
		},
		"empty list item": {
			block("bulletList", block("listItem", para()), block("listItem", para(text("둘")))),
		},
		"code block": {
			para(text("앞")),
			withAttrs(block("codeBlock", text("func main() {\n\n\n\t// # not a heading\n\t- not a list\n}")), map[string]any{"language": "go"}),
			withAttrs(block("codeBlock", text("```\nfenced inside\n```")), map[string]any{"language": nil}),
			para(text("뒤")),
		},
		"code block in a list": {
			block("bulletList",
				block("listItem", para(text("명령")), withAttrs(block("codeBlock", text("make test\n\nmake build")), map[string]any{"language": "sh"})),
			),
		},
		"blockquote": {
			block("blockquote", para(text("첫 줄"), hardBreak, text("둘째 줄"))),
			block("blockquote", para(text("문단 하나")), para(text("문단 둘")), block("bulletList", block("listItem", para(text("인용 속 목록"))))),
		},
		"rule": {
			para(text("앞")), map[string]any{"type": "horizontalRule"}, para(text("뒤")),
		},
		"prose that looks like syntax": {
			para(text("- 그는 웃었다")),
			para(text("1. 첫째로")),
			para(text("# 해시로 시작")),
			para(text("> 꺾쇠로 시작")),
			para(text("---")),
			para(text("***")),
			para(text("a*b*c _d_ `e` ~~f~~ [g](h) <u>i</u> back\\slash")),
			para(text("첫 줄"), hardBreak, text("- 줄 바꾼 뒤 줄표"), hardBreak, text("2) 번호")),
			para(text("[시스템] 접속 ~ 완료")),
		},
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			doc := map[string]any{"type": "doc", "content": content}
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			md, err := export.DocToMarkdown(raw)
			if err != nil {
				t.Fatal(err)
			}
			back := reparse(t, md)
			want := normalize(t, doc)
			if !reflect.DeepEqual(back, want) {
				g, _ := json.MarshalIndent(back, "", " ")
				w, _ := json.MarshalIndent(want, "", " ")
				t.Errorf("markdown:\n%s\n got %s\nwant %s", md, g, w)
			}
		})
	}
}

// The whole-project path: the outline pass must not take a "# line" inside a
// code block for a heading, and collapsing blank lines must leave code alone.
func TestParseDocument_codeBlockSurvivesTheOutlinePass(t *testing.T) {
	md := "# 제목\n\n## 1장\n\n앞\n\n```sh\n# 주석\n\n\n## 이것도 주석\n```\n\n뒤\n"
	doc := ParseDocument(md)
	if len(doc.Outline.Roots) != 1 {
		t.Fatalf("roots = %d, want 1: %+v", len(doc.Outline.Roots), doc.Outline.Roots)
	}
	body := doc.Outline.Roots[0].Body
	if len(body) != 3 || body[1].Type != "codeBlock" {
		t.Fatalf("body = %+v", body)
	}
	got := body[1].Content[0].(TiptapInline).Text
	if want := "# 주석\n\n\n## 이것도 주석"; got != want {
		t.Errorf("code = %q, want %q", got, want)
	}
}

func reparse(t *testing.T, md string) any {
	t.Helper()
	blocks := ParseBlocks(strings.TrimRight(md, "\n"))
	return normalize(t, map[string]any{"type": "doc", "content": blocks})
}

func normalize(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
