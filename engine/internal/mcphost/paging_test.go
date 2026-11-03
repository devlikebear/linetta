//go:build !mobile

package mcphost

import (
	"strings"
	"testing"
)

func TestPageRowsReadsEveryRowOnce(t *testing.T) {
	rows := make([]string, 57)
	for i := range rows {
		rows[i] = strings.Repeat("가", 300+i)
	}
	seen, cursor, pages := 0, "", 0
	for {
		page, next, total := pageRows(rows, cursor, 2000)
		if total != len(rows) {
			t.Fatalf("total = %d", total)
		}
		for _, r := range page {
			if r != rows[seen] {
				t.Fatalf("row %d out of order", seen)
			}
			seen++
		}
		pages++
		if next == "" {
			break
		}
		cursor = next
	}
	if seen != len(rows) || pages < 2 {
		t.Fatalf("seen %d in %d pages", seen, pages)
	}
}

func TestPageRowsLeavesAShortListAlone(t *testing.T) {
	page, next, total := pageRows([]string{"a", "b"}, "", pageMaxRunes)
	if len(page) != 2 || next != "" || total != 2 {
		t.Fatalf("page %v next %q total %d", page, next, total)
	}
}

func TestPageRowsMovesPastARowLongerThanAPage(t *testing.T) {
	page, next, _ := pageRows([]string{strings.Repeat("가", 50), "b"}, "", 10)
	if len(page) != 1 || next != "1" {
		t.Fatalf("page %v next %q", page, next)
	}
}

func TestPageRowsStartsOverOnABadCursor(t *testing.T) {
	page, _, _ := pageRows([]string{"a"}, "nonsense", pageMaxRunes)
	if len(page) != 1 {
		t.Fatalf("page %v", page)
	}
}
