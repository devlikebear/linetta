// Command style-measure prints what internal/style counts and checks in a few
// groups of short posts. scripts/eval-voice.py uses it to hold generated posts
// against the writer's own with the same code the app runs on scenes.
//
// It reads {"groups": {"name": ["post", ...]}} on stdin and writes
// {"name": {"stats": ..., "check": {"rule": count}}} on stdout. Each post is
// one scene and each of its lines one paragraph, since a post breaks lines
// where prose would break paragraphs. Nothing here calls a model.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/style"
)

type input struct {
	Groups map[string][]string `json:"groups"`
}

type result struct {
	Stats style.Stats    `json:"stats"`
	Check map[string]int `json:"check"`
}

func main() {
	var in input
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		fmt.Fprintln(os.Stderr, "style-measure:", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(measure(in)); err != nil {
		fmt.Fprintln(os.Stderr, "style-measure:", err)
		os.Exit(1)
	}
}

func measure(in input) map[string]result {
	out := map[string]result{}
	for name, posts := range in.Groups {
		scenes := asScenes(posts)
		counts := map[string]int{}
		for _, v := range style.Check(scenes, style.Rules{}).Violations {
			counts[v.Rule]++
		}
		out[name] = result{Stats: style.Measure(scenes), Check: counts}
	}
	return out
}

func asScenes(posts []string) []style.Scene {
	scenes := make([]style.Scene, 0, len(posts))
	for i, p := range posts {
		var paras []string
		for _, line := range strings.Split(p, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				paras = append(paras, line)
			}
		}
		id := fmt.Sprint(i + 1)
		scenes = append(scenes, style.Scene{NodeID: id, Label: id, Paragraphs: paras})
	}
	return scenes
}
