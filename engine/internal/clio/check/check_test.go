package check

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/clio/post"
	"github.com/devlikebear/linetta/engine/internal/clio/voice"
)

// plainPosts are n synthetic posts ending in plain speech.
func plainPosts(platform, kind string, n int) []post.Post {
	out := make([]post.Post, n)
	for i := range out {
		out[i] = post.Post{ID: fmt.Sprint("p", i), Platform: platform, Kind: kind, Body: fmt.Sprintf("오늘 %d번째 메모를 적었다", i)}
	}
	return out
}

func politePosts(platform, kind string, n int) []post.Post {
	out := make([]post.Post, n)
	for i := range out {
		out[i] = post.Post{ID: fmt.Sprint("q", i), Platform: platform, Kind: kind, Body: fmt.Sprintf("공지 %d번째 안내를 드려요", i)}
	}
	return out
}

func setup(posts []post.Post, platform, kind string) (voice.ContextProfile, Corpus) {
	ctx := voice.Build(posts).For(platform, kind)
	return ctx, BuildCorpus(posts, ctx.Fallback)
}

func find(fs []Finding, rule string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Rule == rule {
			out = append(out, f)
		}
	}
	return out
}

func TestRunNeverNilAndSorted(t *testing.T) {
	ctx, corpus := setup(plainPosts("x", "post", 30), "x", "post")
	fs := Run("", ctx, corpus)
	if fs == nil || len(fs) != 0 {
		t.Fatalf("empty body: %#v", fs)
	}
	fs = Run("결론적으로 핵심은 꾸준함입니다 🚀 😀😀😀", ctx, corpus)
	for i := 1; i < len(fs); i++ {
		a, b := fs[i-1], fs[i]
		if a.Start > b.Start || (a.Start == b.Start && rank(a.Severity) > rank(b.Severity)) {
			t.Errorf("unsorted at %d: %+v then %+v", i, a, b)
		}
	}
}

func TestNoCorpusNoRelativeFindings(t *testing.T) {
	fs := Run("결론적으로 핵심은 꾸준함입니다 🚀", voice.ContextProfile{Key: "x/post", Fallback: "x/post"}, Corpus{})
	if len(fs) != 0 {
		t.Fatalf("got %+v", fs)
	}
}

func TestBuildCorpusKeys(t *testing.T) {
	posts := append(plainPosts("x", "post", 3), plainPosts("x", "reply", 2)...)
	posts = append(posts, plainPosts("threads", "post", 4)...)
	for key, want := range map[string]int{"x/post": 3, "x/reply": 2, "x/*": 5, "threads/*": 4, "*": 9, "blog/*": 0} {
		if got := BuildCorpus(posts, key).N; got != want {
			t.Errorf("%s: N=%d want %d", key, got, want)
		}
	}
	// Same grouping as voice.Build.
	p := voice.Build(posts)
	for k, c := range p.Contexts {
		if got := BuildCorpus(posts, k).N; got != c.N {
			t.Errorf("%s: corpus %d profile %d", k, got, c.N)
		}
	}
}

func TestBuildCorpusRegisterCounts(t *testing.T) {
	posts := []post.Post{
		{Platform: "x", Kind: "post", Body: "오늘은 쉬었다. 내일은 갈게요"},
		{Platform: "x", Kind: "post", Body: "공지 드려요. 안내 드려요. 쉬었다"},
		{Platform: "x", Kind: "post", Body: "그냥 쉬었다. 또 쉬었다"},
	}
	c := BuildCorpus(posts, "x/post")
	if c.MixedPosts != 2 || c.PolitePosts != 1 || c.PlainPosts != 1 {
		t.Fatalf("got %+v", c)
	}
	ctx := voice.Build(posts).Contexts["x/post"]
	if float64(c.MixedPosts)/float64(c.N) != ctx.MixedShare {
		t.Errorf("mixed share differs: %v vs %v", c.MixedPosts, ctx.MixedShare)
	}
}

func TestAIPhraseFlagsRareOnly(t *testing.T) {
	posts := plainPosts("x", "post", 50)
	for i := 0; i < 5; i++ { // 10% use "다양한"
		posts[i].Body += " 다양한 시도였다"
	}
	ctx, corpus := setup(posts, "x", "post")
	fs := find(Run("결론적으로 다양한 시도를 했다", ctx, corpus), RuleAIPhrase)
	if len(fs) != 1 {
		t.Fatalf("want only 결론적으로, got %+v", fs)
	}
	f := fs[0]
	if f.Severity != Strong || f.Start != 0 || f.End != 5 {
		t.Errorf("finding %+v", f)
	}
	if !strings.Contains(f.Evidence, "내 X 글 50편 중 0편에 있음") {
		t.Errorf("evidence %q", f.Evidence)
	}
}

func TestAIPhraseRuneOffsets(t *testing.T) {
	ctx, corpus := setup(plainPosts("x", "post", 30), "x", "post")
	fs := find(Run("😀 좋다 🚀", ctx, corpus), RuleAIPhrase)
	if len(fs) != 1 || fs[0].Start != 5 || fs[0].End != 6 {
		t.Fatalf("got %+v", fs)
	}
}

func TestAIPattern(t *testing.T) {
	ctx, corpus := setup(plainPosts("x", "post", 30), "x", "post")
	fs := find(Run("오늘은 맑았다. 나는 늦었고, 너는 빨랐다.", ctx, corpus), RuleAIPattern)
	if len(fs) != 1 || fs[0].Severity != Strong {
		t.Fatalf("got %+v", fs)
	}
	body := []rune("오늘은 맑았다. 나는 늦었고, 너는 빨랐다.")
	if got := string(body[fs[0].Start:fs[0].End]); got != "나는 늦었고, 너는 빨랐다." {
		t.Errorf("span text %q", got)
	}
	if len(find(Run(`"나는 늦었고, 너는 빨랐다."`, ctx, corpus), RuleAIPattern)) != 0 {
		t.Error("dialogue should be spared")
	}
}

func TestSpecDraftAgainstPlainCorpusWithSomePolite(t *testing.T) {
	posts := append(plainPosts("x", "post", 92), politePosts("x", "post", 8)...)
	ctx, corpus := setup(posts, "x", "post")
	fs := Run("결론적으로 핵심은 꾸준함입니다 🚀", ctx, corpus)
	phrases := find(fs, RuleAIPhrase)
	if len(phrases) < 2 {
		t.Fatalf("want ai_phrase findings, got %+v", fs)
	}
	for _, f := range phrases {
		if f.Severity != Strong {
			t.Errorf("severity %+v", f)
		}
	}
	if m := find(fs, RuleRegisterMismatch); len(m) != 0 {
		t.Errorf("polite is 8%% of posts, got mismatch %+v", m)
	}
}

func TestRegisterMismatch(t *testing.T) {
	ctx, corpus := setup(plainPosts("x", "post", 50), "x", "post")
	fs := find(Run("오늘 배포했습니다. 잘 됩니다.", ctx, corpus), RuleRegisterMismatch)
	if len(fs) != 2 || fs[0].Severity != Medium {
		t.Fatalf("got %+v", fs)
	}
	if !strings.Contains(fs[0].Message, "존댓말") {
		t.Errorf("message %q", fs[0].Message)
	}
	// A plain draft in a plain context is fine.
	if got := find(Run("오늘 배포했다. 잘 된다.", ctx, corpus), RuleRegisterMismatch); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestRegisterMismatchFallbackEvidence(t *testing.T) {
	posts := plainPosts("x", "post", 30)
	ctx, corpus := setup(posts, "x", "reply") // x/reply is empty, falls back to x/*
	if ctx.Fallback != "x/*" {
		t.Fatalf("fallback %q", ctx.Fallback)
	}
	fs := find(Run("오늘 배포했습니다.", ctx, corpus), RuleRegisterMismatch)
	if len(fs) != 1 || !strings.Contains(fs[0].Evidence, "X 전체 기준") {
		t.Fatalf("got %+v", fs)
	}
}

func TestRegisterMixed(t *testing.T) {
	ctx, corpus := setup(plainPosts("x", "post", 50), "x", "post")
	body := "써 보니 괜찮았다. 다음엔 다른 것도 해 볼게요."
	fs := find(Run(body, ctx, corpus), RuleRegisterMixed)
	if len(fs) != 1 {
		t.Fatalf("got %+v", fs)
	}
	r := []rune(body)
	if got := string(r[fs[0].Start:fs[0].End]); got != "다음엔 다른 것도 해 볼게요." {
		t.Errorf("span %q", got)
	}
	if fs[0].Message != "한 글에 존댓말과 반말이 섞였어요" {
		t.Errorf("message %q", fs[0].Message)
	}
	// Not flagged when the writer often mixes.
	ctx.MixedShare = 0.5
	if got := find(Run(body, ctx, corpus), RuleRegisterMixed); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestRegisterMixedTieFlagsLater(t *testing.T) {
	ctx, corpus := setup(plainPosts("x", "post", 50), "x", "post")
	body := "공지 드려요. 그냥 쉬었다."
	fs := find(Run(body, ctx, corpus), RuleRegisterMixed)
	if len(fs) != 1 || fs[0].Start != 8 {
		t.Fatalf("got %+v", fs)
	}
}

func TestRegisterQuotedExcluded(t *testing.T) {
	ctx, corpus := setup(plainPosts("x", "post", 50), "x", "post")
	for _, body := range []string{
		`오늘 정리했다. "내일 갈게요."`,
		`오늘 정리했다. “내일 갈게요.”`,
		`오늘 정리했다. 「내일 갈게요.」`,
		`오늘 정리했다. 그가 말했다 '내일 갈게요'`,
	} {
		fs := Run(body, ctx, corpus)
		if len(find(fs, RuleRegisterMixed)) != 0 || len(find(fs, RuleRegisterMismatch)) != 0 {
			t.Errorf("%s: %+v", body, fs)
		}
	}
}

func TestOutOfRange(t *testing.T) {
	ctx := voice.ContextProfile{Key: "x/post", Fallback: "x/post", N: 112, Features: map[string]voice.Stat{
		"emoji": {P10: 0, P50: 0, P90: 0},
		"lines": {P10: 1, P50: 2, P90: 3},
	}}
	fs := find(Run("좋은 날 😀😀😀", ctx, Corpus{}), RuleOutOfRange)
	if len(fs) != 1 || fs[0].Severity != Weak || fs[0].Start != 0 || fs[0].End != 0 {
		t.Fatalf("got %+v", fs)
	}
	if !strings.Contains(fs[0].Message, "이모지") {
		t.Errorf("message %q", fs[0].Message)
	}
	if got := find(Run("한 줄\n두 줄", ctx, Corpus{}), RuleOutOfRange); len(got) != 0 {
		t.Errorf("2 lines should pass: %+v", got)
	}
	if got := find(Run("한 줄\n두 줄\n세 줄\n네 줄", ctx, Corpus{}), RuleOutOfRange); len(got) != 1 {
		t.Errorf("4 lines should flag: %+v", got)
	}
}

func TestUnfamiliarEnding(t *testing.T) {
	ctx, corpus := setup(plainPosts("x", "post", 30), "x", "post")
	one := Run("오늘은 쉬었다. 내일은 쉬겠습니다.", ctx, corpus)
	if got := find(one, RuleUnfamiliarEnding); len(got) != 0 {
		t.Errorf("one unseen ending should not flag: %+v", got)
	}
	two := find(Run("내일은 쉬겠습니다. 모레는 쉬겠습니다.", ctx, corpus), RuleUnfamiliarEnding)
	if len(two) != 2 || two[0].Severity != Weak {
		t.Fatalf("got %+v", two)
	}
}

func TestJSONTags(t *testing.T) {
	b, err := json.Marshal(Finding{Rule: "r", Severity: Strong})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"rule":"r","severity":"strong","start":0,"end":0,"message":"","evidence":""}`
	if string(b) != want {
		t.Errorf("got %s", b)
	}
}

func TestRareHabitIsMedium(t *testing.T) {
	ctx, corpus := setup(plainPosts("x", "post", 100), "x", "post")
	fs := find(Run("오늘도 메모를 적었다 😀", ctx, corpus), RuleRareHabit)
	if len(fs) != 1 || fs[0].Severity != Medium || !strings.Contains(fs[0].Evidence, "100편 중 0편") {
		t.Fatalf("emoji the writer never uses: %+v", fs)
	}
	// A writer who uses emoji in 5% of posts is not flagged for it.
	posts := plainPosts("x", "post", 95)
	for i := 0; i < 5; i++ {
		posts = append(posts, post.Post{ID: fmt.Sprint("e", i), Platform: "x", Kind: "post", Body: fmt.Sprintf("이모지 %d번째 글이다 😀", i)})
	}
	ctx, corpus = setup(posts, "x", "post")
	if fs := find(Run("오늘도 메모를 적었다 😀", ctx, corpus), RuleRareHabit); len(fs) != 0 {
		t.Fatalf("writer's own habit flagged: %+v", fs)
	}
}

func TestAIPhraseTwoPostsOfHundredIsTheWriters(t *testing.T) {
	posts := plainPosts("x", "post", 98)
	for i := 0; i < 2; i++ {
		posts = append(posts, post.Post{ID: fmt.Sprint("d", i), Platform: "x", Kind: "post", Body: fmt.Sprintf("이번 %d번째 글 — 짧게 적었다", i)})
	}
	ctx, corpus := setup(posts, "x", "post")
	if fs := find(Run("새 기능 — 오늘 붙였다", ctx, corpus), RuleAIPhrase); len(fs) != 0 {
		t.Fatalf("em dash in 2%% of posts flagged: %+v", fs)
	}
}

func TestListItemsAndNounPhrasesHaveNoRegister(t *testing.T) {
	ctx, corpus := setup(plainPosts("x", "post", 30), "x", "post")
	body := "새 버전을 올렸어요.\n새로 생긴 것들\n- 앞으로 7일 흐름\n- 뜻을 따로 볼 수 있는 페이지"
	if fs := find(Run(body, ctx, corpus), RuleRegisterMixed); len(fs) != 0 {
		t.Fatalf("list items counted as 반말: %+v", fs)
	}
}
