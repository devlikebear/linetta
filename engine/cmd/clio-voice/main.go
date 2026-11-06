// Command clio-voice prints the voice profile of a file of the writer's posts.
// It exists to check the Go port against scripts/eval-voice.py: the "x/*"
// feature means match Python's features() means over the same file.
//
//	clio-voice <file> [--kind post|reply] [--platform x] [--own-user-id ID]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/devlikebear/linetta/engine/internal/clio/post"
	"github.com/devlikebear/linetta/engine/internal/clio/voice"
	"github.com/devlikebear/linetta/engine/internal/clio/voiceimport"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "clio-voice:", err)
		os.Exit(1)
	}
}

func run(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("clio-voice", flag.ContinueOnError)
	kind := fs.String("kind", "post", "default kind for posts the file does not label: post or reply")
	platform := fs.String("platform", "x", "platform: x, threads or blog")
	own := fs.String("own-user-id", "", "your X user id, to tell your own threads from replies in tweets.js")
	// Accept the file before or after the flags.
	var file string
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		file, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if file == "" && fs.NArg() == 1 {
		file = fs.Arg(0)
	} else if fs.NArg() > 0 {
		return fmt.Errorf("usage: clio-voice <file> [--kind post|reply] [--platform x] [--own-user-id ID]")
	}
	if file == "" {
		return fmt.Errorf("usage: clio-voice <file> [--kind post|reply] [--platform x] [--own-user-id ID]")
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	res, err := voiceimport.Parse(file, content, voiceimport.Options{DefaultKind: *kind, Platform: *platform, OwnUserID: *own})
	if err != nil {
		return err
	}
	posts := make([]post.Post, len(res.Posts))
	for i, p := range res.Posts {
		posts[i] = post.Post{ID: fmt.Sprintf("p%d", i+1), Body: p.Body, Platform: p.Platform, Kind: p.Kind, Source: p.Source, ExternalID: p.ExternalID, PostedAt: p.PostedAt}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(struct {
		Posts    int                             `json:"posts"`
		Skipped  map[string]int                  `json:"skipped"`
		Contexts map[string]voice.ContextProfile `json:"contexts"`
	}{len(posts), res.Skipped, voice.Build(posts).Contexts})
}
