// Command clio-check-eval runs the draft checker over an eval file and prints
// how many texts each arm gets flagged. It prints counts and rule names only,
// never a post body.
//
//	clio-check-eval <posts.json> <rows.json>
//
// posts.json is a list of {id, platform, kind, text}. rows.json is a list of
// {id, original, A, B, C, ...}. The writer's posts that equal a row's original
// are left out of the profile and corpus, so the test texts do not teach the
// baseline.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/clio/check"
	"github.com/devlikebear/linetta/engine/internal/clio/post"
	"github.com/devlikebear/linetta/engine/internal/clio/voice"
)

var arms = []string{"original", "A", "B", "C"}

type postRow struct {
	ID       json.RawMessage `json:"id"`
	Platform string          `json:"platform"`
	Kind     string          `json:"kind"`
	Text     string          `json:"text"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "clio-check-eval:", err)
		os.Exit(1)
	}
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func run(args []string, w io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: clio-check-eval <posts.json> <rows.json>")
	}
	var raw []postRow
	if err := readJSON(args[0], &raw); err != nil {
		return fmt.Errorf("posts: %w", err)
	}
	var rows []map[string]json.RawMessage
	if err := readJSON(args[1], &rows); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	str := func(row map[string]json.RawMessage, key string) string {
		var s string
		_ = json.Unmarshal(row[key], &s)
		return s
	}
	originals := map[string]bool{}
	for _, r := range rows {
		originals[strings.TrimSpace(str(r, "original"))] = true
	}
	kindOf := map[string]postRow{}
	var posts []post.Post
	excluded := 0
	for i, p := range raw {
		body := strings.TrimSpace(p.Text)
		if originals[body] {
			excluded++
			kindOf[body] = p
			continue
		}
		posts = append(posts, post.Post{ID: fmt.Sprintf("p%d", i+1), Body: p.Text, Platform: p.Platform, Kind: p.Kind})
	}
	profile := voice.Build(posts)

	fmt.Fprintf(w, "posts: %d total, %d excluded as originals, %d used\n\n", len(raw), excluded, len(posts))

	flagged := map[string]int{}
	ruleFindings := map[string]map[string]int{}
	rulePosts := map[string]map[string]int{}
	missing := 0
	var perRow []string
	for idx, row := range rows {
		orig := strings.TrimSpace(str(row, "original"))
		src, ok := kindOf[orig]
		platform, kind := "x", "post"
		if ok {
			platform, kind = src.Platform, src.Kind
		} else {
			missing++
		}
		ctx := profile.For(platform, kind)
		corpus := check.BuildCorpus(posts, ctx.Fallback)
		var cells []string
		for _, arm := range arms {
			fs := check.Run(str(row, arm), ctx, corpus)
			if ruleFindings[arm] == nil {
				ruleFindings[arm], rulePosts[arm] = map[string]int{}, map[string]int{}
			}
			hit := false
			seen := map[string]bool{}
			counts := map[string]int{}
			for _, f := range fs {
				ruleFindings[arm][f.Rule]++
				counts[fmt.Sprintf("%s(%s)", f.Rule, string(f.Severity)[:1])]++
				if !seen[f.Rule] {
					seen[f.Rule] = true
					rulePosts[arm][f.Rule]++
				}
				if f.Severity == check.Strong || f.Severity == check.Medium {
					hit = true
				}
			}
			if hit {
				flagged[arm]++
			}
			cells = append(cells, fmt.Sprintf("%s=%s", arm, formatCounts(counts)))
		}
		perRow = append(perRow, fmt.Sprintf("row %d id=%s ctx=%s via %s: %s", idx, strings.Trim(string(row["id"]), `"`), ctx.Key, ctx.Fallback, strings.Join(cells, " ")))
	}

	fmt.Fprintf(w, "texts with at least one strong or medium finding (of %d rows)\n", len(rows))
	for _, arm := range arms {
		fmt.Fprintf(w, "  %-9s %d\n", arm, flagged[arm])
	}
	fmt.Fprintln(w, "\nfindings per rule (findings / texts)")
	ruleNames := []string{check.RuleAIPhrase, check.RuleAIPattern, check.RuleRegisterMismatch, check.RuleRegisterMixed, check.RuleOutOfRange, check.RuleUnfamiliarEnding}
	fmt.Fprintf(w, "  %-19s", "rule")
	for _, arm := range arms {
		fmt.Fprintf(w, " %9s", arm)
	}
	fmt.Fprintln(w)
	for _, rule := range ruleNames {
		fmt.Fprintf(w, "  %-19s", rule)
		for _, arm := range arms {
			fmt.Fprintf(w, " %9s", fmt.Sprintf("%d/%d", ruleFindings[arm][rule], rulePosts[arm][rule]))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "\nper row (rule(S=strong, M=medium, W=weak) x count)")
	for _, l := range perRow {
		fmt.Fprintln(w, "  "+l)
	}
	if missing > 0 {
		fmt.Fprintf(w, "\n%d rows had an original not found in posts; they used x/post\n", missing)
	}
	return nil
}

func formatCounts(m map[string]int) string {
	if len(m) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%sx%d", k, m[k])
	}
	return strings.Join(parts, ",")
}
