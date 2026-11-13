package storycontext

import (
	"strings"
	"testing"
)

// #182: the writer's default style reaches the inline editor and the brief as
// the writer's own direction, ahead of the work's notes, in every language.

func TestWriterStyle_inlineEditorFollowsItFirst(t *testing.T) {
	for lang, want := range map[string][2]string{
		"ko": {"작가의 기본 문체(모든 작품, 반드시 따를 것)", "이 작품의 스타일 노트(반드시 따를 것, 위와 어긋나면 이쪽을 따를 것)"},
		"en": {"The writer's default style (every work, must follow)", "This work's style notes (must follow; where they disagree with the above, follow these)"},
		"ja": {"作家の基本文体（すべての作品、必ず従うこと）", "この作品のスタイルノート（必ず従うこと。上と食い違うときはこちらに従うこと）"},
	} {
		c := Context{
			WriterStyleNotes: "현재형으로 쓴다.",
			StyleNotes:       "이 작품은 과거형.",
			Options:          Options{Tone: TonePresetMy, Language: lang},
			UserPrompt:       "x",
		}
		sys, _ := Render(c)
		w, p := strings.Index(sys, want[0]), strings.Index(sys, want[1])
		if w < 0 || p < 0 || w > p || !strings.Contains(sys, "현재형으로 쓴다.") || !strings.Contains(sys, "이 작품은 과거형.") {
			t.Errorf("%s system:\n%s", lang, sys)
		}
	}
}

func TestWriterStyle_aloneIsStillTheWritersDirection(t *testing.T) {
	sys, _ := Render(Context{WriterStyleNotes: "현재형으로 쓴다.", Options: Options{Tone: TonePresetMy}, UserPrompt: "x"})
	if !strings.Contains(sys, "작가의 기본 문체(모든 작품, 반드시 따를 것):\n현재형으로 쓴다.") || strings.Contains(sys, "이 작품의 스타일 노트") {
		t.Fatalf("system:\n%s", sys)
	}
	_, user := Render(Context{WriterStyleNotes: "현재형으로 쓴다.", Options: Options{Language: "en"}, UserPrompt: "x"})
	if !strings.Contains(user, "## Writer's Default Style\n현재형으로 쓴다.\nThe writer's direction for every work.\n") || strings.Contains(user, "## Author Memo") {
		t.Fatalf("user:\n%s", user)
	}
}

func TestWriterStyle_briefPutsItBeforeTheWorksMemoAndSaysWhichWins(t *testing.T) {
	for lang, heads := range map[string][3]string{
		"ko": {"## 작가 기본 문체", "둘이 어긋나면 작가 메모를 따르십시오", "## 작가 메모"},
		"en": {"## Writer's Default Style", "where the two disagree, follow the memo", "## Author Memo"},
		"ja": {"## 作家の基本文体", "食い違うときは作家メモに従ってください", "## 作家メモ"},
	} {
		_, user := Render(Context{
			WriterStyleNotes: "현재형으로 쓴다.", StyleNotes: "이 작품은 과거형.",
			Options: Options{Language: lang}, UserPrompt: "x",
		})
		a, b, c := strings.Index(user, heads[0]), strings.Index(user, heads[1]), strings.Index(user, heads[2])
		if a < 0 || b < a || c < b {
			t.Errorf("%s user:\n%s", lang, user)
		}
	}
}

// Switching style notes off for a request takes the writer's out too; the
// memory switch, which governs the writer profile, does not.
func TestWriterStyle_followsTheStyleNotesSwitchNotTheMemoryOne(t *testing.T) {
	off := false
	base := Context{WriterStyleNotes: "현재형으로 쓴다.", UserPrompt: "x"}

	noStyle := base
	noStyle.Options = Options{Context: ContextSelection{StyleNotes: &off}}
	if _, user := Render(noStyle); strings.Contains(user, "현재형으로 쓴다.") {
		t.Fatalf("style notes off, writer's still there:\n%s", user)
	}
	noMemory := base
	noMemory.Options = Options{Context: ContextSelection{Memories: &off}}
	if _, user := Render(noMemory); !strings.Contains(user, "현재형으로 쓴다.") {
		t.Fatalf("memories off took the writer's style out:\n%s", user)
	}
}
