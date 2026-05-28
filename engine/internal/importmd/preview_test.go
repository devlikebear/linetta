package importmd

import (
	"testing"
)

func TestPreview_buildsTreeWithCountsAndWarnings(t *testing.T) {
	md := "# 작품\n" +
		"## 1부\n" +
		"### 1장\n" +
		"#### 씬 1\n본문\n" +
		"#### 씬 2\n본문\n" +
		"### 2장\n" +
		"#### 씬 1\n본문\n"
	out := ParseOutline(md)
	pv := Preview(out, "fallback.md")
	if pv.Title != "작품" {
		t.Fatalf("title=%q", pv.Title)
	}
	if pv.ContainerCount != 3 {
		t.Fatalf("ContainerCount=%d want 3", pv.ContainerCount)
	}
	if pv.LeafCount != 3 {
		t.Fatalf("LeafCount=%d want 3", pv.LeafCount)
	}
	if len(pv.Warnings) != 0 {
		t.Fatalf("warnings=%v", pv.Warnings)
	}
	if len(pv.Roots) != 1 || pv.Roots[0].Label != "1부" {
		t.Fatalf("roots=%+v", pv.Roots)
	}
	bu := pv.Roots[0]
	if bu.Kind != "container" || len(bu.Children) != 2 {
		t.Fatalf("part=%+v", bu)
	}
}

func TestPreview_emptyOutlineProducesWarning(t *testing.T) {
	pv := Preview(ParseOutline("그냥 본문만"), "x.md")
	if len(pv.Warnings) == 0 {
		t.Fatal("expected a warning for no headings")
	}
	if pv.ContainerCount != 0 || pv.LeafCount != 0 {
		t.Fatalf("counts: %+v", pv)
	}
}

func TestPreview_titleFallsBackToFileName(t *testing.T) {
	// markdown with no H1
	pv := Preview(ParseOutline("## 1부\n"), "novel.md")
	if pv.Title != "novel" {
		t.Fatalf("title=%q want 'novel'", pv.Title)
	}
}

func TestPreview_containerWithBodyAddsSyntheticLeaf(t *testing.T) {
	// container has both body lines and child headings.
	md := "# 작품\n" +
		"## 1부\n" +
		"이 부의 도입 문단.\n" +
		"### 1장\n#### 씬 1\n본문\n"
	pv := Preview(ParseOutline(md), "x.md")
	bu := pv.Roots[0]
	// children should be: synthetic "씬 1" leaf (for body) + "1장" container
	if len(bu.Children) != 2 {
		t.Fatalf("want 2 children (synth leaf + chapter), got %d", len(bu.Children))
	}
	if bu.Children[0].Kind != "leaf" || bu.Children[0].Label != "씬 1" {
		t.Fatalf("first child should be synthetic 씬 1 leaf, got %+v", bu.Children[0])
	}
}
