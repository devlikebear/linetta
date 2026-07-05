package node

import (
	"fmt"
	"regexp"
	"strings"
)

// Node labels are stored canonically (Korean patterns like "1권", "1화",
// "씬 1" from the outline presets), and translated at display time. The
// desktop UI does this in displayNodeLabel (i18n.tsx); DisplayLabel is the
// engine-side equivalent so AI prompts show labels in the app language and
// the model echoes them back correctly.

var (
	sceneLabelRe      = regexp.MustCompile(`^(?:씬|(?i:scene)|シーン)\s*(\d+)$`)
	chapterLabelRe    = regexp.MustCompile(`^(?:(\d+)장|(?i:chapter)\s+(\d+)|第(\d+)章)$`)
	partLabelRe       = regexp.MustCompile(`^(?:(\d+)부|(?i:part)\s+(\d+)|第(\d+)部)$`)
	webPartLabelRe    = regexp.MustCompile(`^(?:(\d+)권|(?i:arc)\s+(\d+)|第(\d+)巻)$`)
	webChapterLabelRe = regexp.MustCompile(`^(?:(\d+)화|(?i:episode)\s+(\d+)|第(\d+)話)$`)
)

// DisplayLabel renders a canonical node label in the given app language
// ("en*"/"ja*"; anything else is Korean). Unrecognized labels pass through.
func DisplayLabel(label, lang string) string {
	trimmed := strings.TrimSpace(label)
	if m := sceneLabelRe.FindStringSubmatch(trimmed); m != nil {
		return formatNumbered(m[1], lang, "씬 %s", "Scene %s", "シーン %s")
	}
	if n := firstGroup(chapterLabelRe, trimmed); n != "" {
		return formatNumbered(n, lang, "%s장", "Chapter %s", "第%s章")
	}
	if n := firstGroup(partLabelRe, trimmed); n != "" {
		return formatNumbered(n, lang, "%s부", "Part %s", "第%s部")
	}
	if n := firstGroup(webPartLabelRe, trimmed); n != "" {
		return formatNumbered(n, lang, "%s권", "Arc %s", "第%s巻")
	}
	if n := firstGroup(webChapterLabelRe, trimmed); n != "" {
		return formatNumbered(n, lang, "%s화", "Episode %s", "第%s話")
	}
	return label
}

// DisplayBreadcrumb translates each " / "-separated segment of a breadcrumb.
func DisplayBreadcrumb(breadcrumb, lang string) string {
	parts := strings.Split(breadcrumb, " / ")
	for i, p := range parts {
		parts[i] = DisplayLabel(p, lang)
	}
	return strings.Join(parts, " / ")
}

func firstGroup(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			return g
		}
	}
	return ""
}

func formatNumbered(n, lang, ko, en, ja string) string {
	switch {
	case strings.HasPrefix(lang, "en"):
		return fmt.Sprintf(en, n)
	case strings.HasPrefix(lang, "ja"):
		return fmt.Sprintf(ja, n)
	default:
		return fmt.Sprintf(ko, n)
	}
}
