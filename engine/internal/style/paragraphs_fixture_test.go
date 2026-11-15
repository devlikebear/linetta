package style

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/evidence"
)

// The app turns a paragraph number back into a place in the editor with its
// own copy of these rules (apps/desktop/src/lib/paragraphRanges.ts), and its
// test reads these same files. A change to how Paragraphs splits a scene has
// to land in both, or a continuity citation opens the wrong paragraph. The
// quotes are held to evidence.VerifyQuote the same way: the app looks for a
// cited quote where the engine accepted it.
func TestParagraphsMatchTheSharedFixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "paragraphs", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no paragraph fixtures")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Doc        json.RawMessage `json:"doc"`
				Paragraphs []string        `json:"paragraphs"`
				Quotes     []struct {
					From  int    `json:"from"`
					To    int    `json:"to"`
					Quote string `json:"quote"`
					Found bool   `json:"found"`
				} `json:"quotes"`
			}
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			got := Paragraphs(string(fixture.Doc))
			if !reflect.DeepEqual(got, fixture.Paragraphs) {
				t.Fatalf("Paragraphs:\n got %q\nwant %q", got, fixture.Paragraphs)
			}
			for _, q := range fixture.Quotes {
				err := evidence.VerifyQuote(got, q.From, q.To, q.Quote)
				if (err == nil) != q.Found {
					t.Errorf("VerifyQuote(%d-%d, %q) = %v, want found=%v", q.From, q.To, q.Quote, err, q.Found)
				}
			}
		})
	}
}
