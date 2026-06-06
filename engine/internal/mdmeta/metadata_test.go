package mdmeta

import (
	"strings"
	"testing"
)

func TestFrontMatterRoundTripsOutlinePreset(t *testing.T) {
	raw, err := RenderFrontMatter(Metadata{OutlinePreset: "webnovel"})
	if err != nil {
		t.Fatalf("RenderFrontMatter: %v", err)
	}
	if !strings.Contains(raw, "outline_preset: webnovel\n") {
		t.Fatalf("missing outline_preset:\n%s", raw)
	}
	meta, body, err := ExtractFrontMatter(raw + "# 작품\n")
	if err != nil {
		t.Fatalf("ExtractFrontMatter: %v", err)
	}
	if meta.OutlinePreset != "webnovel" {
		t.Fatalf("outline_preset=%q want webnovel", meta.OutlinePreset)
	}
	if strings.TrimSpace(body) != "# 작품" {
		t.Fatalf("body=%q", body)
	}
}
