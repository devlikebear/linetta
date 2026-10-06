package export

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// testdata/editor_schema.json is the editor's real node and mark list:
// apps/desktop/src/components/editor/editorSchema.test.ts builds the schema
// from the extensions the app mounts and fails if it differs from this file.
// This test fails if the exporter has not decided what to do with one of
// them. Together they close the gap #160 came through — an extension the
// editor gained and the exporter never heard of.
func TestEditorSchemaIsFullyHandled(t *testing.T) {
	raw, err := os.ReadFile("testdata/editor_schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Nodes []string `json:"nodes"`
		Marks []string `json:"marks"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	check := func(kind string, want []string, have map[string]string) {
		t.Helper()
		got := make([]string, 0, len(have))
		for name := range have {
			got = append(got, name)
		}
		sort.Strings(got)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Fatalf("%s: exporter handles %v, editor defines %v", kind, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: exporter handles %v, editor defines %v", kind, got, want)
			}
		}
	}
	check("nodes", schema.Nodes, editorNodes)
	check("marks", schema.Marks, editorMarks)
}
