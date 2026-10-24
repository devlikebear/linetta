package continuity

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// RefLookup answers whether a ref an observation cites exists in the work
// currently being validated. Validate takes no context and does no I/O
// itself — same-input-same-output, so it is unit-testable with no
// database — and this is the seam that lets a caller supply that answer
// however it likes: a handful of repo lookups when validating a real
// report, or a fixed map in a table-driven test.
//
// Implementations are expected to already be scoped to the one project_id
// the report is about: HasEntity, HasFact and HasBeat take no project_id of
// their own, so an id belonging to a different work must come back false —
// that is what keeps a report from citing another work's record.
type RefLookup interface {
	HasEntity(id string) bool
	HasFact(id string) bool
	HasBeat(id string) bool
	// SceneParagraphs resolves a scene ref (type=scene): the referenced
	// scene's paragraphs, the same way style.Paragraphs numbers them, and
	// whether nodeID is in fact a leaf scene of this work. ok is false for
	// any other node (container, wrong work, unknown id) — a scene ref is
	// refused the same way an entity/fact/beat ref with an unknown id is.
	SceneParagraphs(nodeID string) (paragraphs []string, ok bool)
}

// Problem is one thing wrong with a report, phrased so a model reading the
// tool's refusal back can act on it directly: fix the field, or drop the
// observation. Index is the 0-based position in ReportInput.Observations
// that the problem is about, or GlobalIndex for a problem about the report
// as a whole.
type Problem struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// GlobalIndex marks a Problem that is not about any one observation.
const GlobalIndex = -1

// Validate checks every observation against the scene it cites and the
// work's own record. It returns ALL problems at once — not the first one —
// each phrased so a model can fix every one of them in its next call rather
// than discovering them one refusal at a time.
//
// paragraphs is the REPORTED scene's paragraphs (style.Paragraphs' split),
// at the content_version the caller has already confirmed matches
// in.SceneVersion — Validate does not re-check the version; that is the
// tool layer's job, before it even reaches here.
func Validate(in ReportInput, paragraphs []string, lookup RefLookup) []Problem {
	var problems []Problem
	add := func(idx int, format string, args ...any) {
		problems = append(problems, Problem{Index: idx, Message: fmt.Sprintf(format, args...)})
	}

	if n := len(in.Observations); n > MaxObservationsPerReport {
		add(GlobalIndex, "report has %d observations; linetta_report_continuity takes at most %d per call — "+
			"split the check across turns or drop the lowest-value findings", n, MaxObservationsPerReport)
	}

	for i, obs := range in.Observations {
		num := i + 1 // 1-based, matching how the writer sees observations listed
		validateOne(add, num, obs, paragraphs, lookup)
	}
	return problems
}

func validateOne(add func(idx int, format string, args ...any), num int, obs ObservationInput, paragraphs []string, lookup RefLookup) {
	idx := num - 1

	if !ValidCategory(obs.Category) {
		add(idx, "observation %d: category %q is not valid; use one of character, place, item, "+
			"relationship, timeline, fact, plot, direction, other", num, obs.Category)
	}
	if !ValidKind(obs.Kind) {
		add(idx, "observation %d: kind %q is not valid; use contradiction, gap, or question", num, obs.Kind)
	}
	if !ValidSeverity(obs.Severity) {
		add(idx, "observation %d: severity %q is not valid; use high, medium, or low", num, obs.Severity)
	}

	summary := strings.TrimSpace(obs.Summary)
	if summary == "" {
		add(idx, "observation %d: summary is required", num)
	} else if n := utf8.RuneCountInString(summary); n > MaxSummaryRunes {
		add(idx, "observation %d: summary is %d characters; the limit is %d", num, n, MaxSummaryRunes)
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(obs.Suggestion)); n > MaxSuggestionRunes {
		add(idx, "observation %d: suggestion is %d characters; the limit is %d", num, n, MaxSuggestionRunes)
	}

	quote := strings.TrimSpace(obs.Quote)
	if quote == "" {
		add(idx, "observation %d: quote is required; copy it exactly from the numbered text", num)
	} else if n := utf8.RuneCountInString(quote); n > MaxQuoteRunes {
		add(idx, "observation %d: quote is %d characters; the limit is %d", num, n, MaxQuoteRunes)
	}

	rangeOK := validateRange(add, num, obs.ParagraphFrom, obs.ParagraphTo, len(paragraphs))
	if rangeOK && quote != "" {
		haystack := strings.Join(paragraphs[obs.ParagraphFrom-1:obs.ParagraphTo], "\n")
		if !containsNormalized(haystack, quote) {
			add(idx, "observation %d: quote not found in %s; copy it exactly from the numbered text or "+
				"drop the observation", num, paragraphsPhrase(obs.ParagraphFrom, obs.ParagraphTo))
		}
	}

	if len(obs.Refs) > MaxRefsPerObservation {
		add(idx, "observation %d has %d refs; at most %d", num, len(obs.Refs), MaxRefsPerObservation)
	}
	for r, ref := range obs.Refs {
		validateRef(add, num, r, ref, lookup)
	}

	if obs.Kind == KindContradiction && len(obs.Refs) == 0 {
		add(idx, "observation %d: kind is contradiction but has no refs; point at what it contradicts "+
			"(an entity, fact, beat, or earlier scene)", num)
	}
}

func validateRef(add func(idx int, format string, args ...any), num, refIdx int, ref Ref, lookup RefLookup) {
	idx := num - 1
	if !ValidRefType(ref.Type) {
		add(idx, "observation %d ref %d: type %q is not valid; use entity, fact, beat, or scene", num, refIdx, ref.Type)
		return
	}
	if strings.TrimSpace(ref.ID) == "" {
		add(idx, "observation %d ref %d: id is required", num, refIdx)
		return
	}
	switch ref.Type {
	case RefTypeEntity:
		if lookup == nil || !lookup.HasEntity(ref.ID) {
			add(idx, "observation %d ref %d: entity %q not found in this work; check it with linetta_list_characters",
				num, refIdx, ref.ID)
		}
	case RefTypeFact:
		if lookup == nil || !lookup.HasFact(ref.ID) {
			add(idx, "observation %d ref %d: fact %q not found in this work; check it with linetta_get_fact_cards",
				num, refIdx, ref.ID)
		}
	case RefTypeBeat:
		if lookup == nil || !lookup.HasBeat(ref.ID) {
			add(idx, "observation %d ref %d: beat %q not found in this work; check it with linetta_get_plot",
				num, refIdx, ref.ID)
		}
	case RefTypeScene:
		if lookup == nil {
			add(idx, "observation %d ref %d: scene %q not found in this work", num, refIdx, ref.ID)
			return
		}
		scenePars, ok := lookup.SceneParagraphs(ref.ID)
		if !ok {
			add(idx, "observation %d ref %d: scene %q is not a leaf scene in this work", num, refIdx, ref.ID)
			return
		}
		quote := strings.TrimSpace(ref.Quote)
		if quote == "" {
			return
		}
		if n := utf8.RuneCountInString(quote); n > MaxQuoteRunes {
			add(idx, "observation %d ref %d: quote is %d characters; the limit is %d", num, refIdx, n, MaxQuoteRunes)
			return
		}
		if ref.Paragraph < 1 || ref.Paragraph > len(scenePars) {
			add(idx, "observation %d ref %d: paragraph %d is out of range; scene %q has %d paragraphs",
				num, refIdx, ref.Paragraph, ref.ID, len(scenePars))
			return
		}
		if !containsNormalized(scenePars[ref.Paragraph-1], quote) {
			add(idx, "observation %d ref %d: quote not found in scene %q paragraph %d; copy it exactly or drop the quote",
				num, refIdx, ref.ID, ref.Paragraph)
		}
	}
}

// validateRange checks an observation's 1-based [paragraph_from,
// paragraph_to] range against a scene with n paragraphs, adding a Problem
// and returning false when it is invalid.
func validateRange(add func(idx int, format string, args ...any), num int, from, to, n int) bool {
	idx := num - 1
	switch {
	case from < 1:
		add(idx, "observation %d: paragraph_from %d must be at least 1", num, from)
		return false
	case to < from:
		add(idx, "observation %d: paragraph_to %d is before paragraph_from %d", num, to, from)
		return false
	case to > n:
		add(idx, "observation %d: paragraph %d is out of range; the scene has %d paragraphs", num, to, n)
		return false
	default:
		return true
	}
}

func paragraphsPhrase(from, to int) string {
	if from == to {
		return fmt.Sprintf("paragraph %d", from)
	}
	return fmt.Sprintf("paragraphs %d–%d", from, to)
}

// containsNormalized reports whether quote appears in haystack once both are
// NFC-normalized and their whitespace runs are folded to a single space —
// so a quote that differs from the manuscript only in composed-vs-decomposed
// Hangul or in how many spaces sit between two words still passes, while a
// quote with different WORDS does not.
func containsNormalized(haystack, quote string) bool {
	return strings.Contains(normalizeForCompare(haystack), normalizeForCompare(quote))
}

func normalizeForCompare(s string) string {
	return strings.Join(strings.Fields(norm.NFC.String(s)), " ")
}
