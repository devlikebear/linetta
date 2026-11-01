package manuscript

import "testing"

func TestDocToPlainTextSpacesOnlyWhereTheWriterSeesOne(t *testing.T) {
	const mention = `{"type":"mention","attrs":{"id":"c1","label":"서린"}}`
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "a particle stays on the mention before it",
			content: `{"type":"paragraph","content":[` + mention + `,{"type":"text","text":"이 문을 열었다."}]}`,
			want:    "서린이 문을 열었다.",
		},
		{
			name:    "spaces typed around a mention stay",
			content: `{"type":"paragraph","content":[{"type":"text","text":"그때 "},` + mention + `,{"type":"text","text":" 웃었다."}]}`,
			want:    "그때 서린 웃었다.",
		},
		{
			name:    "a mark splitting a word does not split it here",
			content: `{"type":"paragraph","content":[{"type":"text","marks":[{"type":"bold"}],"text":"곰"},{"type":"text","text":"배는"}]}`,
			want:    "곰배는",
		},
		{
			name:    "a note marker inside a word leaves no gap",
			content: `{"type":"paragraph","content":[{"type":"text","text":"곰"},{"type":"noteMarker","attrs":{"noteId":"n1"}},{"type":"text","text":"배는"}]}`,
			want:    "곰배는",
		},
		{
			name:    "a line break separates its lines",
			content: `{"type":"paragraph","content":[{"type":"text","text":"첫 줄"},{"type":"hardBreak"},{"type":"text","text":"둘째 줄"}]}`,
			want:    "첫 줄 둘째 줄",
		},
		{
			name: "blocks are separated, nested or not",
			content: `{"type":"paragraph","content":[{"type":"text","text":"비"}]},` +
				`{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"눈"}]},` +
				`{"type":"paragraph","content":[{"type":"text","text":"안개"}]}]},` +
				`{"type":"paragraph"},{"type":"paragraph","content":[{"type":"text","text":"바람"}]}`,
			want: "비 눈 안개 바람",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DocToPlainText(`{"type":"doc","content":[` + tc.content + `]}`)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
