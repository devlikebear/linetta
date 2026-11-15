package manuscript

import (
	"context"
	"strings"
)

// locate fills each hit's paragraph range from manuscript_chunks. A scene hit
// means the scene holds every term; the range says where. It prefers one
// paragraph that holds every term (the whole phrase first, then the most
// occurrences), and otherwise the chunk that holds the most terms, narrowed to
// the paragraphs in it that hold any. A hit it cannot place keeps a zero range:
// the scene is still the answer, and a range is only ever a real one.
func (s *Searcher) locate(ctx context.Context, terms []string, hits []Hit) {
	if len(hits) == 0 || len(terms) == 0 {
		return
	}
	needles := make([]string, len(terms))
	for i, t := range terms {
		needles[i] = strings.ToLower(t)
	}
	phrase := strings.Join(needles, " ")
	for i := range hits {
		chunks, err := s.chunksOf(ctx, hits[i].NodeID)
		if err != nil || len(chunks) == 0 {
			continue
		}
		hits[i].ParagraphFrom, hits[i].ParagraphTo = bestRange(chunks, phrase, needles)
	}
}

func (s *Searcher) chunksOf(ctx context.Context, nodeID string) ([]Chunk, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT para_from, para_to, text
  FROM manuscript_chunks
 WHERE node_id = ?
 ORDER BY para_from`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ParaFrom, &c.ParaTo, &c.Text); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type matchScore struct {
	phrase   bool
	distinct int
	count    int
}

func (a matchScore) beats(b matchScore) bool {
	if a.distinct != b.distinct {
		return a.distinct > b.distinct
	}
	if a.phrase != b.phrase {
		return a.phrase
	}
	return a.count > b.count
}

func scoreText(text, phrase string, needles []string) matchScore {
	lower := strings.ToLower(text)
	sc := matchScore{phrase: strings.Contains(lower, phrase)}
	for _, n := range needles {
		if c := strings.Count(lower, n); c > 0 {
			sc.distinct++
			sc.count += c
		}
	}
	return sc
}

// bestRange returns the 1-based, inclusive paragraph range of the best match
// in chunks, or 0, 0 when no paragraph holds any term. Ties go to the earlier
// passage.
func bestRange(chunks []Chunk, phrase string, needles []string) (int, int) {
	// One paragraph with every term.
	bestPara, bestParaScore := 0, matchScore{}
	for _, c := range chunks {
		for j, p := range c.paragraphs() {
			if sc := scoreText(p, phrase, needles); sc.distinct == len(needles) && (bestPara == 0 || sc.beats(bestParaScore)) {
				bestPara, bestParaScore = c.ParaFrom+j, sc
			}
		}
	}
	if bestPara > 0 {
		return bestPara, bestPara
	}
	// Otherwise the chunk with the most terms, narrowed to the paragraphs in
	// it that hold any.
	var best *Chunk
	bestScore := matchScore{}
	for k := range chunks {
		if sc := scoreText(chunks[k].Text, phrase, needles); sc.distinct > 0 && (best == nil || sc.beats(bestScore)) {
			best, bestScore = &chunks[k], sc
		}
	}
	if best == nil {
		return 0, 0
	}
	from, to := 0, 0
	for j, p := range best.paragraphs() {
		if scoreText(p, phrase, needles).distinct > 0 {
			if from == 0 {
				from = best.ParaFrom + j
			}
			to = best.ParaFrom + j
		}
	}
	return from, to
}
