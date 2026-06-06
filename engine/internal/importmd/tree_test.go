package importmd

import "testing"

func TestParseOutline_H1AsTitle(t *testing.T) {
	o := ParseOutline("# My Work\n\nbody line\n")
	if o.Title != "My Work" {
		t.Errorf("title: %q", o.Title)
	}
	// Body under H1 (before any H2) is dropped.
	if len(o.Roots) != 0 {
		t.Errorf("roots = %d, want 0", len(o.Roots))
	}
}

func TestParseOutline_fullTree(t *testing.T) {
	src := "# Title\n## Part A\n### Chapter 1\n#### Scene 1\nbody\n#### Scene 2\nmore\n## Part B\n"
	o := ParseOutline(src)
	if o.Title != "Title" {
		t.Fatalf("title: %q", o.Title)
	}
	if len(o.Roots) != 2 {
		t.Fatalf("roots = %d", len(o.Roots))
	}
	pa := o.Roots[0]
	if pa.Level != 2 || pa.Label != "Part A" {
		t.Errorf("Part A: %+v", pa)
	}
	if len(pa.Children) != 1 {
		t.Fatalf("Part A children = %d", len(pa.Children))
	}
	c1 := pa.Children[0]
	if c1.Level != 3 || c1.Label != "Chapter 1" {
		t.Errorf("Chapter: %+v", c1)
	}
	if len(c1.Children) != 2 {
		t.Fatalf("Chapter children = %d", len(c1.Children))
	}
	s1 := c1.Children[0]
	if s1.Level != 4 || s1.Label != "Scene 1" {
		t.Errorf("Scene 1: %+v", s1)
	}
	if len(s1.Body) == 0 {
		t.Errorf("Scene 1 body empty")
	}
	pb := o.Roots[1]
	if pb.Level != 2 || pb.Label != "Part B" {
		t.Errorf("Part B: %+v", pb)
	}
}

func TestParseOutline_secondH1Demoted(t *testing.T) {
	src := "# Title One\n## Sub\n# Title Two\n"
	o := ParseOutline(src)
	if o.Title != "Title One" {
		t.Errorf("title: %q", o.Title)
	}
	// Title Two becomes H2 — sibling of "Sub" at root.
	if len(o.Roots) != 2 {
		t.Fatalf("roots = %d", len(o.Roots))
	}
	if o.Roots[1].Label != "Title Two" || o.Roots[1].Level != 2 {
		t.Errorf("demoted H1: %+v", o.Roots[1])
	}
}

func TestParseOutline_H5ClampedToH4(t *testing.T) {
	src := "# T\n## A\n### B\n#### C\n##### D\nbody under D\n"
	o := ParseOutline(src)
	// D is clamped to H4, so it's a sibling of C.
	a := o.Roots[0]
	b := a.Children[0]
	if len(b.Children) != 2 {
		t.Fatalf("B children = %d", len(b.Children))
	}
	d := b.Children[1]
	if d.Level != 4 || d.Label != "D" {
		t.Errorf("D: %+v", d)
	}
	if len(d.Body) == 0 {
		t.Errorf("D body empty")
	}
}

func TestParseOutline_H3WithBodyNoH4(t *testing.T) {
	src := "# T\n## P\n### Chapter only\nbody body body\n"
	o := ParseOutline(src)
	ch := o.Roots[0].Children[0]
	if ch.Label != "Chapter only" {
		t.Errorf("label: %q", ch.Label)
	}
	if len(ch.Children) != 0 {
		t.Errorf("expected no children: %+v", ch.Children)
	}
	if len(ch.Body) == 0 {
		t.Errorf("body empty")
	}
}

func TestParseOutline_orphanBodyBeforeFirstHeading(t *testing.T) {
	src := "some stray text\n# Title\n## P\n"
	o := ParseOutline(src)
	// Orphan body dropped.
	if o.Title != "Title" {
		t.Errorf("title: %q", o.Title)
	}
	if len(o.Roots) != 1 {
		t.Errorf("roots = %d", len(o.Roots))
	}
}

func TestParseOutline_malformedNoHeadings(t *testing.T) {
	src := "just\nsome lines\n\nno headings\n"
	o := ParseOutline(src)
	if o.Title != "" {
		t.Errorf("title should be empty: %q", o.Title)
	}
	if len(o.Roots) != 0 {
		t.Errorf("roots should be empty: %+v", o.Roots)
	}
}

func TestParseOutline_empty(t *testing.T) {
	o := ParseOutline("")
	if o.Title != "" || len(o.Roots) != 0 {
		t.Errorf("non-empty outline: %+v", o)
	}
}

func TestParseOutline_leadingWhitespaceOnHeading(t *testing.T) {
	md := "# Title\n" +
		"   ## 1부 어둠\n" +
		"\t### 1장 시작\n" +
		"#### 씬 1\n" +
		"본문이다."
	out := ParseOutline(md)
	if out.Title != "Title" {
		t.Fatalf("title=%q", out.Title)
	}
	if len(out.Roots) != 1 {
		t.Fatalf("want 1 root, got %d", len(out.Roots))
	}
	bu := out.Roots[0]
	if bu.Label != "1부 어둠" || bu.Level != 2 {
		t.Fatalf("part=%+v", bu)
	}
	if len(bu.Children) != 1 || bu.Children[0].Label != "1장 시작" {
		t.Fatalf("chapter mismatch: %+v", bu.Children)
	}
	ch := bu.Children[0]
	if len(ch.Children) != 1 || ch.Children[0].Label != "씬 1" {
		t.Fatalf("scene mismatch: %+v", ch.Children)
	}
}

func TestParseOutline_hashWithoutSpaceIsNotHeading(t *testing.T) {
	out := ParseOutline("##nospace text")
	if len(out.Roots) != 0 {
		t.Fatalf("want no headings, got %d", len(out.Roots))
	}
}

func TestParseDocument_skipsLegacyEntityAppendixAndCapturesEntities(t *testing.T) {
	md := "# 작품\n" +
		"## 1부\n" +
		"### 1장\n" +
		"#### 씬 1\n본문\n\n" +
		"## 등장인물\n\n" +
		"- **해진** (character) · 주인공 — 사진작가\n" +
		"- **항구** (place) · 메인무대 — 오래된 부두\n"

	doc := ParseDocument(md)
	if len(doc.Outline.Roots) != 1 {
		t.Fatalf("roots=%d want 1", len(doc.Outline.Roots))
	}
	for _, root := range doc.Outline.Roots {
		if root.Label == "등장인물" {
			t.Fatalf("entity appendix became an outline node: %+v", doc.Outline.Roots)
		}
	}
	if len(doc.Metadata.Entities) != 2 {
		t.Fatalf("entities=%d want 2: %+v", len(doc.Metadata.Entities), doc.Metadata.Entities)
	}
	if doc.Metadata.Entities[1].Kind != "place" || doc.Metadata.Entities[1].Name != "항구" {
		t.Fatalf("place entity not captured: %+v", doc.Metadata.Entities[1])
	}
}
