package voiceimport

import (
	"errors"
	"os"
	"testing"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTweets(t *testing.T) {
	res, err := Parse("tweets.js", read(t, "tweets.js"), Options{OwnUserID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Posts) != 3 {
		t.Fatalf("posts %+v", res.Posts)
	}
	want := []string{"post", "reply", "post"}
	for i, p := range res.Posts {
		if p.Kind != want[i] || p.Platform != "x" || p.Source != "import" {
			t.Errorf("post %d = %+v", i, p)
		}
	}
	if res.Posts[0].Body != "오늘은 날씨가 정말 좋아서 산책을 오래 했다  & 커피도 마심" {
		t.Errorf("body %q", res.Posts[0].Body)
	}
	if res.Posts[0].ExternalID != "101" || res.Posts[0].PostedAt == 0 {
		t.Errorf("meta %+v", res.Posts[0])
	}
	// post 106 equals 101 once its link is gone, so it counts as a duplicate
	if res.Skipped["retweet"] != 1 || res.Skipped["short"] != 1 || res.Skipped["duplicate"] != 1 {
		t.Errorf("skipped %v", res.Skipped)
	}
}

func TestJSON(t *testing.T) {
	res, err := Parse("posts.json", read(t, "posts.json"), Options{DefaultKind: "reply"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Posts) != 3 {
		t.Fatalf("posts %+v", res.Posts)
	}
	if res.Posts[0].Kind != "post" || res.Posts[1].Kind != "reply" || res.Posts[2].Platform != "threads" || res.Posts[2].Kind != "reply" {
		t.Errorf("posts %+v", res.Posts)
	}
	if res.Posts[0].ExternalID != "a1" {
		t.Errorf("id %q", res.Posts[0].ExternalID)
	}
	if res.Skipped["duplicate"] != 1 || res.Skipped["short"] != 1 {
		t.Errorf("skipped %v", res.Skipped)
	}
}

func TestJSONGenericShapes(t *testing.T) {
	res, err := Parse("export.json", []byte(`{"posts":[{"content":"모든 문자열 키를 훑어서 글을 찾아내는 방식이다"}],"meta":{"name":"ignored value here that is long enough"}}`), Options{})
	if err != nil || len(res.Posts) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUnmangle(t *testing.T) {
	mangled := string([]rune{0xEC, 0x95, 0x88, 0xEB, 0x85, 0x95}) // "안녕" as latin-1 code points of UTF-8 bytes
	if got := unmangle(mangled); got != "안녕" {
		t.Errorf("got %q", got)
	}
	if got := unmangle("그대로 둔다"); got != "그대로 둔다" {
		t.Errorf("got %q", got)
	}
	if got := unmangle("plain ascii"); got != "plain ascii" {
		t.Errorf("got %q", got)
	}
}

func TestText(t *testing.T) {
	res, err := Parse("posts.txt", read(t, "posts.txt"), Options{DefaultKind: "reply", Platform: "threads"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Posts) != 2 || res.Posts[0].Kind != "reply" || res.Posts[0].Platform != "threads" {
		t.Fatalf("%+v", res.Posts)
	}
	if res.Skipped["short"] != 1 || res.Skipped["duplicate"] != 1 {
		t.Errorf("skipped %v", res.Skipped)
	}
}

func TestErrors(t *testing.T) {
	if _, err := Parse("a.txt", nil, Options{Platform: "mastodon"}); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("err %v", err)
	}
	if _, err := Parse("a.txt", nil, Options{DefaultKind: "dm"}); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("err %v", err)
	}
	if _, err := Parse("a.json", []byte("{nope"), Options{}); !errors.Is(err, ErrUnreadable) {
		t.Errorf("err %v", err)
	}
	if _, err := Parse("tweets.js", []byte("window.YTD.tweets.part0 = nothing"), Options{}); !errors.Is(err, ErrUnreadable) {
		t.Errorf("err %v", err)
	}
}
