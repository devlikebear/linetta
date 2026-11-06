package voice

import (
	"fmt"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/clio/post"
)

func mk(n int, platform, kind, body string) []post.Post {
	var out []post.Post
	for i := 0; i < n; i++ {
		out = append(out, post.Post{ID: fmt.Sprintf("%s-%s-%d", platform, kind, i), Platform: platform, Kind: kind, Body: body})
	}
	return out
}

func TestBuildContextsAndFallback(t *testing.T) {
	var posts []post.Post
	posts = append(posts, mk(25, "x", "post", "오늘도 열심히 일했다")...)
	posts = append(posts, mk(5, "x", "reply", "감사합니다 좋은 하루 되세요")...)
	posts = append(posts, mk(3, "threads", "post", "스레드 글을 처음 써 본다")...)
	p := Build(posts)
	for k, n := range map[string]int{"x/post": 25, "x/reply": 5, "x/*": 30, "*": 33, "threads/post": 3, "threads/*": 3} {
		if p.Contexts[k].N != n {
			t.Errorf("%s N=%d want %d", k, p.Contexts[k].N, n)
		}
	}
	if c := p.For("x", "post"); c.Fallback != "x/post" || c.N != 25 {
		t.Errorf("x/post %+v", c)
	}
	if c := p.For("x", "reply"); c.Fallback != "x/*" || c.N != 30 || c.Key != "x/reply" {
		t.Errorf("x/reply %q %d", c.Fallback, c.N)
	}
	if c := p.For("threads", "post"); c.Fallback != "*" || c.N != 33 {
		t.Errorf("threads %q %d", c.Fallback, c.N)
	}
	if c := p.For("blog", "post"); c.Fallback != "*" {
		t.Errorf("blog %q", c.Fallback)
	}
	if len(p.Contexts["x/post"].Examples) != 5 {
		t.Error("examples")
	}
}

func TestPolitenessShares(t *testing.T) {
	p := Build([]post.Post{
		{ID: "1", Platform: "x", Kind: "post", Body: "감사합니다. 그런데 별로임"},
		{ID: "2", Platform: "x", Kind: "post", Body: "정말 감사합니다"},
		{ID: "3", Platform: "x", Kind: "post", Body: "그냥 쓴다"},
		{ID: "4", Platform: "x", Kind: "post", Body: "좋아요. 그리고 했다"},
	})
	c := p.Contexts["x/*"]
	if c.MixedShare != 0.5 {
		t.Errorf("mixed %v", c.MixedShare)
	}
	if want := (0.5 + 1 + 0 + 0.5) / 4; c.PoliteShare != want {
		t.Errorf("polite %v want %v", c.PoliteShare, want)
	}
}

func TestPercentilesNearestRank(t *testing.T) {
	var posts []post.Post
	for i := 1; i <= 10; i++ {
		posts = append(posts, post.Post{ID: fmt.Sprint(i), Platform: "x", Kind: "post", Body: "가나다라마바사아자차" + repeat("!", i)})
	}
	s := Build(posts).Contexts["x/post"].Features["exclaim"]
	if s.P10 != 1 || s.P50 != 5 || s.P90 != 9 || s.Mean != 5.5 {
		t.Errorf("%+v", s)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func TestEmptyAndTop(t *testing.T) {
	p := Build(nil)
	c := p.For("x", "post")
	if c.N != 0 || c.Fallback != "*" || c.Top == nil {
		t.Errorf("%+v", c)
	}
	var posts []post.Post
	for _, b := range []string{"가나다라", "가나다마", "가나다바", "가나다사", "가나다아", "가나다자", "가나다차", "가나다카", "가나다타", "가나다파"} {
		posts = append(posts, post.Post{ID: b, Platform: "x", Kind: "post", Body: b})
	}
	if got := len(Build(posts).Contexts["*"].Top); got != 8 {
		t.Errorf("top %d", got)
	}
}
