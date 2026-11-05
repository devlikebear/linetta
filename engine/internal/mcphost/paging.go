package mcphost

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

// pageMaxRunes is how much of a list one tool result carries. It sits under the
// built-in agent's 24,000-rune cap, past which a result is cut mid-row and the
// model never learns what followed.
const pageMaxRunes = 20000

// pageRows returns the rows from cursor that fit in maxRunes of JSON, the cursor
// of the next page ("" on the last), and how many rows there are. A row longer
// than maxRunes still gets a page of its own, so paging always moves forward.
// A bad cursor starts over rather than failing: a model that mangles one still
// gets a usable page.
func pageRows[T any](rows []T, cursor string, maxRunes int) ([]T, string, int) {
	total := len(rows)
	from, err := strconv.Atoi(cursor)
	if err != nil || from < 0 || from > total {
		from = 0
	}
	used, to := 0, from
	for to < total {
		raw, err := json.Marshal(rows[to])
		if err != nil {
			break
		}
		cost := utf8.RuneCount(raw) + 1
		if used+cost > maxRunes && to > from {
			break
		}
		used += cost
		to++
	}
	next := ""
	if to < total {
		next = strconv.Itoa(to)
	}
	return rows[from:to], next, total
}
