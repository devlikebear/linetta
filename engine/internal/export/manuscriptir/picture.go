package manuscriptir

// Picture is a gallery picture a writer embeds where an illustration block
// names it. Width and Height are its pixels.
type Picture struct {
	MIME          string
	Data          []byte
	Width, Height int
	// Ext is the file extension the package stores it under: png, jpeg, gif.
	Ext string
}

// SizeMM is the picture's printed size: its pixels at 96 dpi, scaled down,
// never up, to fit maxW by maxH millimetres.
func (p Picture) SizeMM(maxW, maxH float64) (w, h float64) {
	if p.Width <= 0 || p.Height <= 0 {
		return maxW, maxW * 3 / 4
	}
	const mmPerPx = 25.4 / 96
	w, h = float64(p.Width)*mmPerPx, float64(p.Height)*mmPerPx
	if w > maxW {
		h, w = h*maxW/w, maxW
	}
	if h > maxH {
		w, h = w*maxH/h, maxH
	}
	return w, h
}

// ImageIDs lists the pictures doc's illustration blocks name, first use first.
func (doc Doc) ImageIDs() []string {
	var ids []string
	seen := map[string]bool{}
	for _, sec := range doc.Sections {
		for _, b := range sec.Blocks {
			if b.Kind == IllustrationPlaceholder && b.ImageID != "" && !seen[b.ImageID] {
				seen[b.ImageID] = true
				ids = append(ids, b.ImageID)
			}
		}
	}
	return ids
}

// Contents is the table of contents: each section heading down to maxLevel,
// in reading order.
func (doc Doc) Contents(maxLevel int) []Section {
	var out []Section
	for _, sec := range doc.Sections {
		if sec.Heading != "" && sec.Level <= maxLevel {
			out = append(out, Section{Heading: sec.Heading, Level: sec.Level})
		}
	}
	return out
}
