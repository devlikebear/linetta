package novel

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunCreatesNovelArtifactsThroughTessera(t *testing.T) {
	report, err := Run(context.Background(), Config{
		Goal:       "희망적인 기후 소설의 첫 장을 써줘",
		Title:      "녹색 항구의 밤",
		ApprovedBy: "tester",
		ApprovedAt: time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if report.App != "linetta" {
		t.Fatalf("app = %q, want linetta", report.App)
	}
	if report.Closure != "normal" {
		t.Fatalf("closure = %q, want normal", report.Closure)
	}
	if report.Succeeded != 4 {
		t.Fatalf("succeeded = %d, want 4", report.Succeeded)
	}
	for _, id := range []string{"research-world", "outline-plot", "draft-chapter", "review-draft"} {
		if strings.TrimSpace(report.Artifacts[id]) == "" {
			t.Fatalf("artifact %q is empty", id)
		}
	}
	if !strings.Contains(report.Artifacts["draft-chapter"], "녹색 항구의 밤") {
		t.Fatalf("draft-chapter should include title, got %q", report.Artifacts["draft-chapter"])
	}
}

func TestRunRejectsBlankGoal(t *testing.T) {
	_, err := Run(context.Background(), Config{Goal: "   "})
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}
}

func TestRenderMarkdownIncludesNovelPacketSections(t *testing.T) {
	report := Report{
		Title: "샘플",
		Goal:  "첫 장",
		Artifacts: map[string]string{
			"research-world": "world notes",
			"outline-plot":   "outline notes",
			"draft-chapter":  "draft text",
			"review-draft":   "review notes",
		},
	}

	markdown := RenderMarkdown(report)
	for _, want := range []string{
		"# 샘플",
		"## Goal",
		"## World Notes",
		"## Plot Outline",
		"## Chapter Draft",
		"## Editorial Review",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("RenderMarkdown() missing %q in:\n%s", want, markdown)
		}
	}
}
