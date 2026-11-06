// Package voiceimport turns an export file (an X archive's tweets.js, a JSON
// array, or plain text split by ---) into posts ready to store. It ports
// load/from_tweets/from_json/from_text/unmangle from scripts/eval-voice.py and
// never touches the disk: the caller hands over the file's bytes.
package voiceimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/clio/post"
	"github.com/devlikebear/linetta/engine/internal/clio/voice"
)

// Skip reasons reported in Result.Skipped.
const (
	SkipRetweet   = "retweet"
	SkipShort     = "short"
	SkipDuplicate = "duplicate"
)

var (
	// ErrInvalidOptions is returned for an unknown platform or kind in Options.
	ErrInvalidOptions = errors.New("invalid import options")
	// ErrUnreadable is returned when the content cannot be parsed in its format.
	ErrUnreadable = errors.New("unreadable import file")
)

// Options steer a parse. Zero values mean Platform "x" and DefaultKind "post".
type Options struct {
	DefaultKind string
	Platform    string
	// OwnUserID is the writer's X user id. In tweets.js a reply to someone
	// else is a reply; a reply to oneself (a thread) is a post.
	OwnUserID string
}

// Result is the posts kept and, by reason, how many were dropped.
type Result struct {
	Posts   []post.NewPost
	Skipped map[string]int
}

type item struct {
	text       string
	kind       string
	platform   string
	externalID string
	postedAt   int64
	retweet    bool
}

// Parse reads content as tweets.js, JSON or text (by extension, then by what
// the content looks like) and returns the cleaned, de-duplicated posts.
func Parse(fileName string, content []byte, opts Options) (Result, error) {
	platform, kind := opts.Platform, opts.DefaultKind
	if platform == "" {
		platform = post.PlatformX
	}
	if kind == "" {
		kind = post.KindPost
	}
	if !validPlatform(platform) || !validKind(kind) {
		return Result{}, fmt.Errorf("%w: platform %q kind %q", ErrInvalidOptions, platform, kind)
	}
	opts.Platform, opts.DefaultKind = platform, kind

	text := strings.TrimPrefix(string(content), "\xef\xbb\xbf")
	var items []item
	var err error
	switch detect(fileName, text) {
	case "tweets":
		items, err = fromTweets(text, opts)
	case "json":
		items, err = fromJSON(text, opts)
	default:
		items = fromText(text, opts)
	}
	if err != nil {
		return Result{}, err
	}

	res := Result{Posts: []post.NewPost{}, Skipped: map[string]int{}}
	seen := map[string]bool{}
	for _, it := range items {
		if it.retweet {
			res.Skipped[SkipRetweet]++
			continue
		}
		body := strings.TrimSpace(voice.URLRe.ReplaceAllString(it.text, ""))
		if voice.Letters(body) < voice.MinLetters {
			res.Skipped[SkipShort]++
			continue
		}
		key := it.platform + "\x00" + body
		if seen[key] {
			res.Skipped[SkipDuplicate]++
			continue
		}
		seen[key] = true
		res.Posts = append(res.Posts, post.NewPost{
			Body: body, Platform: it.platform, Kind: it.kind, Source: post.SourceImport,
			ExternalID: it.externalID, PostedAt: it.postedAt,
		})
	}
	return res, nil
}

func validPlatform(p string) bool {
	return p == post.PlatformX || p == post.PlatformThreads || p == post.PlatformBlog
}

func validKind(k string) bool { return k == post.KindPost || k == post.KindReply }

func detect(fileName, text string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	base := strings.ToLower(filepath.Base(fileName))
	trimmed := strings.TrimSpace(text)
	switch {
	case ext == ".js" || strings.HasPrefix(trimmed, "window.YTD."):
		return "tweets"
	case ext == ".json":
		return "json"
	case ext == ".txt" || ext == ".md" || base == "":
		return "text"
	case strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{"):
		return "json"
	}
	return "text"
}

// unmangle undoes Meta's exports, which write UTF-8 bytes as Latin-1 code
// points. Text that is not such a mangling comes back unchanged.
func unmangle(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xFF {
			return s
		}
		b = append(b, byte(r))
	}
	if !utf8.Valid(b) {
		return s
	}
	return string(b)
}

func decode(text string, into any) error {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	return nil
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	}
	return ""
}

func fromTweets(text string, opts Options) ([]item, error) {
	i := strings.Index(text, "[")
	if i < 0 {
		return nil, fmt.Errorf("%w: no tweet array", ErrUnreadable)
	}
	var data []map[string]any
	if err := decode(text[i:], &data); err != nil {
		return nil, err
	}
	var out []item
	for _, entry := range data {
		t := entry
		if inner, ok := entry["tweet"].(map[string]any); ok {
			t = inner
		}
		body := str(t["full_text"])
		it := item{text: html.UnescapeString(body), platform: opts.Platform, kind: opts.DefaultKind, externalID: str(t["id_str"])}
		if rt, _ := t["retweeted"].(bool); rt || strings.HasPrefix(body, "RT @") {
			it.retweet = true
		}
		if reply := str(t["in_reply_to_user_id_str"]); reply != "" {
			// a reply to someone else is a reply; a reply to oneself is the
			// writer's own thread, which reads as a post.
			it.kind = post.KindPost
			if reply != opts.OwnUserID {
				it.kind = post.KindReply
			}
		}
		if ts, err := time.Parse("Mon Jan 02 15:04:05 -0700 2006", str(t["created_at"])); err == nil {
			it.postedAt = ts.UnixMilli()
		}
		out = append(out, it)
	}
	return out, nil
}

// jsonKeys are the string keys from_json collects, in Python's order.
var jsonKeys = map[string]bool{"text": true, "full_text": true, "content": true, "body": true, "caption": true, "title": true, "post": true}

// primaryKeys carry the post text of an object that also has kind/platform/id.
var primaryKeys = []string{"text", "full_text", "body"}

func fromJSON(text string, opts Options) ([]item, error) {
	var data any
	if err := decode(text, &data); err != nil {
		return nil, err
	}
	var out []item
	base := item{platform: opts.Platform, kind: opts.DefaultKind}
	var walk func(v any, key string, hasKey bool, meta item)
	walk = func(v any, key string, hasKey bool, meta item) {
		switch x := v.(type) {
		case map[string]any:
			m := base
			primary := ""
			for _, k := range primaryKeys {
				if _, ok := x[k].(string); ok {
					primary = k
					break
				}
			}
			if primary != "" {
				if k := str(x["kind"]); validKind(k) {
					m.kind = k
				}
				if p := str(x["platform"]); validPlatform(p) {
					m.platform = p
				}
				m.externalID = str(x["id"])
			}
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if k == primary {
					walk(x[k], k, true, m)
				} else {
					walk(x[k], k, true, base)
				}
			}
		case []any:
			for _, e := range x {
				walk(e, key, hasKey, meta)
			}
		case string:
			if !hasKey || jsonKeys[key] {
				it := meta
				it.text = unmangle(x)
				out = append(out, it)
			}
		}
	}
	walk(data, "", false, base)
	return out, nil
}

var splitRe = regexp.MustCompile(`\n-{3,}\n`)

func fromText(text string, opts Options) []item {
	text = string(bytes.ReplaceAll([]byte(text), []byte("\r\n"), []byte("\n")))
	var out []item
	for _, part := range splitRe.Split(text, -1) {
		out = append(out, item{text: part, platform: opts.Platform, kind: opts.DefaultKind})
	}
	return out
}
